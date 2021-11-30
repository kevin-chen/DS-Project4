package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"net"
	"raft"
	"strings"
	"sync"
)

var httpPort *string
var raftNode *raft.Raft
var allClear = make(chan int)
var wg sync.WaitGroup

func checkError(mes string, err error) {
	if err != nil {
		fmt.Println(mes, err)
		return
	}
}

func handleConnection(conn net.Conn) {
	requestMessage, err := bufio.NewReader(conn).ReadString('\n')
	checkError("Error reading connection", err)

	requestSplitMessage := strings.Split(requestMessage, " ")
	fmt.Println(requestSplitMessage[0] == "AllClear")
	if requestSplitMessage[0] == "RequestVote" {
		// On receive of RequestVote, call raft node to handle the message
		response := raftNode.HandleRequestVote(requestSplitMessage[1])
		// Gets response of whether to grant vote and Marshal it to send to requester
		result, err := json.Marshal(*response)
		checkError("Error encoding request vote", err)
		conn.Write(result)
	} else if requestSplitMessage[0] == "AppendEntries" {
		response := raftNode.HandleAppendEntries(requestSplitMessage[1])
		result, err := json.Marshal(*response)
		checkError("Error encoding append entries", err)
		conn.Write(result)
	} else if requestSplitMessage[0] == "AllClear" {
		allClear <- 0
		response := "Received AllClear\n"
		fmt.Fprintf(conn, response)
	}

	conn.Close()
}

func main() {
	httpPort = flag.String(
		"listen",
		"8090",
		"http port number for current backend server to listen on",
	)
	backendPorts := flag.String("backend", ":8091", "list of other backends available")
	flag.Parse()
	backendList := strings.Split(*backendPorts, ",")
	fmt.Println("BackendList:", backendList)

	wg.Add(1)
	go func() {
		defer wg.Done()
		ln, err := net.Listen("tcp", ":"+*httpPort)
		checkError("Error listening to port", err)
		//fmt.Println(fmt.Sprintf("Listening to frontend on port: %v", *httpPort))
		defer ln.Close()
		for {
			conn, err := ln.Accept()
			checkError("Error accepting connection", err)
			go handleConnection(conn)
		}
	}()

	// Wait for all clear from frontend
	//<-allClear

	raftNode = raft.NewNode(*httpPort, backendList)
	fmt.Println(raftNode)

	wg.Wait()
}
