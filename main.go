package main

import (
	"flag"
	"fmt"
	"log"
	"raft/raft"
)

func main() {

	nodeID := flag.Int("id", 1, "The ID of this node")
	count := flag.Int("n", 5, "Total number of nodes in the cluster")
	flag.Parse()

	appCfg, err := NewConfig()
	if err != nil {
		log.Fatalf("Failed to initialize config: %v", err)
	}

	peers := make(map[int]string)

	HTTPPorts = make(map[int]string)

	for i := 1; i <= *count; i++ {
		peers[i] = fmt.Sprintf("localhost:%d", 8000+i)
		HTTPPorts[i] = fmt.Sprintf("%d", 9000+i)
	}

	if _, ok := peers[*nodeID]; !ok {
		log.Fatalf("Invalid Node ID: %d. Must be between 1 and %d", *nodeID, *count)
	}

	appCfg.Logger.Infow("Starting Raft Node...", "nodeID", *nodeID, "clusterSize", *count)

	transport := raft.NewGrpcTransport(peers[*nodeID])
	raftCfg := &raft.Config{
		ID:        *nodeID,
		Logger:    appCfg.Logger,
		Peers:     peers,
		Transport: transport,
	}
	node := raft.NewNode(raftCfg)

	go func() {
		if err := transport.ListenAndServe(node); err != nil {
			log.Fatalf("Failed to start RPC server: %v", err)
		}
	}()
	appCfg.Logger.Info("RPC Server started")

	StartHTTPServer(*nodeID, node)

	go node.RunElectionTimer()

	select {}
}
