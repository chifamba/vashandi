package routes

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupAccessTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dbName := fmt.Sprintf("file:access_%s?mode=memory&cache=shared", url.QueryEscape(t.Name()))
	db, err := gorm.Open(sqlite.Open(dbName), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	db.Exec("DROP TABLE IF EXISTS companies")
	db.Exec("DROP TABLE IF EXISTS invites")
	db.Exec("DROP TABLE IF EXISTS cli_auth_challenges")
	db.Exec("DROP TABLE IF EXISTS join_requests")
	db.Exec("DROP TABLE IF EXISTS company_memberships")
	db.Exec("DROP TABLE IF EXISTS instance_user_roles")
	db.Exec(`CREATE TABLE invites (
		id text PRIMARY KEY,
		company_id text,
		invite_type text NOT NULL DEFAULT 'company_join',
		token_hash text NOT NULL UNIQUE,
		allowed_join_types text NOT NULL DEFAULT 'both',
		defaults_payload text,
		expires_at datetime NOT NULL,
		invited_by_user_id text,
		revoked_at datetime,
		accepted_at datetime,
		created_at datetime DEFAULT CURRENT_TIMESTAMP,
		updated_at datetime DEFAULT CURRENT_TIMESTAMP
	)`)
	db.Exec(`CREATE TABLE cli_auth_challenges (
		id text PRIMARY KEY,
		secret_hash text NOT NULL,
		command text NOT NULL DEFAULT 'auth',
		challenge_token text,
		client_name text,
		requested_access text NOT NULL DEFAULT 'board',
		requested_company_id text,
		pending_key_hash text NOT NULL DEFAULT '',
		pending_key_name text NOT NULL DEFAULT '',
		approved_by_user_id text,
		board_api_key_id text,
		approved_at datetime,
		cancelled_at datetime,
		expires_at datetime NOT NULL DEFAULT '2099-01-01',
		created_at datetime DEFAULT CURRENT_TIMESTAMP,
		updated_at datetime DEFAULT CURRENT_TIMESTAMP
	)`)
	db.Exec(`CREATE TABLE join_requests (
		id text PRIMARY KEY,
		invite_id text NOT NULL,
		company_id text NOT NULL,
		request_type text NOT NULL DEFAULT 'agent',
		status text NOT NULL DEFAULT 'pending_approval',
		request_ip text NOT NULL DEFAULT '127.0.0.1',
		requesting_user_id text,
		request_email_snapshot text,
		agent_name text,
		adapter_type text,
		capabilities text,
		agent_defaults_payload text DEFAULT '{}',
		claim_secret_hash text,
		claim_secret_expires_at datetime,
		claim_secret_consumed_at datetime,
		created_agent_id text,
		approved_by_user_id text,
		approved_at datetime,
		rejected_by_user_id text,
		rejected_at datetime,
		created_at datetime DEFAULT CURRENT_TIMESTAMP,
		updated_at datetime DEFAULT CURRENT_TIMESTAMP
	)`)
	db.Exec(`CREATE TABLE company_memberships (
		id text PRIMARY KEY,
		company_id text NOT NULL,
		principal_type text NOT NULL,
		principal_id text NOT NULL,
		status text NOT NULL DEFAULT 'active',
		membership_role text,
		created_at datetime DEFAULT CURRENT_TIMESTAMP,
		updated_at datetime DEFAULT CURRENT_TIMESTAMP
	)`)
	db.Exec(`CREATE TABLE instance_user_roles (
		id text PRIMARY KEY,
		user_id text NOT NULL,
		role text NOT NULL DEFAULT 'member',
		created_at datetime DEFAULT CURRENT_TIMESTAMP,
		updated_at datetime DEFAULT CURRENT_TIMESTAMP
	)`)
	db.Exec(`CREATE TABLE companies (
		id text PRIMARY KEY,
		name text NOT NULL,
		created_at datetime DEFAULT CURRENT_TIMESTAMP,
		updated_at datetime DEFAULT CURRENT_TIMESTAMP
	)`)
	return db
}

// --- InviteAcceptHandler tests ---

