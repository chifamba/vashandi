package routes

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"gorm.io/datatypes"
	"gorm.io/gorm"

	"github.com/chifamba/vashandi/vashandi/backend/db/models"
	"github.com/chifamba/vashandi/vashandi/backend/server/services"
)

func validateAgentReportsTo(db *gorm.DB, companyID, agentID string, reportsTo *string) error {
	if reportsTo == nil || strings.TrimSpace(*reportsTo) == "" {
		return nil
	}

	parentID := strings.TrimSpace(*reportsTo)
	if agentID != "" && parentID == agentID {
		return gorm.ErrInvalidData
	}

	var parent models.Agent
	if err := db.Select("id", "company_id", "reports_to").First(&parent, "id = ?", parentID).Error; err != nil {
		return err
	}
	if parent.CompanyID != companyID {
		return gorm.ErrInvalidData
	}

	seen := map[string]struct{}{parentID: {}}
	current := parent.ReportsTo
	for current != nil && strings.TrimSpace(*current) != "" {
		currentID := strings.TrimSpace(*current)
		if currentID == agentID {
			return gorm.ErrInvalidData
		}
		if _, ok := seen[currentID]; ok {
			return gorm.ErrInvalidData
		}
		seen[currentID] = struct{}{}

		var next models.Agent
		if err := db.Select("id", "company_id", "reports_to").First(&next, "id = ?", currentID).Error; err != nil {
			return err
		}
		if next.CompanyID != companyID {
			return gorm.ErrInvalidData
		}
		current = next.ReportsTo
	}

	return nil
}

func validateUniqueAgentRole(db *gorm.DB, companyID, agentID, role string) error {
	if !strings.EqualFold(strings.TrimSpace(role), "ceo") {
		return nil
	}

	var count int64
	query := db.Model(&models.Agent{}).Where("company_id = ? AND lower(role) = ?", companyID, "ceo")
	if agentID != "" {
		query = query.Where("id <> ?", agentID)
	}
	if err := query.Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return gorm.ErrDuplicatedKey
	}
	return nil
}

// ListAgentsHandler returns a list of agents for a company
func ListAgentsHandler(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		companyID := chi.URLParam(r, "companyId")

		var agents []models.Agent
		if err := db.Where("company_id = ?", companyID).Find(&agents).Error; err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(agents)
	}
}

// GetAgentHandler returns a specific agent
func GetAgentHandler(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")

		var agent models.Agent
		if err := db.First(&agent, "id = ?", id).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				http.Error(w, "Agent not found", http.StatusNotFound)
			} else {
				http.Error(w, err.Error(), http.StatusInternalServerError)
			}
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(agent)
	}
}

// CreateAgentHandler unmarshals JSON into an Agent, saves it, and triggers OpenBrain sync
func CreateAgentHandler(db *gorm.DB, memory services.MemoryAdapter) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		companyID := chi.URLParam(r, "companyId")

		payload, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		var agent models.Agent
		if err := json.Unmarshal(payload, &agent); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		var aliases struct {
			ReportsToID *string `json:"reportsToId"`
			ReportsTo   *string `json:"reportsTo"`
		}
		if err := json.Unmarshal(payload, &aliases); err == nil {
			switch {
			case aliases.ReportsToID != nil:
				agent.ReportsTo = aliases.ReportsToID
			case aliases.ReportsTo != nil:
				agent.ReportsTo = aliases.ReportsTo
			}
		}
		agent.CompanyID = companyID

		// Default permissions to an empty object if not provided to satisfy not-null constraints
		if len(agent.Permissions) == 0 {
			agent.Permissions = []byte("{}")
		}

		if err := validateUniqueAgentRole(db, companyID, agent.ID, agent.Role); err != nil {
			http.Error(w, "company already has a CEO agent", http.StatusBadRequest)
			return
		}
		if err := validateAgentReportsTo(db, companyID, agent.ID, agent.ReportsTo); err != nil {
			http.Error(w, "invalid reportsTo relationship", http.StatusBadRequest)
			return
		}

		if err := db.Create(&agent).Error; err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		// Fire HTTP POST to OpenBrain webhook for Agent Sync Lifecycle Events (Task 2.3)
		go func(agentID, agentName, compID string) {
			if memory != nil {
				_ = memory.RegisterAgent(context.Background(), compID, agentID, agentName)
			}
		}(agent.ID, agent.Name, companyID)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(agent)
	}
}

