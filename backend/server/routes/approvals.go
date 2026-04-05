package routes

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/chifamba/paperclip/backend/db/models"
	"github.com/go-chi/chi/v5"
	"gorm.io/gorm"
)

// ListApprovalsHandler lists approvals for a company
func ListApprovalsHandler(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		companyID := chi.URLParam(r, "companyId")
		status := r.URL.Query().Get("status")

		query := db.Where("company_id = ?", companyID)
		if status != "" {
			query = query.Where("status = ?", status)
		}

		var approvals []models.Approval
		query.Order("created_at DESC").Find(&approvals)

		json.NewEncoder(w).Encode(approvals)
	}
}

// GetApprovalHandler gets a specific approval
func GetApprovalHandler(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		id := chi.URLParam(r, "id")

		var approval models.Approval
		if err := db.Where("id = ?", id).First(&approval).Error; err != nil {
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(map[string]string{"error": "Approval not found"})
			return
		}

		json.NewEncoder(w).Encode(approval)
	}
}

// CreateApprovalHandler creates a new approval
func CreateApprovalHandler(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		companyID := chi.URLParam(r, "companyId")

		var approval models.Approval
		if err := json.NewDecoder(r.Body).Decode(&approval); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "invalid payload"})
			return
		}

		approval.CompanyID = companyID
		approval.Status = "pending"

		if err := db.Create(&approval).Error; err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": "failed to create approval"})
			return
		}

		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(approval)
	}
}

// ResolveApprovalRequest is used to parse approval/rejection body
type ResolveApprovalRequest struct {
	DecidedByUserID *string `json:"decidedByUserId"`
	DecisionNote    *string `json:"decisionNote"`
}

// ApproveApprovalHandler marks an approval as approved
func ApproveApprovalHandler(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		id := chi.URLParam(r, "id")

		var req ResolveApprovalRequest
		json.NewDecoder(r.Body).Decode(&req)

		var approval models.Approval
		if err := db.Where("id = ?", id).First(&approval).Error; err != nil {
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(map[string]string{"error": "Approval not found"})
			return
		}

		now := time.Now()
		updates := map[string]interface{}{
			"status":             "approved",
			"decided_at":         now,
			"decided_by_user_id": req.DecidedByUserID,
			"decision_note":      req.DecisionNote,
		}

		db.Model(&approval).Updates(updates)

		// Handle hire_agent logic
		if approval.Type == "hire_agent" {
			var payload map[string]interface{}
			json.Unmarshal(approval.Payload, &payload)

			agentID, _ := payload["agentId"].(string)
			if agentID != "" {
				// Activate existing pending agent
				db.Model(&models.Agent{}).Where("id = ? AND status = ?", agentID, "pending_approval").Update("status", "idle")
			} else {
				// Create new agent from payload
				newAgent := models.Agent{
					CompanyID:     approval.CompanyID,
					Name:          fmt.Sprintf("%v", payload["name"]),
					Role:          fmt.Sprintf("%v", payload["role"]),
					AdapterType:   fmt.Sprintf("%v", payload["adapterType"]),
					Status:        "idle",
				}
				// Simplified: in a real port, we'd map all fields (adapterConfig, etc.)
				db.Create(&newAgent)
			}
		}

		db.Where("id = ?", id).First(&approval) // Refresh
		json.NewEncoder(w).Encode(approval)
	}
}

// RejectApprovalHandler marks an approval as rejected
func RejectApprovalHandler(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		id := chi.URLParam(r, "id")

		var req ResolveApprovalRequest
		json.NewDecoder(r.Body).Decode(&req)

		var approval models.Approval
		if err := db.Where("id = ?", id).First(&approval).Error; err != nil {
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(map[string]string{"error": "Approval not found"})
			return
		}

		now := time.Now()
		updates := map[string]interface{}{
			"status":             "rejected",
			"decided_at":         now,
			"decided_by_user_id": req.DecidedByUserID,
			"decision_note":      req.DecisionNote,
		}

		db.Model(&approval).Updates(updates)

		// Handle hire_agent rejection
		if approval.Type == "hire_agent" {
			var payload map[string]interface{}
			json.Unmarshal(approval.Payload, &payload)

			agentID, _ := payload["agentId"].(string)
			if agentID != "" {
				// Terminate pending agent
				db.Model(&models.Agent{}).Where("id = ? AND status = ?", agentID, "pending_approval").Update("status", "terminated")
			}
		}

		db.Where("id = ?", id).First(&approval) // Refresh
		json.NewEncoder(w).Encode(approval)
	}
}
