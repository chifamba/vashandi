package routes

import (
	"encoding/json"
	"net/http"


	"gorm.io/gorm"
)

// The Access API manages user invitations, join requests, CLI challenges, and role management.

func AccessNotImplementedHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotImplemented)
		json.NewEncoder(w).Encode(map[string]string{"error": "Access management endpoint is pending full Go port of auth utilities"})
	}
}

// These are placeholders to wire up the router for Phase 3 completion.
func ClaimBoardHandler(db *gorm.DB) http.HandlerFunc { return AccessNotImplementedHandler() }
func ProcessBoardClaimHandler(db *gorm.DB) http.HandlerFunc { return AccessNotImplementedHandler() }
func CreateCliAuthChallengeHandler(db *gorm.DB) http.HandlerFunc { return AccessNotImplementedHandler() }
func GetCliAuthChallengeHandler(db *gorm.DB) http.HandlerFunc { return AccessNotImplementedHandler() }
func ResolveCliAuthChallengeHandler(db *gorm.DB) http.HandlerFunc { return AccessNotImplementedHandler() }
func CliAuthMeHandler(db *gorm.DB) http.HandlerFunc { return AccessNotImplementedHandler() }
func RevokeCliAuthHandler(db *gorm.DB) http.HandlerFunc { return AccessNotImplementedHandler() }
func GetInviteHandler(db *gorm.DB) http.HandlerFunc { return AccessNotImplementedHandler() }
func GetInviteOnboardingHandler(db *gorm.DB) http.HandlerFunc { return AccessNotImplementedHandler() }
func AcceptInviteHandler(db *gorm.DB) http.HandlerFunc { return AccessNotImplementedHandler() }
func RevokeInviteHandler(db *gorm.DB) http.HandlerFunc { return AccessNotImplementedHandler() }
func ListJoinRequestsHandler(db *gorm.DB) http.HandlerFunc { return AccessNotImplementedHandler() }
func ClaimJoinRequestApiKeyHandler(db *gorm.DB) http.HandlerFunc { return AccessNotImplementedHandler() }
func CreateCompanyInviteHandler(db *gorm.DB) http.HandlerFunc { return AccessNotImplementedHandler() }
func ListCompanyMembersHandler(db *gorm.DB) http.HandlerFunc { return AccessNotImplementedHandler() }
func UpdateMemberPermissionsHandler(db *gorm.DB) http.HandlerFunc { return AccessNotImplementedHandler() }
func UpdateUserCompanyAccessHandler(db *gorm.DB) http.HandlerFunc { return AccessNotImplementedHandler() }
