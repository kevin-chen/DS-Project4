package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"net"
	"raft"
	"strconv"
	"strings"
	"sync"
)

var httpPort *string
var raftNode *raft.Raft
var allClear = make(chan int)
var wg sync.WaitGroup

type Umbrella struct {
	Name         string `json:"name"`
	Price        int    `json:"price"`
	Brand        string `json:"brand"`
	Availability bool   `json:"availability"`
	Color        string `json:"color"`
}

var inventory = make([]Umbrella, 0)

func checkError(mes string, err error) {
	if err != nil {
		fmt.Println(mes, err)
		return
	}
}

func applyAction(message string) {
	// reading msg
	action := ""
	index := -1
	var err error
	requestSplitMessage := strings.Split(message, " ")
	for _, item := range requestSplitMessage {
		if action == "" {
			action = strings.TrimRight(item, "\n")
			if action == "READ" && len(requestSplitMessage) < 2 {
				result, err := json.Marshal(inventory)
				checkError("Error encoding json", err)
				fmt.Println("Sending Back Data:", result)
				//conn.Write(result)
			}
		} else {
			if action == "READ" {
				index, err = strconv.Atoi(strings.TrimRight(item, "\n"))
				checkError("Error converting str index to int", err)
				result, err := json.Marshal(inventory[index])
				checkError("Error encoding json", err)
				fmt.Println("Sending Back Data:", result)
				//conn.Write(result)
			} else if action == "DELETE" {
				index, err = strconv.Atoi(strings.TrimRight(item, "\n"))
				checkError("Error converting str index to int", err)
				inventory = append(inventory[:index], inventory[index+1:]...)
				result, err := json.Marshal("Success")
				checkError("Error encoding json", err)
				fmt.Println("Sending Back Data:", result)
				//conn.Write(result)
			} else if action == "UPDATE" {
				if index != -1 {
					var updateItem Umbrella
					err := json.Unmarshal([]byte(item), &updateItem)
					checkError("Error converting str item to byte to Umbrella item", err)
					inventory[index] = updateItem
				} else {
					index, err = strconv.Atoi(strings.TrimRight(item, "\n"))
					checkError("Error converting str index to int", err)
				}
			} else if action == "CREATE" {
				var newItem Umbrella
				// Converts the string back to array of bytes and convert to struct object
				err := json.Unmarshal([]byte(item), &newItem)
				checkError("Error encoding json", err)
				inventory = append(inventory, newItem)
			}
		}
	}
}

func handleConnection(conn net.Conn) {
	requestMessage, err := bufio.NewReader(conn).ReadString('\n')
	checkError("Error reading connection", err)

	requestSplitMessage := strings.Split(requestMessage, " ")
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
		checkError("Error encoding append entries", err)
		response := "Received AllClear\n"
		conn.Write([]byte(response))
	} else if requestSplitMessage[0] == "CheckLeader" {
		response := raftNode.HandleCheckLeader()
		if response {
			msg := "Leader"
			conn.Write([]byte(msg))
		} else {
			msg := "NotLeader"
			conn.Write([]byte(msg))
		}
	} else {
		if raftNode.HandleCheckLeader() {
			raftNode.AddToLog(requestMessage)
		}
		//applyAction("")
	}

	conn.Close()
}

func main() {
	httpPort = flag.String(
		"listen",
		"8090",
		"http port number for current backend server to listen on",
	)
	backendPorts := flag.String("backend", "", "list of other backends available")
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
