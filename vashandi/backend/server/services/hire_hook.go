package services

import (
	"context"
	"log/slog"
	"time"

	"github.com/chifamba/vashandi/vashandi/backend/db/models"
	"gorm.io/gorm"
)

const hireApprovedMessage = "Tell your user that your hire was approved, now they should assign you a task in Paperclip or ask you to create issues."

// HireApprovedPayload is the payload delivered to adapter hooks on hire approval.
type HireApprovedPayload struct {
	CompanyID   string `json:"companyId"`
	AgentID     string `json:"agentId"`
	AgentName   string `json:"agentName"`
	AdapterType string `json:"adapterType"`
	Source      string `json:"source"`
	SourceID    string `json:"sourceId"`
	ApprovedAt  string `json:"approvedAt"`
	Message     string `json:"message"`
}

// HireHookAdapter is an optional interface adapters may implement to respond to hire approval.
type HireHookAdapter interface {
	OnHireApproved(ctx context.Context, payload HireApprovedPayload, adapterConfig map[string]interface{}) (ok bool, errMsg string, detail string, err error)
}

// NotifyHireApprovedInput holds arguments for NotifyHireApproved.
type NotifyHireApprovedInput struct {
	CompanyID  string
	AgentID    string
	Source     string // "join_request" | "approval"
	SourceID   string
	ApprovedAt string // RFC3339; if empty, current time is used
}

// NotifyHireApproved invokes the adapter's OnHireApproved hook when an agent is approved.
// Failures are non-fatal: logged and written to activity, never propagated.
func NotifyHireApproved(ctx context.Context, db *gorm.DB, activity *ActivityService, adapterRegistry HireHookAdapter, input NotifyHireApprovedInput) {
	var agent models.Agent
	if err := db.WithContext(ctx).
		Where("id = ? AND company_id = ?", input.AgentID, input.CompanyID).
		First(&agent).Error; err != nil {
		slog.WarnContext(ctx, "hire hook: agent not found in company, skipping",
			"companyId", input.CompanyID,
			"agentId", input.AgentID,
			"source", input.Source,
			"sourceId", input.SourceID,
		)
		return
	}

	if adapterRegistry == nil {
		return
	}

	approvedAt := input.ApprovedAt
	if approvedAt == "" {
		approvedAt = time.Now().UTC().Format(time.RFC3339)
	}

	payload := HireApprovedPayload{
		CompanyID:   input.CompanyID,
		AgentID:     input.AgentID,
		AgentName:   agent.Name,
		AdapterType: agent.AdapterType,
		Source:      input.Source,
		SourceID:    input.SourceID,
		ApprovedAt:  approvedAt,
		Message:     hireApprovedMessage,
	}

	adapterConfig := jsonToMap(agent.AdapterConfig)

	ok, errMsg, detail, err := adapterRegistry.OnHireApproved(ctx, payload, adapterConfig)
	if err != nil {
		slog.ErrorContext(ctx, "hire hook: adapter threw",
			"error", err,
			"companyId", input.CompanyID,
			"agentId", input.AgentID,
			"adapterType", agent.AdapterType,
			"source", input.Source,
			"sourceId", input.SourceID,
		)
		if activity != nil {
			_, _ = activity.Log(ctx, LogEntry{
				CompanyID:  input.CompanyID,
				ActorType:  "system",
				ActorID:    "hire_hook",
				Action:     "hire_hook.error",
				EntityType: "agent",
				EntityID:   input.AgentID,
				Details: map[string]interface{}{
					"source":      input.Source,
					"sourceId":    input.SourceID,
					"adapterType": agent.AdapterType,
					"error":       err.Error(),
				},
			})
		}
		return
	}

	if ok {
		if activity != nil {
			_, _ = activity.Log(ctx, LogEntry{
				CompanyID:  input.CompanyID,
				ActorType:  "system",
				ActorID:    "hire_hook",
				Action:     "hire_hook.succeeded",
				EntityType: "agent",
				EntityID:   input.AgentID,
				Details: map[string]interface{}{
					"source":      input.Source,
					"sourceId":    input.SourceID,
					"adapterType": agent.AdapterType,
				},
			})
		}
		return
	}

	slog.WarnContext(ctx, "hire hook: adapter returned failure",
		"companyId", input.CompanyID,
		"agentId", input.AgentID,
		"adapterType", agent.AdapterType,
		"source", input.Source,
		"sourceId", input.SourceID,
		"error", errMsg,
		"detail", detail,
	)
	if activity != nil {
		_, _ = activity.Log(ctx, LogEntry{
			CompanyID:  input.CompanyID,
			ActorType:  "system",
			ActorID:    "hire_hook",
			Action:     "hire_hook.failed",
			EntityType: "agent",
			EntityID:   input.AgentID,
			Details: map[string]interface{}{
				"source":      input.Source,
				"sourceId":    input.SourceID,
				"adapterType": agent.AdapterType,
				"error":       errMsg,
				"detail":      detail,
			},
		})
	}
}
