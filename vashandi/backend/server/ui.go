package server

import (
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
)

// NewStaticUIHandler creates an http.Handler that serves a pre-built frontend
// SPA from distDir.  Static assets that exist on disk are served directly;
// every other path falls back to index.html so that the client-side router can
// handle it.  The index.html content is read and branded once at startup.
func NewStaticUIHandler(distDir string) (http.Handler, error) {
	indexPath := filepath.Join(distDir, "index.html")
	raw, err := os.ReadFile(indexPath)
	if err != nil {
		return nil, err
	}
	indexHTML := ApplyUIBranding(raw)
	fs := http.FileServer(http.Dir(distDir))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Resolve the requested path inside the dist directory.
		// filepath.Clean prevents path traversal.
		fpath := filepath.Join(distDir, filepath.Clean("/"+r.URL.Path))
		if _, err := os.Stat(fpath); os.IsNotExist(err) {
			// Unknown path — serve index.html for client-side routing (SPA fallback).
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(indexHTML)
			return
		}
		fs.ServeHTTP(w, r)
	}), nil
}

// DiscoverUIDistDir searches for a pre-built frontend asset directory relative
// to the running executable.  It returns an empty string when nothing is found.
func DiscoverUIDistDir() string {
	execPath, err := os.Executable()
	if err != nil {
		return ""
	}
	execDir := filepath.Dir(execPath)
	candidates := []string{
		filepath.Join(execDir, "ui-dist"),
		filepath.Join(execDir, "..", "ui-dist"),
	}
	for _, c := range candidates {
		if _, err := os.Stat(filepath.Join(c, "index.html")); err == nil {
			abs, err := filepath.Abs(c)
			if err != nil {
				return c
			}
			return abs
		}
	}
	return ""
}

// newUIHandlerFromConfig discovers the UI dist directory and creates a static
// UI handler when serveUI is true.  Returns nil (no UI serving) with a warning
// when the dist directory cannot be found.
func newUIHandlerFromConfig(serveUI bool) http.Handler {
	if !serveUI {
		return nil
	}
	distDir := DiscoverUIDistDir()
	if distDir == "" {
		slog.Warn("serveUi is enabled but no UI dist directory was found; running in API-only mode")
		return nil
	}
	h, err := NewStaticUIHandler(distDir)
	if err != nil {
		slog.Warn("serveUi is enabled but failed to read UI dist", "dir", distDir, "error", err)
		return nil
	}
	slog.Info("Serving UI from dist directory", "dir", distDir)
	return h
}
