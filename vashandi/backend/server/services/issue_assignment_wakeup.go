package services

import (
	"context"
	"log/slog"
)

// WakeupSource identifies what triggered a wakeup.
type WakeupSource string

const (
	WakeupSourceTimer      WakeupSource = "timer"
	WakeupSourceAssignment WakeupSource = "assignment"
	WakeupSourceOnDemand   WakeupSource = "on_demand"
	WakeupSourceAutomation WakeupSource = "automation"
)

// WakeupTriggerDetail classifies the trigger detail.
type WakeupTriggerDetail string

const (
	WakeupTriggerManual   WakeupTriggerDetail = "manual"
	WakeupTriggerPing     WakeupTriggerDetail = "ping"
	WakeupTriggerCallback WakeupTriggerDetail = "callback"
	WakeupTriggerSystem   WakeupTriggerDetail = "system"
)

// AssignmentWakeupOptions holds options for QueueIssueAssignmentWakeup.
type AssignmentWakeupOptions struct {
	// Issue fields needed for the wakeup decision.
	IssueID         string
	AssigneeAgentID string // empty string means no agent assigned
	IssueStatus     string

	// Wakeup context
	Reason        string
	Mutation      string
	ContextSource string

	// Optional actor context
	RequestedByActorType string // "user" | "agent" | "system"
	RequestedByActorID   string

	// When true the error is re-raised instead of swallowed.
	RethrowOnError bool
}

// QueueIssueAssignmentWakeup enqueues a wakeup for the issue assignee if applicable.
// It mirrors the TypeScript queueIssueAssignmentWakeup function.
func QueueIssueAssignmentWakeup(
	ctx context.Context,
	heartbeat *HeartbeatService,
	companyID string,
	opts AssignmentWakeupOptions,
) {
	if opts.AssigneeAgentID == "" || opts.IssueStatus == "backlog" {
		return
	}

	wakeupOpts := WakeupOptions{
		Source:        string(WakeupSourceAssignment),
		TriggerDetail: string(WakeupTriggerSystem),
		Context: map[string]interface{}{
			"issueId":  opts.IssueID,
			"mutation": opts.Mutation,
			"source":   opts.ContextSource,
		},
	}

	_, err := heartbeat.Wakeup(ctx, companyID, opts.AssigneeAgentID, wakeupOpts)
	if err != nil {
		slog.WarnContext(ctx, "failed to wake assignee on issue assignment",
			"error", err,
			"issueId", opts.IssueID,
		)
		if opts.RethrowOnError {
			panic(err)
		}
	}
}
