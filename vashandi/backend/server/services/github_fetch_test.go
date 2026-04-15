package services

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGitHubAPIBase(t *testing.T) {
	if base := GitHubAPIBase("github.com"); base != "https://api.github.com" {
		t.Errorf("expected https://api.github.com, got %s", base)
	}
	if base := GitHubAPIBase("WWW.GITHUB.COM"); base != "https://api.github.com" {
		t.Errorf("expected https://api.github.com, got %s", base)
	}
	if base := GitHubAPIBase("git.mycompany.com"); base != "https://git.mycompany.com/api/v3" {
		t.Errorf("expected https://git.mycompany.com/api/v3, got %s", base)
	}
}

func TestResolveRawGitHubURL(t *testing.T) {
	if url := ResolveRawGitHubURL("github.com", "owner", "repo", "main", "file.txt"); url != "https://raw.githubusercontent.com/owner/repo/main/file.txt" {
		t.Errorf("expected https://raw.githubusercontent.com/owner/repo/main/file.txt, got %s", url)
	}

	if url := ResolveRawGitHubURL("git.mycompany.com", "owner", "repo", "main", "/file.txt"); url != "https://git.mycompany.com/raw/owner/repo/main/file.txt" {
		t.Errorf("expected https://git.mycompany.com/raw/owner/repo/main/file.txt, got %s", url)
	}
}

func TestGHFetch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	}))
	defer server.Close()

	ctx := context.Background()
	req, _ := http.NewRequest("GET", server.URL, nil)

	// Success
	resp, err := GHFetch(ctx, server.URL, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200 OK, got %d", resp.StatusCode)
	}

	// Failure connection refused
	reqFail, _ := http.NewRequest("GET", "http://127.0.0.1:0", nil)
	_, err = GHFetch(ctx, "http://127.0.0.1:0", reqFail)
	if err == nil {
		t.Fatalf("expected error on invalid connection")
	}

	expectedErrSubstring := "could not connect to 127.0.0.1"
	if err.Error()[:len(expectedErrSubstring)] != expectedErrSubstring {
		t.Errorf("expected error starting with %s, got %v", expectedErrSubstring, err)
	}
}
