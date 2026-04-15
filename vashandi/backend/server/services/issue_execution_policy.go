package services

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
)

// ──────────────────────────────────────────────────────────────────────────────
// Domain types
// ──────────────────────────────────────────────────────────────────────────────

type IssueExecutionStagePrincipal struct {
	Type    string  `json:"type"` // "agent" | "user"
	AgentID *string `json:"agentId"`
	UserID  *string `json:"userId"`
}

type IssueExecutionStageParticipant struct {
	ID      string  `json:"id"`
	Type    string  `json:"type"` // "agent" | "user"
	AgentID *string `json:"agentId"`
	UserID  *string `json:"userId"`
}

type IssueExecutionStage struct {
	ID               string                           `json:"id"`
	Type             string                           `json:"type"` // "review" | "approval"
	ApprovalsNeeded  int                              `json:"approvalsNeeded"`
	Participants     []IssueExecutionStageParticipant `json:"participants"`
}

type IssueExecutionPolicy struct {
	Mode            string                `json:"mode"`
	CommentRequired bool                  `json:"commentRequired"`
	Stages          []IssueExecutionStage `json:"stages"`
}

type IssueExecutionState struct {
	Status              string                         `json:"status"` // "pending" | "completed" | "changes_requested"
	CurrentStageID      *string                        `json:"currentStageId"`
	CurrentStageIndex   *int                           `json:"currentStageIndex"`
	CurrentStageType    *string                        `json:"currentStageType"`
	CurrentParticipant  *IssueExecutionStagePrincipal  `json:"currentParticipant"`
	ReturnAssignee      *IssueExecutionStagePrincipal  `json:"returnAssignee"`
	CompletedStageIDs   []string                       `json:"completedStageIds"`
	LastDecisionID      *string                        `json:"lastDecisionId"`
	LastDecisionOutcome *string                        `json:"lastDecisionOutcome"`
}

type IssueExecutionDecisionInfo struct {
	StageID   string `json:"stageId"`
	StageType string `json:"stageType"`
	Outcome   string `json:"outcome"`
	Body      string `json:"body"`
}

type IssueExecutionTransitionResult struct {
	Patch                      map[string]interface{}
	Decision                   *IssueExecutionDecisionInfo
	WorkflowControlledAssignment bool
}

// ──────────────────────────────────────────────────────────────────────────────
// Parsing / normalization
// ──────────────────────────────────────────────────────────────────────────────

// ParseIssueExecutionPolicy parses raw JSON into a policy, returning nil on failure.
func ParseIssueExecutionPolicy(raw json.RawMessage) *IssueExecutionPolicy {
	if raw == nil {
		return nil
	}
	var p IssueExecutionPolicy
	if err := json.Unmarshal(raw, &p); err != nil {
		return nil
	}
	return &p
}

// ParseIssueExecutionState parses raw JSON into a state, returning nil on failure.
func ParseIssueExecutionState(raw json.RawMessage) *IssueExecutionState {
	if raw == nil {
		return nil
	}
	var s IssueExecutionState
	if err := json.Unmarshal(raw, &s); err != nil {
		return nil
	}
	return &s
}

// NormalizeIssueExecutionPolicy validates and normalizes a policy, removing
// empty or duplicated participants and stages with no eligible participants.
// Returns nil if the resulting policy would have no stages.
func NormalizeIssueExecutionPolicy(raw json.RawMessage) (*IssueExecutionPolicy, error) {
	if raw == nil {
		return nil, nil
	}

	var input IssueExecutionPolicy
	if err := json.Unmarshal(raw, &input); err != nil {
		return nil, fmt.Errorf("invalid execution policy JSON: %w", err)
	}

	var stages []IssueExecutionStage
	for _, stage := range input.Stages {
		var participants []IssueExecutionStageParticipant
		seen := make(map[string]struct{})
		for _, p := range stage.Participants {
			var key string
			switch p.Type {
			case "agent":
				if p.AgentID == nil || *p.AgentID == "" {
					continue
				}
				key = "agent:" + *p.AgentID
			case "user":
				if p.UserID == nil || *p.UserID == "" {
					continue
				}
				key = "user:" + *p.UserID
			default:
				continue
			}
			if _, dup := seen[key]; dup {
				continue
			}
			seen[key] = struct{}{}
			if p.ID == "" {
				p.ID = uuid.New().String()
			}
			var agentID, userID *string
			if p.Type == "agent" {
				agentID = p.AgentID
			} else {
				userID = p.UserID
			}
			participants = append(participants, IssueExecutionStageParticipant{
				ID:      p.ID,
				Type:    p.Type,
				AgentID: agentID,
				UserID:  userID,
			})
		}
		if len(participants) == 0 {
			continue
		}
		if stage.ID == "" {
			stage.ID = uuid.New().String()
		}
		stages = append(stages, IssueExecutionStage{
			ID:              stage.ID,
			Type:            stage.Type,
			ApprovalsNeeded: 1,
			Participants:    participants,
		})
	}

	if len(stages) == 0 {
		return nil, nil
	}

	mode := input.Mode
	if mode == "" {
		mode = "normal"
	}
	return &IssueExecutionPolicy{
		Mode:            mode,
		CommentRequired: true,
		Stages:          stages,
	}, nil
}

