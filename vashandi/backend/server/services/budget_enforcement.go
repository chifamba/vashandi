package services

import (
	"context"
	"fmt"
	"time"

	"github.com/chifamba/vashandi/vashandi/backend/db/models"
	"gorm.io/gorm"
)

type BudgetScope struct {
	CompanyID string
	ScopeType string
	ScopeID   string
}

type BudgetBlock struct {
	ScopeType string
	ScopeID   string
	ScopeName string
	Reason    string
}

type budgetScopeRecord struct {
	CompanyID   string
	Name        string
	Status      string
	PauseReason string
	PausedAt    *time.Time
}

func (s *HeartbeatService) buildInvocationBudgetScopes(ctx context.Context, companyID, agentID string, contextSnapshot map[string]interface{}) ([]BudgetScope, error) {
	scopes := []BudgetScope{
		{CompanyID: companyID, ScopeType: "company", ScopeID: companyID},
		{CompanyID: companyID, ScopeType: "agent", ScopeID: agentID},
	}

	projectID := readNonEmptyString(contextSnapshot["projectId"])
	if projectID == "" {
		resolved, err := resolveProjectIDForBudgetScope(ctx, s.DB, companyID, readNonEmptyString(contextSnapshot["issueId"]))
		if err != nil {
			return nil, err
		}
		projectID = resolved
	}
	if projectID != "" {
		scopes = append(scopes, BudgetScope{CompanyID: companyID, ScopeType: "project", ScopeID: projectID})
	}
	return scopes, nil
}

func (s *HeartbeatService) GetInvocationBlock(ctx context.Context, companyID, agentID string, scopes []BudgetScope) (*BudgetBlock, error) {
	seen := make(map[string]struct{}, len(scopes))
	for _, scope := range scopes {
		if scope.ScopeID == "" || scope.CompanyID != companyID {
			continue
		}
		key := scope.ScopeType + ":" + scope.ScopeID
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}

		record, err := loadBudgetScopeRecord(ctx, s.DB, scope)
		if err != nil {
			return nil, err
		}
		if record == nil || record.CompanyID != companyID {
			continue
		}
		if isBudgetPaused(scope.ScopeType, record) {
			return &BudgetBlock{
				ScopeType: scope.ScopeType,
				ScopeID:   scope.ScopeID,
				ScopeName: record.Name,
				Reason:    budgetPausedReason(scope.ScopeType),
			}, nil
		}

		policy, err := findActiveHardStopPolicy(ctx, s.DB, scope)
		if err != nil {
			return nil, err
		}
		if policy == nil {
			continue
		}

		incidentOpen, err := hasOpenHardIncident(ctx, s.DB, companyID, policy.ID)
		if err != nil {
			return nil, err
		}
		if incidentOpen {
			return &BudgetBlock{
				ScopeType: scope.ScopeType,
				ScopeID:   scope.ScopeID,
				ScopeName: record.Name,
				Reason:    budgetExceededReason(scope.ScopeType),
			}, nil
		}

		observedAmount, err := computeObservedBudgetAmount(ctx, s.DB, policy)
		if err != nil {
			return nil, err
		}
		if observedAmount >= int64(policy.Amount) {
			return &BudgetBlock{
				ScopeType: scope.ScopeType,
				ScopeID:   scope.ScopeID,
				ScopeName: record.Name,
				Reason:    budgetExceededReason(scope.ScopeType),
			}, nil
		}
	}

	return nil, nil
}

func (s *HeartbeatService) CancelBudgetScopeWork(ctx context.Context, scope BudgetScope) error {
	now := time.Now()
	reason := "Cancelled due to budget pause"

	runs, err := s.listBudgetScopedRuns(ctx, scope)
	if err != nil {
		return err
	}
	if len(runs) > 0 {
		runIDs := make([]string, 0, len(runs))
		agentIDs := make(map[string]struct{}, len(runs))
		for _, run := range runs {
			runIDs = append(runIDs, run.ID)
			agentIDs[run.AgentID] = struct{}{}
		}
		if err := s.DB.WithContext(ctx).
			Model(&models.HeartbeatRun{}).
			Where("id IN ?", runIDs).
			Where("status IN ?", []string{"starting", "queued", "running"}).
			Updates(map[string]interface{}{
				"status":      "cancelled",
				"finished_at": now,
				"error":       reason,
				"error_code":  "cancelled",
				"updated_at":  now,
			}).Error; err != nil {
			return err
		}
		for _, run := range runs {
			if run.WakeupRequestID != nil && *run.WakeupRequestID != "" {
				if err := s.updateWakeupRequestStatus(ctx, *run.WakeupRequestID, "cancelled", reason, now, &run.ID); err != nil {
					return err
				}
			}
			s.cancelBudgetRun(run.ID)
		}
		for agentID := range agentIDs {
			_ = s.finalizeAgentStatus(ctx, agentID, "cancelled")
		}
	}

	wakeupIDs, err := s.listBudgetScopedPendingWakeupIDs(ctx, scope)
	if err != nil {
		return err
	}
	if len(wakeupIDs) > 0 {
		if err := s.DB.WithContext(ctx).
			Model(&models.AgentWakeupRequest{}).
			Where("id IN ?", wakeupIDs).
			Updates(map[string]interface{}{
				"status":      "cancelled",
				"finished_at": now,
				"error":       reason,
				"updated_at":  now,
			}).Error; err != nil {
			return err
		}
	}

	return nil
}

