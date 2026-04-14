package services

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRunLogStore_BeginCreatesFile(t *testing.T) {
	tmpDir := t.TempDir()
	store := NewRunLogStore(tmpDir)

	handle, err := store.Begin("comp-1", "agent-1", "run-abc")
	if err != nil {
		t.Fatalf("Begin failed: %v", err)
	}

	if handle.Store != "local_file" {
		t.Errorf("expected store 'local_file', got %q", handle.Store)
	}

	expectedRef := filepath.Join("comp-1", "agent-1", "run-abc.ndjson")
	if handle.LogRef != expectedRef {
		t.Errorf("expected LogRef %q, got %q", expectedRef, handle.LogRef)
	}

	// Verify the file exists
	absPath := filepath.Join(tmpDir, handle.LogRef)
	if _, err := os.Stat(absPath); os.IsNotExist(err) {
		t.Errorf("expected log file to exist at %q", absPath)
	}
}

func TestRunLogStore_AppendAndRead(t *testing.T) {
	tmpDir := t.TempDir()
	store := NewRunLogStore(tmpDir)

	handle, err := store.Begin("comp-1", "agent-1", "run-123")
	if err != nil {
		t.Fatalf("Begin failed: %v", err)
	}

	// Append some events
	if err := store.Append(handle, "stdout", "Hello world"); err != nil {
		t.Fatalf("Append stdout failed: %v", err)
	}
	if err := store.Append(handle, "stderr", "Warning: something happened"); err != nil {
		t.Fatalf("Append stderr failed: %v", err)
	}
	if err := store.Append(handle, "stdout", "Done"); err != nil {
		t.Fatalf("Append stdout #2 failed: %v", err)
	}

	// Read back
	events, err := store.Read(handle)
	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}

	if len(events) != 3 {
		t.Fatalf("expected 3 events, got %d", len(events))
	}

	if events[0].Stream != "stdout" || events[0].Chunk != "Hello world" {
		t.Errorf("event[0]: expected stdout 'Hello world', got %q %q", events[0].Stream, events[0].Chunk)
	}
	if events[1].Stream != "stderr" || events[1].Chunk != "Warning: something happened" {
		t.Errorf("event[1]: expected stderr warning, got %q %q", events[1].Stream, events[1].Chunk)
	}
	if events[2].Stream != "stdout" || events[2].Chunk != "Done" {
		t.Errorf("event[2]: expected stdout 'Done', got %q %q", events[2].Stream, events[2].Chunk)
	}

	// Verify timestamps are RFC3339
	for i, e := range events {
		if e.TS == "" {
			t.Errorf("event[%d]: expected non-empty timestamp", i)
		}
	}
}

func TestRunLogStore_ReadEmptyFile(t *testing.T) {
	tmpDir := t.TempDir()
	store := NewRunLogStore(tmpDir)

	handle, err := store.Begin("comp-1", "agent-1", "run-empty")
	if err != nil {
		t.Fatalf("Begin failed: %v", err)
	}

	events, err := store.Read(handle)
	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}

	if len(events) != 0 {
		t.Errorf("expected 0 events for empty log, got %d", len(events))
	}
}

func TestRunLogStore_ReadNonExistentFile(t *testing.T) {
	tmpDir := t.TempDir()
	store := NewRunLogStore(tmpDir)

	handle := &RunLogHandle{Store: "local_file", LogRef: "nonexistent/path.ndjson"}
	_, err := store.Read(handle)
	if err == nil {
		t.Error("expected error when reading non-existent file")
	}
}

func TestRunLogStore_DefaultBasePath(t *testing.T) {
	store := NewRunLogStore("")
	if store.BasePath != "data/run-logs" {
		t.Errorf("expected default base path 'data/run-logs', got %q", store.BasePath)
	}
}

func TestRunLogStore_MultipleRuns(t *testing.T) {
	tmpDir := t.TempDir()
	store := NewRunLogStore(tmpDir)

	h1, err := store.Begin("comp-1", "agent-1", "run-1")
	if err != nil {
		t.Fatalf("Begin run-1 failed: %v", err)
	}
	h2, err := store.Begin("comp-1", "agent-1", "run-2")
	if err != nil {
		t.Fatalf("Begin run-2 failed: %v", err)
	}

	store.Append(h1, "stdout", "Run 1 output")
	store.Append(h2, "stdout", "Run 2 output")
	store.Append(h1, "stdout", "Run 1 more output")

	events1, _ := store.Read(h1)
	events2, _ := store.Read(h2)

	if len(events1) != 2 {
		t.Errorf("expected 2 events for run-1, got %d", len(events1))
	}
	if len(events2) != 1 {
		t.Errorf("expected 1 event for run-2, got %d", len(events2))
	}
}
