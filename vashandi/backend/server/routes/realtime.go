package routes

import (
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"
)

// HeartbeatRunEventsSSEHandler — GET /heartbeat-runs/:runId/events
// Establishes an SSE stream for heartbeat run events. Real events will come
// from the heartbeat service once that integration is wired up.
func HeartbeatRunEventsSSEHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		runID := chi.URLParam(r, "runId")

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.Header().Set("X-Accel-Buffering", "no")

		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "streaming not supported", http.StatusInternalServerError)
			return
		}

		// Send initial connected event
		fmt.Fprintf(w, "data: {\"type\":\"connected\",\"runId\":%q}\n\n", runID)
		flusher.Flush()

		// Block until the client disconnects or the request context is done
		<-r.Context().Done()
	}
}

// SidebarBadgesSSEHandler — GET /companies/:companyId/sidebar-badges/stream
// Establishes an SSE stream for live sidebar badge updates.
func SidebarBadgesSSEHandler() http.HandlerFunc {
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

		// Send initial connected event with empty badge data
		fmt.Fprintf(w, "data: {\"type\":\"connected\",\"companyId\":%q,\"badges\":{}}\n\n", companyID)
		flusher.Flush()

		// Block until the client disconnects or the request context is done
		<-r.Context().Done()
	}
}
