package routes

import (
"crypto/sha256"
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
hash := fmt.Sprintf("%x", sha256.Sum256([]byte(body.Token)))
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
db.WithContext(r.Context()).Save(&invite)
w.Header().Set("Content-Type", "application/json")
json.NewEncoder(w).Encode(invite)
}
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
