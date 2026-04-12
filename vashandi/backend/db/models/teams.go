package models

import "time"

type Team struct {
	ID          string     `gorm:"primaryKey" json:"id"`
	CompanyID   string     `gorm:"index;not null" json:"company_id"`
	Name        string     `gorm:"type:text;not null" json:"name"`
	Description *string    `gorm:"type:text" json:"description"`
	LeadAgentID *string    `gorm:"index" json:"lead_agent_id"`
	Status      string     `gorm:"type:varchar(50);not null;default:'active'" json:"status"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

func (Team) TableName() string { return "teams" }

type TeamMembership struct {
	ID        string    `gorm:"primaryKey" json:"id"`
	CompanyID string    `gorm:"index:idx_team_memberships_company_team,priority:1;index:idx_team_memberships_company_agent,priority:1;not null" json:"company_id"`
	TeamID    string    `gorm:"index:idx_team_memberships_company_team,priority:2;not null" json:"team_id"`
	AgentID   string    `gorm:"index:idx_team_memberships_company_agent,priority:2;not null" json:"agent_id"`
	Role      string    `gorm:"type:varchar(50);not null" json:"role"`
	JoinedAt  time.Time `json:"joined_at"`
}

func (TeamMembership) TableName() string { return "team_memberships" }

type TeamBudget struct {
	ID        string    `gorm:"primaryKey" json:"id"`
	CompanyID string    `gorm:"index;not null" json:"company_id"`
	TeamID    string    `gorm:"index;not null" json:"team_id"`
	Limit     float64   `gorm:"not null" json:"limit"`
	Period    string    `gorm:"type:varchar(50);not null" json:"period"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (TeamBudget) TableName() string { return "team_budgets" }
