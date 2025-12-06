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
	ID        int // Unique ID for this node
	Logger    *zap.SugaredLogger
	Peers     map[int]string // Map of peer IDs to their addresses
	Transport Transport      //transport to communicate
}

type Node struct {
	mu     sync.Mutex // Protects all fields below
	logger *zap.SugaredLogger

	// Persistent state (must be saved to stable storage)
	currentTerm int
	votedFor    int
	log         []LogEntry
	// This channel is used to reset the election timer
	// when we get a valid heartbeat or grant a vote.
	resetElectionTimer chan bool

	// Volatile state on all servers
	commitIndex int
	lastApplied int
	state       State

	// Private fields
	id    int
	peers map[int]string // Map of peer IDs to their addresses

	transport Transport
	// electionReset time.Time // We'll manage timers directly
}

func NewNode(cfg *Config) *Node {
	nodeLogger := cfg.Logger.With("nodeID", cfg.ID)

	return &Node{
		logger:             nodeLogger,
		state:              Follower,
		id:                 cfg.ID,
		peers:              cfg.Peers,
		transport:          cfg.Transport,
		resetElectionTimer: make(chan bool, 1),
	}
}

// RunElectionTimer starts the main loop for the Raft node.
func (n *Node) RunElectionTimer() {
	getTimeout := func() time.Duration {
		return time.Duration(2000+rand.Intn(2000)) * time.Millisecond
	}
	initialDuration := getTimeout()

	timer := time.NewTimer(initialDuration)
	n.logger.Infow("Starting as a Follower.", "timeout", initialDuration)

	for {
		select {
		//This channel fires exactly once when the timer expires.
		case <-timer.C:
			// Timer fired. Time to start an election.
			n.mu.Lock()
			if n.state == Follower {
				n.state = Candidate
				n.currentTerm++
				n.votedFor = n.id

				n.logger.Warnw("Timed out. Becoming a Candidate.", "newTerm", n.currentTerm)
				go n.startElection()
			}
			// Reset for another (potential) election
			timer.Reset(getTimeout())
			n.mu.Unlock()

		case <-n.resetElectionTimer:
			// We received a valid RPC. Reset the timer.
			if !timer.Stop() {
				select {
				case <-timer.C: // Drain timer
				default: //prevent blocking
				}
			}
			timer.Reset(getTimeout())
			n.logger.Debug("Election timer reset by RPC")
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

// In raft.go

func (n *Node) startElection() {
	n.mu.Lock()
	n.currentTerm++
	n.votedFor = n.id
	term := n.currentTerm
	args := &RequestVoteArgs{
		Term:        term,
		CandidateId: n.id,
	}
	n.mu.Unlock()

	var votesGranted int = 1
	var voteMu sync.Mutex

	n.logger.Infow("Starting election", "term", term)

	for peerID, addr := range n.peers {
		if peerID == n.id {
			continue
		}

		go func(peerID int, targetAddr string) {
			// 1. Send RPC
			reply, err := n.transport.SendRequestVote(targetAddr, args)
			if err != nil {
				return
			}

			n.mu.Lock()
			defer n.mu.Unlock()

			// 2. Check if we are still a candidate and term hasn't changed
			if n.state != Candidate || n.currentTerm != term {
				return
			}

			// 3. Check if peer has a higher term
			if reply.Term > term {
				n.currentTerm = reply.Term
				n.state = Follower
				n.votedFor = -1
				return
			}

			// 4. Count the vote
			if reply.VoteGranted {
				voteMu.Lock()
				votesGranted++
				majority := (len(n.peers))/2 + 1

				// 5. Become Leader if majority reached
				if votesGranted >= majority && n.state == Candidate {
					n.logger.Infow("Became Leader", "term", n.currentTerm)
					n.state = Leader
					// Start sending heartbeats immediately!
					go n.startHeartbeatLoop()
				}
				voteMu.Unlock()
			}
		}(peerID, addr)
	}
}

// HandleRequestVote is the callback from the transport layer.
func (n *Node) HandleRequestVote(args *RequestVoteArgs, reply *RequestVoteReply) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.logger.Debugw("Handling RequestVote RPC", "args", args)

	// Rule 1: Reply false if term < currentTerm
	if args.Term < n.currentTerm {
		reply.Term = n.currentTerm
		reply.VoteGranted = false
		return nil
	}

	// Rule: If we receive a request with a higher term,
	// we must step down and become a follower.
	if args.Term > n.currentTerm {
		n.state = Follower
		n.currentTerm = args.Term
		n.votedFor = -1 // -1 means null
	}

	reply.Term = n.currentTerm

	// Rule 2: If votedFor is null (-1) or candidateId
	grantVote := (n.votedFor == -1 || n.votedFor == args.CandidateId)

	// Rule 3: ... and candidate's log is at least as up-to-date
	// (will stub this for now, as it's the next big step)
	logIsUpToDate := true // TODO: Implement log comparison logic

	if grantVote && logIsUpToDate {
		n.logger.Infow("Granting vote", "candidate", args.CandidateId, "term", args.Term)
		n.votedFor = args.CandidateId
		reply.VoteGranted = true

		// We granted a vote, so we must reset our timer
		// Use a non-blocking send in case channel is full
		select {
		case n.resetElectionTimer <- true:
		default:
		}
	} else {
		reply.VoteGranted = false
	}

	return nil
}

// HandleAppendEntries is the callback for heartbeats and log replication.
func (n *Node) HandleAppendEntries(args *AppendEntriesArgs, reply *AppendEntriesReply) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.logger.Debugw("Handling AppendEntries RPC (heartbeat)", "args", args)

	// Rule 1: Reply false if term < currentTerm
	if args.Term < n.currentTerm {
		reply.Term = n.currentTerm
		reply.Success = false
		return nil
	}

	// Rule: If we receive a request with a higher term,
	// we must step down and become a follower.
	if args.Term > n.currentTerm {
		n.state = Follower
		n.currentTerm = args.Term
		n.votedFor = -1 // -1 means null
	}

	reply.Term = n.currentTerm

	// This is a valid heartbeat from a real leader
	// If we were a Candidate, we must step down to Follower
	if n.state == Candidate {
		n.state = Follower
		n.logger.Info("Stepping down from Candidate to Follower")
	}

	// This is the most important part of a heartbeat:
	// We reset our election timer because the leader is alive.
	select {
	case n.resetElectionTimer <- true:
	default:
	}

	// TODO: Add log consistency checks (Rules 3, 4, 5)

	reply.Success = true
	return nil
}
func (n *Node) startHeartbeatLoop() {
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			n.mu.Lock()
			if n.state != Leader {
				n.mu.Unlock()
				return
			}
			term := n.currentTerm
			leaderId := n.id
			n.mu.Unlock()

			// Send empty AppendEntries to all peers
			for peerID, addr := range n.peers {
				if peerID == n.id {
					continue
				}
				//For a heartbeat, the Entries list is empty.
				// It just carries the Term and LeaderID to prove to the followers that the leader is still alive and dominan
				go func(address string) {
					args := &AppendEntriesArgs{
						Term:     term,
						LeaderId: leaderId,
					}
					n.transport.SendAppendEntries(address, args)
				}(addr)
			}
		}
	}
}