// DeleteAgentHandler soft deletes an Agent and triggers OpenBrain namespace closure
func DeleteAgentHandler(db *gorm.DB, memory services.MemoryAdapter) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")

		var agent models.Agent
		if err := db.First(&agent, "id = ?", id).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				http.Error(w, "Agent not found", http.StatusNotFound)
			} else {
				http.Error(w, err.Error(), http.StatusInternalServerError)
			}
			return
		}

		if err := db.Delete(&agent).Error; err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		// Fire HTTP DELETE to OpenBrain webhook for Agent Sync Lifecycle Events (Task 2.3)
		go func(agentID, compID string) {
			if memory != nil {
				_ = memory.DeregisterAgent(context.Background(), compID, agentID)
			}
		}(agent.ID, agent.CompanyID)

		w.WriteHeader(http.StatusNoContent)
	}
}

// UpdateAgentHandler handles PATCH /agents/:id
func UpdateAgentHandler(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")

		var agent models.Agent
		if err := db.First(&agent, "id = ?", id).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				http.Error(w, "Agent not found", http.StatusNotFound)
			} else {
				http.Error(w, err.Error(), http.StatusInternalServerError)
			}
			return
		}

		var body map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		updates := map[string]interface{}{}
		if v, ok := body["name"]; ok {
			updates["name"] = v
		}
		if v, ok := body["role"]; ok {
			updates["role"] = v
		}
		if v, ok := body["reportsToId"]; ok {
			updates["reports_to"] = v
		}
		if v, ok := body["status"]; ok {
			updates["status"] = v
		}
		if v, ok := body["pauseReason"]; ok {
			updates["pause_reason"] = v
		}
		if v, ok := body["runtimeConfig"]; ok {
			var existing map[string]interface{}
			if len(agent.RuntimeConfig) > 0 {
				_ = json.Unmarshal(agent.RuntimeConfig, &existing)
			}
			if existing == nil {
				existing = map[string]interface{}{}
			}
			if incoming, ok := v.(map[string]interface{}); ok {
				for k, val := range incoming {
					existing[k] = val
				}
			}
			merged, _ := json.Marshal(existing)
			updates["runtime_config"] = datatypes.JSON(merged)
		}

		if role, ok := updates["role"].(string); ok {
			if err := validateUniqueAgentRole(db, agent.CompanyID, agent.ID, role); err != nil {
				http.Error(w, "company already has a CEO agent", http.StatusBadRequest)
				return
			}
		}
		if reportsToRaw, ok := updates["reports_to"]; ok {
			var reportsTo *string
			switch v := reportsToRaw.(type) {
			case string:
				reportsTo = &v
			case nil:
				reportsTo = nil
			default:
				http.Error(w, "invalid reportsTo relationship", http.StatusBadRequest)
				return
			}
			if err := validateAgentReportsTo(db, agent.CompanyID, agent.ID, reportsTo); err != nil {
				http.Error(w, "invalid reportsTo relationship", http.StatusBadRequest)
				return
			}
		}

		if len(updates) > 0 {
			if err := db.Model(&agent).Updates(updates).Error; err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
		}

		if err := db.First(&agent, "id = ?", id).Error; err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(agent)
	}
}

// PauseAgentHandler handles POST /agents/:id/pause
func PauseAgentHandler(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")

		var agent models.Agent
		if err := db.First(&agent, "id = ?", id).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				http.Error(w, "Agent not found", http.StatusNotFound)
			} else {
				http.Error(w, err.Error(), http.StatusInternalServerError)
			}
			return
		}

		now := time.Now()
		if err := db.Model(&agent).Updates(map[string]interface{}{
			"status":    "paused",
			"paused_at": now,
		}).Error; err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		if err := db.First(&agent, "id = ?", id).Error; err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(agent)
	}
}

