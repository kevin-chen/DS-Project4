package raft

import (
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"net"
	"sync"
	"time"
)

type Raft struct {
	id          string
	currentTerm int
	state       State
	peers       []string
	votedFor    string

	lastTimeout       time.Time
	mu                sync.Mutex
	isTimerRunning    bool
	isElectionRunning bool

	log []LogEntry
	//commitChan chan LogEntry

	// ALL STATES
	commitIndex int
	lastApplied int

	// LEADER
	nextIndex  map[string]int
	matchIndex map[string]int
}

type State int

const (
	Follower State = iota
	Leader
	Candidate
)

func (state State) String() string {
	switch state {
	case Follower:
		return "Follower"
	case Leader:
		return "Leader"
	case Candidate:
		return "Candidate"
	}
	return "Invalid State"
}

func (raft *Raft) String() string {
	return fmt.Sprintf("Raft Node with id: %s, term: %d, and state: %s", raft.id, raft.currentTerm, raft.state)
}

type RequestVoteArgument struct {
	Term         int    `json:"term"`
	CandidateId  string `json:"candidateId"`
	LastLogIndex int    `json:"lastLogIndex"`
	LastLogTerm  int    `json:"lastLogTerm"`
}

func (rv RequestVoteArgument) String() string {
	return fmt.Sprintf("Term: %d, CandidateID: %s, LastLogIndex: %d, LastLogTerm: %d", rv.Term, rv.CandidateId, rv.LastLogIndex, rv.LastLogTerm)
}

type RequestVoteResponse struct {
	Term        int  `json:"term"`
	VoteGranted bool `json:"voteGranted"`
}

func (rv RequestVoteResponse) String() string {
	return fmt.Sprintf("Term: %d, voteGranted: %t", rv.Term, rv.VoteGranted)
}

type AppendEntriesArgument struct {
	Term         int
	LeaderId     string
	prevIndex    int
	prevTerm     int
	entries      []string
	leaderCommit int // Leader's commit index
}

func (ae AppendEntriesArgument) String() string {
	return fmt.Sprintf("Term: %d, LeaderId: %s", ae.Term, ae.LeaderId)
}

type AppendEntriesResponse struct {
	Term    int
	Success bool
}

func (ae AppendEntriesResponse) String() string {
	return fmt.Sprintf("Term: %d, Success: %t", ae.Term, ae.Success)
}

type LogEntry struct {
	Command string
	Term    int
}

func (le LogEntry) String() string {
	return fmt.Sprintf("Term: %d, Command: %s", le.Term, le.Command)
}

func checkError(message string, err error) {
	if err != nil {
		log.Printf(message, err)
		return
	}
}

func NewNode(id string, peers []string) *Raft {
	log.Printf(fmt.Sprintf("Creating new node with id: %s", id))

	raft := new(Raft)

	raft.id = id
	raft.currentTerm = 1
	raft.state = Follower
	raft.peers = peers
	raft.votedFor = ""

	raft.lastTimeout = time.Now()
	raft.isTimerRunning = false
	raft.isElectionRunning = false

	raft.commitIndex = 0
	raft.lastApplied = 0

	raft.nextIndex = make(map[string]int)
	raft.matchIndex = make(map[string]int)

	go raft.runNodes() // main loop/routine for running nodes

	return raft
}

func (raft *Raft) HandleCheckLeader() bool {
	raft.mu.Lock()
	defer raft.mu.Unlock()
	return raft.state == Leader
}

func (raft *Raft) runNodes() {
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		<-ticker.C

		raft.mu.Lock()
		state := raft.state
		raft.mu.Unlock()

		switch state {
		case Follower:
			raft.runFollower()
		case Leader:
			raft.runLeader()
		case Candidate:
			raft.runCandidate()
		}
	}
}

// run election timer to keep checking
func (raft *Raft) runFollower() {
	raft.mu.Lock()
	defer raft.mu.Unlock()

	isTimerRunning := raft.isTimerRunning

	// runs election timer on follower
	if !isTimerRunning {
		raft.isTimerRunning = true
		go raft.runElectionTimer(Follower)
	}
}

func (raft *Raft) runCandidate() {
	raft.mu.Lock()
	defer raft.mu.Unlock()

	isTimerRunning := raft.isTimerRunning
	isElectionRunning := raft.isElectionRunning

	//// runs election timer on follower
	//if !isTimerRunning {
	//	raft.isTimerRunning = true
	//	go raft.runElectionTimer(Follower)
	//}

	// runs election timer and election on candidate
	if !isElectionRunning && !isTimerRunning {
		term := raft.currentTerm
		raft.isElectionRunning = true
		raft.isTimerRunning = true
		go raft.runElectionTimer(Candidate)
		go raft.runElection(term)
	}
}

func (raft *Raft) runLeader() {
	raft.mu.Lock()
	raft.lastTimeout = time.Now()
	raft.mu.Unlock()

	heartbeatTicker := time.NewTicker(30 * time.Millisecond)
	defer heartbeatTicker.Stop()

	for {
		<-heartbeatTicker.C
		go raft.sendHeartbeat()
	}
}

