package portability

import (
	"time"
)

type Manifest struct {
	Version   string          `json:"version"`
	Generated time.Time       `json:"generated"`
	Company   CompanyEntry    `json:"company"`
	Agents    []AgentEntry    `json:"agents,omitempty"`
	Projects  []ProjectEntry  `json:"projects,omitempty"`
	Issues    []IssueEntry    `json:"issues,omitempty"`
	Skills    []SkillEntry    `json:"skills,omitempty"`
	Assets    []AssetEntry    `json:"assets,omitempty"`
}

type CompanyEntry struct {
	ID                         string  `json:"id"`
	Name                       string  `json:"name"`
	Description                *string `json:"description"`
	FeedbackDataSharingEnabled bool    `json:"feedbackDataSharingEnabled"`
}

type AgentEntry struct {
	ID            string                 `json:"id"`
	Slug          string                 `json:"slug"`
	Name          string                 `json:"name"`
	Role          string                 `json:"role"`
	AdapterType   string                 `json:"adapterType"`
	AdapterConfig map[string]interface{} `json:"adapterConfig"`
	ReportsToSlug *string                `json:"reportsToSlug"`
}

type ProjectEntry struct {
	ID          string  `json:"id"`
	Slug        string  `json:"slug"`
	Name        string  `json:"name"`
	Description *string `json:"description"`
	Status      string  `json:"status"`
}

type IssueEntry struct {
	ID          string  `json:"id"`
	Identifier  *string `json:"identifier"`
	Title       string  `json:"title"`
	Description *string `json:"description"`
	Status      string  `json:"status"`
	Priority    string  `json:"priority"`
}

type SkillEntry struct {
	ID    string `json:"id"`
	Slug  string `json:"slug"`
	Name  string `json:"name"`
	Key   string `json:"key"`
	Path  string `json:"path"`
}

type AssetEntry struct {
	ID          string `json:"id"`
	FileName    string `json:"fileName"`
	ContentType string `json:"contentType"`
	Path        string `json:"path"`
}
