Kevin Chen

# Project 4

Files in src/backend, src/frontend, src/raft

## Instructions

### Running
```
go run backend.go --listen 8090 --backend :8091,:8092
go run backend.go --listen 8091 --backend :8090,:8092
go run backend.go --listen 8092 --backend :8090,:8091
go run frontend.go --listen 8080 --backend :8090,:8091,:8092
```
### Building
```
go build
go install
```
### Testing
```
./backend --listen 8090 --backend :8091,:8092
./backend --listen 8091 --backend :8090,:8092
./backend --listen 8092 --backend :8090,:8091
./frontend --listen 8080 --backend :8090,:8091,:8092
```

## State of Project 
Project not completed. Implemented leader election, communication between backends and frontend,
heartbeat messages from leader.
Did not finish implementing log replication. 

## Resources
- [Raft Paper](https://raft.github.io/raft.pdf) + Figure 2 in paper
- [Raft visualization overview and steps ](http://thesecretlivesofdata.com/raft/)
- [Go Ticker](https://gobyexample.com/tickers)
- [Understanding Distributed Consensus with Raft](https://kasunindrasiri.medium.com/understanding-raft-distributed-consensus-242ec1d2f521)

## Resilience
Did not finish log replication

## Design Decisions
1. Replication Strategy: Raft. Easy to understand and seemed simplest to implement. With Raft, the commands be store
on the logs and replicated to all followers/applied to data when committed.
2. Pros: intuitive to understand, Cons: 
3. To develop further, I would add a channel on the backend to wait before replying to the frontend.
