package routes

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/chifamba/vashandi/vashandi/backend/db/models"
	"github.com/go-chi/chi/v5"
	"gorm.io/gorm"
)

func InviteAcceptHandler(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Token string `json:"token"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		acceptInviteByToken(db, w, r, body.Token)
	}
}

// InviteAcceptByPathHandler handles POST /invites/:token/accept
// It extracts the token from the URL path and calls the shared accept logic.
func InviteAcceptByPathHandler(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := chi.URLParam(r, "token")
		if token == "" {
			http.Error(w, "Token is required", http.StatusBadRequest)
			return
		}
		acceptInviteByToken(db, w, r, token)
	}
}

// acceptInviteByToken contains the shared invite acceptance logic.
func acceptInviteByToken(db *gorm.DB, w http.ResponseWriter, r *http.Request, token string) {
	hash := fmt.Sprintf("%x", sha256.Sum256([]byte(token)))
	var invite models.Invite
	if err := db.WithContext(r.Context()).Where("token_hash = ?", hash).First(&invite).Error; err != nil {
		http.Error(w, "Invite not found", http.StatusNotFound)
		return
	}
	if invite.ExpiresAt.Before(time.Now()) {
		http.Error(w, "Invite expired", http.StatusGone)
		return
	}
	if invite.AcceptedAt != nil {
		http.Error(w, "Invite already accepted", http.StatusConflict)
		return
	}
	now := time.Now()
	invite.AcceptedAt = &now
	if err := db.WithContext(r.Context()).Save(&invite).Error; err != nil {
		http.Error(w, "Failed to accept invite", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(invite)
}

func CLIAuthChallengeHandler(db *gorm.DB) http.HandlerFunc {
return func(w http.ResponseWriter, r *http.Request) {
var challenge models.CLIAuthChallenge
if err := json.NewDecoder(r.Body).Decode(&challenge); err != nil {
http.Error(w, err.Error(), http.StatusBadRequest)
return
}
if err := db.WithContext(r.Context()).Create(&challenge).Error; err != nil {
http.Error(w, err.Error(), http.StatusInternalServerError)
return
}
w.Header().Set("Content-Type", "application/json")
w.WriteHeader(http.StatusCreated)
json.NewEncoder(w).Encode(challenge)
}
}

func ResolveCLIAuthHandler(db *gorm.DB) http.HandlerFunc {
return func(w http.ResponseWriter, r *http.Request) {
token := chi.URLParam(r, "token")
var challenge models.CLIAuthChallenge
if err := db.WithContext(r.Context()).Where("challenge_token = ?", token).First(&challenge).Error; err != nil {
http.Error(w, "Challenge not found", http.StatusNotFound)
return
}
w.Header().Set("Content-Type", "application/json")
json.NewEncoder(w).Encode(challenge)
}
}

func ListJoinRequestsHandler(db *gorm.DB) http.HandlerFunc {
return func(w http.ResponseWriter, r *http.Request) {
companyID := chi.URLParam(r, "companyId")
status := r.URL.Query().Get("status")
var reqs []models.JoinRequest
q := db.WithContext(r.Context()).Where("company_id = ?", companyID)
if status != "" {
q = q.Where("status = ?", status)
}
q.Order("created_at DESC").Find(&reqs)
w.Header().Set("Content-Type", "application/json")
json.NewEncoder(w).Encode(reqs)
}
}

func ClaimJoinRequestHandler(db *gorm.DB) http.HandlerFunc {
return func(w http.ResponseWriter, r *http.Request) {
id := chi.URLParam(r, "id")
var req models.JoinRequest
if err := db.WithContext(r.Context()).First(&req, "id = ?", id).Error; err != nil {
http.Error(w, "Not found", http.StatusNotFound)
return
}
now := time.Now()
req.Status = "approved"
req.ApprovedAt = &now
db.WithContext(r.Context()).Save(&req)
w.Header().Set("Content-Type", "application/json")
json.NewEncoder(w).Encode(map[string]string{"status": "approved"})
}
}

func UpdateMemberPermissionsHandler(db *gorm.DB) http.HandlerFunc {
return func(w http.ResponseWriter, r *http.Request) {
id := chi.URLParam(r, "id")
var role models.InstanceUserRole
if err := db.WithContext(r.Context()).First(&role, "id = ?", id).Error; err != nil {
http.Error(w, "Not found", http.StatusNotFound)
return
}
var body map[string]interface{}
if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
http.Error(w, "invalid request body", http.StatusBadRequest)
return
}
db.WithContext(r.Context()).Save(&role)
w.Header().Set("Content-Type", "application/json")
json.NewEncoder(w).Encode(role)
}
}

func ListSkillsHandler() http.HandlerFunc {
return func(w http.ResponseWriter, r *http.Request) {
w.Header().Set("Content-Type", "text/plain")
w.Write([]byte("bash\npython\ngit\ndocker\nkubernetes\nterraform\n"))
}
}

// BoardClaimTokenHandler — GET /board-claim/:token
// Looks up a CLI auth challenge by SHA256(token) and returns its status.
func BoardClaimTokenHandler(db *gorm.DB) http.HandlerFunc {
return func(w http.ResponseWriter, r *http.Request) {
token := chi.URLParam(r, "token")
hash := fmt.Sprintf("%x", sha256.Sum256([]byte(token)))

var challenge models.CLIAuthChallenge
if err := db.WithContext(r.Context()).Where("secret_hash = ?", hash).First(&challenge).Error; err != nil {
http.Error(w, "Not found", http.StatusNotFound)
return
}

status := "pending"
switch {
case challenge.CancelledAt != nil:
status = "cancelled"
case challenge.ApprovedAt != nil:
status = "approved"
case challenge.ExpiresAt.Before(time.Now()):
status = "expired"
}

w.Header().Set("Content-Type", "application/json")
json.NewEncoder(w).Encode(map[string]interface{}{
"status":    status,
"companyId": challenge.RequestedCompanyID,
})
}
}

// ClaimBoardTokenHandler — POST /board-claim/:token/claim
// Marks a CLI auth challenge as approved.
func ClaimBoardTokenHandler(db *gorm.DB) http.HandlerFunc {
return func(w http.ResponseWriter, r *http.Request) {
token := chi.URLParam(r, "token")
hash := fmt.Sprintf("%x", sha256.Sum256([]byte(token)))

var challenge models.CLIAuthChallenge
if err := db.WithContext(r.Context()).Where("secret_hash = ?", hash).First(&challenge).Error; err != nil {
http.Error(w, "Not found", http.StatusNotFound)
return
}

now := time.Now()
challenge.ApprovedAt = &now
db.WithContext(r.Context()).Save(&challenge)

w.Header().Set("Content-Type", "application/json")
json.NewEncoder(w).Encode(map[string]string{"status": "approved"})
}
}

// GetUserCompanyAccessHandler — GET /admin/users/:userId/company-access
// Returns all company memberships for a user.
func GetUserCompanyAccessHandler(db *gorm.DB) http.HandlerFunc {
return func(w http.ResponseWriter, r *http.Request) {
userID := chi.URLParam(r, "userId")

var memberships []models.CompanyMembership
db.WithContext(r.Context()).
Where("principal_id = ? AND principal_type = ?", userID, "user").
Find(&memberships)

w.Header().Set("Content-Type", "application/json")
json.NewEncoder(w).Encode(memberships)
}
}

// UpdateUserCompanyAccessHandler — PUT /admin/users/:userId/company-access
// Upserts a company membership for a user.
func UpdateUserCompanyAccessHandler(db *gorm.DB) http.HandlerFunc {
return func(w http.ResponseWriter, r *http.Request) {
userID := chi.URLParam(r, "userId")

var body struct {
CompanyID string  `json:"companyId"`
Role      *string `json:"role"`
}
if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
http.Error(w, err.Error(), http.StatusBadRequest)
return
}
if body.CompanyID == "" {
http.Error(w, "companyId is required", http.StatusBadRequest)
return
}

var membership models.CompanyMembership
db.WithContext(r.Context()).
Where("principal_id = ? AND principal_type = ? AND company_id = ?", userID, "user", body.CompanyID).
FirstOrInit(&membership)

membership.CompanyID = body.CompanyID
membership.PrincipalID = userID
membership.PrincipalType = "user"
membership.MembershipRole = body.Role
if membership.Status == "" {
membership.Status = "active"
}

if err := db.WithContext(r.Context()).Save(&membership).Error; err != nil {
http.Error(w, err.Error(), http.StatusInternalServerError)
return
}

w.Header().Set("Content-Type", "application/json")
json.NewEncoder(w).Encode(membership)
}
}

// ListCompanyMembersHandler handles GET /companies/:companyId/members
func ListCompanyMembersHandler(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		companyID := chi.URLParam(r, "companyId")
		var memberships []models.CompanyMembership
		db.WithContext(r.Context()).
			Where("company_id = ? AND status = 'active'", companyID).
			Find(&memberships)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(memberships)
	}
}

// UpdateCompanyMemberHandler handles PATCH /companies/:companyId/members/:userId
func UpdateCompanyMemberHandler(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		companyID := chi.URLParam(r, "companyId")
		userID := chi.URLParam(r, "userId")
		var body struct {
			Role *string `json:"role"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		var membership models.CompanyMembership
		if err := db.WithContext(r.Context()).
			Where("company_id = ? AND principal_id = ? AND principal_type = 'user'", companyID, userID).
			First(&membership).Error; err != nil {
			http.Error(w, "Member not found", http.StatusNotFound)
			return
		}
		if body.Role != nil {
			membership.MembershipRole = body.Role
		}
		db.WithContext(r.Context()).Save(&membership)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(membership)
	}
}

