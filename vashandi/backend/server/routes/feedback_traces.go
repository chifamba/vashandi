package routes

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/chifamba/vashandi/vashandi/backend/server/services"
	"github.com/go-chi/chi/v5"
	"gorm.io/gorm"
)

func ListIssueFeedbackTracesHandler(svc *services.FeedbackService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := AssertBoard(r); err != nil {
			http.Error(w, "Only board users can view feedback traces", http.StatusForbidden)
			return
		}
		traces, err := svc.ListTraces(r.Context(), services.FeedbackTraceFilters{
			IssueID:        chi.URLParam(r, "id"),
			TargetType:     r.URL.Query().Get("targetType"),
			Vote:           r.URL.Query().Get("vote"),
			Status:         r.URL.Query().Get("status"),
			From:           parseFeedbackTimeQuery(r, "from"),
			To:             parseFeedbackTimeQuery(r, "to"),
			SharedOnly:     r.URL.Query().Get("sharedOnly") == "true",
			IncludePayload: r.URL.Query().Get("includePayload") == "true",
		})
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				http.Error(w, "Issue not found", http.StatusNotFound)
				return
			}
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(traces)
	}
}

func GetFeedbackTraceHandler(svc *services.FeedbackService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := AssertBoard(r); err != nil {
			http.Error(w, "Only board users can view feedback traces", http.StatusForbidden)
			return
		}
		trace, err := svc.GetTraceByID(r.Context(), chi.URLParam(r, "traceId"), r.URL.Query().Get("includePayload") != "false")
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				http.Error(w, "Feedback trace not found", http.StatusNotFound)
				return
			}
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if err := AssertCompanyAccess(r, trace.CompanyID); err != nil {
			http.Error(w, "Feedback trace not found", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(trace)
	}
}

func GetFeedbackTraceBundleHandler(svc *services.FeedbackService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := AssertBoard(r); err != nil {
			http.Error(w, "Only board users can view feedback trace bundles", http.StatusForbidden)
			return
		}
		bundle, err := svc.GetTraceBundle(r.Context(), chi.URLParam(r, "traceId"))
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				http.Error(w, "Feedback trace not found", http.StatusNotFound)
				return
			}
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if err := AssertCompanyAccess(r, bundle.CompanyID); err != nil {
			http.Error(w, "Feedback trace not found", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(bundle)
	}
}

func parseFeedbackTimeQuery(r *http.Request, key string) *time.Time {
	raw := r.URL.Query().Get(key)
	if raw == "" {
		return nil
	}
	for _, layout := range []string{time.RFC3339, "2006-01-02"} {
		if parsed, err := time.Parse(layout, raw); err == nil {
			return &parsed
		}
	}
	return nil
}