// ResumeAgentHandler handles POST /agents/:id/resume
func ResumeAgentHandler(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")

		var agent models.Agent
		if err := db.First(&agent, "id = ?", id).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				http.Error(w, "Agent not found", http.StatusNotFound)
			} else {
				http.Error(w, err.Error(), http.StatusInternalServerError)
			}
			return
		}

		if err := db.Model(&agent).Updates(map[string]interface{}{
			"status":    "active",
			"paused_at": nil,
		}).Error; err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		if err := db.First(&agent, "id = ?", id).Error; err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(agent)
	}
}

// TerminateAgentHandler handles POST /agents/:id/terminate
func TerminateAgentHandler(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")

		var agent models.Agent
		if err := db.First(&agent, "id = ?", id).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				http.Error(w, "Agent not found", http.StatusNotFound)
			} else {
				http.Error(w, err.Error(), http.StatusInternalServerError)
			}
			return
		}

		if err := db.Model(&agent).Update("status", "terminated").Error; err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		if err := db.First(&agent, "id = ?", id).Error; err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(agent)
	}
}

// GetAgentRuntimeStateHandler handles GET /agents/:id/runtime-state
func GetAgentRuntimeStateHandler(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")

		var state models.AgentRuntimeState
		if err := db.First(&state, "agent_id = ?", id).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusNotFound)
				w.Write([]byte("{}"))
				return
			}
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(state)
	}
}

// ResetAgentSessionHandler handles POST /agents/:id/runtime-state/reset-session
func ResetAgentSessionHandler(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")

		if err := db.Where("agent_id = ?", id).Delete(&models.AgentTaskSession{}).Error; err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusNoContent)
	}
}

// GetAgentTaskSessionsHandler handles GET /agents/:id/task-sessions
func GetAgentTaskSessionsHandler(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")

		var sessions []models.AgentTaskSession
		if err := db.Where("agent_id = ?", id).Order("created_at DESC").Find(&sessions).Error; err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(sessions)
	}
}

// ListConfigRevisionsHandler handles GET /agents/:id/config-revisions
func ListConfigRevisionsHandler(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")

		var revisions []models.AgentConfigRevision
		if err := db.Where("agent_id = ?", id).Order("created_at DESC").Limit(50).Find(&revisions).Error; err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(revisions)
	}
}

// GetConfigRevisionHandler handles GET /agents/:id/config-revisions/:revisionId
func GetConfigRevisionHandler(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		revisionID := chi.URLParam(r, "revisionId")

		var revision models.AgentConfigRevision
		if err := db.First(&revision, "id = ? AND agent_id = ?", revisionID, id).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				http.Error(w, "Config revision not found", http.StatusNotFound)
			} else {
				http.Error(w, err.Error(), http.StatusInternalServerError)
			}
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(revision)
	}
}

// RollbackConfigRevisionHandler handles POST /agents/:id/config-revisions/:revisionId/rollback
func RollbackConfigRevisionHandler(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		revisionID := chi.URLParam(r, "revisionId")

		var original models.AgentConfigRevision
		if err := db.First(&original, "id = ? AND agent_id = ?", revisionID, id).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				http.Error(w, "Config revision not found", http.StatusNotFound)
			} else {
				http.Error(w, err.Error(), http.StatusInternalServerError)
			}
			return
		}

		// Use the most recent revision's AfterConfig as the current state (BeforeConfig for the rollback).
		// Fall back to the agent's RuntimeConfig snapshot if no revisions exist.
		var currentBeforeConfig datatypes.JSON
		var latestRevision models.AgentConfigRevision
		if err := db.Where("agent_id = ?", id).Order("created_at DESC").First(&latestRevision).Error; err == nil {
			currentBeforeConfig = latestRevision.AfterConfig
		} else {
			var agentForConfig models.Agent
			if err2 := db.First(&agentForConfig, "id = ?", id).Error; err2 == nil {
				currentBeforeConfig = agentForConfig.RuntimeConfig
			} else {
				currentBeforeConfig = datatypes.JSON("{}")
			}
		}

		newRevision := models.AgentConfigRevision{
			CompanyID:                original.CompanyID,
			AgentID:                  original.AgentID,
			Source:                   "rollback",
			RolledBackFromRevisionID: &original.ID,
			ChangedKeys:              original.ChangedKeys,
			BeforeConfig:             currentBeforeConfig,
			AfterConfig:              original.AfterConfig,
		}

		if err := db.Create(&newRevision).Error; err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(newRevision)
	}
}

