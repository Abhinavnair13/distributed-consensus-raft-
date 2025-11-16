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

	// 2. Create the Raft Node's specific config
	// We pass the application logger *into* the raft config.
	raftCfg := &raft.Config{
		ID:     1, // Or get this from appCfg
		Logger: appCfg.Logger,
	}

	// 3. Create the Node from the raft package
	node := raft.NewNode(raftCfg)

	// 4. Start the node's main loop
	go node.RunElectionTimer()

	appCfg.Logger.Info("Application started. Node is running.")
	select {}
}