// ──────────────────────────────────────────────────────────────────────────────
// Principal helpers
// ──────────────────────────────────────────────────────────────────────────────

func principalFromAgentID(id *string) *IssueExecutionStagePrincipal {
	if id == nil || *id == "" {
		return nil
	}
	return &IssueExecutionStagePrincipal{Type: "agent", AgentID: id}
}

func principalFromUserID(id *string) *IssueExecutionStagePrincipal {
	if id == nil || *id == "" {
		return nil
	}
	return &IssueExecutionStagePrincipal{Type: "user", UserID: id}
}

func principalsEqual(a, b *IssueExecutionStagePrincipal) bool {
	if a == nil || b == nil {
		return false
	}
	if a.Type != b.Type {
		return false
	}
	switch a.Type {
	case "agent":
		return a.AgentID != nil && b.AgentID != nil && *a.AgentID == *b.AgentID
	case "user":
		return a.UserID != nil && b.UserID != nil && *a.UserID == *b.UserID
	}
	return false
}

func participantToPrincipal(p IssueExecutionStageParticipant) *IssueExecutionStagePrincipal {
	return &IssueExecutionStagePrincipal{Type: p.Type, AgentID: p.AgentID, UserID: p.UserID}
}

// AssigneePrincipal returns the assignee principal from an issue's assignee fields.
func AssigneePrincipal(assigneeAgentID, assigneeUserID *string) *IssueExecutionStagePrincipal {
	if p := principalFromAgentID(assigneeAgentID); p != nil {
		return p
	}
	return principalFromUserID(assigneeUserID)
}

func actorPrincipal(agentID, userID *string) *IssueExecutionStagePrincipal {
	if p := principalFromAgentID(agentID); p != nil {
		return p
	}
	return principalFromUserID(userID)
}

func patchForPrincipal(p *IssueExecutionStagePrincipal) map[string]interface{} {
	if p == nil {
		return map[string]interface{}{"assignee_agent_id": nil, "assignee_user_id": nil}
	}
	if p.Type == "agent" {
		return map[string]interface{}{"assignee_agent_id": p.AgentID, "assignee_user_id": nil}
	}
	return map[string]interface{}{"assignee_agent_id": nil, "assignee_user_id": p.UserID}
}

func stageHasParticipant(stage IssueExecutionStage, principal *IssueExecutionStagePrincipal) bool {
	for _, p := range stage.Participants {
		if principalsEqual(participantToPrincipal(p), principal) {
			return true
		}
	}
	return false
}

func selectStageParticipant(stage IssueExecutionStage, preferred, exclude *IssueExecutionStagePrincipal) *IssueExecutionStagePrincipal {
	var candidates []IssueExecutionStageParticipant
	for _, p := range stage.Participants {
		if !principalsEqual(participantToPrincipal(p), exclude) {
			candidates = append(candidates, p)
		}
	}
	if len(candidates) == 0 {
		return nil
	}
	if preferred != nil {
		for _, p := range candidates {
			if principalsEqual(participantToPrincipal(p), preferred) {
				return participantToPrincipal(p)
			}
		}
	}
	return participantToPrincipal(candidates[0])
}

func findStageByID(policy *IssueExecutionPolicy, stageID *string) *IssueExecutionStage {
	if stageID == nil || *stageID == "" || policy == nil {
		return nil
	}
	for i := range policy.Stages {
		if policy.Stages[i].ID == *stageID {
			return &policy.Stages[i]
		}
	}
	return nil
}

func nextPendingStage(policy *IssueExecutionPolicy, state *IssueExecutionState) *IssueExecutionStage {
	completed := make(map[string]struct{})
	if state != nil {
		for _, id := range state.CompletedStageIDs {
			completed[id] = struct{}{}
		}
	}
	for i := range policy.Stages {
		if _, done := completed[policy.Stages[i].ID]; !done {
			return &policy.Stages[i]
		}
	}
	return nil
}