// RemoveCompanyMemberHandler handles POST /companies/:companyId/members/:userId/remove
func RemoveCompanyMemberHandler(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		companyID := chi.URLParam(r, "companyId")
		userID := chi.URLParam(r, "userId")
		db.WithContext(r.Context()).
			Model(&models.CompanyMembership{}).
			Where("company_id = ? AND principal_id = ? AND principal_type = 'user'", companyID, userID).
			Update("status", "removed")
		w.WriteHeader(http.StatusNoContent)
	}
}

// GetInviteHandler handles GET /invites/:token
func GetInviteHandler(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := chi.URLParam(r, "token")
		hash := hashToken(token)
		var invites []models.Invite
		db.WithContext(r.Context()).
			Where("token_hash = ? AND revoked_at IS NULL AND expires_at > NOW()", hash).
			Find(&invites)
		if len(invites) == 0 {
			http.Error(w, "Invite not found or expired", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(invites[0])
	}
}

// GetInviteOnboardingHandler handles GET /invites/:token/onboarding
func GetInviteOnboardingHandler(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := chi.URLParam(r, "token")
		hash := hashToken(token)
		var invites []models.Invite
		db.WithContext(r.Context()).
			Where("token_hash = ? AND revoked_at IS NULL AND expires_at > NOW()", hash).
			Find(&invites)
		if len(invites) == 0 {
			http.Error(w, "Invite not found or expired", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"invite":       invites[0],
			"onboardingUrl": "/onboarding",
		})
	}
}

