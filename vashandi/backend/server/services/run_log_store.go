package services

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

type RunLogHandle struct {
	Store  string `json:"store"`
	LogRef string `json:"logRef"`
}

type RunLogEvent struct {
	TS     string `json:"ts"`
	Stream string `json:"stream"`
	Chunk  string `json:"chunk"`
}

type RunLogStore struct {
	BasePath string
}

func NewRunLogStore(basePath string) *RunLogStore {
	if basePath == "" {
		// Default path matching Node.js structure
		basePath = "data/run-logs"
	}
	return &RunLogStore{BasePath: basePath}
}

// Begin initializes a new log file for a run.
func (s *RunLogStore) Begin(companyID, agentID, runID string) (*RunLogHandle, error) {
	relDir := filepath.Join(companyID, agentID)
	absDir := filepath.Join(s.BasePath, relDir)
	if err := os.MkdirAll(absDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create log directory: %w", err)
	}

	relPath := filepath.Join(relDir, fmt.Sprintf("%s.ndjson", runID))
	absPath := filepath.Join(s.BasePath, relPath)

	// Create/Truncate
	f, err := os.Create(absPath)
	if err != nil {
		return nil, fmt.Errorf("failed to create log file: %w", err)
	}
	f.Close()

	return &RunLogHandle{Store: "local_file", LogRef: relPath}, nil
}

// Append writes a new log entry to the NDJSON file.
func (s *RunLogStore) Append(handle *RunLogHandle, stream, chunk string) error {
	absPath := filepath.Join(s.BasePath, handle.LogRef)
	f, err := os.OpenFile(absPath, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()

	event := RunLogEvent{
		TS:     time.Now().Format(time.RFC3339),
		Stream: stream,
		Chunk:  chunk,
	}

	line, err := json.Marshal(event)
	if err != nil {
		return err
	}

	_, err = f.Write(append(line, '\n'))
	return err
}

// Read reads a portion of the log file.
func (s *RunLogStore) Read(handle *RunLogHandle) ([]RunLogEvent, error) {
	absPath := filepath.Join(s.BasePath, handle.LogRef)
	f, err := os.Open(absPath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var events []RunLogEvent
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		var event RunLogEvent
		if err := json.Unmarshal(scanner.Bytes(), &event); err == nil {
			events = append(events, event)
		}
	}

	return events, scanner.Err()
}
