package services

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/chifamba/vashandi/vashandi/backend/db/models"
	"gorm.io/gorm"
)

const (
	BoardAPIKeyTTL       = 30 * 24 * time.Hour
	CLIAuthChallengeTTL  = 10 * time.Minute
)

// CliAuthChallengeStatus represents the status of a CLI auth challenge.
type CliAuthChallengeStatus string

const (
	CLIAuthStatusPending   CliAuthChallengeStatus = "pending"
	CLIAuthStatusApproved  CliAuthChallengeStatus = "approved"
	CLIAuthStatusCancelled CliAuthChallengeStatus = "cancelled"
	CLIAuthStatusExpired   CliAuthChallengeStatus = "expired"
)

// HashBearerToken returns the SHA-256 hex digest of a bearer token.
func HashBearerToken(token string) string {
	h := sha256.Sum256([]byte(token))
	return hex.EncodeToString(h[:])
}

// TokenHashesMatch performs a constant-time comparison of two hash strings.
func TokenHashesMatch(left, right string) bool {
	lb := []byte(left)
	rb := []byte(right)
	return len(lb) == len(rb) && subtle.ConstantTimeCompare(lb, rb) == 1
}

// CreateBoardAPIToken generates a new board API token.
func CreateBoardAPIToken() (string, error) {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return "pcp_board_" + hex.EncodeToString(b), nil
}

// CreateCLIAuthSecret generates a new CLI auth secret.
func CreateCLIAuthSecret() (string, error) {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return "pcp_cli_auth_" + hex.EncodeToString(b), nil
}

// BoardAPIKeyExpiresAt returns the expiry time for a new board API key.
func BoardAPIKeyExpiresAt(now time.Time) time.Time {
	return now.Add(BoardAPIKeyTTL)
}

// CLIAuthChallengeExpiresAt returns the expiry time for a new CLI auth challenge.
func CLIAuthChallengeExpiresAt(now time.Time) time.Time {
	return now.Add(CLIAuthChallengeTTL)
}

// challengeStatusForRow computes the status of a CLI auth challenge from its DB row.
func challengeStatusForRow(row *models.CLIAuthChallenge) CliAuthChallengeStatus {
	if row.CancelledAt != nil {
		return CLIAuthStatusCancelled
	}
	if row.ExpiresAt.Before(time.Now()) {
		return CLIAuthStatusExpired
	}
	if row.ApprovedAt != nil && row.BoardAPIKeyID != nil {
		return CLIAuthStatusApproved
	}
	return CLIAuthStatusPending
}

// BoardAccessResult is returned by ResolveBoardAccess.
type BoardAccessResult struct {
	User            *models.User
	CompanyIDs      []string
	IsInstanceAdmin bool
}

// CLIAuthChallengeDescription is returned by DescribeCLIAuthChallenge.
type CLIAuthChallengeDescription struct {
	ID                   string                 `json:"id"`
	Status               CliAuthChallengeStatus `json:"status"`
	Command              string                 `json:"command"`
	ClientName           *string                `json:"clientName"`
	RequestedAccess      string                 `json:"requestedAccess"`
	RequestedCompanyID   *string                `json:"requestedCompanyId"`
	RequestedCompanyName *string                `json:"requestedCompanyName"`
	ApprovedAt           *string                `json:"approvedAt"`
	CancelledAt          *string                `json:"cancelledAt"`
	ExpiresAt            string                 `json:"expiresAt"`
	ApprovedByUser       *approvedByUserSummary `json:"approvedByUser"`
}

type approvedByUserSummary struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
}

// CLIAuthChallengeCreateInput holds parameters for creating a CLI auth challenge.
type CLIAuthChallengeCreateInput struct {
	Command            string
	ClientName         *string
	RequestedAccess    string // "board" | "instance_admin_required"
	RequestedCompanyID *string
}

// CLIAuthChallengeCreateResult holds the result of creating a CLI auth challenge.
type CLIAuthChallengeCreateResult struct {
	Challenge        *models.CLIAuthChallenge
	ChallengeSecret  string
	PendingBoardToken string
}

// BoardAuthService handles board API key and CLI auth challenge management.
type BoardAuthService struct {
	db *gorm.DB
}

// NewBoardAuthService creates a new BoardAuthService.
func NewBoardAuthService(db *gorm.DB) *BoardAuthService {
	return &BoardAuthService{db: db}
}

