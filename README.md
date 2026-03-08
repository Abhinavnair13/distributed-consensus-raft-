# Raft Consensus Engine in Go

[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)
[![Go Version](https://img.shields.io/badge/Go-1.21+-00ADD8?logo=go)](https://go.dev/)

A resilient, ground-up implementation of the Raft Consensus Algorithm in Go. Built to explore the complexities of distributed systems, this project creates a highly available, fault-tolerant replicated state machine based on the paper _"In Search of an Understandable Consensus Algorithm."_

**📺 [Watch the Full Project Breakdown on YouTube](INSERT_YOUR_YOUTUBE_LINK_HERE)**

---

## 🚀 Key Features

- **Leader Election & Heartbeats:** Automatic cluster self-healing and randomized timeouts to resolve split votes and prevent stalemates.
- **Log Replication:** Ensures absolute data safety across the distributed state machine. Data is only committed when a majority consensus is reached.
- **Fault Tolerance & Persistence:** Nodes save their state (`Term`, `VotedFor`, and `Log`) to a local `node_storage/` directory. If a node crashes, it can recover its state and seamlessly rejoin the cluster, catching up on missed logs.
- **gRPC Transport Layer:** High-performance, multiplexed inter-node communication using Protocol Buffers. Implemented using the **Adapter Design Pattern** to keep the core Raft domain logic decoupled from the network layer.
- **Smart Asynchronous Client:** A custom CLI that queues commands and intelligently follows HTTP 307 redirects to locate the active cluster Leader.

## 🏗️ Architecture Overview

The system runs as a cluster of independent nodes. Each node consists of:

1. **The Consensus Engine:** Handles state transitions (Follower -> Candidate -> Leader), timer management, and the `commitIndex`.
2. **The Transport Layer:** A gRPC-based module with connection pooling for fast, multiplexed node-to-node RPCs.
3. **The Persistence Layer:** A storage module that performs atomic writes to disk to survive system crashes.
4. **The HTTP Gateway:** Exposes endpoints for the external client to submit commands.

## 🛠️ Getting Started

### Prerequisites

- [Go](https://go.dev/doc/install) (1.21 or higher)
- [Protocol Buffers Compiler](https://grpc.io/docs/protoc-installation/) (`protoc`) - _Only required if you want to modify and regenerate the `.proto` files._

### 1. Start the Cluster

To simulate a distributed environment, open separate terminal windows to spin up multiple nodes. For a 5-node cluster, run the following commands in 5 different terminals:

# Terminal 1

go run . -id 1 -n 5

# Terminal 2

go run . -id 2 -n 5

# Terminal 3

go run . -id 3 -n 5

# Terminal 4

go run . -id 4 -n 5

# Terminal 5

go run . -id 5 -n 5

_Watch the logs as the nodes initialize as Followers, trigger an election timeout, and successfully elect a Leader._

### 2. Run the Smart Client

Open a 6th terminal window for the client. The client can connect to _any_ node; if it connects to a Follower, it will be automatically redirected to the Leader.

go run ./client -node 1

Once the client is running, you can start typing commands (e.g., `set user=abhinav`) to see them replicated across the cluster.

### 3. Test Fault Tolerance (Chaos Testing)

1. Identify the current Leader from the terminal logs.
2. Terminate the Leader process (`Ctrl+C`).
3. Watch the remaining nodes detect the missing heartbeats and automatically elect a new Leader.
4. Send new data from the client to the new Leader.
5. Restart the old Leader process. Watch it rejoin the cluster as a Follower and fetch the missing log entries it missed while down.

## 📁 Project Structure

.
├── client.go # Asynchronous smart client CLI
├── main.go # Node entry point and HTTP gateway
├── proto/ # Protobuf definitions and generated gRPC code
│ ├── raft.proto
│ └── ...
├── raft/ # Core Raft implementation
│ ├── raft.go # Consensus engine, state transitions, log replication
│ ├── storage.go # Disk persistence and crash recovery
│ ├── transport.go # Transport interfaces
│ └── grpc_transport.go # gRPC adapter and connection pooling
└── node_storage/ # Auto-generated directory for persistent node state

## 📜 License

This project is licensed under the MIT License - see the LICENSE file for details.

Copyright (c) 2026 Abhinav
