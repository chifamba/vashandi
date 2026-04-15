// Package telemetry provides a lightweight, privacy-respecting telemetry client
// for the Paperclip Go backend. It mirrors the behaviour of the TypeScript
// TelemetryClient found in packages/shared/src/telemetry/.
package telemetry

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const (
	defaultEndpoint     = "https://telemetry.paperclip.ing/ingest"
	defaultApp          = "paperclip"
	defaultSchemaVersion = "1"
	batchSize           = 50
	sendTimeout         = 5 * time.Second
)

// State holds the persistent telemetry identity for an installation.
type State struct {
	InstallID        string `json:"installId"`
	Salt             string `json:"salt"`
	CreatedAt        string `json:"createdAt"`
	FirstSeenVersion string `json:"firstSeenVersion"`
}

// Config holds effective telemetry settings.
type Config struct {
	Enabled       bool
	Endpoint      string
	App           string
	SchemaVersion string
}

// event is a single telemetry event sent to the ingest endpoint.
type event struct {
	Name       string                 `json:"name"`
	OccurredAt string                 `json:"occurredAt"`
	Dimensions map[string]interface{} `json:"dimensions"`
}

// Client queues telemetry events and flushes them periodically.
// It is safe for concurrent use.
type Client struct {
	config       Config
	stateFactory func() State
	version      string

	stateMu sync.Mutex
	state   *State

	queueMu sync.Mutex
	queue   []event

	stopOnce sync.Once
	stopCh   chan struct{}
}

// NewClient creates a new Client. stateFactory is called lazily on the first
// Track call to avoid I/O at startup.
func NewClient(config Config, stateFactory func() State, version string) *Client {
	return &Client{
		config:       config,
		stateFactory: stateFactory,
		version:      version,
		stopCh:       make(chan struct{}),
	}
}

// Track enqueues a telemetry event. It is a no-op when telemetry is disabled.
func (c *Client) Track(name string, dims map[string]interface{}) {
	if !c.config.Enabled {
		return
	}
	c.getState() // lazy init of state file

	if dims == nil {
		dims = map[string]interface{}{}
	}

	c.queueMu.Lock()
	c.queue = append(c.queue, event{
		Name:       name,
		OccurredAt: time.Now().UTC().Format(time.RFC3339),
		Dimensions: dims,
	})
	shouldFlush := len(c.queue) >= batchSize
	c.queueMu.Unlock()

	if shouldFlush {
		go c.Flush()
	}
}

// Flush sends all queued events to the telemetry endpoint.
// Failures are silently discarded (fire-and-forget).
func (c *Client) Flush() {
	if !c.config.Enabled {
		return
	}

	c.queueMu.Lock()
	if len(c.queue) == 0 {
		c.queueMu.Unlock()
		return
	}
	events := c.queue
	c.queue = nil
	c.queueMu.Unlock()

	state := c.getState()
	endpoint := c.config.Endpoint
	if endpoint == "" {
		endpoint = defaultEndpoint
	}
	app := c.config.App
	if app == "" {
		app = defaultApp
	}
	schemaVersion := c.config.SchemaVersion
	if schemaVersion == "" {
		schemaVersion = defaultSchemaVersion
	}

	body, err := json.Marshal(map[string]interface{}{
		"app":           app,
		"schemaVersion": schemaVersion,
		"installId":     state.InstallID,
		"version":       c.version,
		"events":        events,
	})
	if err != nil {
		return
	}

	httpClient := &http.Client{Timeout: sendTimeout}
	req, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return
	}
	resp.Body.Close()
}

// StartPeriodicFlush starts a background goroutine that calls Flush every
// interval. The goroutine exits when Stop is called.
func (c *Client) StartPeriodicFlush(interval time.Duration) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				c.Flush()
			case <-c.stopCh:
				return
			}
		}
	}()
}

// Stop terminates the periodic flush goroutine. It is safe to call more than once.
func (c *Client) Stop() {
	c.stopOnce.Do(func() { close(c.stopCh) })
}

// HashPrivateRef returns the first 16 hex characters of the salted SHA-256
// hash of value, mirroring TelemetryClient.hashPrivateRef in TypeScript.
func (c *Client) HashPrivateRef(value string) string {
	state := c.getState()
	h := sha256.New()
	h.Write([]byte(state.Salt + value))
	return hex.EncodeToString(h.Sum(nil))[:16]
}

func (c *Client) getState() State {
	c.stateMu.Lock()
	defer c.stateMu.Unlock()
	if c.state == nil {
		s := c.stateFactory()
		c.state = &s
	}
	return *c.state
}

// LoadOrCreateState loads the telemetry state from stateDir/state.json,
// creating it (with a fresh install ID and random salt) when it does not exist
// or is corrupted. It mirrors loadOrCreateState in packages/shared/src/telemetry/state.ts.
func LoadOrCreateState(stateDir, version string) State {
	filePath := filepath.Join(stateDir, "state.json")
	if data, err := os.ReadFile(filePath); err == nil {
		var s State
		if json.Unmarshal(data, &s) == nil && s.InstallID != "" && s.Salt != "" {
			return s
		}
	}

	s := State{
		InstallID:        newUUID(),
		Salt:             randomHex(32),
		CreatedAt:        time.Now().UTC().Format(time.RFC3339),
		FirstSeenVersion: version,
	}

	if err := os.MkdirAll(stateDir, 0o755); err == nil {
		data, _ := json.MarshalIndent(s, "", "  ")
		_ = os.WriteFile(filePath, append(data, '\n'), 0o644)
	}
	return s
}

var ciEnvVars = []string{
	"CI", "CONTINUOUS_INTEGRATION", "BUILD_NUMBER", "GITHUB_ACTIONS", "GITLAB_CI",
}

func isCI() bool {
	for _, key := range ciEnvVars {
		v := os.Getenv(key)
		if v == "true" || v == "1" {
			return true
		}
	}
	return false
}

// ResolveConfig determines the effective telemetry configuration from
// environment variables and the optional file-level enabled flag.
// It mirrors resolveTelemetryConfig in packages/shared/src/telemetry/config.ts.
func ResolveConfig(fileEnabled *bool) Config {
	if os.Getenv("PAPERCLIP_TELEMETRY_DISABLED") == "1" {
		return Config{Enabled: false}
	}
	if os.Getenv("DO_NOT_TRACK") == "1" {
		return Config{Enabled: false}
	}
	if isCI() {
		return Config{Enabled: false}
	}
	if fileEnabled != nil && !*fileEnabled {
		return Config{Enabled: false}
	}
	return Config{
		Enabled:  true,
		Endpoint: os.Getenv("PAPERCLIP_TELEMETRY_ENDPOINT"),
	}
}

// newUUID generates a random UUID v4 without an external dependency.
func newUUID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	b[6] = (b[6] & 0x0f) | 0x40 // version 4
	b[8] = (b[8] & 0x3f) | 0x80 // variant bits
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
}

// randomHex generates n random bytes encoded as a lower-case hex string.
func randomHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
