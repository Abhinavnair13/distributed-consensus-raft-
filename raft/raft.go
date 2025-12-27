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
	commitIndex   int
	lastApplied   int
	state         State
	currentLeader int

	// Private fields
	id    int
	peers map[int]string // Map of peer IDs to their addresses

	// --- NEW LEADER STATE ---
	// Reinitialized after election
	nextIndex  map[int]int
	matchIndex map[int]int

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
		currentLeader:      -1,
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
			// If we are NOT a leader, we should start an election.
			// This covers both:
			// 1. Follower -> Candidate (Leader died)
			// 2. Candidate -> Candidate (Election timeout/split vote - RETRY)
			if n.state != Leader {
				n.logger.Warn("Election timeout - Starting new election")
				go n.startElection() // Delegate everything to this function
			}
			// Reset for another (potential) election
			n.mu.Unlock()
			timer.Reset(getTimeout())

		case <-n.resetElectionTimer:
			// We received a valid RPC. Reset the timer.
			if !timer.Stop() {
				select {
				case <-timer.C: // Drain timer
				default: //prevent blocking
				}
			}
			timer.Reset(getTimeout())
			// n.logger.Debug("Election timer reset by RPC")
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
	n.state = Candidate
	n.currentTerm++
	n.votedFor = n.id
	term := n.currentTerm
	savedId := n.id
	args := &RequestVoteArgs{
		Term:        term,
		CandidateId: savedId,
	}
	n.mu.Unlock()

	var votesGranted int = 1
	var voteMu sync.Mutex

	n.logger.Infow("Starting election", "term", term)

	for peerID, addr := range n.peers {
		if peerID == savedId {
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

					// Initialize leader state maps
					n.nextIndex = make(map[int]int)
					n.matchIndex = make(map[int]int)

					// Initialize nextIndex to leader's last log index + 1
					// (Using len(n.log) implies 0-indexed log logic for simplicity)
					for peerID := range n.peers {
						n.nextIndex[peerID] = len(n.log)
						n.matchIndex[peerID] = -1 // Nothing matched yet
					}
					// Start sending heartbeats immediately!
					go n.startReplicationLoop()
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

	// --- 1. Term Checks (Standard Raft Logic) ---
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
		n.votedFor = -1
	}
	n.currentLeader = args.LeaderId

	// 3. If we are a Candidate and receive a valid AppendEntries from the
	// current term leader, we must step down.
	if n.state == Candidate {
		n.state = Follower
		n.logger.Info("Stepping down from Candidate to Follower")
	}

	// We recognize the leader, so we reset the timer
	select {
	case n.resetElectionTimer <- true:
	default:
	}
	reply.Term = n.currentTerm
	// --- 2. Log Consistency Check (The "Gatekeeper") ---

	// Check A: Do we even have the log index the leader is referring to?
	// If Leader says "I am sending data after Index 10", and we only have 5 entries,
	// we are missing data. We must return False so the Leader decrements nextIndex.
	if args.PrevLogIndex >= len(n.log) {
		reply.Success = false
		return nil
	}

	// Check B: Does the term match at that index?
	// If Leader says "The entry at Index 5 was Term 2", but our entry at Index 5
	// is Term 1, we have a conflict. We must return False.
	// (We skip this check if PrevLogIndex is -1, which means "start of log")
	if args.PrevLogIndex >= 0 {
		if n.log[args.PrevLogIndex].Term != args.PrevLogTerm {
			reply.Success = false
			return nil
		}
	}

	// --- 3. Append Entries (The "Write") ---

	// If we survived the checks above, our log matches the Leader's up to PrevLogIndex.
	// Now we append the new entries (if any).
	if len(args.Entries) > 0 {
		// We delete everything after PrevLogIndex and append the new entries.
		// This ensures our log becomes identical to the Leader's from this point on.
		n.log = append(n.log[:args.PrevLogIndex+1], args.Entries...)
		n.logger.Infow("Appended entries", "count", len(args.Entries), "newLogLen", len(n.log))
	}

	// TODO: Update Commit Index (Next step)

	reply.Success = true
	return nil
}
func (n *Node) startReplicationLoop() {
	// Send heartbeats/replication every 100ms
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for range ticker.C {
		n.mu.Lock()
		if n.state != Leader {
			n.mu.Unlock()
			return
		}
		savedTerm := n.currentTerm
		savedLeaderID := n.id
		n.mu.Unlock()

		for peerID, addr := range n.peers {
			if peerID == n.id {
				continue
			}

			go func(pID int, address string) {
				n.mu.Lock()
				// 1. Get the next index for this follower
				ni := n.nextIndex[pID]

				// 2. Prepare the entries to send
				var entries []LogEntry
				prevLogIndex := ni - 1
				prevLogTerm := -1

				// Check bounds for prevLogIndex
				if prevLogIndex >= 0 && prevLogIndex < len(n.log) {
					prevLogTerm = n.log[prevLogIndex].Term
				}

				// If we have new entries, slice them from the log
				if ni < len(n.log) {
					entries = n.log[ni:]
				}

				args := &AppendEntriesArgs{
					Term:         savedTerm,
					LeaderId:     savedLeaderID,
					PrevLogIndex: prevLogIndex,
					PrevLogTerm:  prevLogTerm,
					Entries:      entries,
					// LeaderCommit: n.commitIndex, // TODO: Add later
				}
				n.mu.Unlock()

				reply, err := n.transport.SendAppendEntries(address, args)
				if err != nil {
					return
				}

				n.mu.Lock()
				defer n.mu.Unlock()

				if reply.Term > savedTerm {
					n.state = Follower
					n.currentTerm = reply.Term
					n.votedFor = -1
					return
				}

				if reply.Success {
					// Success! Advance nextIndex and matchIndex for this peer
					if len(entries) > 0 {
						n.nextIndex[pID] = ni + len(entries)
						n.matchIndex[pID] = n.nextIndex[pID] - 1
						n.logger.Debugw("AppendEntries success", "peer", pID, "term", savedTerm, "nextIndex", n.nextIndex[pID])
					}
				} else {
					// Failure: Follower is behind. Decrement nextIndex and retry.
					// This is the simplified log repair mechanism.
					if n.nextIndex[pID] > 0 {
						n.nextIndex[pID]--
					}
					n.logger.Debugw("AppendEntries rejected, decrementing nextIndex", "peer", pID, "term", savedTerm, "nextIndex", n.nextIndex[pID])
				}
			}(peerID, addr)
		}
	}
}

// Submit attempts to append a command to the log.
// Returns:
// 1. success (bool): True if the command was accepted.
// 2. leaderID (int): The ID of the current leader (if known), or -1.
func (n *Node) Submit(command interface{}) (bool, int) {
	n.mu.Lock()
	defer n.mu.Unlock()

	if n.state != Leader {
		return false, n.currentLeader // Return false and point to the known leader
	}

	// Append command to local log
	entry := LogEntry{
		Term:    n.currentTerm,
		Command: command,
	}
	n.log = append(n.log, entry)
	n.logger.Infow("Log command received", "command", command, "index", len(n.log)-1)

	return true, n.id
}
