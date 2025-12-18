package main

import (
	"fmt"
	"log"
	"net/http"
	"raft/raft"

	"github.com/gin-gonic/gin"
)

// HTTPPorts maps Node IDs to their client-facing HTTP ports.
var HTTPPorts = map[int]string{
	1: "9001",
	2: "9002",
	3: "9003",
}

// RaftServer wraps the Raft node to provide HTTP handlers.
type RaftServer struct {
	NodeID int
	Node   *raft.Node
}

// StartHTTPServer checks the port and starts the listener.
// This is the entry point called by main.go.
func StartHTTPServer(nodeID int, node *raft.Node) {
	server := &RaftServer{
		NodeID: nodeID,
		Node:   node,
	}

	// Run in a goroutine so it doesn't block main.go
	go server.start()
}

func (s *RaftServer) start() {
	// Set Gin to release mode to reduce console noise
	gin.SetMode(gin.ReleaseMode)
	router := gin.Default()

	// Register routes
	router.POST("/submit", s.handleSubmit)

	port := HTTPPorts[s.NodeID]
	log.Printf("HTTP Server for Node %d listening on :%s", s.NodeID, port)

	if err := router.Run(":" + port); err != nil {
		log.Fatalf("Failed to start HTTP server for Node %d: %v", s.NodeID, err)
	}
}

// handleSubmit is the specific handler for client commands.
func (s *RaftServer) handleSubmit(c *gin.Context) {
	// 1. Parse Request
	var req struct {
		Command string `json:"command"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid JSON"})
		return
	}

	// 2. Submit to Raft Logic
	success, leaderID := s.Node.Submit(req.Command)

	if success {
		// Case A: We are the leader and accepted the write.
		c.JSON(http.StatusOK, gin.H{
			"status": "committed",
			"leader": s.NodeID,
		})
		return
	}

	// 3. Handle Failure / Redirection
	s.handleRedirect(c, leaderID)
}

// handleRedirect calculates where the client should go next.
func (s *RaftServer) handleRedirect(c *gin.Context, leaderID int) {
	// Case B: Unknown leader (Election in progress)
	if leaderID == -1 {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error": "Election in progress, please retry",
		})
		return
	}

	// Case C: Known leader -> Redirect
	leaderPort, ok := HTTPPorts[leaderID]
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Leader configuration missing"})
		return
	}

	leaderURL := fmt.Sprintf("http://localhost:%s/submit", leaderPort)

	fmt.Printf("🔀 Node %d redirecting to Leader Node %d (%s)\n", s.NodeID, leaderID, leaderURL)

	//  Set the Location header so the client knows WHERE to go.
	c.Header("Location", leaderURL)
	//  Send 307 so the client knows to RESEND the data.
	c.JSON(http.StatusTemporaryRedirect, gin.H{
		"error":  "Not Leader",
		"leader": leaderID,
		"url":    leaderURL,
	})
}