func (s *HeartbeatService) registerBudgetRunCancel(runID string, cancel context.CancelFunc) {
	s.budgetRunCancelsMu.Lock()
	defer s.budgetRunCancelsMu.Unlock()
	s.budgetRunCancels[runID] = cancel
}

func (s *HeartbeatService) releaseBudgetRunCancel(runID string) {
	s.budgetRunCancelsMu.Lock()
	defer s.budgetRunCancelsMu.Unlock()
	delete(s.budgetRunCancels, runID)
}

func (s *HeartbeatService) cancelBudgetRun(runID string) {
	s.budgetRunCancelsMu.RLock()
	cancel := s.budgetRunCancels[runID]
	s.budgetRunCancelsMu.RUnlock()
	if cancel != nil {
		cancel()
	}
}

func (s *HeartbeatService) loadRunStatus(ctx context.Context, runID string) (string, error) {
	var run models.HeartbeatRun
	if err := s.DB.WithContext(ctx).Select("status", "error").Where("id = ?", runID).First(&run).Error; err != nil {
		return "", err
	}
	return run.Status, nil
}

func (s *HeartbeatService) listBudgetScopedRuns(ctx context.Context, scope BudgetScope) ([]models.HeartbeatRun, error) {
	var runs []models.HeartbeatRun
	query := s.DB.WithContext(ctx).Where("company_id = ? AND status IN ?", scope.CompanyID, []string{"starting", "queued", "running"})
	switch scope.ScopeType {
	case "company":
		err := query.Find(&runs).Error
		return runs, err
	case "agent":
		err := query.Where("agent_id = ?", scope.ScopeID).Find(&runs).Error
		return runs, err
	case "project":
		err := query.Find(&runs).Error
		if err != nil {
			return nil, err
		}
		filtered := make([]models.HeartbeatRun, 0, len(runs))
		for _, run := range runs {
			projectID, resolveErr := resolveProjectIDForBudgetScope(ctx, s.DB, scope.CompanyID, readNonEmptyString(parseJSONObject(run.ContextSnapshot)["issueId"]))
			if resolveErr != nil {
				return nil, resolveErr
			}
			if direct := readNonEmptyString(parseJSONObject(run.ContextSnapshot)["projectId"]); direct != "" {
				projectID = direct
			}
			if projectID == scope.ScopeID {
				filtered = append(filtered, run)
			}
		}
		return filtered, nil
	default:
		return nil, fmt.Errorf("unsupported budget scope type %q", scope.ScopeType)
	}
}

func (s *HeartbeatService) listBudgetScopedPendingWakeupIDs(ctx context.Context, scope BudgetScope) ([]string, error) {
	var requests []models.AgentWakeupRequest
	query := s.DB.WithContext(ctx).
		Where("company_id = ? AND run_id IS NULL AND status IN ?", scope.CompanyID, []string{"queued", "deferred_issue_execution"})
	switch scope.ScopeType {
	case "company":
		if err := query.Find(&requests).Error; err != nil {
			return nil, err
		}
	case "agent":
		if err := query.Where("agent_id = ?", scope.ScopeID).Find(&requests).Error; err != nil {
			return nil, err
		}
	case "project":
		if err := query.Find(&requests).Error; err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("unsupported budget scope type %q", scope.ScopeType)
	}

	ids := make([]string, 0, len(requests))
	for _, request := range requests {
		if scope.ScopeType == "project" {
			payload := parseJSONObject(request.Payload)
			projectID := readNonEmptyString(payload["projectId"])
			if projectID == "" {
				resolved, err := resolveProjectIDForBudgetScope(ctx, s.DB, scope.CompanyID, readNonEmptyString(payload["issueId"]))
				if err != nil {
					return nil, err
				}
				projectID = resolved
			}
			if projectID != scope.ScopeID {
				continue
			}
		}
		ids = append(ids, request.ID)
	}
	return ids, nil
}