// GetAgentAPIKeysHandler handles GET /agents/:id/keys
func GetAgentAPIKeysHandler(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")

		var keys []models.AgentAPIKey
		if err := db.Where("agent_id = ? AND revoked_at IS NULL", id).Find(&keys).Error; err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		// Strip key_hash before returning
		type safeKey struct {
			ID         string     `json:"id"`
			AgentID    string     `json:"agentId"`
			CompanyID  string     `json:"companyId"`
			Name       string     `json:"name"`
			LastUsedAt *time.Time `json:"lastUsedAt"`
			CreatedAt  time.Time  `json:"createdAt"`
		}
		result := make([]safeKey, len(keys))
		for i, k := range keys {
			result[i] = safeKey{
				ID:         k.ID,
				AgentID:    k.AgentID,
				CompanyID:  k.CompanyID,
				Name:       k.Name,
				LastUsedAt: k.LastUsedAt,
				CreatedAt:  k.CreatedAt,
			}
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(result)
	}
}

// CreateAgentAPIKeyHandler handles POST /agents/:id/keys
func CreateAgentAPIKeyHandler(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")

		var agent models.Agent
		if err := db.First(&agent, "id = ?", id).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				http.Error(w, "Agent not found", http.StatusNotFound)
			} else {
				http.Error(w, err.Error(), http.StatusInternalServerError)
			}
			return
		}

		var body struct {
			Name string `json:"name"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		rawBytes := make([]byte, 24)
		if _, err := rand.Read(rawBytes); err != nil {
			http.Error(w, "failed to generate token", http.StatusInternalServerError)
			return
		}
		token := "pcp_agent_" + hex.EncodeToString(rawBytes)

		sum := sha256.Sum256([]byte(token))
		keyHash := hex.EncodeToString(sum[:])

		key := models.AgentAPIKey{
			AgentID:   id,
			CompanyID: agent.CompanyID,
			Name:      body.Name,
			KeyHash:   keyHash,
		}
		if err := db.Create(&key).Error; err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"id":    key.ID,
			"name":  key.Name,
			"token": token,
		})
	}
}

// RevokeAgentAPIKeyHandler handles DELETE /agents/:id/keys/:keyId
func RevokeAgentAPIKeyHandler(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		keyID := chi.URLParam(r, "keyId")

		now := time.Now()
		result := db.Model(&models.AgentAPIKey{}).
			Where("id = ? AND agent_id = ?", keyID, id).
			Update("revoked_at", now)
		if result.Error != nil {
			http.Error(w, result.Error.Error(), http.StatusInternalServerError)
			return
		}
		if result.RowsAffected == 0 {
			http.Error(w, "API key not found", http.StatusNotFound)
			return
		}

		w.WriteHeader(http.StatusNoContent)
	}
}

// WakeupAgentHandler handles POST /agents/:id/wakeup
func WakeupAgentHandler(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")

		var agent models.Agent
		if err := db.First(&agent, "id = ?", id).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				http.Error(w, "Agent not found", http.StatusNotFound)
			} else {
				http.Error(w, err.Error(), http.StatusInternalServerError)
			}
			return
		}

		var body struct {
			IssueID *string `json:"issueId"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)

		var payload datatypes.JSON
		if body.IssueID != nil {
			p, _ := json.Marshal(map[string]interface{}{"issueId": *body.IssueID})
			payload = datatypes.JSON(p)
		}

		req := models.AgentWakeupRequest{
			CompanyID: agent.CompanyID,
			AgentID:   id,
			Source:    "api",
			Payload:   payload,
		}
		if err := db.Create(&req).Error; err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		json.NewEncoder(w).Encode(req)
	}
}

