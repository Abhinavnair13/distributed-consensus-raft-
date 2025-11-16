// Filename: raft/raft.go
package raft

import (
	"math/rand"
	"sync"
	"time"

	"go.uber.org/zap"
)

// State is the enum for a Raft node.
type State int

const (
	Follower State = iota
	Candidate
	Leader
	Dead
)

type LogEntry struct {
	Term    int
	Command interface{} // The command for the state machine
}

// Config holds the configuration for a Raft node.
// This is injected upon creation.
type Config struct {
	ID     int // Unique ID for this node
	Logger *zap.SugaredLogger
	Peers  []int // IDs of all other nodes in the cluster
}

type Node struct {
	mu     sync.Mutex // Protects all fields below
	logger *zap.SugaredLogger

	// Persistent state (must be saved to stable storage)
	currentTerm int
	votedFor    int
	log         []LogEntry

	// Volatile state on all servers
	commitIndex int
	lastApplied int
	state       State

	// Private fields
	id    int
	peers []int

	// electionReset time.Time // We'll manage timers directly
}

func NewNode(cfg *Config) *Node {
	nodeLogger := cfg.Logger.With("nodeID", cfg.ID)
	nodeLogger.Debug("hi there")

	return &Node{
		logger: nodeLogger,
		state:  Follower,
		id:     cfg.ID,
		peers:  cfg.Peers,
	}
}

// RunElectionTimer starts the main loop for the Raft node.
func (n *Node) RunElectionTimer() {
	timeout := time.Duration(150+rand.Intn(150)) * time.Millisecond
	timer := time.NewTimer(timeout)

	n.logger.Infow("Starting as a Follower.",
		"timeout", timeout,
	)

	for {
		select {
		case <-timer.C:
			// Timer fired. Time to start an election.
			n.mu.Lock()

			// Only start an election if we are a Follower
			if n.state == Follower {
				n.state = Candidate
				n.currentTerm++
				n.votedFor = n.id

				n.logger.Warnw("Timed out. Becoming a Candidate.",
					"newTerm", n.currentTerm,
				)

				// TODO: Send RequestVote RPCs to all peers in a new goroutine
			}

			// Reset the timer for the next potential election
			timer.Reset(time.Duration(150+rand.Intn(150)) * time.Millisecond)
			n.mu.Unlock()
		}
	}
}

// --- RPC Structs ---
// These define the API between Raft nodes, as per the paper.

// RequestVoteArgs are the arguments for the RequestVote RPC.
type RequestVoteArgs struct {
	Term         int
	CandidateId  int
	LastLogIndex int
	LastLogTerm  int
}

// RequestVoteReply is the reply from the RequestVote RPC.
type RequestVoteReply struct {
	Term        int
	VoteGranted bool
}

// AppendEntriesArgs are the arguments for the AppendEntries RPC.
type AppendEntriesArgs struct {
	Term         int
	LeaderId     int
	PrevLogIndex int
	PrevLogTerm  int
	Entries      []LogEntry
	LeaderCommit int
}

// AppendEntriesReply is the reply from the AppendEntries RPC.
type AppendEntriesReply struct {
	Term    int
	Success bool
}
