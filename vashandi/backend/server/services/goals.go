package services

import (
	"context"
	"time"

	"github.com/chifamba/vashandi/vashandi/backend/db/models"
	"gorm.io/gorm"
)

// GoalService manages goal CRUD and company-goal lookup behavior.
type GoalService struct {
	db *gorm.DB
}

// NewGoalService creates a new GoalService.
func NewGoalService(db *gorm.DB) *GoalService {
	return &GoalService{db: db}
}

// ListGoals returns all goals for a company.
func (s *GoalService) ListGoals(ctx context.Context, companyID string) ([]models.Goal, error) {
	var goals []models.Goal
	err := s.db.WithContext(ctx).
		Where("company_id = ?", companyID).
		Find(&goals).Error
	return goals, err
}

// GetGoalByID returns a goal by id, or nil when absent.
func (s *GoalService) GetGoalByID(ctx context.Context, id string) (*models.Goal, error) {
	var goal models.Goal
	err := s.db.WithContext(ctx).Where("id = ?", id).First(&goal).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	return &goal, nil
}

// GetDefaultCompanyGoal resolves the same fallback order as the TypeScript service.
func (s *GoalService) GetDefaultCompanyGoal(ctx context.Context, companyID string) (*models.Goal, error) {
	queries := []func(*gorm.DB) *gorm.DB{
		func(db *gorm.DB) *gorm.DB {
			return db.Where("company_id = ? AND level = ? AND status = ? AND parent_id IS NULL", companyID, "company", "active")
		},
		func(db *gorm.DB) *gorm.DB {
			return db.Where("company_id = ? AND level = ? AND parent_id IS NULL", companyID, "company")
		},
		func(db *gorm.DB) *gorm.DB {
			return db.Where("company_id = ? AND level = ?", companyID, "company")
		},
	}

	for _, build := range queries {
		var goal models.Goal
		err := build(s.db.WithContext(ctx)).
			Order("created_at ASC").
			First(&goal).Error
		if err == nil {
			return &goal, nil
		}
		if err != gorm.ErrRecordNotFound {
			return nil, err
		}
	}

	return nil, nil
}

// CreateGoal stores a goal under the given company.
func (s *GoalService) CreateGoal(ctx context.Context, companyID string, goal *models.Goal) (*models.Goal, error) {
	goal.CompanyID = companyID
	if goal.Level == "" {
		goal.Level = "task"
	}
	if goal.Status == "" {
		goal.Status = "planned"
	}
	if err := s.db.WithContext(ctx).Create(goal).Error; err != nil {
		return nil, err
	}
	return goal, nil
}

// UpdateGoal applies a partial update and returns the updated record, or nil when absent.
func (s *GoalService) UpdateGoal(ctx context.Context, id string, updates map[string]interface{}) (*models.Goal, error) {
	var goal models.Goal
	err := s.db.WithContext(ctx).Where("id = ?", id).First(&goal).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}

	if updates == nil {
		updates = map[string]interface{}{}
	}
	updates["updated_at"] = time.Now()

	if err := s.db.WithContext(ctx).Model(&goal).Updates(updates).Error; err != nil {
		return nil, err
	}
	if err := s.db.WithContext(ctx).Where("id = ?", id).First(&goal).Error; err != nil {
		return nil, err
	}
	return &goal, nil
}

// RemoveGoal deletes a goal and returns the deleted record, or nil when absent.
func (s *GoalService) RemoveGoal(ctx context.Context, id string) (*models.Goal, error) {
	var goal models.Goal
	err := s.db.WithContext(ctx).Where("id = ?", id).First(&goal).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	if err := s.db.WithContext(ctx).Delete(&goal).Error; err != nil {
		return nil, err
	}
	return &goal, nil
}