// ListCompanyHeartbeatRunsHandler handles GET /companies/:companyId/heartbeat-runs
func ListCompanyHeartbeatRunsHandler(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		companyID := chi.URLParam(r, "companyId")

		var runs []models.HeartbeatRun
		if err := db.Where("company_id = ?", companyID).Order("created_at DESC").Limit(100).Find(&runs).Error; err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(runs)
	}
}

// GetHeartbeatRunHandler handles GET /heartbeat-runs/:runId
func GetHeartbeatRunHandler(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		runID := chi.URLParam(r, "runId")

		var run models.HeartbeatRun
		if err := db.First(&run, "id = ?", runID).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				http.Error(w, "Heartbeat run not found", http.StatusNotFound)
			} else {
				http.Error(w, err.Error(), http.StatusInternalServerError)
			}
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(run)
	}
}

// CancelHeartbeatRunHandler handles POST /heartbeat-runs/:runId/cancel
func CancelHeartbeatRunHandler(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		runID := chi.URLParam(r, "runId")

		var run models.HeartbeatRun
		if err := db.First(&run, "id = ?", runID).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				http.Error(w, "Heartbeat run not found", http.StatusNotFound)
			} else {
				http.Error(w, err.Error(), http.StatusInternalServerError)
			}
			return
		}

		now := time.Now()
		if err := db.Model(&run).Updates(map[string]interface{}{
			"status":      "cancelled",
			"finished_at": now,
		}).Error; err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		if err := db.First(&run, "id = ?", runID).Error; err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(run)
	}
}

// GetHeartbeatRunWorkspaceOperationsHandler handles GET /heartbeat-runs/:runId/workspace-operations
func GetHeartbeatRunWorkspaceOperationsHandler(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		runID := chi.URLParam(r, "runId")

		var ops []models.WorkspaceOperation
		if err := db.Where("heartbeat_run_id = ?", runID).Find(&ops).Error; err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(ops)
	}
}

// GetAgentSkillsHandler handles GET /agents/:id/skills
func GetAgentSkillsHandler(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		var agent models.Agent
		if err := db.WithContext(r.Context()).First(&agent, "id = ?", id).Error; err != nil {
			http.Error(w, "Agent not found", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"agentId":     agent.ID,
			"adapterType": agent.AdapterType,
			"supported":   false,
			"mode":        "unsupported",
			"entries":     []interface{}{},
			"warnings":    []interface{}{},
		})
	}
}

// SyncAgentSkillsHandler handles POST /agents/:id/skills/sync
func SyncAgentSkillsHandler(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		var agent models.Agent
		if err := db.WithContext(r.Context()).First(&agent, "id = ?", id).Error; err != nil {
			http.Error(w, "Agent not found", http.StatusNotFound)
			return
		}
		var body map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"agentId":     agent.ID,
			"adapterType": agent.AdapterType,
			"supported":   false,
			"mode":        "unsupported",
			"entries":     []interface{}{},
			"warnings":    []interface{}{},
		})
	}
}

// GetAgentConfigurationHandler handles GET /agents/:id/configuration
func GetAgentConfigurationHandler(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		var agent models.Agent
		if err := db.WithContext(r.Context()).First(&agent, "id = ?", id).Error; err != nil {
			http.Error(w, "Agent not found", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"agentId":       agent.ID,
			"adapterType":   agent.AdapterType,
			"adapterConfig": agent.AdapterConfig,
			"runtimeConfig": agent.RuntimeConfig,
		})
	}
}

// GetAgentInstructionsBundleHandler handles GET /agents/:id/instructions-bundle
func GetAgentInstructionsBundleHandler(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		var agent models.Agent
		if err := db.WithContext(r.Context()).First(&agent, "id = ?", id).Error; err != nil {
			http.Error(w, "Agent not found", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"agentId": agent.ID,
			"mode":    "none",
			"files":   []interface{}{},
		})
	}
}