// ResolveBoardAccess returns user info, company IDs, and admin status for a user.
func (s *BoardAuthService) ResolveBoardAccess(ctx context.Context, userID string) (*BoardAccessResult, error) {
	var user models.User
	if err := s.db.WithContext(ctx).Where("id = ?", userID).First(&user).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return &BoardAccessResult{}, nil
		}
		return nil, err
	}

	var memberships []models.CompanyMembership
	if err := s.db.WithContext(ctx).
		Where("principal_type = ? AND principal_id = ? AND status = ?", "user", userID, "active").
		Find(&memberships).Error; err != nil {
		return nil, err
	}
	companyIDs := make([]string, 0, len(memberships))
	for _, m := range memberships {
		companyIDs = append(companyIDs, m.CompanyID)
	}

	var adminRole models.InstanceUserRole
	isAdmin := false
	if err := s.db.WithContext(ctx).
		Where("user_id = ? AND role = ?", userID, "instance_admin").
		First(&adminRole).Error; err == nil {
		isAdmin = true
	}

	return &BoardAccessResult{
		User:            &user,
		CompanyIDs:      companyIDs,
		IsInstanceAdmin: isAdmin,
	}, nil
}

// ResolveBoardActivityCompanyIDs resolves company IDs for board activity context.
func (s *BoardAuthService) ResolveBoardActivityCompanyIDs(ctx context.Context, userID string, requestedCompanyID *string, boardAPIKeyID *string) ([]string, error) {
	access, err := s.ResolveBoardAccess(ctx, userID)
	if err != nil {
		return nil, err
	}

	seen := make(map[string]struct{})
	for _, id := range access.CompanyIDs {
		seen[id] = struct{}{}
	}

	if len(seen) == 0 && requestedCompanyID != nil && *requestedCompanyID != "" {
		seen[*requestedCompanyID] = struct{}{}
	}

	if len(seen) == 0 && boardAPIKeyID != nil && *boardAPIKeyID != "" {
		var challenges []models.CLIAuthChallenge
		if err := s.db.WithContext(ctx).
			Where("board_api_key_id = ?", *boardAPIKeyID).
			Find(&challenges).Error; err != nil {
			return nil, err
		}
		for _, ch := range challenges {
			if ch.RequestedCompanyID != nil && *ch.RequestedCompanyID != "" {
				seen[*ch.RequestedCompanyID] = struct{}{}
			}
		}
	}

	if len(seen) == 0 && access.IsInstanceAdmin {
		var companies []models.Company
		if err := s.db.WithContext(ctx).Select("id").Find(&companies).Error; err != nil {
			return nil, err
		}
		for _, c := range companies {
			seen[c.ID] = struct{}{}
		}
	}

	result := make([]string, 0, len(seen))
	for id := range seen {
		result = append(result, id)
	}
	return result, nil
}

// FindBoardAPIKeyByToken looks up a board API key by its plain-text token.
func (s *BoardAuthService) FindBoardAPIKeyByToken(ctx context.Context, token string) (*models.BoardAPIKey, error) {
	tokenHash := HashBearerToken(token)
	now := time.Now()

	var keys []models.BoardAPIKey
	if err := s.db.WithContext(ctx).
		Where("key_hash = ? AND revoked_at IS NULL", tokenHash).
		Find(&keys).Error; err != nil {
		return nil, err
	}
	for i := range keys {
		k := &keys[i]
		if k.ExpiresAt == nil || k.ExpiresAt.After(now) {
			return k, nil
		}
	}
	return nil, nil
}

// TouchBoardAPIKey updates the last-used timestamp on a board API key.
func (s *BoardAuthService) TouchBoardAPIKey(ctx context.Context, id string) error {
	now := time.Now()
	return s.db.WithContext(ctx).Model(&models.BoardAPIKey{}).
		Where("id = ?", id).
		Update("last_used_at", now).Error
}

// RevokeBoardAPIKey revokes a board API key by ID.
func (s *BoardAuthService) RevokeBoardAPIKey(ctx context.Context, id string) (*models.BoardAPIKey, error) {
	now := time.Now()
	var key models.BoardAPIKey
	err := s.db.WithContext(ctx).
		Model(&key).
		Where("id = ? AND revoked_at IS NULL", id).
		Updates(map[string]interface{}{"revoked_at": now, "last_used_at": now}).
		First(&key).Error
	if err != nil {
		return nil, err
	}
	return &key, nil
}

