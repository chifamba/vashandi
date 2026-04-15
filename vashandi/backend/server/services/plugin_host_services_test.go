package services

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestIsPrivateIP(t *testing.T) {
	cases := []struct {
		ip       string
		expected bool
	}{
		{"127.0.0.1", true},
		{"10.0.0.1", true},
		{"172.16.0.1", true},
		{"192.168.1.1", true},
		{"169.254.1.1", true},
		{"::1", true},
		{"fc00::1", true},
		{"fe80::1", true},
		{"::ffff:127.0.0.1", true},
		{"0.0.0.0", true},
		{"8.8.8.8", false},
		{"203.0.113.1", false},
	}

	for _, c := range cases {
		if isPrivateIP(c.ip) != c.expected {
			t.Errorf("expected %t for %s", c.expected, c.ip)
		}
	}
}

func TestValidateAndResolveFetchURL_Safe(t *testing.T) {
	ctx := context.Background()

	// 1. Safe domain
	target, err := ValidateAndResolveFetchURL(ctx, "https://example.com")
	if err != nil {
		t.Fatalf("unexpected error for safe domain: %v", err)
	}
	if target.HostHeader != "example.com" {
		t.Errorf("expected host header example.com, got %s", target.HostHeader)
	}
	if target.ResolvedAddress == "" || isPrivateIP(target.ResolvedAddress) {
		t.Errorf("expected public IP for example.com, got %s", target.ResolvedAddress)
	}

	// 2. Safe IP
	target, err = ValidateAndResolveFetchURL(ctx, "http://8.8.8.8")
	if err != nil {
		t.Fatalf("unexpected error for safe IP: %v", err)
	}
	if target.ResolvedAddress != "8.8.8.8" {
		t.Errorf("expected 8.8.8.8, got %s", target.ResolvedAddress)
	}
}

func TestValidateAndResolveFetchURL_Unsafe(t *testing.T) {
	ctx := context.Background()

	// 1. Disallowed protocol
	_, err := ValidateAndResolveFetchURL(ctx, "ftp://example.com")
	if err == nil || !strings.Contains(err.Error(), "disallowed protocol") {
		t.Errorf("expected disallowed protocol error, got %v", err)
	}

	// 2. Private IP
	_, err = ValidateAndResolveFetchURL(ctx, "http://127.0.0.1")
	if err == nil || !strings.Contains(err.Error(), "all resolved IPs") {
		t.Errorf("expected private IP error, got %v", err)
	}

	// 3. Localhost hostname
	_, err = ValidateAndResolveFetchURL(ctx, "http://localhost")
	if err == nil || !strings.Contains(err.Error(), "all resolved IPs") {
		t.Errorf("expected private IP error for localhost, got %v", err)
	}
}

func TestSafeFetch_HeadersAndRouting(t *testing.T) {
	var capturedHost string
	var capturedHeader string

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedHost = r.Host
		capturedHeader = r.Header.Get("X-Custom")
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	importUrl := ts.URL
	importUrl = strings.ReplaceAll(importUrl, "127.0.0.1", "localhost")

	parsed, _ := url.Parse(importUrl)
	port := parsed.Port()

	target := &ValidatedFetchTarget{
		ParsedURL:       parsed,
		ResolvedAddress: "127.0.0.1",
		HostHeader:      "api.example.com",
		TLSServerName:   "",
		UseTLS:          false,
	}

	client := BuildSafeHTTPClient(target)

	req, _ := http.NewRequestWithContext(context.Background(), "GET", target.ParsedURL.String(), nil)
	req.Host = target.HostHeader
	req.Header.Set("X-Custom", "test-val")

	addr := "127.0.0.1:" + port
	client.Transport.(*http.Transport).DialContext = func(ctx context.Context, network, _ string) (net.Conn, error) {
		dialer := &net.Dialer{}
		return dialer.DialContext(ctx, network, addr)
	}

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if capturedHost != "api.example.com" {
		t.Errorf("expected Host header api.example.com, got %s", capturedHost)
	}
	if capturedHeader != "test-val" {
		t.Errorf("expected X-Custom header test-val, got %s", capturedHeader)
	}
}