// PatchAgentInstructionsBundleHandler handles PATCH /agents/:id/instructions-bundle
func PatchAgentInstructionsBundleHandler(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		var agent models.Agent
		if err := db.WithContext(r.Context()).First(&agent, "id = ?", id).Error; err != nil {
			http.Error(w, "Agent not found", http.StatusNotFound)
			return
		}
		var body map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"agentId": agent.ID,
			"mode":    body["mode"],
			"files":   []interface{}{},
		})
	}
}

// GetAgentInstructionsBundleFileHandler handles GET /agents/:id/instructions-bundle/file
func GetAgentInstructionsBundleFileHandler(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		if err := db.WithContext(r.Context()).Select("id").Where("id = ?", id).First(&models.Agent{}).Error; err != nil {
			http.Error(w, "Agent not found", http.StatusNotFound)
			return
		}
		http.Error(w, "No bundle file configured", http.StatusNotFound)
	}
}

// PutAgentInstructionsBundleFileHandler handles PUT /agents/:id/instructions-bundle/file
func PutAgentInstructionsBundleFileHandler(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		if err := db.WithContext(r.Context()).Select("id").Where("id = ?", id).First(&models.Agent{}).Error; err != nil {
			http.Error(w, "Agent not found", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	}
}

// DeleteAgentInstructionsBundleFileHandler handles DELETE /agents/:id/instructions-bundle/file
func DeleteAgentInstructionsBundleFileHandler(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		if err := db.WithContext(r.Context()).Select("id").Where("id = ?", id).First(&models.Agent{}).Error; err != nil {
			http.Error(w, "Agent not found", http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

// PatchAgentInstructionsPathHandler handles PATCH /agents/:id/instructions-path
func PatchAgentInstructionsPathHandler(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		var agent models.Agent
		if err := db.WithContext(r.Context()).First(&agent, "id = ?", id).Error; err != nil {
			http.Error(w, "Agent not found", http.StatusNotFound)
			return
		}
		var body map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		// Merge the path key update into adapterConfig
		var adapterConfig map[string]interface{}
		_ = json.Unmarshal(agent.AdapterConfig, &adapterConfig)
		if adapterConfig == nil {
			adapterConfig = make(map[string]interface{})
		}
		if key, ok := body["adapterConfigKey"].(string); ok {
			if val, ok2 := body["path"]; ok2 {
				adapterConfig[key] = val
			}
		}
		updated, _ := json.Marshal(adapterConfig)
		agent.AdapterConfig = updated
		db.WithContext(r.Context()).Model(&agent).Update("adapter_config", agent.AdapterConfig)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(agent)
	}
}

// GetAdapterModelsHandler handles GET /companies/:companyId/adapters/:type/models
func GetAdapterModelsHandler(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		adapterType := chi.URLParam(r, "type")
		type adapterModel struct {
			ID    string `json:"id"`
			Label string `json:"label"`
		}
		// Return a static list of known models per adapter type using the same
		// array-of-objects contract as the TypeScript server route.
		modelsByType := map[string][]adapterModel{
			"claude": {
				{ID: "claude-opus-4-5", Label: "claude-opus-4-5"},
				{ID: "claude-sonnet-4-5", Label: "claude-sonnet-4-5"},
				{ID: "claude-haiku-4-5", Label: "claude-haiku-4-5"},
			},
			"codex": {
				{ID: "gpt-4o", Label: "gpt-4o"},
				{ID: "gpt-4o-mini", Label: "gpt-4o-mini"},
				{ID: "o1", Label: "o1"},
				{ID: "o3", Label: "o3"},
			},
			"gemini": {
				{ID: "gemini-2.0-flash", Label: "gemini-2.0-flash"},
				{ID: "gemini-1.5-pro", Label: "gemini-1.5-pro"},
			},
			"cursor": {
				{ID: "cursor-default", Label: "cursor-default"},
			},
			"windsurf": {
				{ID: "windsurf-default", Label: "windsurf-default"},
			},
		}
		models := modelsByType[adapterType]
		if models == nil {
			models = []adapterModel{}
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(models)
	}
}

// DetectAdapterModelHandler handles GET /companies/:companyId/adapters/:type/detect-model
func DetectAdapterModelHandler(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		adapterType := chi.URLParam(r, "type")
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"adapterType": adapterType,
			"model":       nil,
			"detected":    false,
		})
	}
}

