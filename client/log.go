package main

import (
	"fmt"
	"os"
	"time"
)

type TrafficLogger struct {
	file *os.File
}

func NewTrafficLogger(filename string) (*TrafficLogger, error) {
	// Open file in Append mode, Create if not exists, Write only
	f, err := os.OpenFile(filename, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return nil, err
	}
	return &TrafficLogger{file: f}, nil
}

func (l *TrafficLogger) Close() {
	if l.file != nil {
		l.file.Close()
	}
}

func (l *TrafficLogger) LogRequest(url, command string) {
	timestamp := time.Now().Format("15:04:05")
	// Format: [Time] REQUEST: Command -> URL
	msg := fmt.Sprintf("[%s] 📤 REQUEST:  %s  (to %s)\n", timestamp, command, url)
	l.write(msg)
}

func (l *TrafficLogger) LogResponse(status string, duration time.Duration, body string) {
	timestamp := time.Now().Format("15:04:05")
	// Format: [Time] RESPONSE: Status (Duration) | Body
	msg := fmt.Sprintf("[%s] 📥 RESPONSE: %s (%v)\n", timestamp, status, duration)
	msg += fmt.Sprintf("       📦 Body:     %s\n", body)
	msg += "----------------------------------------------------------------------\n"
	l.write(msg)
}

func (l *TrafficLogger) LogError(err error) {
	timestamp := time.Now().Format("15:04:05")
	msg := fmt.Sprintf("[%s] ❌ ERROR:    %v\n", timestamp, err)
	msg += "----------------------------------------------------------------------\n"
	l.write(msg)
}

func (l *TrafficLogger) write(msg string) {
	if _, err := l.file.WriteString(msg); err != nil {
		fmt.Printf("Error writing to log file: %v\n", err)
	}
}
