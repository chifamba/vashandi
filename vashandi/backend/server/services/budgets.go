package services

import (
	"gorm.io/gorm"

	"github.com/chifamba/vashandi/vashandi/backend/db/models"
)

// CheckProjectBudget queries the budget_policies table where ScopeType = "project" and ScopeID = projectID,
// sums the CostCents from CostEvent for the same ProjectID, and returns true if sum >= Amount.
func CheckProjectBudget(db *gorm.DB, projectID string) (bool, error) {
	// 1. Get the budget policy for the project
	var policy models.BudgetPolicy
	err := db.Where("scope_type = ? AND scope_id = ? AND is_active = ?", "project", projectID, true).First(&policy).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			// No budget policy found, assume limitless
			return false, nil
		}
		return false, err
	}

	// 2. Sum the costs associated with this project
	var totalCostCents int64
	err = db.Model(&models.CostEvent{}).Where("project_id = ?", projectID).Select("COALESCE(SUM(cost_cents), 0)").Scan(&totalCostCents).Error
	if err != nil {
		return false, err
	}

	// 3. Return true if the limit has been reached or exceeded
	return totalCostCents >= int64(policy.Amount), nil
}
