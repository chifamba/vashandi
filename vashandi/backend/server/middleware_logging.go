package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5/middleware"

	"github.com/chifamba/vashandi/vashandi/backend/shared"
)

// LoggingConfig configures the structured logging middleware.
type LoggingConfig struct {
	// LogDir is the directory where log files are written.
	// Defaults to the standard logs directory under the instance root.
	LogDir string

	// LogLevel is the minimum log level for console output. Defaults to "info".
	LogLevel string

	// FileLogLevel is the minimum log level for file output. Defaults to "debug".
	FileLogLevel string

	// ColorOutput enables colored output to console. Defaults to true when TTY is detected.
	ColorOutput *bool

	// LogFile is the name of the log file. Defaults to "server.log".
	LogFile string
}

// LoggingMiddleware provides structured request logging with:
// - File logging to a configurable directory
// - Request body/params/query logging on 4xx/5xx responses
// - Authorization header redaction
// - Custom log levels based on status codes
// - Color output to console
type LoggingMiddleware struct {
	config     LoggingConfig
	fileLogger *slog.Logger
	logFile    *os.File
	mu         sync.Mutex
}

// responseRecorder wraps http.ResponseWriter to capture status code and response.
type responseRecorder struct {
	http.ResponseWriter
	statusCode int
	written    bool
}

func (rr *responseRecorder) WriteHeader(code int) {
	if !rr.written {
		rr.statusCode = code
		rr.written = true
		rr.ResponseWriter.WriteHeader(code)
	}
}

func (rr *responseRecorder) Write(b []byte) (int, error) {
	if !rr.written {
		rr.statusCode = http.StatusOK
		rr.written = true
	}
	return rr.ResponseWriter.Write(b)
}

// NewLoggingMiddleware creates a new LoggingMiddleware with the given configuration.
func NewLoggingMiddleware(config LoggingConfig) (*LoggingMiddleware, error) {
	// Resolve log directory
	logDir := config.LogDir
	if logDir == "" {
		logDir = resolveDefaultLogsDir()
	}

	// Ensure log directory exists
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		return nil, fmt.Errorf("failed to create log directory %s: %w", logDir, err)
	}

	// Resolve log file name
	logFileName := config.LogFile
	if logFileName == "" {
		logFileName = "server.log"
	}

	// Open log file
	logFilePath := filepath.Join(logDir, logFileName)
	logFile, err := os.OpenFile(logFilePath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, fmt.Errorf("failed to open log file %s: %w", logFilePath, err)
	}

	// Create file logger
	fileLogLevel := parseSlogLevel(config.FileLogLevel, slog.LevelDebug)
	fileLogger := slog.New(slog.NewJSONHandler(logFile, &slog.HandlerOptions{
		Level: fileLogLevel,
	}))

	return &LoggingMiddleware{
		config:     config,
		fileLogger: fileLogger,
		logFile:    logFile,
	}, nil
}

// resolveDefaultLogsDir returns the default logs directory.
func resolveDefaultLogsDir() string {
	envOverride := strings.TrimSpace(os.Getenv("PAPERCLIP_LOG_DIR"))
	if envOverride != "" {
		return resolveHomeAwarePath(envOverride)
	}
	return filepath.Join(shared.ResolvePaperclipInstanceRoot(), "logs")
}

// resolveHomeAwarePath expands ~ prefixes in paths.
func resolveHomeAwarePath(path string) string {
	if path == "~" {
		home, _ := os.UserHomeDir()
		return home
	}
	if strings.HasPrefix(path, "~/") {
		home, _ := os.UserHomeDir()
		return filepath.Join(home, path[2:])
	}
	return path
}

// parseSlogLevel parses a log level string to slog.Level.
func parseSlogLevel(level string, defaultLevel slog.Level) slog.Level {
	switch strings.ToLower(level) {
	case "debug":
		return slog.LevelDebug
	case "info":
		return slog.LevelInfo
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return defaultLevel
	}
}

// Close closes the log file.
func (m *LoggingMiddleware) Close() error {
	if m.logFile != nil {
		return m.logFile.Close()
	}
	return nil
}

