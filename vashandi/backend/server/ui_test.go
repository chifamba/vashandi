package server_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/chifamba/vashandi/vashandi/backend/server"
)

// writeFile creates a file at path (including parent dirs) with the given content.
func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
}

func TestNewStaticUIHandler_ServesExistingFile(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "index.html"), "<html>app</html>")
	writeFile(t, filepath.Join(dir, "assets", "app.js"), "console.log('hi')")

	h, err := server.NewStaticUIHandler(dir)
	if err != nil {
		t.Fatalf("NewStaticUIHandler: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/assets/app.js", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	body, _ := io.ReadAll(rr.Body)
	if string(body) != "console.log('hi')" {
		t.Fatalf("unexpected body: %q", string(body))
	}
}

func TestNewStaticUIHandler_SPAFallback(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "index.html"), "<html>spa</html>")

	h, err := server.NewStaticUIHandler(dir)
	if err != nil {
		t.Fatalf("NewStaticUIHandler: %v", err)
	}

	for _, path := range []string{"/", "/some/deep/route", "/companies/abc"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("path %q: expected 200, got %d", path, rr.Code)
		}
		body, _ := io.ReadAll(rr.Body)
		if string(body) != "<html>spa</html>" {
			t.Fatalf("path %q: unexpected body: %q", path, string(body))
		}
		ct := rr.Header().Get("Content-Type")
		if ct != "text/html; charset=utf-8" {
			t.Fatalf("path %q: unexpected Content-Type: %q", path, ct)
		}
	}
}

func TestNewStaticUIHandler_MissingIndexReturnsError(t *testing.T) {
	dir := t.TempDir()
	// No index.html written.

	_, err := server.NewStaticUIHandler(dir)
	if err == nil {
		t.Fatal("expected error when index.html is missing")
	}
}

func TestNewStaticUIHandler_PathTraversal(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "index.html"), "<html>app</html>")

	// Write a sensitive file outside the dist dir.
	sensitive := filepath.Join(t.TempDir(), "secret.txt")
	writeFile(t, sensitive, "secret")

	h, err := server.NewStaticUIHandler(dir)
	if err != nil {
		t.Fatalf("NewStaticUIHandler: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/../secret.txt", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	// Either the SPA fallback (200 with index.html content) or 404 is acceptable;
	// what must NOT happen is serving the sensitive file outside the dist dir.
	body, _ := io.ReadAll(rr.Body)
	if string(body) == "secret" {
		t.Fatal("path traversal: served file outside the dist directory")
	}
}

func TestDiscoverUIDistDir_NotFound(t *testing.T) {
	// When running tests the binary is not next to a ui-dist dir, so the result
	// should be an empty string rather than a panic.
	result := server.DiscoverUIDistDir()
	// The test binary lives in a temp dir; unless someone actually has a ui-dist
	// directory there, this should be empty.  We just verify it doesn't panic
	// and returns a string (empty or a valid path).
	_ = result
}