func (raft *Raft) sendHeartbeat() {
	log.Printf(fmt.Sprintf("Sending new heartbeat"))

	raft.mu.Lock()
	term := raft.currentTerm
	leaderId := raft.id
	peers := raft.peers
	raft.mu.Unlock()

	appendEntriesArgument := AppendEntriesArgument{
		Term:     term,
		LeaderId: leaderId,
	}

	log.Printf("Created new AppendEntriesArgument: %s", appendEntriesArgument)

	for _, peer := range peers {
		response := raft.sendAppendEntries(peer, &appendEntriesArgument)
		log.Printf("Received response from %s, response: %s", peer, response)
	}

}

func (raft *Raft) runElection(term int) {
	log.Printf(fmt.Sprintf("New Election started for term: %d", term))
	raft.mu.Lock()
	peers := raft.peers
	id := raft.id
	var lastLogIndex int
	var lastLogTerm int
	if len(raft.log) > 0 {
		lastLogIndex = len(raft.log) - 1
		lastLogTerm = raft.log[lastLogIndex].Term
	} else {
		lastLogIndex = 0
		lastLogTerm = 0
	}
	raft.mu.Unlock()

	votesAcquired := 1
	quorum := ((len(peers) + 1) / 2) + 1

	requestVoteArgument := RequestVoteArgument{
		Term:         term,
		CandidateId:  id,
		LastLogIndex: lastLogIndex,
		LastLogTerm:  lastLogTerm,
	}

	log.Printf("Created new RequestVoteArgument with id: %s", id)

	for _, peer := range peers {
		go func(peer string) {
			// This is the request, wrapped in another function
			response := raft.sendRequestVote(peer, &requestVoteArgument)

			// This is the response from the RequestVote to another backend
			raft.mu.Lock()
			if term != response.Term { // old RequestVote from prior term
				log.Printf(fmt.Sprintf("%s (term: %d) got old RequestVote (term: %d) from %s", id, term, response.Term, peer))
				raft.isElectionRunning = false
				return
			}
			if response.Term > term { // for two candidates in election
				raft.state = Follower
				raft.votedFor = ""
			}
			if response.VoteGranted {
				log.Printf(fmt.Sprintf("%s has granted %s their vote", peer, id))
				votesAcquired++
			}
			if votesAcquired >= quorum {
				raft.startNewLeader()
				raft.mu.Unlock()
				return
			}
			raft.mu.Unlock()
		}(peer)
	}
}

func (raft *Raft) startNewLeader() {
	raft.state = Leader
	raft.isElectionRunning = false
	log.Printf(fmt.Sprintf("%s is elected leader", raft.id))

	peers := raft.peers
	for _, peer := range peers {
		raft.nextIndex[peer] = len(raft.log)
		//raft.matchIndex[peer] = -1
	}
}

// Send and Receive message for RequestVote for Requester (the one requesting)
func (raft *Raft) sendRequestVote(peer string, requestVoteArgument *RequestVoteArgument) *RequestVoteResponse {
	log.Printf(fmt.Sprintf("RequestVote sent from %s to %s", raft.id, peer))

	// Try to marshal the RequestVote arguments and attach to request message
	result, err := json.Marshal(*requestVoteArgument)
	log.Printf("Result from marshal %s", result)
	checkError("Error encoding RequestVote arg %s", err)
	message := fmt.Sprintf("RequestVote %s\n", string(result))
	log.Printf("Message %s", message)

	// Connect to peer and send the request
	conn, err := net.Dial("tcp", peer)
	checkError(fmt.Sprintf("Error TCP connection to: %s", peer), err)
	request, err := conn.Write([]byte(message))
	checkError(fmt.Sprintf("Error writing to TCP connection to: %s, Request: %d", peer, request), err)

	// Receive reply from peer with RequestVote response
	decoder := json.NewDecoder(conn)
	var response RequestVoteResponse
	responseErr := decoder.Decode(&response)
	checkError("Error decoding %s", responseErr)
	log.Printf(fmt.Sprintf("RequestVote response from %s, response: %s", peer, response))

	return &response
}

// HandleRequestVote handles a RequestVote message for the Requestee (being received)
func (raft *Raft) HandleRequestVote(message string) *RequestVoteResponse {
	// Try to unmarshall the message which is a RequestVoteArgument
	var requestVoteArgument RequestVoteArgument
	err := json.Unmarshal([]byte(message), &requestVoteArgument)
	log.Printf(fmt.Sprintf("Received RequestVote Argument: %s", requestVoteArgument))
	checkError("Error converting str arg to byte to requestVoteArgument item", err)

	// Get variables
	raft.mu.Lock()
	defer raft.mu.Unlock()
	raft.lastTimeout = time.Now()
	currentTerm := raft.currentTerm
	votedFor := raft.votedFor
	lastLogIndex := len(raft.log) - 1

	requestVoteResponse := RequestVoteResponse{
		Term:        currentTerm,
		VoteGranted: false,
	}

	if (requestVoteArgument.Term > currentTerm) && (votedFor == "" || votedFor == requestVoteArgument.CandidateId) && (requestVoteArgument.LastLogIndex >= lastLogIndex) {
		raft.state = Follower
		raft.votedFor = requestVoteArgument.CandidateId
		raft.currentTerm = requestVoteArgument.Term
		requestVoteResponse.Term = raft.currentTerm
		requestVoteResponse.VoteGranted = true
	}

	// Return the RequestVoteResponse with a decision of whether to grant vote
	return &requestVoteResponse
}