// Handler returns the chi middleware handler.
func (m *LoggingMiddleware) Handler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		// Get request ID from chi middleware
		reqID := middleware.GetReqID(r.Context())

		// Capture request body for potential logging on error
		var bodyBytes []byte
		if r.Body != nil && r.ContentLength > 0 && r.ContentLength <= 1024*1024 { // Limit to 1MB
			bodyBytes, _ = io.ReadAll(r.Body)
			r.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
		}

		// Wrap response writer to capture status code
		rec := &responseRecorder{ResponseWriter: w, statusCode: http.StatusOK}

		// Call next handler
		next.ServeHTTP(rec, r)

		// Calculate duration
		duration := time.Since(start)

		// Determine log level based on status code
		level := m.logLevelForStatus(rec.statusCode)

		// Build log entry
		entry := m.buildLogEntry(r, rec.statusCode, duration, reqID, bodyBytes)

		// Log to file
		m.logToFile(level, entry)

		// Log to console
		m.logToConsole(level, r.Method, r.URL.Path, rec.statusCode, duration)
	})
}

// logLevelForStatus returns the appropriate log level for a status code.
func (m *LoggingMiddleware) logLevelForStatus(status int) slog.Level {
	switch {
	case status >= 500:
		return slog.LevelError
	case status >= 400:
		return slog.LevelWarn
	default:
		return slog.LevelInfo
	}
}

// logEntry represents a structured log entry.
type logEntry struct {
	Method       string                 `json:"method"`
	URL          string                 `json:"url"`
	Status       int                    `json:"status"`
	Duration     string                 `json:"duration"`
	DurationMs   float64                `json:"duration_ms"`
	RequestID    string                 `json:"request_id,omitempty"`
	RemoteAddr   string                 `json:"remote_addr"`
	UserAgent    string                 `json:"user_agent,omitempty"`
	ReqBody      interface{}            `json:"req_body,omitempty"`
	ReqParams    map[string]string      `json:"req_params,omitempty"`
	ReqQuery     map[string][]string    `json:"req_query,omitempty"`
	Headers      map[string]string      `json:"headers,omitempty"`
	ErrorContext map[string]interface{} `json:"error_context,omitempty"`
}

// buildLogEntry creates a log entry from the request and response.
func (m *LoggingMiddleware) buildLogEntry(r *http.Request, status int, duration time.Duration, reqID string, bodyBytes []byte) logEntry {
	entry := logEntry{
		Method:     r.Method,
		URL:        r.URL.String(),
		Status:     status,
		Duration:   duration.String(),
		DurationMs: float64(duration.Milliseconds()),
		RequestID:  reqID,
		RemoteAddr: r.RemoteAddr,
		UserAgent:  r.UserAgent(),
	}

	// Include request details on 4xx/5xx responses
	if status >= 400 {
		// Include body if present and not too large
		if len(bodyBytes) > 0 && len(bodyBytes) <= 10000 {
			var bodyJSON interface{}
			if err := json.Unmarshal(bodyBytes, &bodyJSON); err == nil {
				entry.ReqBody = bodyJSON
			} else {
				entry.ReqBody = string(bodyBytes)
			}
		}

		// Include query parameters
		if len(r.URL.Query()) > 0 {
			entry.ReqQuery = r.URL.Query()
		}

		// Include redacted headers (for debugging)
		entry.Headers = m.redactHeaders(r.Header)
	}

	return entry
}

// redactHeaders returns a map of headers with sensitive values redacted.
func (m *LoggingMiddleware) redactHeaders(headers http.Header) map[string]string {
	redacted := make(map[string]string)
	sensitiveHeaders := map[string]bool{
		"authorization":       true,
		"cookie":              true,
		"set-cookie":          true,
		"x-api-key":           true,
		"x-auth-token":        true,
		"x-paperclip-api-key": true,
	}

	for key, values := range headers {
		lowerKey := strings.ToLower(key)
		if sensitiveHeaders[lowerKey] {
			redacted[key] = "[REDACTED]"
		} else if len(values) > 0 {
			redacted[key] = values[0]
		}
	}
	return redacted
}

// logToFile writes the log entry to the file logger.
func (m *LoggingMiddleware) logToFile(level slog.Level, entry logEntry) {
	attrs := []slog.Attr{
		slog.String("method", entry.Method),
		slog.String("url", entry.URL),
		slog.Int("status", entry.Status),
		slog.String("duration", entry.Duration),
		slog.Float64("duration_ms", entry.DurationMs),
		slog.String("remote_addr", entry.RemoteAddr),
	}

	if entry.RequestID != "" {
		attrs = append(attrs, slog.String("request_id", entry.RequestID))
	}
	if entry.UserAgent != "" {
		attrs = append(attrs, slog.String("user_agent", entry.UserAgent))
	}
	if entry.ReqBody != nil {
		if bodyJSON, err := json.Marshal(entry.ReqBody); err == nil {
			attrs = append(attrs, slog.String("req_body", string(bodyJSON)))
		}
	}
	if len(entry.ReqQuery) > 0 {
		if queryJSON, err := json.Marshal(entry.ReqQuery); err == nil {
			attrs = append(attrs, slog.String("req_query", string(queryJSON)))
		}
	}
	if len(entry.Headers) > 0 {
		if headersJSON, err := json.Marshal(entry.Headers); err == nil {
			attrs = append(attrs, slog.String("headers", string(headersJSON)))
		}
	}

	msg := fmt.Sprintf("%s %s %d", entry.Method, entry.URL, entry.Status)

	m.mu.Lock()
	defer m.mu.Unlock()
	m.fileLogger.LogAttrs(nil, level, msg, attrs...)
}

