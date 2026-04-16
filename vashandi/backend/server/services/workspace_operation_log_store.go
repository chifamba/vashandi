package services

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

// WorkspaceOperationLogStoreType represents the type of log storage
type WorkspaceOperationLogStoreType string

const (
	LogStoreTypeLocalFile WorkspaceOperationLogStoreType = "local_file"
)

// WorkspaceOperationLogHandle represents a handle to an operation log
type WorkspaceOperationLogHandle struct {
	Store  WorkspaceOperationLogStoreType `json:"store"`
	LogRef string                         `json:"logRef"`
}

// WorkspaceOperationLogReadOptions specifies options for reading logs
type WorkspaceOperationLogReadOptions struct {
	Offset     int64 `json:"offset"`
	LimitBytes int64 `json:"limitBytes"`
}

// WorkspaceOperationLogReadResult is the result of reading log content
type WorkspaceOperationLogReadResult struct {
	Content    string `json:"content"`
	NextOffset *int64 `json:"nextOffset,omitempty"`
}

// WorkspaceOperationLogFinalizeSummary is the summary after finalizing a log
type WorkspaceOperationLogFinalizeSummary struct {
	Bytes      int64   `json:"bytes"`
	SHA256     *string `json:"sha256,omitempty"`
	Compressed bool    `json:"compressed"`
}

// WorkspaceOperationLogEvent represents a single log event
type WorkspaceOperationLogEvent struct {
	TS     string `json:"ts"`
	Stream string `json:"stream"` // "stdout", "stderr", or "system"
	Chunk  string `json:"chunk"`
}

// WorkspaceOperationLogStore is the interface for log storage
type WorkspaceOperationLogStore interface {
	// Begin starts a new log stream for an operation
	Begin(input WorkspaceOperationLogBeginInput) (*WorkspaceOperationLogHandle, error)
	// Append adds content to the log stream
	Append(handle *WorkspaceOperationLogHandle, event WorkspaceOperationLogEvent) error
	// Finalize completes the log stream and computes checksum
	Finalize(handle *WorkspaceOperationLogHandle) (*WorkspaceOperationLogFinalizeSummary, error)
	// Read reads content from the log with pagination support
	Read(handle *WorkspaceOperationLogHandle, opts *WorkspaceOperationLogReadOptions) (*WorkspaceOperationLogReadResult, error)
}

// WorkspaceOperationLogBeginInput is the input for beginning a log stream
type WorkspaceOperationLogBeginInput struct {
	CompanyID   string `json:"companyId"`
	OperationID string `json:"operationId"`
}

var (
	ErrLogNotFound   = errors.New("workspace operation log not found")
	ErrInvalidLogRef = errors.New("invalid log path")
)

// safeSegmentPattern matches valid path segment characters
var safeSegmentPattern = regexp.MustCompile(`[^a-zA-Z0-9._-]`)

// safeSegments sanitizes path segments for safe file names
func safeSegments(segments ...string) []string {
	result := make([]string, len(segments))
	for i, s := range segments {
		result[i] = safeSegmentPattern.ReplaceAllString(s, "_")
	}
	return result
}

// resolveWithin resolves a path within a base directory and ensures it doesn't escape
func resolveWithin(basePath, relativePath string) (string, error) {
	base, err := filepath.Abs(basePath)
	if err != nil {
		return "", err
	}
	resolved, err := filepath.Abs(filepath.Join(base, relativePath))
	if err != nil {
		return "", err
	}
	// Ensure resolved path is within base
	rel, err := filepath.Rel(base, resolved)
	if err != nil {
		return "", ErrInvalidLogRef
	}
	// Check for path traversal: block paths that traverse up (start with "..")
	// Also block paths that resolve to the base itself (rel == ".")
	if strings.HasPrefix(rel, "..") || rel == "." {
		return "", ErrInvalidLogRef
	}
	return resolved, nil
}

// localFileWorkspaceOperationLogStore implements WorkspaceOperationLogStore using local files
type localFileWorkspaceOperationLogStore struct {
	basePath string
	mu       sync.Mutex
}

// NewLocalFileWorkspaceOperationLogStore creates a new local file log store
func NewLocalFileWorkspaceOperationLogStore(basePath string) WorkspaceOperationLogStore {
	return &localFileWorkspaceOperationLogStore{basePath: basePath}
}

func (s *localFileWorkspaceOperationLogStore) ensureDir(relativeDir string) error {
	dir, err := resolveWithin(s.basePath, relativeDir)
	if err != nil {
		return err
	}
	return os.MkdirAll(dir, 0755)
}

func (s *localFileWorkspaceOperationLogStore) Begin(input WorkspaceOperationLogBeginInput) (*WorkspaceOperationLogHandle, error) {
	segments := safeSegments(input.CompanyID)
	companyID := segments[0]
	operationID := safeSegments(input.OperationID)[0]
	
	relDir := companyID
	relPath := filepath.Join(relDir, operationID+".ndjson")

	if err := s.ensureDir(relDir); err != nil {
		return nil, fmt.Errorf("failed to create log directory: %w", err)
	}

	absPath, err := resolveWithin(s.basePath, relPath)
	if err != nil {
		return nil, err
	}

	// Create empty file
	if err := os.WriteFile(absPath, []byte{}, 0644); err != nil {
		return nil, fmt.Errorf("failed to create log file: %w", err)
	}

	return &WorkspaceOperationLogHandle{
		Store:  LogStoreTypeLocalFile,
		LogRef: relPath,
	}, nil
}