// GetCompanyAgentConfigurationsHandler handles GET /companies/:companyId/agent-configurations
func GetCompanyAgentConfigurationsHandler(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		companyID := chi.URLParam(r, "companyId")
		var agents []models.Agent
		db.WithContext(r.Context()).Where("company_id = ?", companyID).Find(&agents)
		configs := make([]map[string]interface{}, 0, len(agents))
		for _, a := range agents {
			configs = append(configs, map[string]interface{}{
				"agentId":       a.ID,
				"name":          a.Name,
				"adapterType":   a.AdapterType,
				"adapterConfig": a.AdapterConfig,
			})
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(configs)
	}
}

// GetSchedulerHeartbeatsHandler handles GET /instance/scheduler-heartbeats
func GetSchedulerHeartbeatsHandler(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var agents []models.Agent
		db.WithContext(r.Context()).Where("last_heartbeat_at IS NOT NULL").
			Order("last_heartbeat_at DESC").Limit(50).Find(&agents)
		result := make([]map[string]interface{}, 0, len(agents))
		for _, a := range agents {
			result = append(result, map[string]interface{}{
				"agentId":         a.ID,
				"name":            a.Name,
				"lastHeartbeatAt": a.LastHeartbeatAt,
			})
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(result)
	}
}

// GetCompanyLiveRunsHandler handles GET /companies/:companyId/live-runs
func GetCompanyLiveRunsHandler(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		companyID := chi.URLParam(r, "companyId")
		var runs []models.HeartbeatRun
		db.WithContext(r.Context()).
			Where("company_id = ? AND status IN ('queued','running')", companyID).
			Order("started_at DESC").Find(&runs)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(runs)
	}
}

// GetHeartbeatRunLogHandler handles GET /heartbeat-runs/:runId/log
func GetHeartbeatRunLogHandler(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		runID := chi.URLParam(r, "runId")
		var run models.HeartbeatRun
		if err := db.WithContext(r.Context()).First(&run, "id = ?", runID).Error; err != nil {
			http.Error(w, "Not found", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"runId":    run.ID,
			"logStore": run.LogStore,
			"logRef":   run.LogRef,
			"logBytes": run.LogBytes,
		})
	}
}

// GetWorkspaceOperationLogHandler handles GET /workspace-operations/:operationId/log
func GetWorkspaceOperationLogHandler(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		opID := chi.URLParam(r, "operationId")
		var op models.WorkspaceOperation
		if err := db.WithContext(r.Context()).First(&op, "id = ?", opID).Error; err != nil {
			http.Error(w, "Not found", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"operationId": op.ID,
			"logStore":    op.LogStore,
			"logRef":      op.LogRef,
			"logBytes":    op.LogBytes,
		})
	}
}

// GetAgentMeHandler handles GET /agents/me
func GetAgentMeHandler(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Try to get agent ID from context (set by auth middleware for agent API keys)
		agentID := r.Header.Get("X-Agent-ID")
		if agentID == "" {
			http.Error(w, "Agent authentication required", http.StatusUnauthorized)
			return
		}
		var agent models.Agent
		if err := db.WithContext(r.Context()).First(&agent, "id = ?", agentID).Error; err != nil {
			http.Error(w, "Agent not found", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(agent)
	}
}

// GetAgentMeInboxLiteHandler handles GET /agents/me/inbox-lite
func GetAgentMeInboxLiteHandler(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		agentID := r.Header.Get("X-Agent-ID")
		if agentID == "" {
			http.Error(w, "Agent authentication required", http.StatusUnauthorized)
			return
		}
		var issues []models.Issue
		db.WithContext(r.Context()).
			Where("assignee_agent_id = ? AND status IN ('todo','in_progress','blocked')", agentID).
			Order("updated_at DESC").
			Find(&issues)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(issues)
	}
}