func TestInviteAcceptHandler_Success(t *testing.T) {
	db := setupAccessTestDB(t)
	tokenStr := "test-token-123"
	tokenHash := fmt.Sprintf("%x", sha256.Sum256([]byte(tokenStr)))
	futureExpiry := time.Now().Add(24 * time.Hour).Format("2006-01-02 15:04:05")

	db.Exec("INSERT INTO invites (id, token_hash, expires_at) VALUES ('inv-1', ?, ?)", tokenHash, futureExpiry)

	body, _ := json.Marshal(map[string]string{"token": tokenStr})
	req := httptest.NewRequest(http.MethodPost, "/invites/accept", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	InviteAcceptHandler(db)(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", w.Code, w.Body.String())
	}
}

func TestInviteAcceptHandler_NotFound(t *testing.T) {
	db := setupAccessTestDB(t)

	body, _ := json.Marshal(map[string]string{"token": "nonexistent"})
	req := httptest.NewRequest(http.MethodPost, "/invites/accept", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	InviteAcceptHandler(db)(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestInviteAcceptHandler_Expired(t *testing.T) {
	db := setupAccessTestDB(t)
	tokenStr := "expired-token"
	tokenHash := fmt.Sprintf("%x", sha256.Sum256([]byte(tokenStr)))
	pastExpiry := time.Now().Add(-24 * time.Hour).Format("2006-01-02 15:04:05")

	db.Exec("INSERT INTO invites (id, token_hash, expires_at) VALUES ('inv-exp', ?, ?)", tokenHash, pastExpiry)

	body, _ := json.Marshal(map[string]string{"token": tokenStr})
	req := httptest.NewRequest(http.MethodPost, "/invites/accept", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	InviteAcceptHandler(db)(w, req)

	if w.Code != http.StatusGone {
		t.Errorf("expected 410 (Gone), got %d", w.Code)
	}
}

func TestInviteAcceptHandler_AlreadyAccepted(t *testing.T) {
	db := setupAccessTestDB(t)
	tokenStr := "accepted-token"
	tokenHash := fmt.Sprintf("%x", sha256.Sum256([]byte(tokenStr)))
	futureExpiry := time.Now().Add(24 * time.Hour).Format("2006-01-02 15:04:05")
	acceptedAt := time.Now().Add(-1 * time.Hour).Format("2006-01-02 15:04:05")

	db.Exec("INSERT INTO invites (id, token_hash, expires_at, accepted_at) VALUES ('inv-acc', ?, ?, ?)", tokenHash, futureExpiry, acceptedAt)

	body, _ := json.Marshal(map[string]string{"token": tokenStr})
	req := httptest.NewRequest(http.MethodPost, "/invites/accept", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	InviteAcceptHandler(db)(w, req)

	if w.Code != http.StatusConflict {
		t.Errorf("expected 409 (Conflict), got %d", w.Code)
	}
}

// --- InviteAcceptByPathHandler tests ---

func TestInviteAcceptByPathHandler_Success(t *testing.T) {
	db := setupAccessTestDB(t)
	tokenStr := "path-token-123"
	tokenHash := fmt.Sprintf("%x", sha256.Sum256([]byte(tokenStr)))
	futureExpiry := time.Now().Add(24 * time.Hour).Format("2006-01-02 15:04:05")

	db.Exec("INSERT INTO invites (id, token_hash, expires_at) VALUES ('inv-path-1', ?, ?)", tokenHash, futureExpiry)

	w := httptest.NewRecorder()
	r := chi.NewRouter()
	r.Post("/invites/{token}/accept", InviteAcceptByPathHandler(db))
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/invites/"+tokenStr+"/accept", nil))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", w.Code, w.Body.String())
	}
}

func TestInviteAcceptByPathHandler_NotFound(t *testing.T) {
	db := setupAccessTestDB(t)

	w := httptest.NewRecorder()
	r := chi.NewRouter()
	r.Post("/invites/{token}/accept", InviteAcceptByPathHandler(db))
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/invites/nonexistent/accept", nil))

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestInviteAcceptByPathHandler_Expired(t *testing.T) {
	db := setupAccessTestDB(t)
	tokenStr := "path-expired-token"
	tokenHash := fmt.Sprintf("%x", sha256.Sum256([]byte(tokenStr)))
	pastExpiry := time.Now().Add(-24 * time.Hour).Format("2006-01-02 15:04:05")

	db.Exec("INSERT INTO invites (id, token_hash, expires_at) VALUES ('inv-path-exp', ?, ?)", tokenHash, pastExpiry)

	w := httptest.NewRecorder()
	r := chi.NewRouter()
	r.Post("/invites/{token}/accept", InviteAcceptByPathHandler(db))
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/invites/"+tokenStr+"/accept", nil))

	if w.Code != http.StatusGone {
		t.Errorf("expected 410 (Gone), got %d", w.Code)
	}
}

func TestInviteAcceptByPathHandler_AlreadyAccepted(t *testing.T) {
	db := setupAccessTestDB(t)
	tokenStr := "path-accepted-token"
	tokenHash := fmt.Sprintf("%x", sha256.Sum256([]byte(tokenStr)))
	futureExpiry := time.Now().Add(24 * time.Hour).Format("2006-01-02 15:04:05")
	acceptedAt := time.Now().Add(-1 * time.Hour).Format("2006-01-02 15:04:05")

	db.Exec("INSERT INTO invites (id, token_hash, expires_at, accepted_at) VALUES ('inv-path-acc', ?, ?, ?)", tokenHash, futureExpiry, acceptedAt)

	w := httptest.NewRecorder()
	r := chi.NewRouter()
	r.Post("/invites/{token}/accept", InviteAcceptByPathHandler(db))
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/invites/"+tokenStr+"/accept", nil))

	if w.Code != http.StatusConflict {
		t.Errorf("expected 409 (Conflict), got %d", w.Code)
	}
}

// --- CLIAuthChallengeHandler tests ---

func TestCLIAuthChallengeHandler_Create(t *testing.T) {
	db := setupAccessTestDB(t)

	challengeBody, _ := json.Marshal(map[string]string{
		"id":               "ch-1",
		"secret_hash":      "abc123",
		"command":          "auth",
		"pending_key_hash": "keyhash",
		"pending_key_name": "my-laptop",
	})
	req := httptest.NewRequest(http.MethodPost, "/cli-auth/challenges", bytes.NewBuffer(challengeBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	CLIAuthChallengeHandler(db)(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d; body: %s", w.Code, w.Body.String())
	}
}

// --- ResolveCLIAuthHandler tests ---

func TestResolveCLIAuthHandler_Found(t *testing.T) {
	db := setupAccessTestDB(t)
	db.Exec("INSERT INTO cli_auth_challenges (id, secret_hash, command, challenge_token, pending_key_hash, pending_key_name) VALUES ('ch-1', 'hash1', 'auth', 'tok-abc', 'kh', 'name')")

	router := chi.NewRouter()
	router.Get("/cli-auth/resolve/{token}", ResolveCLIAuthHandler(db))

	req := httptest.NewRequest(http.MethodGet, "/cli-auth/resolve/tok-abc", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestResolveCLIAuthHandler_NotFound(t *testing.T) {
	db := setupAccessTestDB(t)

	router := chi.NewRouter()
	router.Get("/cli-auth/resolve/{token}", ResolveCLIAuthHandler(db))

	req := httptest.NewRequest(http.MethodGet, "/cli-auth/resolve/missing", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

// --- ListJoinRequestsHandler tests ---

func TestListJoinRequestsHandler_CompanyScoping(t *testing.T) {
	db := setupAccessTestDB(t)
	db.Exec("INSERT INTO join_requests (id, invite_id, company_id, request_type) VALUES ('jr-1', 'inv-1', 'comp-a', 'agent')")
	db.Exec("INSERT INTO join_requests (id, invite_id, company_id, request_type) VALUES ('jr-2', 'inv-2', 'comp-b', 'agent')")
	db.Exec("INSERT INTO join_requests (id, invite_id, company_id, request_type) VALUES ('jr-3', 'inv-3', 'comp-a', 'user')")

	router := chi.NewRouter()
	router.Get("/companies/{companyId}/join-requests", ListJoinRequestsHandler(db))

	req := httptest.NewRequest(http.MethodGet, "/companies/comp-a/join-requests", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var reqs []map[string]interface{}
	json.NewDecoder(w.Body).Decode(&reqs)
	if len(reqs) != 2 {
		t.Errorf("expected 2 join requests for comp-a, got %d", len(reqs))
	}
}

func TestListJoinRequestsHandler_StatusFilter(t *testing.T) {
	db := setupAccessTestDB(t)
	db.Exec("INSERT INTO join_requests (id, invite_id, company_id, request_type, status) VALUES ('jr-1', 'inv-1', 'comp-a', 'agent', 'pending_approval')")
	db.Exec("INSERT INTO join_requests (id, invite_id, company_id, request_type, status) VALUES ('jr-2', 'inv-2', 'comp-a', 'agent', 'approved')")

	router := chi.NewRouter()
	router.Get("/companies/{companyId}/join-requests", ListJoinRequestsHandler(db))

	req := httptest.NewRequest(http.MethodGet, "/companies/comp-a/join-requests?status=pending_approval", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	var reqs []map[string]interface{}
	json.NewDecoder(w.Body).Decode(&reqs)
	if len(reqs) != 1 {
		t.Errorf("expected 1 pending join request, got %d", len(reqs))
	}
}

// --- ClaimJoinRequestHandler tests ---

func TestClaimJoinRequestHandler_Success(t *testing.T) {
	db := setupAccessTestDB(t)
	db.Exec("INSERT INTO join_requests (id, invite_id, company_id, request_type, status) VALUES ('jr-claim', 'inv-1', 'comp-a', 'agent', 'pending_approval')")

	router := chi.NewRouter()
	router.Post("/join-requests/{id}/claim", ClaimJoinRequestHandler(db))

	req := httptest.NewRequest(http.MethodPost, "/join-requests/jr-claim/claim", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", w.Code, w.Body.String())
	}

	var resp map[string]string
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["status"] != "approved" {
		t.Errorf("expected status 'approved', got %q", resp["status"])
	}
}

func TestClaimJoinRequestHandler_NotFound(t *testing.T) {
	db := setupAccessTestDB(t)

	router := chi.NewRouter()
	router.Post("/join-requests/{id}/claim", ClaimJoinRequestHandler(db))

	req := httptest.NewRequest(http.MethodPost, "/join-requests/missing/claim", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

// --- BoardClaimTokenHandler tests ---

func TestBoardClaimTokenHandler_Pending(t *testing.T) {
	db := setupAccessTestDB(t)
	token := "board-claim-token"
	hash := fmt.Sprintf("%x", sha256.Sum256([]byte(token)))
	futureExpiry := time.Now().Add(24 * time.Hour).Format("2006-01-02 15:04:05")

	db.Exec("INSERT INTO cli_auth_challenges (id, secret_hash, command, pending_key_hash, pending_key_name, expires_at) VALUES ('bc-1', ?, 'auth', 'kh', 'kn', ?)", hash, futureExpiry)

	router := chi.NewRouter()
	router.Get("/board-claim/{token}", BoardClaimTokenHandler(db))

	req := httptest.NewRequest(http.MethodGet, "/board-claim/"+token, nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp map[string]interface{}
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["status"] != "pending" {
		t.Errorf("expected status 'pending', got %v", resp["status"])
	}
}

func TestBoardClaimTokenHandler_NotFound(t *testing.T) {
	db := setupAccessTestDB(t)

	router := chi.NewRouter()
	router.Get("/board-claim/{token}", BoardClaimTokenHandler(db))

	req := httptest.NewRequest(http.MethodGet, "/board-claim/nonexistent", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

// --- ListSkillsHandler tests ---

func TestListSkillsHandler(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/skills", nil)
	w := httptest.NewRecorder()

	ListSkillsHandler()(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	ct := w.Header().Get("Content-Type")
	if ct != "text/plain" {
		t.Errorf("expected Content-Type text/plain, got %q", ct)
	}

	body := w.Body.String()
	if body == "" {
		t.Error("expected non-empty body")
	}
}

// --- ListCompanyMembersHandler tests ---

func TestListCompanyMembersHandler_Scoping(t *testing.T) {
	db := setupAccessTestDB(t)
	db.Exec("INSERT INTO company_memberships (id, company_id, principal_type, principal_id, status) VALUES ('m1', 'comp-a', 'user', 'u1', 'active')")
	db.Exec("INSERT INTO company_memberships (id, company_id, principal_type, principal_id, status) VALUES ('m2', 'comp-b', 'user', 'u2', 'active')")
	db.Exec("INSERT INTO company_memberships (id, company_id, principal_type, principal_id, status) VALUES ('m3', 'comp-a', 'user', 'u3', 'removed')")

	router := chi.NewRouter()
	router.Get("/companies/{companyId}/members", ListCompanyMembersHandler(db))

	req := httptest.NewRequest(http.MethodGet, "/companies/comp-a/members", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var members []map[string]interface{}
	json.NewDecoder(w.Body).Decode(&members)
	// Only active members for comp-a
	if len(members) != 1 {
		t.Errorf("expected 1 active member for comp-a, got %d", len(members))
	}
}

// --- GetCLIAuthMeHandler tests ---

func TestGetCLIAuthMeHandler(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/cli-auth/me", nil)
	req.Header.Set("X-User-ID", "user-123")
	w := httptest.NewRecorder()

	GetCLIAuthMeHandler(nil)(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp map[string]interface{}
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["actorId"] != "user-123" {
		t.Errorf("expected actorId 'user-123', got %v", resp["actorId"])
	}
}

// --- RevokeCLIAuthCurrentHandler tests ---

func TestRevokeCLIAuthCurrentHandler(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/cli-auth/revoke-current", nil)
	w := httptest.NewRecorder()

	RevokeCLIAuthCurrentHandler(nil)(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp map[string]string
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["status"] != "revoked" {
		t.Errorf("expected status 'revoked', got %q", resp["status"])
	}
}

func TestGetInviteOnboardingTextHandler_Found(t *testing.T) {
	db := setupAccessTestDB(t)
	db.Exec("INSERT INTO companies (id, name) VALUES ('comp-1', 'Acme')")

	token := "invite-token"
	tokenHash := fmt.Sprintf("%x", sha256.Sum256([]byte(token)))
	futureExpiry := time.Now().Add(24 * time.Hour).Format("2006-01-02 15:04:05")
	db.Exec("INSERT INTO invites (id, company_id, token_hash, allowed_join_types, expires_at) VALUES ('inv-1', 'comp-1', ?, 'agent', ?)", tokenHash, futureExpiry)

	router := chi.NewRouter()
	router.Get("/invites/{token}/onboarding.txt", GetInviteOnboardingTextHandler(db))

	req := httptest.NewRequest(http.MethodGet, "/invites/"+token+"/onboarding.txt", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", w.Code, w.Body.String())
	}
	if ct := w.Header().Get("Content-Type"); ct != "text/plain; charset=utf-8" {
		t.Fatalf("expected text/plain content type, got %q", ct)
	}
	body := w.Body.String()
	if !strings.Contains(body, "Acme") {
		t.Fatalf("expected onboarding text to include company name, got %q", body)
	}
	if !strings.Contains(body, token) {
		t.Fatalf("expected onboarding text to include invite token, got %q", body)
	}
}

func TestApproveCLIAuthChallengeHandler_Success(t *testing.T) {
	db := setupAccessTestDB(t)
	token := "pcp_board_test_token"
	futureExpiry := time.Now().Add(24 * time.Hour).Format("2006-01-02 15:04:05")
	db.Exec(`INSERT INTO cli_auth_challenges (id, secret_hash, pending_key_hash, pending_key_name, board_api_key_id, expires_at)
		VALUES ('chal-1', 'secret-hash', ?, 'CLI board key', 'board-key-1', ?)`, hashToken(token), futureExpiry)

	router := chi.NewRouter()
	router.Post("/cli-auth/challenges/{id}/approve", ApproveCLIAuthChallengeHandler(db))

	req := httptest.NewRequest(http.MethodPost, "/cli-auth/challenges/chal-1/approve", bytes.NewBufferString(`{"token":"`+token+`"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if approved, ok := resp["approved"].(bool); !ok || !approved {
		t.Fatalf("expected approved=true, got %+v", resp)
	}

	var challenge struct {
		ApprovedAt *time.Time
	}
	if err := db.Table("cli_auth_challenges").Select("approved_at").Where("id = ?", "chal-1").Scan(&challenge).Error; err != nil {
		t.Fatalf("load challenge: %v", err)
	}
	if challenge.ApprovedAt == nil {
		t.Fatal("expected approved_at to be set")
	}
}

func TestApproveCLIAuthChallengeHandler_InvalidToken(t *testing.T) {
	db := setupAccessTestDB(t)
	futureExpiry := time.Now().Add(24 * time.Hour).Format("2006-01-02 15:04:05")
	db.Exec(`INSERT INTO cli_auth_challenges (id, secret_hash, pending_key_hash, pending_key_name, expires_at)
		VALUES ('chal-2', 'secret-hash', ?, 'CLI board key', ?)`, hashToken("expected-token"), futureExpiry)

	router := chi.NewRouter()
	router.Post("/cli-auth/challenges/{id}/approve", ApproveCLIAuthChallengeHandler(db))

	req := httptest.NewRequest(http.MethodPost, "/cli-auth/challenges/chal-2/approve", bytes.NewBufferString(`{"token":"wrong-token"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d; body: %s", w.Code, w.Body.String())
	}
}
