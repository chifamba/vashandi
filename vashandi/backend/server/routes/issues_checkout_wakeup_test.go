package routes

import (
	"testing"

	"github.com/chifamba/vashandi/vashandi/backend/db/models"
)

func TestShouldWakeAssigneeOnCheckout_NilIssue(t *testing.T) {
	if ShouldWakeAssigneeOnCheckout(nil, "agent1") {
		t.Error("expected false for nil issue")
	}
}

func TestShouldWakeAssigneeOnCheckout_NoAssignee(t *testing.T) {
	issue := &models.Issue{
		Status: "in_progress",
	}
	if ShouldWakeAssigneeOnCheckout(issue, "agent1") {
		t.Error("expected false when no assignee")
	}
}

func TestShouldWakeAssigneeOnCheckout_SameAgent(t *testing.T) {
	agentID := "agent1"
	issue := &models.Issue{
		AssigneeAgentID: &agentID,
		Status:          "in_progress",
	}
	// Self-checkout should not trigger wakeup
	if ShouldWakeAssigneeOnCheckout(issue, "agent1") {
		t.Error("expected false for self-checkout")
	}
}

func TestShouldWakeAssigneeOnCheckout_DifferentAgent(t *testing.T) {
	agentID := "agent2"
	issue := &models.Issue{
		AssigneeAgentID: &agentID,
		Status:          "in_progress",
	}
	// Different agent should trigger wakeup
	if !ShouldWakeAssigneeOnCheckout(issue, "agent1") {
		t.Error("expected true for cross-agent checkout with in_progress status")
	}
}

func TestShouldWakeAssigneeOnCheckout_NotInProgress(t *testing.T) {
	agentID := "agent2"
	issue := &models.Issue{
		AssigneeAgentID: &agentID,
		Status:          "backlog",
	}
	// Not in_progress should not trigger
	if ShouldWakeAssigneeOnCheckout(issue, "agent1") {
		t.Error("expected false when status is not in_progress")
	}
}

func TestShouldWakeAssigneeOnCheckout_EmptyActor(t *testing.T) {
	agentID := "agent1"
	issue := &models.Issue{
		AssigneeAgentID: &agentID,
		Status:          "in_progress",
	}
	// Empty actorAgentID should trigger wakeup (not a self-checkout)
	if !ShouldWakeAssigneeOnCheckout(issue, "") {
		t.Error("expected true when actorAgentID is empty")
	}
}