// GetCLIAuthChallengeStatusHandler handles GET /cli-auth/challenges/:id
func GetCLIAuthChallengeStatusHandler(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		var challenge models.CLIAuthChallenge
		if err := db.WithContext(r.Context()).First(&challenge, "id = ?", id).Error; err != nil {
			http.Error(w, "Not found", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(challenge)
	}
}

// RevokeCLIAuthCurrentHandler handles POST /cli-auth/revoke-current
func RevokeCLIAuthCurrentHandler(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "revoked"})
	}
}

// GetInviteTestResolutionHandler handles GET /invites/:token/test-resolution
func GetInviteTestResolutionHandler(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := chi.URLParam(r, "token")
		hash := hashToken(token)
		var invites []models.Invite
		db.WithContext(r.Context()).
			Where("token_hash = ? AND revoked_at IS NULL", hash).
			Find(&invites)
		if len(invites) == 0 {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{"valid": false})
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"valid":     true,
			"invite":    invites[0],
			"expired":   invites[0].ExpiresAt.Before(time.Now()),
		})
	}
}

// GetSkillByNameHandler handles GET /skills/:skillName
func GetSkillByNameHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		skillName := chi.URLParam(r, "skillName")
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"name":   skillName,
			"exists": false,
		})
	}
}

// RevokeInviteHandler handles POST /invites/:inviteId/revoke
func RevokeInviteHandler(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		inviteID := chi.URLParam(r, "inviteId")
		var invite models.Invite
		if err := db.WithContext(r.Context()).First(&invite, "id = ?", inviteID).Error; err != nil {
			http.Error(w, "Not found", http.StatusNotFound)
			return
		}
		now := time.Now()
		invite.RevokedAt = &now
		db.WithContext(r.Context()).Save(&invite)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(invite)
	}
}

// GetCLIAuthMeHandler handles GET /cli-auth/me
func GetCLIAuthMeHandler(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Return the current actor's info from context
		actor := actorFromRequest(r)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"actorType": actor["type"],
			"actorId":   actor["id"],
		})
	}
}

