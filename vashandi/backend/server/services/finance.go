package services

import (
	"context"
	"fmt"
	"time"

	"github.com/chifamba/vashandi/vashandi/backend/db/models"
	"gorm.io/gorm"
)

// FinanceDateRange represents an optional date filter for finance queries.
type FinanceDateRange struct {
	From *time.Time
	To   *time.Time
}

// FinanceSummary is the aggregate result for a company's finance events.
type FinanceSummary struct {
	CompanyID           string `json:"companyId"`
	DebitCents          int64  `json:"debitCents"`
	CreditCents         int64  `json:"creditCents"`
	NetCents            int64  `json:"netCents"`
	EstimatedDebitCents int64  `json:"estimatedDebitCents"`
	EventCount          int64  `json:"eventCount"`
}

// FinanceByBillerRow is one row of the by-biller breakdown.
type FinanceByBillerRow struct {
	Biller              string `json:"biller"`
	DebitCents          int64  `json:"debitCents"`
	CreditCents         int64  `json:"creditCents"`
	EstimatedDebitCents int64  `json:"estimatedDebitCents"`
	EventCount          int64  `json:"eventCount"`
	KindCount           int64  `json:"kindCount"`
	NetCents            int64  `json:"netCents"`
}

// FinanceByKindRow is one row of the by-kind breakdown.
type FinanceByKindRow struct {
	EventKind           string `json:"eventKind"`
	DebitCents          int64  `json:"debitCents"`
	CreditCents         int64  `json:"creditCents"`
	EstimatedDebitCents int64  `json:"estimatedDebitCents"`
	EventCount          int64  `json:"eventCount"`
	BillerCount         int64  `json:"billerCount"`
	NetCents            int64  `json:"netCents"`
}

// FinanceService manages finance event processing for a company.
type FinanceService struct {
	db *gorm.DB
}

// NewFinanceService creates a new FinanceService.
func NewFinanceService(db *gorm.DB) *FinanceService {
	return &FinanceService{db: db}
}

// applyRangeConditions adds date range WHERE clauses to a query.
func (s *FinanceService) applyRangeConditions(query *gorm.DB, companyID string, r *FinanceDateRange) *gorm.DB {
	query = query.Where("company_id = ?", companyID)
	if r != nil {
		if r.From != nil {
			query = query.Where("occurred_at >= ?", *r.From)
		}
		if r.To != nil {
			query = query.Where("occurred_at <= ?", *r.To)
		}
	}
	return query
}

// assertBelongsToCompany verifies that a record with the given id exists in the table and belongs to the company.
func (s *FinanceService) assertBelongsToCompany(ctx context.Context, table interface{}, id, companyID, label string) error {
	type hasCompanyID struct {
		CompanyID string
	}
	var row hasCompanyID
	if err := s.db.WithContext(ctx).Model(table).Select("company_id").Where("id = ?", id).First(&row).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return fmt.Errorf("%s not found", label)
		}
		return err
	}
	if row.CompanyID != companyID {
		return fmt.Errorf("%s does not belong to company", label)
	}
	return nil
}

// CreateEvent validates and creates a finance event.
func (s *FinanceService) CreateEvent(ctx context.Context, companyID string, data *models.FinanceEvent) (*models.FinanceEvent, error) {
	if data.AgentID != nil {
		if err := s.assertBelongsToCompany(ctx, &models.Agent{}, *data.AgentID, companyID, "Agent"); err != nil {
			return nil, err
		}
	}
	if data.IssueID != nil {
		if err := s.assertBelongsToCompany(ctx, &models.Issue{}, *data.IssueID, companyID, "Issue"); err != nil {
			return nil, err
		}
	}
	if data.ProjectID != nil {
		if err := s.assertBelongsToCompany(ctx, &models.Project{}, *data.ProjectID, companyID, "Project"); err != nil {
			return nil, err
		}
	}
	if data.GoalID != nil {
		if err := s.assertBelongsToCompany(ctx, &models.Goal{}, *data.GoalID, companyID, "Goal"); err != nil {
			return nil, err
		}
	}
	if data.HeartbeatRunID != nil {
		if err := s.assertBelongsToCompany(ctx, &models.HeartbeatRun{}, *data.HeartbeatRunID, companyID, "Heartbeat run"); err != nil {
			return nil, err
		}
	}
	if data.CostEventID != nil {
		if err := s.assertBelongsToCompany(ctx, &models.CostEvent{}, *data.CostEventID, companyID, "Cost event"); err != nil {
			return nil, err
		}
	}

	data.CompanyID = companyID
	if data.Currency == "" {
		data.Currency = "USD"
	}
	if data.Direction == "" {
		data.Direction = "debit"
	}
	if data.OccurredAt.IsZero() {
		data.OccurredAt = time.Now()
	}

	if err := s.db.WithContext(ctx).Create(data).Error; err != nil {
		return nil, err
	}
	return data, nil
}

