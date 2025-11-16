package raft

// RpcHandler defines the interface that the transport layer uses to
type RpcHandler interface {
	HandleRequestVote(args *RequestVoteArgs, reply *RequestVoteReply) error
	HandleAppendEntries(args *AppendEntriesArgs, reply *AppendEntriesReply) error
}

// Transport defines the interface for the network transport layer.
type Transport interface {

	// ListenAndServe starts the transport and blocks.
	// It takes a handler (our Raft node) to pass RPCs to.
	ListenAndServe(handler RpcHandler) error

	// SendRequestVote sends a RequestVote RPC to a target peer.
	SendRequestVote(targetAddr string, args *RequestVoteArgs) (*RequestVoteReply, error)

	// SendAppendEntries sends an AppendEntries RPC to a target peer.
	SendAppendEntries(targetAddr string, args *AppendEntriesArgs) (*AppendEntriesReply, error)
}
