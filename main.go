package main

import (
	"flag"
	"fmt"
	"log"
	"raft/raft"
)

func main() {
	// 1. Parse flags
	// Example: go run . -id 1 -n 5  (Starts Node 1 of a 5-node cluster)
	nodeID := flag.Int("id", 1, "The ID of this node")
	count := flag.Int("n", 3, "Total number of nodes in the cluster")
	flag.Parse()

	appCfg, err := NewConfig()
	if err != nil {
		log.Fatalf("Failed to initialize config: %v", err)
	}

	// 2. Dynamically Generate Topology
	// We assume a strict convention:
	// - RPC Port  = 8000 + ID
	// - HTTP Port = 9000 + ID
	peers := make(map[int]string)

	// Initialize the global HTTPPorts map from http_server.go
	HTTPPorts = make(map[int]string)

	for i := 1; i <= *count; i++ {
		peers[i] = fmt.Sprintf("localhost:%d", 8000+i)
		HTTPPorts[i] = fmt.Sprintf("%d", 9000+i)
	}

	// Validation
	if _, ok := peers[*nodeID]; !ok {
		log.Fatalf("Invalid Node ID: %d. Must be between 1 and %d", *nodeID, *count)
	}

	appCfg.Logger.Infow("Starting Raft Node...", "nodeID", *nodeID, "clusterSize", *count)

	// 3. Configure and Start Node
	transport := raft.NewRpcTransport(peers[*nodeID])
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

	// 4. Start HTTP Server
	StartHTTPServer(*nodeID, node)

	// 5. Start Election Timer
	go node.RunElectionTimer()

	select {}
}