// Summary returns aggregate finance totals for the company within the given date range.
func (s *FinanceService) Summary(ctx context.Context, companyID string, r *FinanceDateRange) (*FinanceSummary, error) {
	type row struct {
		DebitCents          int64
		CreditCents         int64
		EstimatedDebitCents int64
		EventCount          int64
	}
	var result row
	query := s.applyRangeConditions(s.db.WithContext(ctx).Model(&models.FinanceEvent{}), companyID, r)
	err := query.Select(
		"COALESCE(SUM(CASE WHEN direction = 'debit' THEN amount_cents ELSE 0 END), 0) AS debit_cents",
		"COALESCE(SUM(CASE WHEN direction = 'credit' THEN amount_cents ELSE 0 END), 0) AS credit_cents",
		"COALESCE(SUM(CASE WHEN direction = 'debit' AND estimated = true THEN amount_cents ELSE 0 END), 0) AS estimated_debit_cents",
		"COUNT(*) AS event_count",
	).Scan(&result).Error
	if err != nil {
		return nil, err
	}
	return &FinanceSummary{
		CompanyID:           companyID,
		DebitCents:          result.DebitCents,
		CreditCents:         result.CreditCents,
		NetCents:            result.DebitCents - result.CreditCents,
		EstimatedDebitCents: result.EstimatedDebitCents,
		EventCount:          result.EventCount,
	}, nil
}

// ByBiller returns per-biller aggregate totals.
func (s *FinanceService) ByBiller(ctx context.Context, companyID string, r *FinanceDateRange) ([]FinanceByBillerRow, error) {
	type row struct {
		Biller              string
		DebitCents          int64
		CreditCents         int64
		EstimatedDebitCents int64
		EventCount          int64
		KindCount           int64
		NetCents            int64
	}
	var rows []row
	query := s.applyRangeConditions(s.db.WithContext(ctx).Model(&models.FinanceEvent{}), companyID, r)
	err := query.Select(
		"biller",
		"COALESCE(SUM(CASE WHEN direction = 'debit' THEN amount_cents ELSE 0 END), 0) AS debit_cents",
		"COALESCE(SUM(CASE WHEN direction = 'credit' THEN amount_cents ELSE 0 END), 0) AS credit_cents",
		"COALESCE(SUM(CASE WHEN direction = 'debit' AND estimated = true THEN amount_cents ELSE 0 END), 0) AS estimated_debit_cents",
		"COUNT(*) AS event_count",
		"COUNT(DISTINCT event_kind) AS kind_count",
		"COALESCE(SUM(CASE WHEN direction = 'debit' THEN amount_cents ELSE 0 END), 0) - COALESCE(SUM(CASE WHEN direction = 'credit' THEN amount_cents ELSE 0 END), 0) AS net_cents",
	).Group("biller").
		Order("net_cents DESC, biller ASC").
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	result := make([]FinanceByBillerRow, len(rows))
	for i, r := range rows {
		result[i] = FinanceByBillerRow{
			Biller:              r.Biller,
			DebitCents:          r.DebitCents,
			CreditCents:         r.CreditCents,
			EstimatedDebitCents: r.EstimatedDebitCents,
			EventCount:          r.EventCount,
			KindCount:           r.KindCount,
			NetCents:            r.NetCents,
		}
	}
	return result, nil
}

// ByKind returns per-event-kind aggregate totals.
func (s *FinanceService) ByKind(ctx context.Context, companyID string, r *FinanceDateRange) ([]FinanceByKindRow, error) {
	type row struct {
		EventKind           string
		DebitCents          int64
		CreditCents         int64
		EstimatedDebitCents int64
		EventCount          int64
		BillerCount         int64
		NetCents            int64
	}
	var rows []row
	query := s.applyRangeConditions(s.db.WithContext(ctx).Model(&models.FinanceEvent{}), companyID, r)
	err := query.Select(
		"event_kind",
		"COALESCE(SUM(CASE WHEN direction = 'debit' THEN amount_cents ELSE 0 END), 0) AS debit_cents",
		"COALESCE(SUM(CASE WHEN direction = 'credit' THEN amount_cents ELSE 0 END), 0) AS credit_cents",
		"COALESCE(SUM(CASE WHEN direction = 'debit' AND estimated = true THEN amount_cents ELSE 0 END), 0) AS estimated_debit_cents",
		"COUNT(*) AS event_count",
		"COUNT(DISTINCT biller) AS biller_count",
		"COALESCE(SUM(CASE WHEN direction = 'debit' THEN amount_cents ELSE 0 END), 0) - COALESCE(SUM(CASE WHEN direction = 'credit' THEN amount_cents ELSE 0 END), 0) AS net_cents",
	).Group("event_kind").
		Order("net_cents DESC, event_kind ASC").
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	result := make([]FinanceByKindRow, len(rows))
	for i, r := range rows {
		result[i] = FinanceByKindRow{
			EventKind:           r.EventKind,
			DebitCents:          r.DebitCents,
			CreditCents:         r.CreditCents,
			EstimatedDebitCents: r.EstimatedDebitCents,
			EventCount:          r.EventCount,
			BillerCount:         r.BillerCount,
			NetCents:            r.NetCents,
		}
	}
	return result, nil
}

// List returns raw finance events ordered by occurrence, newest first.
func (s *FinanceService) List(ctx context.Context, companyID string, r *FinanceDateRange, limit int) ([]models.FinanceEvent, error) {
	if limit <= 0 {
		limit = 100
	}
	var events []models.FinanceEvent
	query := s.applyRangeConditions(s.db.WithContext(ctx), companyID, r)
	err := query.Order("occurred_at DESC, created_at DESC").
		Limit(limit).
		Find(&events).Error
	return events, err
}