// CreateCLIAuthChallenge creates a new CLI auth challenge and returns secrets.
func (s *BoardAuthService) CreateCLIAuthChallenge(ctx context.Context, input CLIAuthChallengeCreateInput) (*CLIAuthChallengeCreateResult, error) {
	challengeSecret, err := CreateCLIAuthSecret()
	if err != nil {
		return nil, fmt.Errorf("generating challenge secret: %w", err)
	}
	pendingBoardToken, err := CreateBoardAPIToken()
	if err != nil {
		return nil, fmt.Errorf("generating board token: %w", err)
	}

	expiresAt := CLIAuthChallengeExpiresAt(time.Now())

	labelBase := "paperclipai cli"
	if input.ClientName != nil && *input.ClientName != "" {
		labelBase = *input.ClientName
	}
	pendingKeyName := labelBase + " (board)"
	if input.RequestedAccess == "instance_admin_required" {
		pendingKeyName = labelBase + " (instance admin)"
	}

	challenge := &models.CLIAuthChallenge{
		SecretHash:         HashBearerToken(challengeSecret),
		Command:            input.Command,
		ClientName:         input.ClientName,
		RequestedAccess:    input.RequestedAccess,
		RequestedCompanyID: input.RequestedCompanyID,
		PendingKeyHash:     HashBearerToken(pendingBoardToken),
		PendingKeyName:     pendingKeyName,
		ExpiresAt:          expiresAt,
	}

	if err := s.db.WithContext(ctx).Create(challenge).Error; err != nil {
		return nil, err
	}

	return &CLIAuthChallengeCreateResult{
		Challenge:         challenge,
		ChallengeSecret:   challengeSecret,
		PendingBoardToken: pendingBoardToken,
	}, nil
}

// GetCLIAuthChallengeBySecret retrieves a CLI auth challenge by ID and verifies the secret.
func (s *BoardAuthService) GetCLIAuthChallengeBySecret(ctx context.Context, id, token string) (*models.CLIAuthChallenge, error) {
	var challenge models.CLIAuthChallenge
	if err := s.db.WithContext(ctx).Where("id = ?", id).First(&challenge).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	if !TokenHashesMatch(challenge.SecretHash, HashBearerToken(token)) {
		return nil, nil
	}
	return &challenge, nil
}

// DescribeCLIAuthChallenge returns a rich description of a CLI auth challenge.
func (s *BoardAuthService) DescribeCLIAuthChallenge(ctx context.Context, id, token string) (*CLIAuthChallengeDescription, error) {
	challenge, err := s.GetCLIAuthChallengeBySecret(ctx, id, token)
	if err != nil || challenge == nil {
		return nil, err
	}

	var companyName *string
	if challenge.RequestedCompanyID != nil {
		var company models.Company
		if err := s.db.WithContext(ctx).Where("id = ?", *challenge.RequestedCompanyID).First(&company).Error; err == nil {
			companyName = &company.Name
		}
	}

	var approvedBy *approvedByUserSummary
	if challenge.ApprovedByUserID != nil {
		var user models.User
		if err := s.db.WithContext(ctx).Where("id = ?", *challenge.ApprovedByUserID).First(&user).Error; err == nil {
			approvedBy = &approvedByUserSummary{ID: user.ID, Name: user.Name, Email: user.Email}
		}
	}

	var approvedAtStr, cancelledAtStr *string
	if challenge.ApprovedAt != nil {
		s := challenge.ApprovedAt.UTC().Format(time.RFC3339)
		approvedAtStr = &s
	}
	if challenge.CancelledAt != nil {
		s := challenge.CancelledAt.UTC().Format(time.RFC3339)
		cancelledAtStr = &s
	}

	return &CLIAuthChallengeDescription{
		ID:                   challenge.ID,
		Status:               challengeStatusForRow(challenge),
		Command:              challenge.Command,
		ClientName:           challenge.ClientName,
		RequestedAccess:      challenge.RequestedAccess,
		RequestedCompanyID:   challenge.RequestedCompanyID,
		RequestedCompanyName: companyName,
		ApprovedAt:           approvedAtStr,
		CancelledAt:          cancelledAtStr,
		ExpiresAt:            challenge.ExpiresAt.UTC().Format(time.RFC3339),
		ApprovedByUser:       approvedBy,
	}, nil
}

// ApproveCLIAuthChallengeResult is returned by ApproveCLIAuthChallenge.
type ApproveCLIAuthChallengeResult struct {
	Status    CliAuthChallengeStatus
	Challenge *models.CLIAuthChallenge
}