func collectTriggeredBudgetScopes(ctx context.Context, db *gorm.DB, event *models.CostEvent) ([]BudgetScope, error) {
	var policies []models.BudgetPolicy
	if err := db.WithContext(ctx).
		Where("company_id = ? AND is_active = ? AND metric = ? AND hard_stop_enabled = ?", event.CompanyID, true, "billed_cents", true).
		Find(&policies).Error; err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	triggered := make([]BudgetScope, 0, 3)
	seen := make(map[string]struct{})
	for _, policy := range policies {
		if !policyMatchesCostEvent(&policy, event) || policy.Amount <= 0 {
			continue
		}
		observedAmount, err := computeObservedBudgetAmount(ctx, db, &policy)
		if err != nil {
			return nil, err
		}
		if observedAmount < int64(policy.Amount) {
			continue
		}

		if err := upsertHardBudgetIncident(ctx, db, &policy, int(observedAmount), now); err != nil {
			return nil, err
		}
		if err := pauseBudgetScope(ctx, db, &policy, now); err != nil {
			return nil, err
		}

		scope := BudgetScope{CompanyID: policy.CompanyID, ScopeType: policy.ScopeType, ScopeID: policy.ScopeID}
		key := scope.ScopeType + ":" + scope.ScopeID
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		triggered = append(triggered, scope)
	}

	return triggered, nil
}

func findActiveHardStopPolicy(ctx context.Context, db *gorm.DB, scope BudgetScope) (*models.BudgetPolicy, error) {
	var policy models.BudgetPolicy
	err := db.WithContext(ctx).
		Where("company_id = ? AND scope_type = ? AND scope_id = ? AND is_active = ? AND metric = ? AND hard_stop_enabled = ?", scope.CompanyID, scope.ScopeType, scope.ScopeID, true, "billed_cents", true).
		Order("updated_at desc").
		First(&policy).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	return &policy, nil
}

func hasOpenHardIncident(ctx context.Context, db *gorm.DB, companyID, policyID string) (bool, error) {
	var count int64
	err := db.WithContext(ctx).
		Model(&models.BudgetIncident{}).
		Where("company_id = ? AND policy_id = ? AND status = ? AND threshold_type = ?", companyID, policyID, "open", "hard").
		Count(&count).Error
	return count > 0, err
}

func computeObservedBudgetAmount(ctx context.Context, db *gorm.DB, policy *models.BudgetPolicy) (int64, error) {
	query := db.WithContext(ctx).Model(&models.CostEvent{}).Where("company_id = ?", policy.CompanyID)
	switch policy.ScopeType {
	case "agent":
		query = query.Where("agent_id = ?", policy.ScopeID)
	case "project":
		query = query.Where("project_id = ?", policy.ScopeID)
	}

	windowStart, windowEnd := resolveBudgetWindow(policy.WindowKind, time.Now().UTC())
	if appliesMonthlyBudgetWindow(policy.WindowKind) {
		query = query.Where("occurred_at >= ? AND occurred_at < ?", windowStart, windowEnd)
	}

	var total int64
	if err := query.Select("COALESCE(SUM(cost_cents), 0)").Scan(&total).Error; err != nil {
		return 0, err
	}
	return total, nil
}

func resolveBudgetWindow(windowKind string, now time.Time) (time.Time, time.Time) {
	if appliesMonthlyBudgetWindow(windowKind) {
		start := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
		return start, start.AddDate(0, 1, 0)
	}
	return time.Unix(0, 0).UTC(), time.Date(9999, 1, 1, 0, 0, 0, 0, time.UTC)
}

func appliesMonthlyBudgetWindow(windowKind string) bool {
	return windowKind == "" || windowKind == "monthly" || windowKind == "calendar_month_utc"
}

func policyMatchesCostEvent(policy *models.BudgetPolicy, event *models.CostEvent) bool {
	switch policy.ScopeType {
	case "company":
		return policy.ScopeID == event.CompanyID
	case "agent":
		return policy.ScopeID == event.AgentID
	case "project":
		return event.ProjectID != nil && *event.ProjectID == policy.ScopeID
	default:
		return false
	}
}

func upsertHardBudgetIncident(ctx context.Context, db *gorm.DB, policy *models.BudgetPolicy, observedAmount int, now time.Time) error {
	windowStart, windowEnd := resolveBudgetWindow(policy.WindowKind, now)
	var incident models.BudgetIncident
	err := db.WithContext(ctx).
		Where("policy_id = ? AND window_start = ? AND threshold_type = ?", policy.ID, windowStart, "hard").
		First(&incident).Error
	if err != nil && err != gorm.ErrRecordNotFound {
		return err
	}
	if err == gorm.ErrRecordNotFound {
		incident = models.BudgetIncident{
			CompanyID:      policy.CompanyID,
			PolicyID:       policy.ID,
			ScopeType:      policy.ScopeType,
			ScopeID:        policy.ScopeID,
			Metric:         policy.Metric,
			WindowKind:     policy.WindowKind,
			WindowStart:    windowStart,
			WindowEnd:      windowEnd,
			ThresholdType:  "hard",
			AmountLimit:    policy.Amount,
			AmountObserved: observedAmount,
			Status:         "open",
		}
		return db.WithContext(ctx).Create(&incident).Error
	}
	return db.WithContext(ctx).Model(&models.BudgetIncident{}).Where("id = ?", incident.ID).Updates(map[string]interface{}{
		"amount_observed": observedAmount,
		"amount_limit":    policy.Amount,
		"status":          "open",
		"resolved_at":     nil,
		"updated_at":      now,
	}).Error
}

