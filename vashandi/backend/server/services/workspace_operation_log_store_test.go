package services

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWorkspaceOperationLogStore_LocalFile(t *testing.T) {
	// Create a temp directory for testing
	tmpDir, err := os.MkdirTemp("", "workspace-op-log-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	store := NewLocalFileWorkspaceOperationLogStore(tmpDir)

	// Test Begin
	t.Run("Begin creates log file", func(t *testing.T) {
		handle, err := store.Begin(WorkspaceOperationLogBeginInput{
			CompanyID:   "company-123",
			OperationID: "op-456",
		})
		if err != nil {
			t.Fatalf("Begin() error = %v", err)
		}
		if handle.Store != LogStoreTypeLocalFile {
			t.Errorf("Handle.Store = %v, want %v", handle.Store, LogStoreTypeLocalFile)
		}
		if handle.LogRef == "" {
			t.Error("Handle.LogRef is empty")
		}

		// Verify file was created
		absPath := filepath.Join(tmpDir, handle.LogRef)
		if _, err := os.Stat(absPath); os.IsNotExist(err) {
			t.Errorf("Log file not created at %s", absPath)
		}
	})

	// Test Append
	t.Run("Append writes to log file", func(t *testing.T) {
		handle, _ := store.Begin(WorkspaceOperationLogBeginInput{
			CompanyID:   "company-123",
			OperationID: "op-append-test",
		})

		err := store.Append(handle, WorkspaceOperationLogEvent{
			Stream: "stdout",
			Chunk:  "Hello, world!",
			TS:     "2024-01-01T00:00:00Z",
		})
		if err != nil {
			t.Fatalf("Append() error = %v", err)
		}

		err = store.Append(handle, WorkspaceOperationLogEvent{
			Stream: "stderr",
			Chunk:  "Error message",
			TS:     "2024-01-01T00:00:01Z",
		})
		if err != nil {
			t.Fatalf("Append() error = %v", err)
		}

		// Read the file and verify content
		absPath := filepath.Join(tmpDir, handle.LogRef)
		content, err := os.ReadFile(absPath)
		if err != nil {
			t.Fatalf("Failed to read log file: %v", err)
		}
		if len(content) == 0 {
			t.Error("Log file is empty after appending")
		}
	})

	// Test Finalize
	t.Run("Finalize computes checksum", func(t *testing.T) {
		handle, _ := store.Begin(WorkspaceOperationLogBeginInput{
			CompanyID:   "company-123",
			OperationID: "op-finalize-test",
		})

		store.Append(handle, WorkspaceOperationLogEvent{
			Stream: "stdout",
			Chunk:  "Test content for checksum",
			TS:     "2024-01-01T00:00:00Z",
		})

		summary, err := store.Finalize(handle)
		if err != nil {
			t.Fatalf("Finalize() error = %v", err)
		}
		if summary.Bytes == 0 {
			t.Error("Summary.Bytes = 0, expected > 0")
		}
		if summary.SHA256 == nil || *summary.SHA256 == "" {
			t.Error("Summary.SHA256 is nil or empty")
		}
		if summary.Compressed {
			t.Error("Summary.Compressed = true, expected false")
		}
	})

	// Test Read
	t.Run("Read returns log content", func(t *testing.T) {
		handle, _ := store.Begin(WorkspaceOperationLogBeginInput{
			CompanyID:   "company-123",
			OperationID: "op-read-test",
		})

		store.Append(handle, WorkspaceOperationLogEvent{
			Stream: "stdout",
			Chunk:  "First line",
			TS:     "2024-01-01T00:00:00Z",
		})
		store.Append(handle, WorkspaceOperationLogEvent{
			Stream: "stdout",
			Chunk:  "Second line",
			TS:     "2024-01-01T00:00:01Z",
		})

		result, err := store.Read(handle, nil)
		if err != nil {
			t.Fatalf("Read() error = %v", err)
		}
		if result.Content == "" {
			t.Error("Result.Content is empty")
		}
		// File should be small enough that there's no next offset
		if result.NextOffset != nil && *result.NextOffset > 0 {
			t.Logf("NextOffset = %d (file is large enough for pagination)", *result.NextOffset)
		}
	})

	// Test Read with pagination
	t.Run("Read with offset and limit", func(t *testing.T) {
		handle, _ := store.Begin(WorkspaceOperationLogBeginInput{
			CompanyID:   "company-123",
			OperationID: "op-pagination-test",
		})

		// Write enough content to test pagination
		for i := 0; i < 10; i++ {
			store.Append(handle, WorkspaceOperationLogEvent{
				Stream: "stdout",
				Chunk:  "This is a line of content that repeats. ",
				TS:     "2024-01-01T00:00:00Z",
			})
		}

		// Read with small limit
		result, err := store.Read(handle, &WorkspaceOperationLogReadOptions{
			Offset:     0,
			LimitBytes: 50,
		})
		if err != nil {
			t.Fatalf("Read() error = %v", err)
		}
		if len(result.Content) > 50 {
			t.Errorf("Result.Content length = %d, expected <= 50", len(result.Content))
		}
		if result.NextOffset == nil {
			t.Error("NextOffset should not be nil for partial read")
		}
	})
}

func TestSafeSegments(t *testing.T) {
	tests := []struct {
		input []string
		want  []string
	}{
		{
			input: []string{"company-123"},
			want:  []string{"company-123"},
		},
		{
			input: []string{"company/123"},
			want:  []string{"company_123"},
		},
		{
			input: []string{"company@#$123"},
			want:  []string{"company___123"},
		},
		{
			input: []string{"valid.name", "another_valid-name"},
			want:  []string{"valid.name", "another_valid-name"},
		},
	}

	for _, tt := range tests {
		got := safeSegments(tt.input...)
		if len(got) != len(tt.want) {
			t.Errorf("safeSegments(%v) length = %d, want %d", tt.input, len(got), len(tt.want))
			continue
		}
		for i := range got {
			if got[i] != tt.want[i] {
				t.Errorf("safeSegments(%v)[%d] = %s, want %s", tt.input, i, got[i], tt.want[i])
			}
		}
	}
}

func TestResolveWithin(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "resolve-within-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	tests := []struct {
		name        string
		basePath    string
		relativePath string
		wantErr     bool
	}{
		{
			name:        "valid path",
			basePath:    tmpDir,
			relativePath: "company/file.log",
			wantErr:     false,
		},
		{
			name:        "path traversal attempt",
			basePath:    tmpDir,
			relativePath: "../../../etc/passwd",
			wantErr:     true,
		},
		{
			name:        "path with ..",
			basePath:    tmpDir,
			relativePath: "company/../../../etc/passwd",
			wantErr:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := resolveWithin(tt.basePath, tt.relativePath)
			if tt.wantErr && err == nil {
				t.Error("resolveWithin() error = nil, wantErr = true")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("resolveWithin() error = %v, wantErr = false", err)
			}
		})
	}
}
