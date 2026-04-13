package services

import (
	"context"
	"fmt"
	"time"

	"github.com/chifamba/vashandi/vashandi/backend/db/models"
	"gorm.io/gorm"
)

type CostService struct {
	DB *gorm.DB
}

func NewCostService(db *gorm.DB) *CostService {
	return &CostService{DB: db}
}

// CreateEvent records a new cost event and updates monthly spend aggregates.
func (s *CostService) CreateEvent(ctx context.Context, companyID string, event *models.CostEvent) (*models.CostEvent, error) {
	event.CompanyID = companyID
	if event.OccurredAt.IsZero() {
		event.OccurredAt = time.Now()
	}

	err := s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// 1. Create the event
		if err := tx.Create(event).Error; err != nil {
			return err
		}

		// 2. Recalculate Agent Monthly Spend
		startOfMonth := time.Date(event.OccurredAt.Year(), event.OccurredAt.Month(), 1, 0, 0, 0, 0, time.UTC)
		
		var agentSpec models.Agent
		var agentSpend int64
		tx.Model(&models.CostEvent{}).
			Where("company_id = ? AND agent_id = ? AND occurred_at >= ?", companyID, event.AgentID, startOfMonth).
			Select("COALESCE(SUM(cost_cents), 0)").
			Scan(&agentSpend)
		
		if err := tx.Model(&agentSpec).Where("id = ?", event.AgentID).Update("spent_monthly_cents", int(agentSpend)).Error; err != nil {
			return err
		}

		// 3. Recalculate Company Monthly Spend
		var companySpend int64
		tx.Model(&models.CostEvent{}).
			Where("company_id = ? AND occurred_at >= ?", companyID, startOfMonth).
			Select("COALESCE(SUM(cost_cents), 0)").
			Scan(&companySpend)
		
		if err := tx.Model(&models.Company{}).Where("id = ?", companyID).Update("spent_monthly_cents", int(companySpend)).Error; err != nil {
			return err
		}

		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to process cost event: %w", err)
	}

	return event, nil
}