// ApproveCLIAuthChallenge approves a CLI auth challenge and issues a board API key.
func (s *BoardAuthService) ApproveCLIAuthChallenge(ctx context.Context, id, token, userID string) (*ApproveCLIAuthChallengeResult, error) {
	access, err := s.ResolveBoardAccess(ctx, userID)
	if err != nil {
		return nil, err
	}

	var result *ApproveCLIAuthChallengeResult
	txErr := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Lock the row
		if err := tx.Exec(
			"SELECT id FROM cli_auth_challenges WHERE id = ? FOR UPDATE",
			id,
		).Error; err != nil {
			return err
		}

		var challenge models.CLIAuthChallenge
		if err := tx.Where("id = ?", id).First(&challenge).Error; err != nil {
			return fmt.Errorf("cli auth challenge not found")
		}
		if !TokenHashesMatch(challenge.SecretHash, HashBearerToken(token)) {
			return fmt.Errorf("cli auth challenge not found")
		}

		status := challengeStatusForRow(&challenge)
		if status == CLIAuthStatusExpired || status == CLIAuthStatusCancelled {
			result = &ApproveCLIAuthChallengeResult{Status: status, Challenge: &challenge}
			return nil
		}

		if challenge.RequestedAccess == "instance_admin_required" && !access.IsInstanceAdmin {
			return fmt.Errorf("instance admin required")
		}

		boardKeyID := challenge.BoardAPIKeyID
		if boardKeyID == nil {
			expiresAt := BoardAPIKeyExpiresAt(time.Now())
			key := &models.BoardAPIKey{
				UserID:    userID,
				Name:      challenge.PendingKeyName,
				KeyHash:   challenge.PendingKeyHash,
				ExpiresAt: &expiresAt,
			}
			if err := tx.Create(key).Error; err != nil {
				return err
			}
			boardKeyID = &key.ID
		}

		now := time.Now()
		approvedAt := challenge.ApprovedAt
		if approvedAt == nil {
			approvedAt = &now
		}
		if err := tx.Model(&challenge).Updates(map[string]interface{}{
			"approved_by_user_id": userID,
			"board_api_key_id":    boardKeyID,
			"approved_at":         approvedAt,
			"updated_at":          now,
		}).Error; err != nil {
			return err
		}
		result = &ApproveCLIAuthChallengeResult{Status: CLIAuthStatusApproved, Challenge: &challenge}
		return nil
	})
	if txErr != nil {
		return nil, txErr
	}
	return result, nil
}

// CancelCLIAuthChallengeResult is returned by CancelCLIAuthChallenge.
type CancelCLIAuthChallengeResult struct {
	Status    CliAuthChallengeStatus
	Challenge *models.CLIAuthChallenge
}

// CancelCLIAuthChallenge cancels a CLI auth challenge.
func (s *BoardAuthService) CancelCLIAuthChallenge(ctx context.Context, id, token string) (*CancelCLIAuthChallengeResult, error) {
	challenge, err := s.GetCLIAuthChallengeBySecret(ctx, id, token)
	if err != nil {
		return nil, err
	}
	if challenge == nil {
		return nil, fmt.Errorf("cli auth challenge not found")
	}

	status := challengeStatusForRow(challenge)
	if status == CLIAuthStatusApproved || status == CLIAuthStatusExpired || status == CLIAuthStatusCancelled {
		return &CancelCLIAuthChallengeResult{Status: status, Challenge: challenge}, nil
	}

	now := time.Now()
	if err := s.db.WithContext(ctx).Model(challenge).Updates(map[string]interface{}{
		"cancelled_at": now,
		"updated_at":   now,
	}).Error; err != nil {
		return nil, err
	}
	challenge.CancelledAt = &now
	return &CancelCLIAuthChallengeResult{Status: CLIAuthStatusCancelled, Challenge: challenge}, nil
}

// AssertCurrentBoardKey verifies that a board API key belongs to the given user and is not revoked.
func (s *BoardAuthService) AssertCurrentBoardKey(ctx context.Context, keyID, userID string) (*models.BoardAPIKey, error) {
	if keyID == "" || userID == "" {
		return nil, fmt.Errorf("board API key context is required")
	}
	var key models.BoardAPIKey
	if err := s.db.WithContext(ctx).
		Where("id = ? AND user_id = ?", keyID, userID).
		First(&key).Error; err != nil {
		return nil, fmt.Errorf("board API key not found")
	}
	if key.RevokedAt != nil {
		return nil, fmt.Errorf("board API key not found")
	}
	return &key, nil
}
