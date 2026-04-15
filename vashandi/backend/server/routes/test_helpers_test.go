package routes

import (
	"context"
	"net/http"

	"github.com/go-chi/chi/v5"
)

func withBoardActorRequest(req *http.Request) *http.Request {
	return req.WithContext(WithActor(req.Context(), ActorInfo{
		UserID:    "board-user",
		ActorType: "board",
	}))
}

func withAgentActorRequest(req *http.Request, agentID string) *http.Request {
	return req.WithContext(WithActor(req.Context(), ActorInfo{
		AgentID:   agentID,
		IsAgent:   true,
		ActorType: "agent",
	}))
}

// withAgentActorForCompanyRequest creates an agent actor request scoped to a specific company.
// Use this when the route under test calls AssertCompanyAccess (or requireCompanyAccess)
// and the test expects the request to pass the company-scope check.
func withAgentActorForCompanyRequest(req *http.Request, agentID, companyID string) *http.Request {
	return req.WithContext(WithActor(req.Context(), ActorInfo{
		AgentID:   agentID,
		CompanyID: companyID,
		IsAgent:   true,
		ActorType: "agent",
	}))
}

// newChiCtxWithParams wraps a request so that chi.URLParam resolves the given
// key→value pairs. Use this in unit tests that call handlers directly (without
// going through a real chi router).
func newChiCtxWithParams(r *http.Request, params map[string]string) *http.Request {
	rctx := chi.NewRouteContext()
	for k, v := range params {
		rctx.URLParams.Add(k, v)
	}
	return r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rctx))
}
