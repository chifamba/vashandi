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