// GetAgentMeInboxMineHandler handles GET /agents/me/inbox/mine
func GetAgentMeInboxMineHandler(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		agentID := r.Header.Get("X-Agent-ID")
		if agentID == "" {
			http.Error(w, "Agent authentication required", http.StatusUnauthorized)
			return
		}
		status := r.URL.Query().Get("status")
		if status == "" {
			status = "todo,in_progress"
		}
		var issues []models.Issue
		q := db.WithContext(r.Context()).
			Where("assignee_agent_id = ?", agentID)
		statuses := strings.Split(status, ",")
		q = q.Where("status IN ?", statuses)
		q.Order("updated_at DESC").Find(&issues)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(issues)
	}
}

// InvokeAgentHeartbeatHandler handles POST /agents/:id/heartbeat/invoke
func InvokeAgentHeartbeatHandler(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		var agent models.Agent
		if err := db.WithContext(r.Context()).First(&agent, "id = ?", id).Error; err != nil {
			http.Error(w, "Agent not found", http.StatusNotFound)
			return
		}
		now := time.Now()
		db.WithContext(r.Context()).Model(&agent).Update("last_heartbeat_at", now)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"agentId":   agent.ID,
			"invokedAt": now,
		})
	}
}

// AgentClaudeLoginHandler handles POST /agents/:id/claude-login
func AgentClaudeLoginHandler(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		var agent models.Agent
		if err := db.WithContext(r.Context()).First(&agent, "id = ?", id).Error; err != nil {
			http.Error(w, "Agent not found", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"agentId": agent.ID,
			"status":  "ok",
		})
	}
}

// CreateAgentHireHandler handles POST /companies/:companyId/agent-hires
func CreateAgentHireHandler(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		companyID := chi.URLParam(r, "companyId")
		var body map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		// Create agent from hire request
		name, _ := body["name"].(string)
		adapterType, _ := body["adapterType"].(string)
		if adapterType == "" {
			adapterType = "process"
		}
		if name == "" {
			http.Error(w, "name is required", http.StatusBadRequest)
			return
		}
		agent := models.Agent{
			CompanyID:   companyID,
			Name:        name,
			AdapterType: adapterType,
			Status:      "idle",
		}
		if err := db.WithContext(r.Context()).Create(&agent).Error; err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(agent)
	}
}

// GetIssueLiveRunsHandler handles GET /issues/:issueId/live-runs
func GetIssueLiveRunsHandler(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		issueID := chi.URLParam(r, "issueId")
		var runs []models.HeartbeatRun
		db.WithContext(r.Context()).
			Where("status IN ('queued','running')").
			Joins("JOIN issues ON issues.checkout_run_id = heartbeat_runs.id OR issues.execution_run_id = heartbeat_runs.id").
			Where("issues.id = ?", issueID).
			Find(&runs)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(runs)
	}
}

// GetIssueActiveRunHandler handles GET /issues/:issueId/active-run
func GetIssueActiveRunHandler(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		issueID := chi.URLParam(r, "issueId")
		var issue models.Issue
		if err := db.WithContext(r.Context()).First(&issue, "id = ?", issueID).Error; err != nil {
			http.Error(w, "Not found", http.StatusNotFound)
			return
		}
		runID := issue.ExecutionRunID
		if runID == nil {
			runID = issue.CheckoutRunID
		}
		if runID == nil {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(nil)
			return
		}
		var run models.HeartbeatRun
		if err := db.WithContext(r.Context()).First(&run, "id = ?", *runID).Error; err != nil {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(nil)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(run)
	}
}

// UpdateAgentPermissionsHandler handles PATCH /agents/:id/permissions
func UpdateAgentPermissionsHandler(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		var agent models.Agent
		if err := db.WithContext(r.Context()).First(&agent, "id = ?", id).Error; err != nil {
			http.Error(w, "Agent not found", http.StatusNotFound)
			return
		}
		var body struct {
			Permissions json.RawMessage `json:"permissions"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if len(body.Permissions) > 0 {
			agent.Permissions = datatypes.JSON(body.Permissions)
		}
		db.WithContext(r.Context()).Model(&agent).Update("permissions", agent.Permissions)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(agent)
	}
}
