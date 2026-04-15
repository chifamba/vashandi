package server

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/chifamba/vashandi/vashandi/backend/server/routes"
)

// withActor injects an ActorInfo into a test request.
func withActorBMG(r *http.Request, actor routes.ActorInfo) *http.Request {
	return r.WithContext(routes.WithActor(r.Context(), actor))
}

func okHandler(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusNoContent)
}

func TestBoardMutationGuard_SafeMethodsAlwaysPass(t *testing.T) {
	for _, method := range []string{"GET", "HEAD", "OPTIONS"} {
		req := httptest.NewRequest(method, "/api/v1/companies", nil)
		req = withActorBMG(req, routes.ActorInfo{ActorType: "board", ActorSource: "session"})
		rr := httptest.NewRecorder()
		BoardMutationGuard(http.HandlerFunc(okHandler)).ServeHTTP(rr, req)
		if rr.Code != http.StatusNoContent {
			t.Errorf("%s: expected 204, got %d", method, rr.Code)
		}
	}
}

func TestBoardMutationGuard_AgentPassesThrough(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/api/v1/issues", nil)
	req = withActorBMG(req, routes.ActorInfo{ActorType: "agent", IsAgent: true, ActorSource: "agent_key"})
	rr := httptest.NewRecorder()
	BoardMutationGuard(http.HandlerFunc(okHandler)).ServeHTTP(rr, req)
	if rr.Code != http.StatusNoContent {
		t.Errorf("agent: expected 204, got %d", rr.Code)
	}
}

func TestBoardMutationGuard_AnonymousPassesThrough(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/api/v1/issues", nil)
	req = withActorBMG(req, routes.ActorInfo{ActorType: "anonymous"})
	rr := httptest.NewRecorder()
	BoardMutationGuard(http.HandlerFunc(okHandler)).ServeHTTP(rr, req)
	if rr.Code != http.StatusNoContent {
		t.Errorf("anonymous: expected 204, got %d", rr.Code)
	}
}

func TestBoardMutationGuard_LocalImplicitPassesWithoutOrigin(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/api/v1/issues", nil)
	req = withActorBMG(req, routes.ActorInfo{ActorType: "board", ActorSource: "local_implicit"})
	rr := httptest.NewRecorder()
	BoardMutationGuard(http.HandlerFunc(okHandler)).ServeHTTP(rr, req)
	if rr.Code != http.StatusNoContent {
		t.Errorf("local_implicit: expected 204, got %d", rr.Code)
	}
}

func TestBoardMutationGuard_BoardKeyPassesWithoutOrigin(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/api/v1/issues", nil)
	req = withActorBMG(req, routes.ActorInfo{ActorType: "board", ActorSource: "board_key"})
	rr := httptest.NewRecorder()
	BoardMutationGuard(http.HandlerFunc(okHandler)).ServeHTTP(rr, req)
	if rr.Code != http.StatusNoContent {
		t.Errorf("board_key: expected 204, got %d", rr.Code)
	}
}

func TestBoardMutationGuard_SessionBlockedWithoutOrigin(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/api/v1/issues", nil)
	req = withActorBMG(req, routes.ActorInfo{ActorType: "board", ActorSource: "session"})
	rr := httptest.NewRecorder()
	BoardMutationGuard(http.HandlerFunc(okHandler)).ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Errorf("session without origin: expected 403, got %d", rr.Code)
	}
}

func TestBoardMutationGuard_SessionAllowedWithTrustedOrigin(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/api/v1/issues", nil)
	req.Host = "myapp.example.com"
	req.Header.Set("Origin", "http://myapp.example.com")
	req = withActorBMG(req, routes.ActorInfo{ActorType: "board", ActorSource: "session"})
	rr := httptest.NewRecorder()
	BoardMutationGuard(http.HandlerFunc(okHandler)).ServeHTTP(rr, req)
	if rr.Code != http.StatusNoContent {
		t.Errorf("session with trusted origin: expected 204, got %d", rr.Code)
	}
}

func TestBoardMutationGuard_SessionAllowedWithDefaultDevOrigin(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/api/v1/issues", nil)
	req.Header.Set("Origin", "http://localhost:3100")
	req = withActorBMG(req, routes.ActorInfo{ActorType: "board", ActorSource: "session"})
	rr := httptest.NewRecorder()
	BoardMutationGuard(http.HandlerFunc(okHandler)).ServeHTTP(rr, req)
	if rr.Code != http.StatusNoContent {
		t.Errorf("session with localhost:3100 origin: expected 204, got %d", rr.Code)
	}
}

func TestBoardMutationGuard_SessionAllowedWithTrustedReferer(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/api/v1/issues", nil)
	req.Host = "myapp.example.com"
	req.Header.Set("Referer", "http://myapp.example.com/issues/abc")
	req = withActorBMG(req, routes.ActorInfo{ActorType: "board", ActorSource: "session"})
	rr := httptest.NewRecorder()
	BoardMutationGuard(http.HandlerFunc(okHandler)).ServeHTTP(rr, req)
	if rr.Code != http.StatusNoContent {
		t.Errorf("session with trusted referer: expected 204, got %d", rr.Code)
	}
}

func TestBoardMutationGuard_ForwardedHostMatchingOrigin(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/api/v1/issues", nil)
	req.Host = "127.0.0.1"
	req.Header.Set("X-Forwarded-Host", "10.90.10.20:3443")
	req.Header.Set("Origin", "https://10.90.10.20:3443")
	req = withActorBMG(req, routes.ActorInfo{ActorType: "board", ActorSource: "session"})
	rr := httptest.NewRecorder()
	BoardMutationGuard(http.HandlerFunc(okHandler)).ServeHTTP(rr, req)
	if rr.Code != http.StatusNoContent {
		t.Errorf("forwarded host match: expected 204, got %d", rr.Code)
	}
}

func TestBoardMutationGuard_ForwardedHostMismatchOriginBlocked(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/api/v1/issues", nil)
	req.Host = "127.0.0.1"
	req.Header.Set("X-Forwarded-Host", "10.90.10.20:3443")
	req.Header.Set("Origin", "https://evil.example.com")
	req = withActorBMG(req, routes.ActorInfo{ActorType: "board", ActorSource: "session"})
	rr := httptest.NewRecorder()
	BoardMutationGuard(http.HandlerFunc(okHandler)).ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Errorf("forwarded host mismatch: expected 403, got %d", rr.Code)
	}
}
