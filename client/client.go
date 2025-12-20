package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

func main() {
	nodeID := flag.Int("node", 1, "Initial Node ID to send request to")
	flag.Parse()

	httpPorts := map[int]string{
		1: "9001",
		2: "9002",
		3: "9003",
	}

	// 1. Initialize our custom file logger
	trafficLog, err := NewTrafficLogger("client_traffic.log")
	if err != nil {
		panic(fmt.Sprintf("Failed to open log file: %v", err))
	}
	defer trafficLog.Close()

	// 2. UI Setup
	scanner := bufio.NewScanner(os.Stdin)
	client := &http.Client{
		Timeout: 5 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			fmt.Printf("🔀 Redirecting to: %s\n", req.URL)
			// returning nil means "allow the redirect"
			return nil
		},
	}
	fmt.Print("\033[H\033[2J") // Clear screen
	fmt.Printf("✅ Interactive Client Started (Node %d)\n", *nodeID)
	fmt.Println("   - Input:  Type commands here.")
	fmt.Println("   - Exit: Type exit to quit.")
	fmt.Println("------------------------------------------------")

	// 3. Main Loop
	for {
		fmt.Print("> ") // Prompt
		if !scanner.Scan() {
			break
		}

		text := strings.TrimSpace(scanner.Text())
		if text == "exit" || text == "quit" {
			break
		}
		if text == "" {
			continue
		}

		port, ok := httpPorts[*nodeID]
		if !ok {
			fmt.Println("Error: Invalid node config")
			continue
		}
		targetURL := fmt.Sprintf("http://localhost:%s/submit", port)

		// Log the attempt
		trafficLog.LogRequest(targetURL, text)

		// Execute
		sendRequest(client, targetURL, text, trafficLog)
	}
}

func sendRequest(client *http.Client, url string, command string, logger *TrafficLogger) {
	jsonData, _ := json.Marshal(map[string]string{"command": command})

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		logger.LogError(fmt.Errorf("creating request: %v", err))
		return
	}
	req.Header.Set("Content-Type", "application/json")

	start := time.Now()
	resp, err := client.Do(req)
	if err != nil {
		logger.LogError(fmt.Errorf("connection failed: %v", err))
		return
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	duration := time.Since(start)

	// Log the success
	logger.LogResponse(resp.Status, duration, string(body))
}
