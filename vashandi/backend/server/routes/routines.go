package routes

import (
	"encoding/json"
	"io"
	"net/http"
	"os"

	"github.com/chifamba/vashandi/vashandi/backend/server/services"
	"github.com/go-chi/chi/v5"
	"gorm.io/gorm"
)

// NewRoutineService creates a RoutineService with all dependencies.
// Note: HeartbeatService is passed as nil because the routes layer doesn't need
// agent wakeup functionality. The RoutineService handles nil HeartbeatService gracefully.
func NewRoutineService(db *gorm.DB) *services.RoutineService {
	activity := services.NewActivityService(db)
	issuesSvc := services.NewIssueService(db, activity)
	secrets := services.NewSecretService(db, activity)
	return services.NewRoutineService(db, issuesSvc, activity, nil, secrets)
}

func ListRoutinesHandler(db *gorm.DB) http.HandlerFunc {
	svc := NewRoutineService(db)
	return func(w http.ResponseWriter, r *http.Request) {
		companyID := chi.URLParam(r, "companyId")
		routines, err := svc.List(r.Context(), companyID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(routines)
	}
}

func GetRoutineHandler(db *gorm.DB) http.HandlerFunc {
	svc := NewRoutineService(db)
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		detail, err := svc.GetDetail(r.Context(), id)
		if err != nil || detail == nil {
			http.Error(w, "Not found", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(detail)
	}
}

func CreateRoutineHandler(db *gorm.DB) http.HandlerFunc {
	svc := NewRoutineService(db)
	return func(w http.ResponseWriter, r *http.Request) {
		companyID := chi.URLParam(r, "companyId")
		actor := getActor(r)
		var input services.CreateRoutineInput
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		routine, err := svc.Create(r.Context(), companyID, input, actor)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(routine)
	}
}

func UpdateRoutineHandler(db *gorm.DB) http.HandlerFunc {
	svc := NewRoutineService(db)
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		actor := getActor(r)
		var input services.UpdateRoutineInput
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		routine, err := svc.Update(r.Context(), id, input, actor)
		if err != nil || routine == nil {
			http.Error(w, "Not found", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(routine)
	}
}

func ListRoutineRunsHandler(db *gorm.DB) http.HandlerFunc {
	svc := NewRoutineService(db)
	return func(w http.ResponseWriter, r *http.Request) {
		routineID := chi.URLParam(r, "id")
		runs, err := svc.ListRuns(r.Context(), routineID, 50)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(runs)
	}
}

func DeleteRoutineHandler(db *gorm.DB) http.HandlerFunc {
	svc := NewRoutineService(db)
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		if err := svc.Delete(r.Context(), id); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func CreateRoutineTriggerHandler(db *gorm.DB) http.HandlerFunc {
	svc := NewRoutineService(db)
	return func(w http.ResponseWriter, r *http.Request) {
		routineID := chi.URLParam(r, "id")
		actor := getActor(r)
		apiURL := os.Getenv("PAPERCLIP_API_URL")
		if apiURL == "" {
			apiURL = "http://localhost:3100"
		}
		var input services.CreateTriggerInput
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		trigger, secretMaterial, err := svc.CreateTrigger(r.Context(), routineID, input, actor, apiURL)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		result := map[string]interface{}{"trigger": trigger}
		if secretMaterial != nil {
			result["secretMaterial"] = secretMaterial
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(result)
	}
}

func UpdateRoutineTriggerHandler(db *gorm.DB) http.HandlerFunc {
	svc := NewRoutineService(db)
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "triggerId")
		actor := getActor(r)
		var input services.UpdateTriggerInput
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		trigger, err := svc.UpdateTrigger(r.Context(), id, input, actor)
		if err != nil || trigger == nil {
			http.Error(w, "Not found", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(trigger)
	}
}

func DeleteRoutineTriggerHandler(db *gorm.DB) http.HandlerFunc {
	svc := NewRoutineService(db)
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "triggerId")
		if err := svc.DeleteTrigger(r.Context(), id); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func RotateTriggerSecretHandler(db *gorm.DB) http.HandlerFunc {
	svc := NewRoutineService(db)
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "triggerId")
		actor := getActor(r)
		apiURL := os.Getenv("PAPERCLIP_API_URL")
		if apiURL == "" {
			apiURL = "http://localhost:3100"
		}
		trigger, secretMaterial, err := svc.RotateTriggerSecret(r.Context(), id, actor, apiURL)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"trigger":        trigger,
			"secretMaterial": secretMaterial,
		})
	}
}

func FirePublicRoutineTriggerHandler(db *gorm.DB) http.HandlerFunc {
	svc := NewRoutineService(db)
	return func(w http.ResponseWriter, r *http.Request) {
		publicID := chi.URLParam(r, "publicId")
		
		// Read raw body for signature verification
		rawBody, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "Failed to read body", http.StatusBadRequest)
			return
		}
		
		var payload map[string]interface{}
		if len(rawBody) > 0 {
			json.Unmarshal(rawBody, &payload)
		}

		idempotencyKey := r.Header.Get("Idempotency-Key")
		var idempotencyKeyPtr *string
		if idempotencyKey != "" {
			idempotencyKeyPtr = &idempotencyKey
		}

		input := services.FirePublicTriggerInput{
			AuthorizationHeader: r.Header.Get("Authorization"),
			SignatureHeader:     r.Header.Get("X-Paperclip-Signature"),
			HubSignatureHeader:  r.Header.Get("X-Hub-Signature-256"),
			TimestampHeader:     r.Header.Get("X-Paperclip-Timestamp"),
			IdempotencyKey:      idempotencyKeyPtr,
			RawBody:             rawBody,
			Payload:             payload,
		}

		run, err := svc.FirePublicTrigger(r.Context(), publicID, input)
		if err != nil {
			if err.Error() == "routine trigger not found" || err.Error() == "routine not found" {
				http.Error(w, err.Error(), http.StatusNotFound)
				return
			}
			if err.Error() == "unauthorized" {
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}
			if err.Error() == "routine trigger is not active" {
				http.Error(w, err.Error(), http.StatusConflict)
				return
			}
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(run)
	}
}

func RunRoutineNowHandler(db *gorm.DB) http.HandlerFunc {
	svc := NewRoutineService(db)
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		var input services.RunRoutineInput
		if r.Body != nil {
			json.NewDecoder(r.Body).Decode(&input)
		}
		if input.Source == "" {
			input.Source = services.SourceManual
		}
		run, err := svc.RunRoutine(r.Context(), id, input)
		if err != nil {
			if err.Error() == "routine not found" {
				http.Error(w, err.Error(), http.StatusNotFound)
				return
			}
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(run)
	}
}

// Helper to extract actor from request
func getActor(r *http.Request) services.Actor {
	actor := services.Actor{}
	// Try to get actor info from context (if middleware sets it)
	info := GetActorInfo(r)
	if info.AgentID != "" {
		agentID := info.AgentID
		actor.AgentID = &agentID
	}
	if info.UserID != "" {
		userID := info.UserID
		actor.UserID = &userID
	}
	return actor
}