func actorFromRequest(r *http.Request) map[string]interface{} {
	// Extract basic actor info from headers or context
	return map[string]interface{}{
		"type": "user",
		"id":   r.Header.Get("X-User-ID"),
	}
}

// generateSecureToken generates a cryptographically random hex token.
func generateSecureToken(prefix string, byteLen int) (string, error) {
	raw := make([]byte, byteLen)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return prefix + hex.EncodeToString(raw), nil
}

// hashToken returns the SHA-256 hex digest of a token string.
func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// GetInviteOnboardingTextHandler handles GET /invites/:token/onboarding.txt
// Returns a plain-text onboarding document for the invite.
func GetInviteOnboardingTextHandler(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := chi.URLParam(r, "token")
		hash := hashToken(token)
		var invite models.Invite
		if err := db.WithContext(r.Context()).
			Where("token_hash = ? AND revoked_at IS NULL AND expires_at > ?", hash, time.Now()).
			First(&invite).Error; err != nil {
			http.Error(w, "Invite not found or expired", http.StatusNotFound)
			return
		}
		var companyName string
		if invite.CompanyID != nil {
			var company models.Company
			if err := db.WithContext(r.Context()).
				Select("name").First(&company, "id = ?", *invite.CompanyID).Error; err == nil {
				companyName = company.Name
			}
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		fmt.Fprintf(w, "You have been invited to join %s.\n\nInvite token: %s\nAllowed join types: %s\nExpires: %s\n",
			companyName, token, invite.AllowedJoinTypes, invite.ExpiresAt.Format(time.RFC3339))
	}
}

// CreateCompanyInviteHandler handles POST /companies/:companyId/invites
func CreateCompanyInviteHandler(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		companyID := chi.URLParam(r, "companyId")
		var body struct {
			AllowedJoinTypes string      `json:"allowedJoinTypes"`
			DefaultsPayload  interface{} `json:"defaultsPayload"`
			AgentMessage     *string     `json:"agentMessage"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		token, err := generateSecureToken("inv_", 24)
		if err != nil {
			http.Error(w, "failed to generate invite token", http.StatusInternalServerError)
			return
		}
		tokenHash := hashToken(token)

		allowedJoinTypes := body.AllowedJoinTypes
		if allowedJoinTypes == "" {
			allowedJoinTypes = "both"
		}

		invite := models.Invite{
			CompanyID:        &companyID,
			InviteType:       "company_join",
			TokenHash:        tokenHash,
			AllowedJoinTypes: allowedJoinTypes,
			ExpiresAt:        time.Now().Add(7 * 24 * time.Hour),
		}
		if err := db.WithContext(r.Context()).Create(&invite).Error; err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		var companyName string
		var company models.Company
		if err := db.WithContext(r.Context()).Select("name").First(&company, "id = ?", companyID).Error; err == nil {
			companyName = company.Name
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"id":               invite.ID,
			"companyId":        invite.CompanyID,
			"inviteType":       invite.InviteType,
			"allowedJoinTypes": invite.AllowedJoinTypes,
			"expiresAt":        invite.ExpiresAt,
			"createdAt":        invite.CreatedAt,
			"token":            token,
			"inviteUrl":        fmt.Sprintf("/invite/%s", token),
			"companyName":      companyName,
		})
	}
}

// OpenClawInvitePromptHandler handles POST /companies/:companyId/openclaw/invite-prompt
// Creates an agent-only invite for OpenClaw agents to join the company.
func OpenClawInvitePromptHandler(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		companyID := chi.URLParam(r, "companyId")
		var body struct {
			AgentMessage *string `json:"agentMessage"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		token, err := generateSecureToken("ocl_", 24)
		if err != nil {
			http.Error(w, "failed to generate invite token", http.StatusInternalServerError)
			return
		}
		tokenHash := hashToken(token)

		invite := models.Invite{
			CompanyID:        &companyID,
			InviteType:       "company_join",
			TokenHash:        tokenHash,
			AllowedJoinTypes: "agent",
			ExpiresAt:        time.Now().Add(7 * 24 * time.Hour),
		}
		if err := db.WithContext(r.Context()).Create(&invite).Error; err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		var companyName string
		var company models.Company
		if err := db.WithContext(r.Context()).Select("name").First(&company, "id = ?", companyID).Error; err == nil {
			companyName = company.Name
		}

		agentMessage := ""
		if body.AgentMessage != nil {
			agentMessage = *body.AgentMessage
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"id":               invite.ID,
			"companyId":        invite.CompanyID,
			"inviteType":       invite.InviteType,
			"allowedJoinTypes": invite.AllowedJoinTypes,
			"expiresAt":        invite.ExpiresAt,
			"createdAt":        invite.CreatedAt,
			"token":            token,
			"inviteUrl":        fmt.Sprintf("/invite/%s", token),
			"companyName":      companyName,
			"agentMessage":     agentMessage,
		})
	}
}

