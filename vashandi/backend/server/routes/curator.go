package routes

import (

	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"gorm.io/gorm"

	"github.com/chifamba/paperclip/backend/server/services"
)

// ListProposalsHandler returns a list of pending deduplication proposals
// For now, we will query the OpenBrain Adapter to get proposals. We need to add this endpoint.
// But as we are sharing the db connection in tests, let's query it via adapter for decoupling.
func ListProposalsHandler(db *gorm.DB) http.HandlerFunc {
	adapter := services.NewOpenBrainAdapter()
	return func(w http.ResponseWriter, r *http.Request) {
		companyID := chi.URLParam(r, "companyId")

		proposals, err := adapter.ListProposals(r.Context(), companyID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(proposals)
	}
}

// ApproveProposalHandler approves or rejects a proposal
func ApproveProposalHandler(db *gorm.DB) http.HandlerFunc {
	adapter := services.NewOpenBrainAdapter()
	return func(w http.ResponseWriter, r *http.Request) {
		companyID := chi.URLParam(r, "companyId")
		proposalID := chi.URLParam(r, "proposalId")

		var req struct {
			Action string `json:"action"` // approve or reject
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		err := adapter.ResolveProposal(r.Context(), companyID, proposalID, req.Action)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"status": req.Action})
	}
}
