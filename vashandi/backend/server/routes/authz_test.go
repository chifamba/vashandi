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
		AgentID:   "agent-1",
		IsAgent:   true,
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
		UserID:          "admin-1",
		ActorType:       "board",
		IsInstanceAdmin: true,
	})
	req = req.WithContext(ctx)

	if err := AssertInstanceAdmin(req); err != nil {
		t.Errorf("expected board actor with IsInstanceAdmin to be allowed, got: %v", err)
	}
}

func TestAssertInstanceAdmin_NonAdminBoardActor(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	ctx := WithActor(req.Context(), ActorInfo{
		UserID:    "user-1",
		ActorType: "board",
	})
	req = req.WithContext(ctx)

	if err := AssertInstanceAdmin(req); err == nil {
		t.Error("expected non-admin board actor to be rejected")
	}
}

func TestAssertInstanceAdmin_Anonymous(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	if err := AssertInstanceAdmin(req); err == nil {
		t.Error("expected error for anonymous request to instance admin route")
	}
}

func TestAssertInstanceAdmin_SystemActor(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	ctx := WithActor(req.Context(), ActorInfo{
		IsSystem:  true,
		ActorType: "system",
	})
	req = req.WithContext(ctx)

	if err := AssertInstanceAdmin(req); err != nil {
		t.Errorf("expected system actor to be allowed for instance admin, got: %v", err)
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

func TestAssertCompanyAccess_AgentMatchingCompany(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	ctx := WithActor(req.Context(), ActorInfo{
		AgentID:   "agent-1",
		CompanyID: "company-abc",
		IsAgent:   true,
		ActorType: "agent",
	})
	req = req.WithContext(ctx)

	if err := AssertCompanyAccess(req, "company-abc"); err != nil {
		t.Errorf("expected agent to have access to its own company, got: %v", err)
	}
}

func TestAssertCompanyAccess_AgentMismatchedCompany(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	ctx := WithActor(req.Context(), ActorInfo{
		AgentID:   "agent-1",
		CompanyID: "company-abc",
		IsAgent:   true,
		ActorType: "agent",
	})
	req = req.WithContext(ctx)

	if err := AssertCompanyAccess(req, "company-xyz"); err == nil {
		t.Error("expected agent to be denied access to a different company, got nil")
	}
}

func TestAssertCompanyAccess_AgentNoAgentID(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	ctx := WithActor(req.Context(), ActorInfo{
		IsAgent:   true,
		ActorType: "agent",
	})
	req = req.WithContext(ctx)

	if err := AssertCompanyAccess(req, "company-abc"); err == nil {
		t.Error("expected error for agent with no AgentID, got nil")
	}
}

func TestAssertCompanyAccess_Anonymous(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	// no WithActor — context contains anonymous actor
	if err := AssertCompanyAccess(req, "company-abc"); err == nil {
		t.Error("expected error for anonymous actor, got nil")
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
	if got.ActorType != expected.ActorType {
		t.Errorf("expected ActorType %q, got %q", expected.ActorType, got.ActorType)
	}
}

func TestGetActorInfo_NoContext(t *testing.T) {
	// No WithActor call — middleware not in chain; should return anonymous.
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer sometoken")

	actor := GetActorInfo(req)
	if actor.ActorType != "anonymous" {
		t.Errorf("expected anonymous actor when no context actor is set, got %q", actor.ActorType)
	}
	if actor.UserID != "" {
		t.Errorf("expected empty UserID for anonymous actor, got %q", actor.UserID)
	}
}

func TestGetActorInfo_NoHeader(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	actor := GetActorInfo(req)
	if actor.ActorType != "anonymous" {
		t.Errorf("expected anonymous actor type, got %q", actor.ActorType)
	}
}

func TestGetActorInfo_PreservesCompanyID(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	ctx := WithActor(req.Context(), ActorInfo{
		AgentID:   "agent-42",
		CompanyID: "co-99",
		IsAgent:   true,
		ActorType: "agent",
	})
	req = req.WithContext(ctx)

	got := GetActorInfo(req)
	if got.CompanyID != "co-99" {
		t.Errorf("expected CompanyID %q, got %q", "co-99", got.CompanyID)
	}
}
