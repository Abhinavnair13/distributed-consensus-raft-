package main

import (
	"log"
	"raft/raft"
)

func main() {
	appCfg, err := NewConfig()
	if err != nil {
		log.Fatalf("Failed to initialize config: %v", err)
	}

	appCfg.Logger.Info("Initializing Raft Cluster...")

	// We map Node IDs to their local addresses.
	peers := map[int]string{
		1: "localhost:8001",
		2: "localhost:8002",
		3: "localhost:8003",
	}

	nodes := make([]*raft.Node, 0)

	// We loop through the map to instantiate each node.
	for id, addr := range peers {
		// A. Create the Transport
		// This tells the node how to listen on its specific port.
		transport := raft.NewRpcTransport(addr)

		// B. Create the Node Configuration
		raftCfg := &raft.Config{
			ID:        id,
			Logger:    appCfg.Logger,
			Peers:     peers,
			Transport: transport, // Inject the transport layer
		}

		// C. Create the Node instances
		node := raft.NewNode(raftCfg)
		nodes = append(nodes, node)

		// D. Start the RPC Listener
		// We run this in a goroutine because ListenAndServe is blocking.
		go func(t *raft.NetRpcTransport, n *raft.Node) {
			if err := t.ListenAndServe(n); err != nil {
				log.Fatalf("Failed to start RPC server for node %d: %v", id, err)
			}
		}(transport, node)
	}

	// Start the Election Timers
	// We start these *after* all RPC servers are up to minimize connection errors.
	appCfg.Logger.Info("Cluster initialized. Starting election timers...")
	for _, node := range nodes {
		go node.RunElectionTimer()
	}

	// Block forever
	select {}
}
