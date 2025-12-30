package raft

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// PersistentState defines what we must save to disk.
type PersistentState struct {
	CurrentTerm int
	VotedFor    int
	Log         []LogEntry
}

// Storage handles file I/O for Raft persistence.
type Storage struct {
	mu       sync.Mutex
	filename string
}

func NewStorage(nodeID int) *Storage {

	dir := "node_storage"

	if err := os.MkdirAll(dir, 0755); err != nil {
		panic(fmt.Sprintf("Failed to create storage directory: %v", err))
	}

	// 3. Construct the full path: node_storage/raft_storage_1.json
	filename := fmt.Sprintf("raft_storage_%d.json", nodeID)
	fullPath := filepath.Join(dir, filename)

	return &Storage{filename: fullPath}
}

func (s *Storage) save(term int, votedFor int, log []LogEntry) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	state := PersistentState{
		CurrentTerm: term,
		VotedFor:    votedFor,
		Log:         log,
	}

	data, err := json.Marshal(state)
	if err != nil {
		return err
	}

	// atomic write: write to temp file then rename
	tmpFile := s.filename + ".tmp"
	if err := os.WriteFile(tmpFile, data, 0644); err != nil {
		return err
	}
	return os.Rename(tmpFile, s.filename)
}

func (s *Storage) Load() (*PersistentState, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := os.ReadFile(s.filename)
	if os.IsNotExist(err) {
		return nil, nil // First boot, no file yet
	}
	if err != nil {
		return nil, err
	}

	var state PersistentState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, err
	}
	return &state, nil
}

// Helper to save state
func (n *Node) persist() {
	if err := n.storage.save(n.currentTerm, n.votedFor, n.log); err != nil {
		n.logger.Errorw("Failed to persist state", "error", err)
	}
}
