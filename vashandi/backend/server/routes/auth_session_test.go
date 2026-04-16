package routes

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupSessionTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared&session_test=1"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	db.Exec("DROP TABLE IF EXISTS users")
	db.Exec(`CREATE TABLE users (id text PRIMARY KEY, email text, name text)`)
	return db
}

func TestGetSessionHandler(t *testing.T) {
	db := setupSessionTestDB(t)

	tests := []struct {
		name           string
		actor          ActorInfo
		wantStatus     int
		wantUserID     string
		wantSessionSet bool
		wantName       *string
	}{
		{
			name:       "anonymous actor returns 401",
			actor:      ActorInfo{ActorType: "anonymous"},
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "agent actor returns 401",
			actor:      ActorInfo{ActorType: "agent", AgentID: "ag1", IsAgent: true},
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:           "board actor returns session",
			actor:          ActorInfo{ActorType: "board", UserID: "user-123"},
			wantStatus:     http.StatusOK,
			wantUserID:     "user-123",
			wantSessionSet: true,
		},
		{
			name: "local-board actor returns name=Local Board",
			actor: ActorInfo{ActorType: "board", UserID: "local-board", IsInstanceAdmin: true},
			wantStatus:     http.StatusOK,
			wantUserID:     "local-board",
			wantSessionSet: true,
			wantName:       strPtr("Local Board"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/auth/get-session", nil)
			req = req.WithContext(WithActor(req.Context(), tt.actor))
			rr := httptest.NewRecorder()

			GetSessionHandler(db)(rr, req)

			if rr.Code != tt.wantStatus {
				t.Fatalf("want status %d, got %d", tt.wantStatus, rr.Code)
			}
			if rr.Code != http.StatusOK {
				return
			}

			var resp struct {
				Session *struct {
					ID     string `json:"id"`
					UserID string `json:"userId"`
				} `json:"session"`
				User *struct {
					ID   string  `json:"id"`
					Name *string `json:"name"`
				} `json:"user"`
			}
			if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
				t.Fatalf("failed to decode response: %v", err)
			}
			if tt.wantSessionSet && resp.Session == nil {
				t.Fatal("expected non-nil session in response")
			}
			if resp.Session != nil && resp.Session.UserID != tt.wantUserID {
				t.Errorf("want session.userId %q, got %q", tt.wantUserID, resp.Session.UserID)
			}
			if resp.User != nil && resp.User.ID != tt.wantUserID {
				t.Errorf("want user.id %q, got %q", tt.wantUserID, resp.User.ID)
			}
			if tt.wantName != nil {
				if resp.User == nil || resp.User.Name == nil || *resp.User.Name != *tt.wantName {
					var got *string
					if resp.User != nil {
						got = resp.User.Name
					}
					t.Errorf("want user.name %q, got %v", *tt.wantName, got)
				}
			}
		})
	}
}

func strPtr(s string) *string { return &s }
