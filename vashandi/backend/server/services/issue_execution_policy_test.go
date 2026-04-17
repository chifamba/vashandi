package services

import (
	"testing"
)

func TestPrincipalsEqual(t *testing.T) {
	agentA := "agentA"
	agentB := "agentB"
	userA := "userA"

	tests := []struct {
		name     string
		a        *IssueExecutionStagePrincipal
		b        *IssueExecutionStagePrincipal
		expected bool
	}{
		{"both nil", nil, nil, false},
		{"a nil", nil, &IssueExecutionStagePrincipal{Type: "agent", AgentID: &agentA}, false},
		{"type mismatch", &IssueExecutionStagePrincipal{Type: "agent", AgentID: &agentA}, &IssueExecutionStagePrincipal{Type: "user", UserID: &userA}, false},
		{"agents match", &IssueExecutionStagePrincipal{Type: "agent", AgentID: &agentA}, &IssueExecutionStagePrincipal{Type: "agent", AgentID: &agentA}, true},
		{"agents mismatch", &IssueExecutionStagePrincipal{Type: "agent", AgentID: &agentA}, &IssueExecutionStagePrincipal{Type: "agent", AgentID: &agentB}, false},
		{"agent missing id", &IssueExecutionStagePrincipal{Type: "agent"}, &IssueExecutionStagePrincipal{Type: "agent", AgentID: &agentA}, false},
		{"users match", &IssueExecutionStagePrincipal{Type: "user", UserID: &userA}, &IssueExecutionStagePrincipal{Type: "user", UserID: &userA}, true},
		{"unsupported type", &IssueExecutionStagePrincipal{Type: "other"}, &IssueExecutionStagePrincipal{Type: "other"}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := principalsEqual(tt.a, tt.b)
			if got != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, got)
			}
		})
	}
}

func TestAssigneePrincipal(t *testing.T) {
	agentID := "agent123"
	userID := "user456"

	tests := []struct {
		name            string
		assigneeAgentID *string
		assigneeUserID  *string
		expectedType    string
		expectedAgentID *string
		expectedUserID  *string
		expectNil       bool
	}{
		{"agent only", &agentID, nil, "agent", &agentID, nil, false},
		{"user only", nil, &userID, "user", nil, &userID, false},
		{"both present (prefers agent)", &agentID, &userID, "agent", &agentID, nil, false},
		{"neither present", nil, nil, "", nil, nil, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := AssigneePrincipal(tt.assigneeAgentID, tt.assigneeUserID)
			if tt.expectNil {
				if got != nil {
					t.Errorf("expected nil, got %v", got)
				}
				return
			}
			if got == nil {
				t.Fatalf("expected non-nil")
			}
			if got.Type != tt.expectedType {
				t.Errorf("expected type %q, got %q", tt.expectedType, got.Type)
			}
			if (got.AgentID == nil && tt.expectedAgentID != nil) || (got.AgentID != nil && tt.expectedAgentID == nil) || (got.AgentID != nil && *got.AgentID != *tt.expectedAgentID) {
				t.Errorf("expected agentID %v, got %v", tt.expectedAgentID, got.AgentID)
			}
			if (got.UserID == nil && tt.expectedUserID != nil) || (got.UserID != nil && tt.expectedUserID == nil) || (got.UserID != nil && *got.UserID != *tt.expectedUserID) {
				t.Errorf("expected userID %v, got %v", tt.expectedUserID, got.UserID)
			}
		})
	}
}
