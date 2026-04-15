package routes

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"

	"github.com/chifamba/vashandi/vashandi/backend/db/models"
	"github.com/go-chi/chi/v5"
	"gorm.io/gorm"
)

// ListHeartbeatRunEventsHandler — GET /heartbeat-runs/:runId/events
// Returns stored HeartbeatRunEvent records for a run. Supports cursor-based
// pagination via ?afterSeq= (exclusive lower bound on the seq column) and
// ?limit= (default 200, max 1000). This mirrors the TypeScript REST route.
func ListHeartbeatRunEventsHandler(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		runID := chi.URLParam(r, "runId")

		var run models.HeartbeatRun
		if err := db.WithContext(r.Context()).First(&run, "id = ?", runID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusNotFound)
				fmt.Fprintf(w, `{"error":"heartbeat run not found"}`)
				return
			}
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		afterSeq := 0
		if s := r.URL.Query().Get("afterSeq"); s != "" {
			if v, err := strconv.Atoi(s); err == nil {
				afterSeq = v
			}
		}
		limit := 200
		if l := r.URL.Query().Get("limit"); l != "" {
			if v, err := strconv.Atoi(l); err == nil && v > 0 && v <= 1000 {
				limit = v
			}
		}

		var events []models.HeartbeatRunEvent
		if err := db.WithContext(r.Context()).
			Where("run_id = ? AND seq > ?", runID, afterSeq).
			Order("seq ASC").
			Limit(limit).
			Find(&events).Error; err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		if events == nil {
			events = []models.HeartbeatRunEvent{}
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(events) //nolint:errcheck
	}
}

// SidebarBadgesSSEHandler — GET /companies/:companyId/sidebar-badges/stream
// Establishes an SSE stream for live sidebar badge updates. Immediately sends
// the current badge counts, then re-sends them whenever the hub publishes an
// event for the company (e.g. a heartbeat run status change or new approval).
//
// subscribe is hub.Subscribe — a function that registers the caller as a
// receiver of raw event payloads for a given companyID. Using a function
// parameter avoids an import cycle between the routes and realtime packages.
func SidebarBadgesSSEHandler(db *gorm.DB, subscribe func(companyID string) (<-chan []byte, func())) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		companyID := chi.URLParam(r, "companyId")

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.Header().Set("X-Accel-Buffering", "no")

		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "streaming not supported", http.StatusInternalServerError)
			return
		}

		push := func() {
			badges := computeBadgeCounts(r.Context(), db, companyID)
			data, _ := json.Marshal(badges)
			fmt.Fprintf(w, "data: %s\n\n", data) //nolint:errcheck
			flusher.Flush()
		}

		// Subscribe to hub events for this company.
		events, cancel := subscribe(companyID)
		defer cancel()

		// Send the current badge counts immediately.
		push()

		for {
			select {
			case <-r.Context().Done():
				return
			case <-events:
				// Any hub event for this company (run status change, approval
				// created, etc.) is a signal to recompute and push badge counts.
				push()
			}
		}
	}
}

// computeBadgeCounts returns the current sidebar badge counts for companyID.
// Extracted so that both SidebarBadgesHandler and SidebarBadgesSSEHandler
// share the same query logic.
func computeBadgeCounts(ctx context.Context, db *gorm.DB, companyID string) map[string]int64 {
	return computeBadgeCountsWithLookback(ctx, db, companyID, failedRunsLookbackHours)
}