// ApproveCLIAuthChallengeHandler handles POST /cli-auth/challenges/:id/approve
func ApproveCLIAuthChallengeHandler(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		var body struct {
			Token string `json:"token"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		var challenge models.CLIAuthChallenge
		if err := db.WithContext(r.Context()).First(&challenge, "id = ?", id).Error; err != nil {
			http.Error(w, "Challenge not found", http.StatusNotFound)
			return
		}
		if challenge.ApprovedAt != nil {
			http.Error(w, "Challenge already approved", http.StatusConflict)
			return
		}
		if challenge.CancelledAt != nil {
			http.Error(w, "Challenge was cancelled", http.StatusConflict)
			return
		}
		if challenge.ExpiresAt.Before(time.Now()) {
			http.Error(w, "Challenge expired", http.StatusGone)
			return
		}

		// Verify the token matches the pending key hash
		presentedHash := hashToken(body.Token)
		if presentedHash != challenge.PendingKeyHash {
			http.Error(w, "Invalid token", http.StatusUnauthorized)
			return
		}

		now := time.Now()
		challenge.ApprovedAt = &now
		if err := db.WithContext(r.Context()).Save(&challenge).Error; err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"approved":  true,
			"status":    "approved",
			"keyId":     challenge.BoardAPIKeyID,
			"expiresAt": challenge.ExpiresAt,
		})
	}
}

// CancelCLIAuthChallengeHandler handles POST /cli-auth/challenges/:id/cancel
func CancelCLIAuthChallengeHandler(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		var body struct {
			Token string `json:"token"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		var challenge models.CLIAuthChallenge
		if err := db.WithContext(r.Context()).First(&challenge, "id = ?", id).Error; err != nil {
			http.Error(w, "Challenge not found", http.StatusNotFound)
			return
		}
		if challenge.ApprovedAt != nil {
			http.Error(w, "Challenge already approved", http.StatusConflict)
			return
		}
		if challenge.CancelledAt != nil {
			http.Error(w, "Challenge already cancelled", http.StatusConflict)
			return
		}

		now := time.Now()
		challenge.CancelledAt = &now
		if err := db.WithContext(r.Context()).Save(&challenge).Error; err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status":    "cancelled",
			"cancelled": true,
		})
	}
}

// ApproveJoinRequestHandler handles POST /companies/:companyId/join-requests/:requestId/approve
func ApproveJoinRequestHandler(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		companyID := chi.URLParam(r, "companyId")
		requestID := chi.URLParam(r, "requestId")

		var joinReq models.JoinRequest
		if err := db.WithContext(r.Context()).
			Where("id = ? AND company_id = ?", requestID, companyID).
			First(&joinReq).Error; err != nil {
			http.Error(w, "Join request not found", http.StatusNotFound)
			return
		}
		if joinReq.Status != "pending_approval" {
			http.Error(w, "Join request is not pending", http.StatusConflict)
			return
		}

		now := time.Now()
		joinReq.Status = "approved"
		joinReq.ApprovedAt = &now
		joinReq.UpdatedAt = now

		if err := db.WithContext(r.Context()).Save(&joinReq).Error; err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(joinReq)
	}
}

// RejectJoinRequestHandler handles POST /companies/:companyId/join-requests/:requestId/reject
func RejectJoinRequestHandler(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		companyID := chi.URLParam(r, "companyId")
		requestID := chi.URLParam(r, "requestId")

		var joinReq models.JoinRequest
		if err := db.WithContext(r.Context()).
			Where("id = ? AND company_id = ?", requestID, companyID).
			First(&joinReq).Error; err != nil {
			http.Error(w, "Join request not found", http.StatusNotFound)
			return
		}
		if joinReq.Status != "pending_approval" {
			http.Error(w, "Join request is not pending", http.StatusConflict)
			return
		}

		now := time.Now()
		joinReq.Status = "rejected"
		joinReq.RejectedAt = &now
		joinReq.UpdatedAt = now

		if err := db.WithContext(r.Context()).Save(&joinReq).Error; err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(joinReq)
	}
}