func (s *localFileWorkspaceOperationLogStore) Append(handle *WorkspaceOperationLogHandle, event WorkspaceOperationLogEvent) error {
	if handle.Store != LogStoreTypeLocalFile {
		return nil
	}

	absPath, err := resolveWithin(s.basePath, handle.LogRef)
	if err != nil {
		return err
	}

	// Ensure timestamp is set
	if event.TS == "" {
		event.TS = time.Now().UTC().Format(time.RFC3339Nano)
	}

	line, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("failed to marshal log event: %w", err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	f, err := os.OpenFile(absPath, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0644)
	if err != nil {
		return fmt.Errorf("failed to open log file: %w", err)
	}
	defer f.Close()

	if _, err := f.Write(append(line, '\n')); err != nil {
		return fmt.Errorf("failed to write log line: %w", err)
	}

	return nil
}

func (s *localFileWorkspaceOperationLogStore) Finalize(handle *WorkspaceOperationLogHandle) (*WorkspaceOperationLogFinalizeSummary, error) {
	if handle.Store != LogStoreTypeLocalFile {
		return &WorkspaceOperationLogFinalizeSummary{
			Bytes:      0,
			Compressed: false,
		}, nil
	}

	absPath, err := resolveWithin(s.basePath, handle.LogRef)
	if err != nil {
		return nil, err
	}

	stat, err := os.Stat(absPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrLogNotFound
		}
		return nil, fmt.Errorf("failed to stat log file: %w", err)
	}

	// Compute SHA256
	hash, err := sha256File(absPath)
	if err != nil {
		return nil, fmt.Errorf("failed to compute log hash: %w", err)
	}

	return &WorkspaceOperationLogFinalizeSummary{
		Bytes:      stat.Size(),
		SHA256:     &hash,
		Compressed: false,
	}, nil
}

func (s *localFileWorkspaceOperationLogStore) Read(handle *WorkspaceOperationLogHandle, opts *WorkspaceOperationLogReadOptions) (*WorkspaceOperationLogReadResult, error) {
	if handle.Store != LogStoreTypeLocalFile {
		return nil, ErrLogNotFound
	}

	absPath, err := resolveWithin(s.basePath, handle.LogRef)
	if err != nil {
		return nil, err
	}

	// Default options
	offset := int64(0)
	limitBytes := int64(256000)
	if opts != nil {
		if opts.Offset > 0 {
			offset = opts.Offset
		}
		if opts.LimitBytes > 0 {
			limitBytes = opts.LimitBytes
		}
	}

	return readFileRange(absPath, offset, limitBytes)
}

// readFileRange reads a range of bytes from a file
func readFileRange(filePath string, offset, limitBytes int64) (*WorkspaceOperationLogReadResult, error) {
	stat, err := os.Stat(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrLogNotFound
		}
		return nil, fmt.Errorf("failed to stat log file: %w", err)
	}

	fileSize := stat.Size()

	// Clamp offset
	start := offset
	if start < 0 {
		start = 0
	}
	if start > fileSize {
		start = fileSize
	}

	// Calculate end position
	end := start + limitBytes
	if end > fileSize {
		end = fileSize
	}

	if start >= end {
		return &WorkspaceOperationLogReadResult{
			Content:    "",
			NextOffset: nil,
		}, nil
	}

	f, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open log file: %w", err)
	}
	defer f.Close()

	// Seek to start position
	if _, err := f.Seek(start, io.SeekStart); err != nil {
		return nil, fmt.Errorf("failed to seek in log file: %w", err)
	}

	// Read the data
	buf := make([]byte, end-start)
	n, err := io.ReadFull(f, buf)
	if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
		return nil, fmt.Errorf("failed to read log file: %w", err)
	}

	content := string(buf[:n])
	var nextOffset *int64
	nextPos := start + int64(n)
	if nextPos < fileSize {
		nextOffset = &nextPos
	}

	return &WorkspaceOperationLogReadResult{
		Content:    content,
		NextOffset: nextOffset,
	}, nil
}

// sha256File computes the SHA256 hash of a file
func sha256File(filePath string) (string, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := sha256.New()
	reader := bufio.NewReader(f)
	if _, err := io.Copy(h, reader); err != nil {
		return "", err
	}

	return hex.EncodeToString(h.Sum(nil)), nil
}

// Global log store instance
var (
	globalLogStore     WorkspaceOperationLogStore
	globalLogStoreOnce sync.Once
)

// GetWorkspaceOperationLogStore returns the global workspace operation log store
func GetWorkspaceOperationLogStore() WorkspaceOperationLogStore {
	globalLogStoreOnce.Do(func() {
		basePath := os.Getenv("WORKSPACE_OPERATION_LOG_BASE_PATH")
		if basePath == "" {
			// Default to data/workspace-operation-logs relative to instance root
			homeDir, err := os.UserHomeDir()
			if err != nil {
				// Fall back to current working directory if home dir is unavailable
				cwd, cwdErr := os.Getwd()
				if cwdErr != nil {
					cwd = "."
				}
				basePath = filepath.Join(cwd, "data", "workspace-operation-logs")
			} else {
				basePath = filepath.Join(homeDir, ".paperclipai", "data", "workspace-operation-logs")
			}
		}
		globalLogStore = NewLocalFileWorkspaceOperationLogStore(basePath)
	})
	return globalLogStore
}

// SetWorkspaceOperationLogStore allows overriding the global log store (for testing)
func SetWorkspaceOperationLogStore(store WorkspaceOperationLogStore) {
	globalLogStore = store
}