func pauseBudgetScope(ctx context.Context, db *gorm.DB, policy *models.BudgetPolicy, now time.Time) error {
	switch policy.ScopeType {
	case "company":
		return db.WithContext(ctx).Model(&models.Company{}).Where("id = ?", policy.ScopeID).Updates(map[string]interface{}{
			"status":       "paused",
			"pause_reason": "budget",
			"paused_at":    now,
			"updated_at":   now,
		}).Error
	case "agent":
		return db.WithContext(ctx).Model(&models.Agent{}).Where("id = ?", policy.ScopeID).Updates(map[string]interface{}{
			"status":       "paused",
			"pause_reason": "budget",
			"paused_at":    now,
			"updated_at":   now,
		}).Error
	case "project":
		return db.WithContext(ctx).Model(&models.Project{}).Where("id = ?", policy.ScopeID).Updates(map[string]interface{}{
			"pause_reason": "budget",
			"paused_at":    now,
			"updated_at":   now,
		}).Error
	default:
		return fmt.Errorf("unsupported budget scope type %q", policy.ScopeType)
	}
}

func loadBudgetScopeRecord(ctx context.Context, db *gorm.DB, scope BudgetScope) (*budgetScopeRecord, error) {
	switch scope.ScopeType {
	case "company":
		var company models.Company
		if err := db.WithContext(ctx).Select("id", "name", "status", "pause_reason", "paused_at").Where("id = ?", scope.ScopeID).First(&company).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				return nil, nil
			}
			return nil, err
		}
		return &budgetScopeRecord{
			CompanyID:   company.ID,
			Name:        company.Name,
			Status:      company.Status,
			PauseReason: derefString(company.PauseReason),
			PausedAt:    company.PausedAt,
		}, nil
	case "agent":
		var agent models.Agent
		if err := db.WithContext(ctx).Select("id", "company_id", "name", "status", "pause_reason", "paused_at").Where("id = ?", scope.ScopeID).First(&agent).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				return nil, nil
			}
			return nil, err
		}
		return &budgetScopeRecord{
			CompanyID:   agent.CompanyID,
			Name:        agent.Name,
			Status:      agent.Status,
			PauseReason: derefString(agent.PauseReason),
			PausedAt:    agent.PausedAt,
		}, nil
	case "project":
		var project models.Project
		if err := db.WithContext(ctx).Select("id", "company_id", "name", "pause_reason", "paused_at").Where("id = ?", scope.ScopeID).First(&project).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				return nil, nil
			}
			return nil, err
		}
		return &budgetScopeRecord{
			CompanyID:   project.CompanyID,
			Name:        project.Name,
			PauseReason: derefString(project.PauseReason),
			PausedAt:    project.PausedAt,
		}, nil
	default:
		return nil, fmt.Errorf("unsupported budget scope type %q", scope.ScopeType)
	}
}

func isBudgetPaused(_ string, record *budgetScopeRecord) bool {
	if record == nil || record.PauseReason != "budget" {
		return false
	}
	return record.Status == "paused" || record.PausedAt != nil
}

func budgetPausedReason(scopeType string) string {
	switch scopeType {
	case "company":
		return "Company is paused because its budget hard-stop was reached."
	case "agent":
		return "Agent is paused because its budget hard-stop was reached."
	case "project":
		return "Project is paused because its budget hard-stop was reached."
	default:
		return "Budget hard-stop was reached."
	}
}

func budgetExceededReason(scopeType string) string {
	switch scopeType {
	case "company":
		return "Company cannot start new work because its budget hard-stop is exceeded."
	case "agent":
		return "Agent cannot start because its budget hard-stop is still exceeded."
	case "project":
		return "Project cannot start work because its budget hard-stop is still exceeded."
	default:
		return "Budget hard-stop is exceeded."
	}
}

func resolveProjectIDForBudgetScope(ctx context.Context, db *gorm.DB, companyID, issueID string) (string, error) {
	if issueID == "" {
		return "", nil
	}
	var issue models.Issue
	if err := db.WithContext(ctx).Select("project_id").Where("id = ? AND company_id = ?", issueID, companyID).First(&issue).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return "", nil
		}
		return "", err
	}
	return derefString(issue.ProjectID), nil
}