func stageIndex(policy *IssueExecutionPolicy, stage *IssueExecutionStage) int {
	for i := range policy.Stages {
		if policy.Stages[i].ID == stage.ID {
			return i
		}
	}
	return -1
}

// ──────────────────────────────────────────────────────────────────────────────
// State builders
// ──────────────────────────────────────────────────────────────────────────────

func completedStageIDs(previous *IssueExecutionState, current *IssueExecutionStage) []string {
	seen := make(map[string]struct{})
	var result []string
	if previous != nil {
		for _, id := range previous.CompletedStageIDs {
			if _, ok := seen[id]; !ok {
				seen[id] = struct{}{}
				result = append(result, id)
			}
		}
	}
	if _, ok := seen[current.ID]; !ok {
		result = append(result, current.ID)
	}
	return result
}

func execPolicyStrPtr(s string) *string { return &s }
func execPolicyIntPtr(i int) *int       { return &i }

func buildCompletedState(previous *IssueExecutionState, stage *IssueExecutionStage) *IssueExecutionState {
	outcome := "approved"
	return &IssueExecutionState{
		Status:              "completed",
		CurrentStageID:      nil,
		CurrentStageIndex:   nil,
		CurrentStageType:    nil,
		CurrentParticipant:  nil,
		ReturnAssignee:      returnAssignee(previous),
		CompletedStageIDs:   completedStageIDs(previous, stage),
		LastDecisionID:      lastDecisionID(previous),
		LastDecisionOutcome: &outcome,
	}
}

func buildPendingState(previous *IssueExecutionState, policy *IssueExecutionPolicy, stage *IssueExecutionStage, participant *IssueExecutionStagePrincipal, retAssignee *IssueExecutionStagePrincipal) *IssueExecutionState {
	idx := stageIndex(policy, stage)
	var prevIDs []string
	if previous != nil {
		prevIDs = previous.CompletedStageIDs
	}
	return &IssueExecutionState{
		Status:              "pending",
		CurrentStageID:      execPolicyStrPtr(stage.ID),
		CurrentStageIndex:   execPolicyIntPtr(idx),
		CurrentStageType:    execPolicyStrPtr(stage.Type),
		CurrentParticipant:  participant,
		ReturnAssignee:      retAssignee,
		CompletedStageIDs:   prevIDs,
		LastDecisionID:      lastDecisionID(previous),
		LastDecisionOutcome: lastDecisionOutcome(previous),
	}
}

func buildChangesRequestedState(previous *IssueExecutionState, stage *IssueExecutionStage) *IssueExecutionState {
	if previous == nil {
		previous = &IssueExecutionState{}
	}
	outcome := "changes_requested"
	return &IssueExecutionState{
		Status:              "changes_requested",
		CurrentStageID:      execPolicyStrPtr(stage.ID),
		CurrentStageIndex:   previous.CurrentStageIndex,
		CurrentStageType:    execPolicyStrPtr(stage.Type),
		CurrentParticipant:  previous.CurrentParticipant,
		ReturnAssignee:      previous.ReturnAssignee,
		CompletedStageIDs:   previous.CompletedStageIDs,
		LastDecisionID:      previous.LastDecisionID,
		LastDecisionOutcome: &outcome,
	}
}

func returnAssignee(s *IssueExecutionState) *IssueExecutionStagePrincipal {
	if s == nil {
		return nil
	}
	return s.ReturnAssignee
}
func lastDecisionID(s *IssueExecutionState) *string {
	if s == nil {
		return nil
	}
	return s.LastDecisionID
}
func lastDecisionOutcome(s *IssueExecutionState) *string {
	if s == nil {
		return nil
	}
	return s.LastDecisionOutcome
}

// ──────────────────────────────────────────────────────────────────────────────
// Patch helpers
// ──────────────────────────────────────────────────────────────────────────────

func applyMergePatch(dst, src map[string]interface{}) {
	for k, v := range src {
		dst[k] = v
	}
}

func buildPendingStagePatch(
	patch map[string]interface{},
	previous *IssueExecutionState,
	policy *IssueExecutionPolicy,
	stage *IssueExecutionStage,
	participant *IssueExecutionStagePrincipal,
	retAssignee *IssueExecutionStagePrincipal,
) {
	patch["status"] = "in_review"
	applyMergePatch(patch, patchForPrincipal(participant))
	patch["execution_state"] = buildPendingState(previous, policy, stage, participant, retAssignee)
}

