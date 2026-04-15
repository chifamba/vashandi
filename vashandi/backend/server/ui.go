package server

import (
	"log/slog"
	"net/http"
	"os"
	"path/filepath"

	"github.com/chifamba/vashandi/vashandi/backend/shared"
)

// NewUIHandlerFromConfigForTest is the exported test shim for newUIHandlerFromConfig.
func NewUIHandlerFromConfigForTest(uiMode string) http.Handler {
	return newUIHandlerFromConfig(uiMode)
}
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

// newUIHandlerFromConfig returns an http.Handler for the given UIMode.
// Returns nil when the mode does not require UI serving (API-only or ui-only
// modes are handled elsewhere).  The caller is responsible for deciding whether
// the returned handler is mounted on the main router or used as a standalone
// server.
func newUIHandlerFromConfig(uiMode string) http.Handler {
	switch uiMode {
	case shared.UIModeStatic, shared.UIModeUIOnly:
		// Both modes need a static file handler; the distinction between them is
		// enforced at the router level in SetupRouter / Run.
	default:
		// "" / "none" or any unrecognised value → no UI serving.
		return nil
	}

	distDir := DiscoverUIDistDir()
	if distDir == "" {
		slog.Warn("uiMode requires UI assets but no ui-dist directory was found; UI will not be served")
		return nil
	}
	h, err := NewStaticUIHandler(distDir)
	if err != nil {
		slog.Warn("uiMode requires UI assets but failed to read ui-dist", "dir", distDir, "error", err)
		return nil
	}
	slog.Info("Serving UI from dist directory", "dir", distDir)
	return h
}
