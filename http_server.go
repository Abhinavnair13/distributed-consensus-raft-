package main

import (
	"fmt"
	"log"
	"net/http"
	"raft/raft"

	"github.com/gin-gonic/gin"
)

// HTTPPorts maps Node IDs to their client-facing HTTP ports.
// We initialize this as nil; main.go will fill it on startup.
var HTTPPorts map[int]string

// RaftServer wraps the Raft node to provide HTTP handlers.
type RaftServer struct {
	NodeID int
	Node   *raft.Node
}

// StartHTTPServer checks the port and starts the listener.
func StartHTTPServer(nodeID int, node *raft.Node) {
	server := &RaftServer{
		NodeID: nodeID,
		Node:   node,
	}
	go server.start()
}

func (s *RaftServer) start() {
	gin.SetMode(gin.ReleaseMode)
	router := gin.Default()
	router.POST("/submit", s.handleSubmit)

	port := HTTPPorts[s.NodeID]
	log.Printf("HTTP Server for Node %d listening on :%s", s.NodeID, port)

	if err := router.Run(":" + port); err != nil {
		log.Fatalf("Failed to start HTTP server for Node %d: %v", s.NodeID, err)
	}
}

func (s *RaftServer) handleSubmit(c *gin.Context) {
	var req struct {
		Command string `json:"command"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid JSON"})
		return
	}

	success, leaderID := s.Node.Submit(req.Command)

	if success {
		c.JSON(http.StatusOK, gin.H{"status": "committed", "leader": s.NodeID})
		return
	}
	s.handleRedirect(c, leaderID)
}

func (s *RaftServer) handleRedirect(c *gin.Context, leaderID int) {
	if leaderID == -1 {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Election in progress"})
		return
	}

	leaderPort, ok := HTTPPorts[leaderID]
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Leader config missing"})
		return
	}

	leaderURL := fmt.Sprintf("http://localhost:%s/submit", leaderPort)
	fmt.Printf("🔀 Redirecting to Leader Node %d (%s)\n", leaderID, leaderURL)

	c.Header("Location", leaderURL)
	c.JSON(http.StatusTemporaryRedirect, gin.H{
		"error":  "Not Leader",
		"leader": leaderID,
		"url":    leaderURL,
	})
}
