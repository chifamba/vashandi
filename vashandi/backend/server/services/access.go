package services

import (
	"context"

	"github.com/chifamba/vashandi/vashandi/backend/db/models"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type AccessService struct {
	DB *gorm.DB
}

func NewAccessService(db *gorm.DB) *AccessService {
	return &AccessService{DB: db}
}

func (s *AccessService) isInstanceAdmin(ctx context.Context, userID string) (bool, error) {
	if userID == "" {
		return false, nil
	}

	var count int64
	err := s.DB.WithContext(ctx).
		Model(&models.InstanceUserRole{}).
		Where("user_id = ? AND role = ?", userID, "instance_admin").
		Count(&count).Error
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

func (s *AccessService) GetMembership(ctx context.Context, companyID, principalType, principalID string) (*models.CompanyMembership, error) {
	var membership models.CompanyMembership
	err := s.DB.WithContext(ctx).
		Where("company_id = ? AND principal_type = ? AND principal_id = ?", companyID, principalType, principalID).
		First(&membership).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	return &membership, nil
}

func (s *AccessService) HasPermission(ctx context.Context, companyID, principalType, principalID, permissionKey string) (bool, error) {
	membership, err := s.GetMembership(ctx, companyID, principalType, principalID)
	if err != nil {
		return false, err
	}
	if membership == nil || membership.Status != "active" {
		return false, nil
	}

	var count int64
	err = s.DB.WithContext(ctx).
		Model(&models.PrincipalPermissionGrant{}).
		Where(
			"company_id = ? AND principal_type = ? AND principal_id = ? AND permission_key = ?",
			companyID,
			principalType,
			principalID,
			permissionKey,
		).
		Count(&count).Error
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

func (s *AccessService) CanUser(ctx context.Context, companyID, userID, permissionKey string) (bool, error) {
	if userID == "" {
		return false, nil
	}

	admin, err := s.isInstanceAdmin(ctx, userID)
	if err != nil {
		return false, err
	}
	if admin {
		return true, nil
	}

	return s.HasPermission(ctx, companyID, "user", userID, permissionKey)
}

func (s *AccessService) EnsureMembership(ctx context.Context, companyID, principalType, principalID string, membershipRole *string, status string) (*models.CompanyMembership, error) {
	if membershipRole == nil {
		defaultRole := "member"
		membershipRole = &defaultRole
	}
	if status == "" {
		status = "active"
	}

	existing, err := s.GetMembership(ctx, companyID, principalType, principalID)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		if existing.Status == status && stringPtrValue(existing.MembershipRole) == stringPtrValue(membershipRole) {
			return existing, nil
		}
		existing.Status = status
		existing.MembershipRole = cloneStringPtr(membershipRole)
		if err := s.DB.WithContext(ctx).Save(existing).Error; err != nil {
			return nil, err
		}
		return existing, nil
	}

	membership := &models.CompanyMembership{
		ID:             uuid.NewString(),
		CompanyID:      companyID,
		PrincipalType:  principalType,
		PrincipalID:    principalID,
		Status:         status,
		MembershipRole: cloneStringPtr(membershipRole),
	}
	if err := s.DB.WithContext(ctx).Create(membership).Error; err != nil {
		return nil, err
	}
	return membership, nil
}

func cloneStringPtr(value *string) *string {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func stringPtrValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
