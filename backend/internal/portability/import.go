package portability

import (
	"encoding/json"
	"fmt"

	"github.com/chifamba/paperclip/backend/db/models"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

// ImportCompany takes a manifest and imports it into the database
func ImportCompany(db *gorm.DB, manifest *Manifest) (string, error) {
	var companyID string

	err := db.Transaction(func(tx *gorm.DB) error {
		// 1. Company
		var company models.Company
		if err := tx.Where("name = ?", manifest.Company.Name).First(&company).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				company = models.Company{
					Name:                       manifest.Company.Name,
					Description:                manifest.Company.Description,
					FeedbackDataSharingEnabled: manifest.Company.FeedbackDataSharingEnabled,
				}
				if err := tx.Create(&company).Error; err != nil {
					return fmt.Errorf("failed to create company: %w", err)
				}
			} else {
				return fmt.Errorf("failed to query company: %w", err)
			}
		}
		companyID = company.ID

		// 2. Agents
		agentIDMap := make(map[string]string) // manifestID -> DB ID
		agentSlugMap := make(map[string]string) // manifestSlug -> DB ID

		for _, a := range manifest.Agents {
			var config datatypes.JSON
			if a.AdapterConfig != nil {
				config, _ = json.Marshal(a.AdapterConfig)
			}

			agent := models.Agent{
				CompanyID:     companyID,
				Slug:          a.Slug,
				Name:          a.Name,
				Role:          a.Role,
				AdapterType:   a.AdapterType,
				AdapterConfig: config,
			}

			// Handle slug collision (simplified: uniqueness by company/slug is enforced in DB)
			// In a real implementation, we'd append -2, -3 etc.
			if err := tx.Create(&agent).Error; err != nil {
				// If duplicate, try to find existing or rename
				// For now, we assume clean import or fail
				return fmt.Errorf("failed to create agent %s: %w", a.Slug, err)
			}
			agentIDMap[a.ID] = agent.ID
			agentSlugMap[a.Slug] = agent.ID
		}

		// Update ReportsToID for agents
		for _, a := range manifest.Agents {
			if a.ReportsToSlug != nil {
				if parentID, ok := agentSlugMap[*a.ReportsToSlug]; ok {
					if err := tx.Model(&models.Agent{}).Where("id = ?", agentIDMap[a.ID]).Update("reports_to_id", parentID).Error; err != nil {
						return fmt.Errorf("failed to update reports_to for agent %s: %w", a.Slug, err)
					}
				}
			}
		}

		// 3. Projects
		for _, p := range manifest.Projects {
			project := models.Project{
				CompanyID:   companyID,
				Slug:        p.Slug,
				Name:        p.Name,
				Description: p.Description,
				Status:      p.Status,
			}
			if err := tx.Create(&project).Error; err != nil {
				return fmt.Errorf("failed to create project %s: %w", p.Slug, err)
			}
		}

		// 4. Issues
		for _, i := range manifest.Issues {
			issue := models.Issue{
				CompanyID:   companyID,
				Identifier:  i.Identifier,
				Title:       i.Title,
				Description: i.Description,
				Status:      i.Status,
				Priority:    i.Priority,
			}
			if err := tx.Create(&issue).Error; err != nil {
				return fmt.Errorf("failed to create issue: %w", err)
			}
		}

		return nil
	})

	return companyID, err
}
