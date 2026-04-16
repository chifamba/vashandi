package services

import (
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

const defaultFeedbackExportBackendURL = "https://telemetry.paperclip.ing"

type FeedbackTraceBundle struct {
	TraceID                string                    `json:"traceId"`
	ExportID               *string                   `json:"exportId,omitempty"`
	CompanyID              string                    `json:"companyId"`
	IssueID                string                    `json:"issueId,omitempty"`
	IssueIdentifier        *string                   `json:"issueIdentifier,omitempty"`
	AdapterType            *string                   `json:"adapterType,omitempty"`
	CaptureStatus          string                    `json:"captureStatus,omitempty"`
	Notes                  []string                  `json:"notes"`
	Envelope               interface{}               `json:"envelope,omitempty"`
	Surface                interface{}               `json:"surface,omitempty"`
	PaperclipRun           interface{}               `json:"paperclipRun,omitempty"`
	RawAdapterTrace        interface{}               `json:"rawAdapterTrace,omitempty"`
	NormalizedAdapterTrace interface{}               `json:"normalizedAdapterTrace,omitempty"`
	Privacy                interface{}               `json:"privacy,omitempty"`
	Integrity              interface{}               `json:"integrity,omitempty"`
	Files                  []FeedbackTraceBundleFile `json:"files"`
	Data                   interface{}               `json:"data,omitempty"`
}

// FeedbackTraceUploadResult is returned after a successful upload.
type FeedbackTraceUploadResult struct {
	ObjectKey string `json:"objectKey"`
}

// FeedbackTraceShareClient uploads feedback trace bundles to a remote endpoint.
type FeedbackTraceShareClient struct {
	baseURL string
	token   string
	client  *http.Client
}

// FeedbackShareClientConfig holds configuration for creating a FeedbackTraceShareClient.
type FeedbackShareClientConfig struct {
	BackendURL string
	Token      string
}

// NewFeedbackTraceShareClient creates a new FeedbackTraceShareClient.
// If backendURL is empty, the default telemetry endpoint is used.
func NewFeedbackTraceShareClient(cfg FeedbackShareClientConfig) *FeedbackTraceShareClient {
	baseURL := strings.TrimSpace(cfg.BackendURL)
	if baseURL == "" {
		baseURL = defaultFeedbackExportBackendURL
	}
	return &FeedbackTraceShareClient{
		baseURL: baseURL,
		token:   strings.TrimSpace(cfg.Token),
		client:  &http.Client{Timeout: 30 * time.Second},
	}
}

// NewFeedbackTraceShareClientFromEnv creates a client from environment variables:
// FEEDBACK_EXPORT_BACKEND_URL and FEEDBACK_EXPORT_BACKEND_TOKEN.
func NewFeedbackTraceShareClientFromEnv() *FeedbackTraceShareClient {
	return NewFeedbackTraceShareClient(FeedbackShareClientConfig{
		BackendURL: os.Getenv("FEEDBACK_EXPORT_BACKEND_URL"),
		Token:      os.Getenv("FEEDBACK_EXPORT_BACKEND_TOKEN"),
	})
}

func buildFeedbackShareObjectKey(bundle *FeedbackTraceBundle, exportedAt time.Time) string {
	year := exportedAt.UTC().Format("2006")
	month := exportedAt.UTC().Format("01")
	day := exportedAt.UTC().Format("02")
	traceKey := bundle.TraceID
	if bundle.ExportID != nil && *bundle.ExportID != "" {
		traceKey = *bundle.ExportID
	}
	return fmt.Sprintf("feedback-traces/%s/%s/%s/%s/%s.json",
		bundle.CompanyID, year, month, day, traceKey)
}

// UploadTraceBundle gzip-compresses the bundle and uploads it to the configured endpoint.
func (c *FeedbackTraceShareClient) UploadTraceBundle(bundle *FeedbackTraceBundle) (*FeedbackTraceUploadResult, error) {
	exportedAt := time.Now()
	objectKey := buildFeedbackShareObjectKey(bundle, exportedAt)

	bodyMap := map[string]interface{}{
		"objectKey":  objectKey,
		"exportedAt": exportedAt.UTC().Format(time.RFC3339),
		"bundle":     bundle,
	}
	bodyJSON, err := json.Marshal(bodyMap)
	if err != nil {
		return nil, fmt.Errorf("marshalling feedback bundle: %w", err)
	}

	var compressed bytes.Buffer
	gz := gzip.NewWriter(&compressed)
	if _, err := gz.Write(bodyJSON); err != nil {
		return nil, fmt.Errorf("gzip compressing feedback bundle: %w", err)
	}
	if err := gz.Close(); err != nil {
		return nil, fmt.Errorf("finalising gzip stream: %w", err)
	}

	payload := map[string]string{
		"encoding": "gzip+base64+json",
		"payload":  base64.StdEncoding.EncodeToString(compressed.Bytes()),
	}
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshalling upload payload: %w", err)
	}

	endpoint := strings.TrimRight(c.baseURL, "/") + "/feedback-traces"
	req, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader(payloadJSON))
	if err != nil {
		return nil, fmt.Errorf("building feedback upload request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("sending feedback upload request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		detail, _ := io.ReadAll(resp.Body)
		msg := strings.TrimSpace(string(detail))
		if msg == "" {
			msg = fmt.Sprintf("feedback trace upload failed with HTTP %d", resp.StatusCode)
		}
		return nil, fmt.Errorf("%s", msg)
	}

	var result struct {
		ObjectKey string `json:"objectKey"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err == nil && strings.TrimSpace(result.ObjectKey) != "" {
		return &FeedbackTraceUploadResult{ObjectKey: result.ObjectKey}, nil
	}
	return &FeedbackTraceUploadResult{ObjectKey: objectKey}, nil
}
