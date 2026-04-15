package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func makeHostRequest(path, host string) *http.Request {
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Host = host
	return req
}

func TestPrivateHostnameGuard_Disabled(t *testing.T) {
	guard := PrivateHostnameGuard(PrivateHostnameGuardOptions{Enabled: false})
	req := makeHostRequest("/api/health", "blocked-host.invalid:3100")
	rr := httptest.NewRecorder()
	guard(http.HandlerFunc(okHandler)).ServeHTTP(rr, req)
	if rr.Code != http.StatusNoContent {
		t.Errorf("disabled: expected 204, got %d", rr.Code)
	}
}

func TestPrivateHostnameGuard_LoopbackAllowed(t *testing.T) {
	guard := PrivateHostnameGuard(PrivateHostnameGuardOptions{Enabled: true})
	for _, host := range []string{"localhost:3100", "127.0.0.1:3100", "[::1]:3100"} {
		req := makeHostRequest("/api/health", host)
		rr := httptest.NewRecorder()
		guard(http.HandlerFunc(okHandler)).ServeHTTP(rr, req)
		if rr.Code != http.StatusNoContent {
			t.Errorf("loopback %s: expected 204, got %d", host, rr.Code)
		}
	}
}

func TestPrivateHostnameGuard_AllowedHostnamePermitted(t *testing.T) {
	guard := PrivateHostnameGuard(PrivateHostnameGuardOptions{
		Enabled:          true,
		AllowedHostnames: []string{"my-laptop"},
	})
	req := makeHostRequest("/api/health", "my-laptop:3100")
	rr := httptest.NewRecorder()
	guard(http.HandlerFunc(okHandler)).ServeHTTP(rr, req)
	if rr.Code != http.StatusNoContent {
		t.Errorf("allowed hostname: expected 204, got %d", rr.Code)
	}
}

func TestPrivateHostnameGuard_UnknownHostnameBlocked_JSON(t *testing.T) {
	const blocked = "blocked-host.invalid"
	guard := PrivateHostnameGuard(PrivateHostnameGuardOptions{
		Enabled:          true,
		AllowedHostnames: []string{"some-other-host"},
	})
	req := makeHostRequest("/api/health", blocked+":3100")
	rr := httptest.NewRecorder()
	guard(http.HandlerFunc(okHandler)).ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("unknown hostname: expected 403, got %d", rr.Code)
	}
	body := rr.Body.String()
	if !strings.Contains(body, "pnpm paperclipai allowed-hostname "+blocked) {
		t.Errorf("expected remediation hint in JSON body, got: %s", body)
	}
}

func TestPrivateHostnameGuard_UnknownHostnameBlocked_PlainText(t *testing.T) {
	const blocked = "blocked-host.invalid"
	guard := PrivateHostnameGuard(PrivateHostnameGuardOptions{
		Enabled:          true,
		AllowedHostnames: []string{"some-other-host"},
	})
	req := makeHostRequest("/dashboard", blocked+":3100")
	rr := httptest.NewRecorder()
	guard(http.HandlerFunc(okHandler)).ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("non-api unknown hostname: expected 403, got %d", rr.Code)
	}
	ct := rr.Header().Get("Content-Type")
	if !strings.HasPrefix(ct, "text/plain") {
		t.Errorf("expected text/plain content-type, got: %s", ct)
	}
	if !strings.Contains(rr.Body.String(), "pnpm paperclipai allowed-hostname "+blocked) {
		t.Errorf("expected remediation hint in plain-text body, got: %s", rr.Body.String())
	}
}

func TestPrivateHostnameGuard_BindHostAddedToAllowSet(t *testing.T) {
	guard := PrivateHostnameGuard(PrivateHostnameGuardOptions{
		Enabled:  true,
		BindHost: "myserver.local",
	})
	req := makeHostRequest("/api/health", "myserver.local:3100")
	rr := httptest.NewRecorder()
	guard(http.HandlerFunc(okHandler)).ServeHTTP(rr, req)
	if rr.Code != http.StatusNoContent {
		t.Errorf("bind host: expected 204, got %d", rr.Code)
	}
}

func TestPrivateHostnameGuard_BindHost_0000_NotAdded(t *testing.T) {
	const blocked = "blocked-host.invalid"
	guard := PrivateHostnameGuard(PrivateHostnameGuardOptions{
		Enabled:  true,
		BindHost: "0.0.0.0",
	})
	req := makeHostRequest("/api/health", blocked+":3100")
	rr := httptest.NewRecorder()
	guard(http.HandlerFunc(okHandler)).ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Errorf("0.0.0.0 bind host should not allow %s: got %d", blocked, rr.Code)
	}
}

func TestPrivateHostnameGuard_MissingHostBlocked(t *testing.T) {
	guard := PrivateHostnameGuard(PrivateHostnameGuardOptions{Enabled: true})
	req := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	req.Host = "" // no Host header
	rr := httptest.NewRecorder()
	guard(http.HandlerFunc(okHandler)).ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Errorf("missing host: expected 403, got %d", rr.Code)
	}
}

func TestResolvePrivateHostnameAllowSet(t *testing.T) {
	allow := ResolvePrivateHostnameAllowSet([]string{"  MyHost  ", "other"}, "myserver")
	for _, want := range []string{"myhost", "other", "myserver"} {
		if _, ok := allow[want]; !ok {
			t.Errorf("expected %q in allow set", want)
		}
	}
	// 0.0.0.0 must not be added
	allow2 := ResolvePrivateHostnameAllowSet(nil, "0.0.0.0")
	if _, ok := allow2["0.0.0.0"]; ok {
		t.Error("0.0.0.0 must not be in allow set")
	}
}
