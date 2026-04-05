package portability

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/chifamba/paperclip/backend/db/models"
	"gorm.io/gorm"
)

// ExportCompany fetches all company data and returns a Manifest
func ExportCompany(db *gorm.DB, companyID string) (*Manifest, error) {
	var company models.Company
	if err := db.Where("id = ?", companyID).First(&company).Error; err != nil {
		return nil, fmt.Errorf("failed to find company: %w", err)
	}

	manifest := &Manifest{
		Version:   "1.0",
		Generated: time.Now(),
		Company: CompanyEntry{
			ID:                         company.ID,
			Name:                       company.Name,
			Description:                company.Description,
			FeedbackDataSharingEnabled: company.FeedbackDataSharingEnabled,
		},
	}

	// Agents
	var agents []models.Agent
	if err := db.Where("company_id = ?", companyID).Find(&agents).Error; err != nil {
		return nil, fmt.Errorf("failed to list agents: %w", err)
	}

	// Create a map for IDs to Slugs for reports_to
	idToSlug := make(map[string]string)
	for _, a := range agents {
		idToSlug[a.ID] = a.Slug
	}

	for _, a := range agents {
		var config map[string]interface{}
		if len(a.AdapterConfig) > 0 {
			json.Unmarshal(a.AdapterConfig, &config)
		}

		var reportsToSlug *string
		if a.ReportsToID != nil {
			if s, ok := idToSlug[*a.ReportsToID]; ok {
				reportsToSlug = &s
			}
		}

		manifest.Agents = append(manifest.Agents, AgentEntry{
			ID:            a.ID,
			Slug:          a.Slug,
			Name:          a.Name,
			Role:          a.Role,
			AdapterType:   a.AdapterType,
			AdapterConfig: config,
			ReportsToSlug: reportsToSlug,
		})
	}

	// Projects
	var projects []models.Project
	if err := db.Where("company_id = ?", companyID).Find(&projects).Error; err != nil {
		return nil, fmt.Errorf("failed to list projects: %w", err)
	}

	for _, p := range projects {
		manifest.Projects = append(manifest.Projects, ProjectEntry{
			ID:          p.ID,
			Slug:        p.Slug,
			Name:        p.Name,
			Description: p.Description,
			Status:      p.Status,
		})
	}

	// Issues (limit to non-archived items or similar?)
	var issues []models.Issue
	if err := db.Where("company_id = ?", companyID).Order("created_at desc").Limit(100).Find(&issues).Error; err != nil {
		return nil, fmt.Errorf("failed to list issues: %w", err)
	}

	for _, i := range issues {
		manifest.Issues = append(manifest.Issues, IssueEntry{
			ID:          i.ID,
			Identifier:  i.Identifier,
			Title:       i.Title,
			Description: i.Description,
			Status:      i.Status,
			Priority:    i.Priority,
		})
	}

	return manifest, nil
}