// ClaimJoinRequestAPIKeyHandler handles POST /join-requests/:requestId/claim-api-key
// Allows an approved agent join request to claim its initial API key.
func ClaimJoinRequestAPIKeyHandler(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		requestID := chi.URLParam(r, "requestId")
		var body struct {
			ClaimSecret string `json:"claimSecret"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		var joinReq models.JoinRequest
		if err := db.WithContext(r.Context()).First(&joinReq, "id = ?", requestID).Error; err != nil {
			http.Error(w, "Join request not found", http.StatusNotFound)
			return
		}
		if joinReq.RequestType != "agent" {
			http.Error(w, "Only agent join requests can claim API keys", http.StatusBadRequest)
			return
		}
		if joinReq.Status != "approved" {
			http.Error(w, "Join request must be approved before key claim", http.StatusConflict)
			return
		}
		if joinReq.CreatedAgentID == nil {
			http.Error(w, "Join request has no created agent", http.StatusConflict)
			return
		}
		if joinReq.ClaimSecretHash == nil {
			http.Error(w, "Join request is missing claim secret metadata", http.StatusConflict)
			return
		}
		presentedHash := hashToken(body.ClaimSecret)
		if presentedHash != *joinReq.ClaimSecretHash {
			http.Error(w, "Invalid claim secret", http.StatusForbidden)
			return
		}
		if joinReq.ClaimSecretExpiresAt != nil && joinReq.ClaimSecretExpiresAt.Before(time.Now()) {
			http.Error(w, "Claim secret expired", http.StatusConflict)
			return
		}
		if joinReq.ClaimSecretConsumedAt != nil {
			http.Error(w, "Claim secret already used", http.StatusConflict)
			return
		}

		// Check if API key already claimed
		var existingKey models.AgentAPIKey
		if err := db.WithContext(r.Context()).
			Where("agent_id = ?", *joinReq.CreatedAgentID).
			First(&existingKey).Error; err == nil {
			http.Error(w, "API key already claimed", http.StatusConflict)
			return
		}

		// Atomically mark claim secret as consumed
		result := db.WithContext(r.Context()).Model(&models.JoinRequest{}).
			Where("id = ? AND claim_secret_consumed_at IS NULL", requestID).
			Updates(map[string]interface{}{
				"claim_secret_consumed_at": time.Now(),
				"updated_at":               time.Now(),
			})
		if result.RowsAffected == 0 {
			http.Error(w, "Claim secret already used", http.StatusConflict)
			return
		}

		// Generate a new agent API key
		rawBytes := make([]byte, 24)
		if _, err := rand.Read(rawBytes); err != nil {
			http.Error(w, "failed to generate API key", http.StatusInternalServerError)
			return
		}
		token := "pcp_agent_" + hex.EncodeToString(rawBytes)
		keyHash := hashToken(token)

		var agent models.Agent
		if err := db.WithContext(r.Context()).First(&agent, "id = ?", *joinReq.CreatedAgentID).Error; err != nil {
			http.Error(w, "Created agent not found", http.StatusInternalServerError)
			return
		}

		apiKey := models.AgentAPIKey{
			AgentID:   *joinReq.CreatedAgentID,
			CompanyID: joinReq.CompanyID,
			Name:      "initial-join-key",
			KeyHash:   keyHash,
		}
		if err := db.WithContext(r.Context()).Create(&apiKey).Error; err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"keyId":     apiKey.ID,
			"token":     token,
			"agentId":   *joinReq.CreatedAgentID,
			"createdAt": apiKey.CreatedAt,
		})
	}
}

// PromoteInstanceAdminHandler handles POST /admin/users/:userId/promote-instance-admin
func PromoteInstanceAdminHandler(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID := chi.URLParam(r, "userId")
		role := models.InstanceUserRole{
			UserID: userID,
			Role:   "instance_admin",
		}
		// Use FirstOrCreate to avoid duplicate
		result := db.WithContext(r.Context()).
			Where("user_id = ? AND role = ?", userID, "instance_admin").
			FirstOrCreate(&role)
		if result.Error != nil {
			http.Error(w, result.Error.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(role)
	}
}

// DemoteInstanceAdminHandler handles POST /admin/users/:userId/demote-instance-admin
func DemoteInstanceAdminHandler(db *gorm.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID := chi.URLParam(r, "userId")
		result := db.WithContext(r.Context()).
			Where("user_id = ? AND role = ?", userID, "instance_admin").
			Delete(&models.InstanceUserRole{})
		if result.RowsAffected == 0 {
			http.Error(w, "Instance admin role not found", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "demoted", "userId": userID})
	}
}