func clearExecutionStatePatch(
	patch map[string]interface{},
	issueStatus string,
	requestedStatus *string,
	retAssignee *IssueExecutionStagePrincipal,
) {
	patch["execution_state"] = nil
	if requestedStatus == nil && issueStatus == "in_review" && retAssignee != nil {
		patch["status"] = "in_progress"
		applyMergePatch(patch, patchForPrincipal(retAssignee))
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// IssueExecutionPolicyTransitionInput
// ──────────────────────────────────────────────────────────────────────────────

// IssueExecutionPolicyTransitionInput holds all the data needed to compute a transition.
type IssueExecutionPolicyTransitionInput struct {
	// Issue state
	IssueStatus      string
	AssigneeAgentID  *string
	AssigneeUserID   *string
	ExecutionState   json.RawMessage
	ExecutionPolicy  *IssueExecutionPolicy

	// Requested changes
	RequestedStatus            *string // nil = not changing
	RequestedAssigneeAgentID   *string // nil = not changing
	RequestedAssigneeUserID    *string // nil = not changing
	AssigneePatchProvided      bool

	// Actor
	ActorAgentID *string
	ActorUserID  *string

	// Comment body (for approval/changes-requested transitions)
	CommentBody *string
}

// ApplyIssueExecutionPolicyTransition computes the DB patch required for a given
// issue state transition, enforcing workflow stages from the execution policy.
func ApplyIssueExecutionPolicyTransition(input IssueExecutionPolicyTransitionInput) (*IssueExecutionTransitionResult, error) {
	patch := make(map[string]interface{})

	existingState := ParseIssueExecutionState(input.ExecutionState)
	currentAssignee := AssigneePrincipal(input.AssigneeAgentID, input.AssigneeUserID)
	actor := actorPrincipal(input.ActorAgentID, input.ActorUserID)
	explicitAssignee := AssigneePrincipal(input.RequestedAssigneeAgentID, input.RequestedAssigneeUserID)
	requestedStatus := input.RequestedStatus
	policy := input.ExecutionPolicy

	currentStage := findStageByID(policy, stateCurrentStageID(existingState))
	activeStage := currentStage
	if existingState == nil || existingState.Status != "pending" {
		activeStage = nil
	}

	// No policy: clear any existing state
	if policy == nil {
		if existingState != nil {
			patch["execution_state"] = nil
			if input.IssueStatus == "in_review" && existingState.ReturnAssignee != nil {
				patch["status"] = "in_progress"
				applyMergePatch(patch, patchForPrincipal(existingState.ReturnAssignee))
			}
		}
		return &IssueExecutionTransitionResult{Patch: patch}, nil
	}

	// Re-opening from terminal status resets state
	if (input.IssueStatus == "done" || input.IssueStatus == "cancelled") &&
		requestedStatus != nil &&
		*requestedStatus != "done" &&
		*requestedStatus != "cancelled" {
		patch["execution_state"] = nil
		return &IssueExecutionTransitionResult{Patch: patch}, nil
	}

	// Stale stage reference: stage was removed from policy
	if existingState != nil && existingState.CurrentStageID != nil && currentStage == nil {
		clearExecutionStatePatch(patch, input.IssueStatus, requestedStatus, returnAssignee(existingState))
		return &IssueExecutionTransitionResult{Patch: patch}, nil
	}

	// Active stage (currently in review/approval)
	if activeStage != nil {
		currentParticipant := existingState.CurrentParticipant
		if currentParticipant == nil {
			currentParticipant = selectStageParticipant(*activeStage, nil, returnAssignee(existingState))
		}
		if currentParticipant == nil {
			return nil, fmt.Errorf("no eligible %s participant is configured for this issue", activeStage.Type)
		}

		// Participant removed from stage
		if !stageHasParticipant(*activeStage, currentParticipant) {
			participant := selectStageParticipant(*activeStage, explicitAssignee, returnAssignee(existingState))
			if participant == nil {
				clearExecutionStatePatch(patch, input.IssueStatus, requestedStatus, returnAssignee(existingState))
				return &IssueExecutionTransitionResult{Patch: patch}, nil
			}
			retAssignee := returnAssignee(existingState)
			if retAssignee == nil {
				retAssignee = currentAssignee
			}
			if retAssignee == nil {
				retAssignee = actor
			}
			buildPendingStagePatch(patch, existingState, policy, activeStage, participant, retAssignee)
			return &IssueExecutionTransitionResult{Patch: patch, WorkflowControlledAssignment: true}, nil
		}

		// The actor is the current reviewer/approver
		if principalsEqual(currentParticipant, actor) {
			if requestedStatus != nil && *requestedStatus == "done" {
				if input.CommentBody == nil || *input.CommentBody == "" {
					return nil, errors.New("approving a review or approval stage requires a comment")
				}
				approvedState := buildCompletedState(existingState, activeStage)
				nextStage := nextPendingStage(policy, approvedState)

				decision := &IssueExecutionDecisionInfo{
					StageID:   activeStage.ID,
					StageType: activeStage.Type,
					Outcome:   "approved",
					Body:      *input.CommentBody,
				}

				if nextStage == nil {
					patch["execution_state"] = approvedState
					return &IssueExecutionTransitionResult{Patch: patch, Decision: decision}, nil
				}

				participant := selectStageParticipant(*nextStage, explicitAssignee, returnAssignee(existingState))
				if participant == nil {
					return nil, fmt.Errorf("no eligible %s participant is configured for this issue", nextStage.Type)
				}
				retAssignee := returnAssignee(existingState)
				if retAssignee == nil {
					retAssignee = currentAssignee
				}
				if retAssignee == nil {
					retAssignee = actor
				}
				buildPendingStagePatch(patch, approvedState, policy, nextStage, participant, retAssignee)
				return &IssueExecutionTransitionResult{Patch: patch, Decision: decision, WorkflowControlledAssignment: true}, nil
			}

			if requestedStatus != nil && *requestedStatus != "in_review" {
				if input.CommentBody == nil || *input.CommentBody == "" {
					return nil, errors.New("requesting changes requires a comment")
				}
				if existingState.ReturnAssignee == nil {
					return nil, errors.New("this execution stage has no return assignee")
				}
				patch["status"] = "in_progress"
				applyMergePatch(patch, patchForPrincipal(existingState.ReturnAssignee))
				patch["execution_state"] = buildChangesRequestedState(existingState, activeStage)
				return &IssueExecutionTransitionResult{
					Patch: patch,
					Decision: &IssueExecutionDecisionInfo{
						StageID:   activeStage.ID,
						StageType: activeStage.Type,
						Outcome:   "changes_requested",
						Body:      *input.CommentBody,
					},
					WorkflowControlledAssignment: true,
				}, nil
			}
		}

		// Non-participant trying to advance stage
		attemptedAdvance :=
			(requestedStatus != nil && *requestedStatus != "in_review") ||
				(input.AssigneePatchProvided && !principalsEqual(explicitAssignee, currentParticipant))
		stateDrifted :=
			input.IssueStatus != "in_review" ||
				!principalsEqual(currentAssignee, currentParticipant) ||
				!principalsEqual(existingState.CurrentParticipant, currentParticipant)

		if attemptedAdvance && !stateDrifted {
			return nil, errors.New("only the active reviewer or approver can advance the current execution stage")
		}
		if stateDrifted {
			retAssignee := returnAssignee(existingState)
			if retAssignee == nil {
				retAssignee = currentAssignee
			}
			if retAssignee == nil {
				retAssignee = actor
			}
			buildPendingStagePatch(patch, existingState, policy, activeStage, currentParticipant, retAssignee)
			return &IssueExecutionTransitionResult{Patch: patch, WorkflowControlledAssignment: true}, nil
		}
		return &IssueExecutionTransitionResult{Patch: patch}, nil
	}

	// Start workflow if transitioning to done/in_review
	shouldStart := requestedStatus != nil && (*requestedStatus == "done" || *requestedStatus == "in_review")
	if !shouldStart {
		return &IssueExecutionTransitionResult{Patch: patch}, nil
	}

	var pendingStage *IssueExecutionStage
	if existingState != nil && existingState.Status == "changes_requested" && currentStage != nil {
		pendingStage = currentStage
	} else {
		pendingStage = nextPendingStage(policy, existingState)
	}
	if pendingStage == nil {
		return &IssueExecutionTransitionResult{Patch: patch}, nil
	}

	retAssignee := returnAssignee(existingState)
	if retAssignee == nil {
		retAssignee = currentAssignee
	}

	var preferred *IssueExecutionStagePrincipal
	if existingState != nil && existingState.Status == "changes_requested" {
		preferred = explicitAssignee
		if preferred == nil {
			preferred = existingState.CurrentParticipant
		}
	} else {
		preferred = explicitAssignee
	}

	participant := selectStageParticipant(*pendingStage, preferred, retAssignee)
	if participant == nil {
		return nil, fmt.Errorf("no eligible %s participant is configured for this issue", pendingStage.Type)
	}

	buildPendingStagePatch(patch, existingState, policy, pendingStage, participant, retAssignee)
	return &IssueExecutionTransitionResult{Patch: patch, WorkflowControlledAssignment: true}, nil
}

func stateCurrentStageID(s *IssueExecutionState) *string {
	if s == nil {
		return nil
	}
	return s.CurrentStageID
}
