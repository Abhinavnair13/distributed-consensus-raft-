package raft

import (
	"net"
	"net/rpc"
)

type RpcBridge struct {
	handler RpcHandler
}

func (b *RpcBridge) RequestVote(args *RequestVoteArgs, reply *RequestVoteReply) error {
	return b.handler.HandleRequestVote(args, reply)
}
func (b *RpcBridge) AppendEntries(args *AppendEntriesArgs, reply *AppendEntriesReply) error {
	return b.handler.HandleAppendEntries(args, reply)
}

type NetRpcTransport struct {
	addr     string
	listener net.Listener
}

func NewRpcTransport(addr string) *NetRpcTransport {
	return &NetRpcTransport{addr: addr}
}
func (t *NetRpcTransport) ListenAndServe(handler RpcHandler) error {
	server := rpc.NewServer()

	// Create the bridge and register it
	bridge := &RpcBridge{handler: handler}
	if err := server.Register(bridge); err != nil {
		return err
	}
	l, err := net.Listen("tcp", t.addr)
	if err != nil {
		return err
	}
	t.listener = l
	//todo log

	// Start accepting connections
	for {
		conn, err := t.listener.Accept()
		if err != nil {
			//Todo
			return err
		}
		go server.ServeConn(conn)
	}
}

func (t *NetRpcTransport) SendRequestVote(targetAddr string, args *RequestVoteArgs) (*RequestVoteReply, error) {
	client, err := rpc.Dial("tcp", targetAddr)
	if err != nil {
		return nil, err
	}
	defer client.Close()

	var reply RequestVoteReply
	if err := client.Call("RpcBridge.RequestVote", args, &reply); err != nil {
		return nil, err
	}
	return &reply, nil
}

func (t *NetRpcTransport) SendAppendEntries(targetAddr string, args *AppendEntriesArgs) (*AppendEntriesReply, error) {
	client, err := rpc.Dial("tcp", targetAddr)
	if err != nil {
		return nil, err
	}
	defer client.Close()
	var reply AppendEntriesReply
	if err := client.Call("RpcBridge.AppendEntries", args, &reply); err != nil {
		return nil, err
	}
	return &reply, nil
}
