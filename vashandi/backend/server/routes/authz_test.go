package routes

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAssertBoard_Anonymous(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	if err := AssertBoard(req); err == nil {
		t.Error("expected error for anonymous request, got nil")
	}
}

func TestAssertBoard_BoardActor(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	ctx := WithActor(req.Context(), ActorInfo{
		UserID:    "user-1",
		ActorType: "board",
	})
	req = req.WithContext(ctx)

	if err := AssertBoard(req); err != nil {
		t.Errorf("expected no error for board actor, got: %v", err)
	}
}

func TestAssertBoard_AgentActor(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	ctx := WithActor(req.Context(), ActorInfo{
		AgentID:  "agent-1",
		IsAgent:  true,
		ActorType: "agent",
	})
	req = req.WithContext(ctx)

	if err := AssertBoard(req); err == nil {
		t.Error("expected error for agent actor calling board-only route, got nil")
	}
}

func TestAssertBoard_SystemActor(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	ctx := WithActor(req.Context(), ActorInfo{
		IsSystem:  true,
		ActorType: "system",
	})
	req = req.WithContext(ctx)

	if err := AssertBoard(req); err != nil {
		t.Errorf("expected system actor to be allowed, got: %v", err)
	}
}

func TestAssertInstanceAdmin_BoardActor(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	ctx := WithActor(req.Context(), ActorInfo{
		UserID:    "admin-1",
		ActorType: "board",
	})
	req = req.WithContext(ctx)

	if err := AssertInstanceAdmin(req); err != nil {
		t.Errorf("expected board actor to be instance admin, got: %v", err)
	}
}

func TestAssertInstanceAdmin_Anonymous(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	if err := AssertInstanceAdmin(req); err == nil {
		t.Error("expected error for anonymous request to instance admin route")
	}
}

func TestAssertCompanyAccess_BoardActor(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	ctx := WithActor(req.Context(), ActorInfo{
		UserID:    "user-1",
		ActorType: "board",
	})
	req = req.WithContext(ctx)

	if err := AssertCompanyAccess(req, "company-123"); err != nil {
		t.Errorf("expected board actor to have company access, got: %v", err)
	}
}

func TestAssertCompanyAccess_SystemActor(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	ctx := WithActor(req.Context(), ActorInfo{
		IsSystem:  true,
		ActorType: "system",
	})
	req = req.WithContext(ctx)

	if err := AssertCompanyAccess(req, "company-123"); err != nil {
		t.Errorf("expected system actor to have company access, got: %v", err)
	}
}

func TestGetActorInfo_FromContext(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	expected := ActorInfo{
		UserID:    "ctx-user",
		ActorType: "board",
	}
	ctx := WithActor(req.Context(), expected)
	req = req.WithContext(ctx)

	got := GetActorInfo(req)
	if got.UserID != expected.UserID {
		t.Errorf("expected UserID %q, got %q", expected.UserID, got.UserID)
	}
}

func TestGetActorInfo_FallbackFromHeader(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer mytoken")

	actor := GetActorInfo(req)
	if actor.UserID != "mytoken" {
		t.Errorf("expected UserID 'mytoken' from header fallback, got %q", actor.UserID)
	}
}

func TestGetActorInfo_NoHeader(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	actor := GetActorInfo(req)
	if actor.ActorType != "anonymous" {
		t.Errorf("expected anonymous actor type, got %q", actor.ActorType)
	}
}