func (raft *Raft) runElectionTimer(callingState State) {
	electionTimeoutDuration := raft.getElectionTimeoutDuration()
	log.Printf(fmt.Sprintf("Running Election Timer with %s state and duration %s", callingState, electionTimeoutDuration))

	raft.mu.Lock()
	raft.lastTimeout = time.Now()
	raft.mu.Unlock()

	electionTicker := time.NewTicker(10 * time.Millisecond)
	defer electionTicker.Stop()

	for {
		<-electionTicker.C

		raft.mu.Lock()
		state := raft.state
		lastTimeout := raft.lastTimeout

		if state != callingState {
			raft.isTimerRunning = false
			raft.mu.Unlock()
			return
		}

		isElectionTimedOut := raft.checkElectionTimeout(lastTimeout, electionTimeoutDuration)
		if isElectionTimedOut {
			raft.timeoutElectionTimer()
			raft.mu.Unlock()
			return
		}

		raft.mu.Unlock()
	}
}

func (raft *Raft) timeoutElectionTimer() {
	log.Printf("Election Timer timed out, becoming candidate")
	raft.isTimerRunning = false
	raft.currentTerm++
	raft.state = Candidate
	raft.votedFor = ""
}

func (raft *Raft) getElectionTimeoutDuration() time.Duration {
	minTimeOut := 2000 // 150 5000
	maxTimeOut := 3000 // 300 10000
	rand.Seed(time.Now().UnixNano())
	randomNum := rand.Intn(maxTimeOut-minTimeOut) + minTimeOut
	randomTimeout := time.Duration(randomNum) * time.Millisecond
	return randomTimeout
}

func (raft *Raft) checkElectionTimeout(lastTimeout time.Time, electionTimeoutDuration time.Duration) bool {
	return time.Now().Sub(lastTimeout).Milliseconds() >= electionTimeoutDuration.Milliseconds()
}

func (raft *Raft) sendAppendEntries(peer string, appendEntriesArgument *AppendEntriesArgument) *AppendEntriesResponse {
	log.Printf(fmt.Sprintf("RequestVote sent from %s to %s", raft.id, peer))

	// Try to marshal the AppendEntries arguments and attach to request message
	result, err := json.Marshal(*appendEntriesArgument)
	log.Printf("Result from marshal %s", result)
	checkError("Error encoding AppendEntries arg %s", err)
	message := fmt.Sprintf("AppendEntries %s\n", string(result))
	log.Printf("Message %s", message)

	// Connect to peer and send the request
	conn, err := net.Dial("tcp", peer)
	checkError(fmt.Sprintf("Error TCP connection to: %s", peer), err)
	request, err := conn.Write([]byte(message))
	checkError(fmt.Sprintf("Error writing to TCP connection to: %s, Request: %d", peer, request), err)

	// Receive reply from peer with RequestVote response
	decoder := json.NewDecoder(conn)
	var response AppendEntriesResponse
	responseErr := decoder.Decode(&response)
	checkError("Error decoding %s", responseErr)
	log.Printf(fmt.Sprintf("AppendEntries response from %s, response: %s", peer, response))

	return &response
}

func (raft *Raft) HandleAppendEntries(message string) *AppendEntriesResponse {
	// Try to unmarshall the message which is a RequestVoteArgument
	var appendEntriesArgument AppendEntriesArgument
	err := json.Unmarshal([]byte(message), &appendEntriesArgument)
	log.Printf(fmt.Sprintf("Received AppendEntries Argument: %s", appendEntriesArgument))
	checkError("Error converting str arg to byte to appendEntriesArgument item", err)

	// Get variables
	raft.mu.Lock()
	raft.lastTimeout = time.Now()
	currentTerm := raft.currentTerm
	raft.mu.Unlock()

	requestVoteResponse := AppendEntriesResponse{
		Term:    currentTerm,
		Success: true,
	}

	if appendEntriesArgument.Term < currentTerm {
		requestVoteResponse.Success = false
	}

	//if true {
	//	raft.mu.Lock()
	//	raft.mu.Unlock()
	//	requestVoteResponse.Success = true
	//}

	// Return the RequestVoteResponse with a decision of whether to grant vote
	return &requestVoteResponse
}

func (raft *Raft) AddToLog(command string) {
	raft.mu.Lock()
	defer raft.mu.Unlock()
	entry := LogEntry{
		Term:    raft.currentTerm,
		Command: command,
	}
	raft.log = append(raft.log, entry)
}
