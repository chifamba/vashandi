package services

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// waitForReadinessURL polls the given URL until it returns a 2xx response or
// the deadline is reached.
func waitForReadinessURL(ctx context.Context, url string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	client := &http.Client{Timeout: 2 * time.Second}

	for time.Now().Before(deadline) {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return fmt.Errorf("invalid readiness URL %q: %w", url, err)
		}
		resp, err := client.Do(req)
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode >= 200 && resp.StatusCode < 300 {
				return nil
			}
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(500 * time.Millisecond):
		}
	}
	return fmt.Errorf("readiness check timed out after %s for URL %s", timeout, url)
}

// pipeOutput reads from r and forwards each chunk to onLog with the given stream
// name and service label.
func pipeOutput(r io.Reader, stream, serviceName string, onLog func(stream, chunk string)) {
	buf := make([]byte, 4096)
	for {
		n, err := r.Read(buf)
		if n > 0 {
			onLog(stream, fmt.Sprintf("[service:%s] %s", serviceName, strings.TrimRight(string(buf[:n]), "")))
		}
		if err != nil {
			return
		}
	}
}
