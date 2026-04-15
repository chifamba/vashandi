package routes

import "net/http"

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