// Color codes for terminal output
const (
	colorReset  = "\033[0m"
	colorRed    = "\033[31m"
	colorGreen  = "\033[32m"
	colorYellow = "\033[33m"
	colorBlue   = "\033[34m"
	colorCyan   = "\033[36m"
	colorGray   = "\033[90m"
)

// logToConsole writes a formatted log line to the console.
func (m *LoggingMiddleware) logToConsole(level slog.Level, method, path string, status int, duration time.Duration) {
	colorEnabled := m.shouldUseColor()

	timestamp := time.Now().Format("15:04:05")
	statusColor := m.statusColor(status, colorEnabled)
	levelStr := m.levelString(level, colorEnabled)
	durationStr := m.formatDuration(duration, colorEnabled)

	if colorEnabled {
		fmt.Printf("%s%s%s %s %s%s%s %s %s%d%s %s\n",
			colorGray, timestamp, colorReset,
			levelStr,
			colorCyan, method, colorReset,
			path,
			statusColor, status, colorReset,
			durationStr,
		)
	} else {
		fmt.Printf("%s %s %s %s %d %s\n",
			timestamp,
			levelStr,
			method,
			path,
			status,
			durationStr,
		)
	}
}

// shouldUseColor determines if color output should be used.
func (m *LoggingMiddleware) shouldUseColor() bool {
	if m.config.ColorOutput != nil {
		return *m.config.ColorOutput
	}
	// Default: use color if stdout is a TTY
	return isTerminal()
}

// isTerminal checks if stdout is a terminal.
func isTerminal() bool {
	// Check TERM environment variable
	if os.Getenv("TERM") == "dumb" {
		return false
	}
	// Check NO_COLOR environment variable (https://no-color.org/)
	if os.Getenv("NO_COLOR") != "" {
		return false
	}
	// Check if running in CI
	if os.Getenv("CI") != "" {
		return false
	}
	// On Windows, check for ConEmu or similar
	if runtime.GOOS == "windows" {
		return os.Getenv("ConEmuANSI") == "ON" || os.Getenv("ANSICON") != ""
	}
	// On Unix-like systems, check if stdout is a TTY
	stat, err := os.Stdout.Stat()
	if err != nil {
		return false
	}
	return (stat.Mode() & os.ModeCharDevice) != 0
}

// statusColor returns the color code for a status code.
func (m *LoggingMiddleware) statusColor(status int, colorEnabled bool) string {
	if !colorEnabled {
		return ""
	}
	switch {
	case status >= 500:
		return colorRed
	case status >= 400:
		return colorYellow
	case status >= 300:
		return colorCyan
	case status >= 200:
		return colorGreen
	default:
		return colorReset
	}
}

// levelString returns the formatted level string.
func (m *LoggingMiddleware) levelString(level slog.Level, colorEnabled bool) string {
	var str, color string
	switch level {
	case slog.LevelError:
		str = "ERR"
		color = colorRed
	case slog.LevelWarn:
		str = "WRN"
		color = colorYellow
	case slog.LevelInfo:
		str = "INF"
		color = colorGreen
	case slog.LevelDebug:
		str = "DBG"
		color = colorBlue
	default:
		str = "???"
		color = colorReset
	}

	if colorEnabled {
		return fmt.Sprintf("%s%s%s", color, str, colorReset)
	}
	return str
}

// formatDuration formats the duration for console output.
func (m *LoggingMiddleware) formatDuration(d time.Duration, colorEnabled bool) string {
	str := d.Round(time.Millisecond).String()
	if colorEnabled {
		return fmt.Sprintf("%s%s%s", colorGray, str, colorReset)
	}
	return str
}

// DefaultLoggingMiddleware creates a LoggingMiddleware with default configuration.
// Returns nil if initialization fails (caller should fall back to chi.Logger).
func DefaultLoggingMiddleware() *LoggingMiddleware {
	m, err := NewLoggingMiddleware(LoggingConfig{})
	if err != nil {
		slog.Warn("Failed to initialize logging middleware, falling back to default", "error", err)
		return nil
	}
	return m
}
