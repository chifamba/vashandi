package db

import (
	"fmt"

	"github.com/chifamba/vashandi/vashandi/backend/db/models"
	"gorm.io/gorm"
)

// RunMigrations applies GORM AutoMigrate to all models in the models package.
func RunMigrations(db *gorm.DB) error {
	// Ensure schema exists
	if err := db.Exec("CREATE SCHEMA IF NOT EXISTS vashandi").Error; err != nil {
		return fmt.Errorf("could not create schema: %w", err)
	}

	// We use the public schema for now to avoid search_path issues with gorm's automigrate
	// until we have a more robust schema management strategy.
	// But let's at least try to migrate the main tables.
	return db.AutoMigrate(
		&models.Company{},
		&models.CompanyLogo{},
		&models.CompanyMembership{},
		&models.User{},
		&models.Session{},
		&models.Account{},
		&models.Verification{},
		&models.InstanceSetting{},
		&models.InstanceUserRole{},
		&models.BudgetPolicy{},
		&models.BudgetIncident{},
		&models.Project{},
		&models.Goal{},
		&models.ProjectGoal{},
		&models.Issue{},
		&models.IssueComment{},
		&models.IssueAttachment{},
		&models.IssueLabel{},
		&models.Label{},
		&models.IssueRelation{},
		&models.IssueExecutionDecision{},
		&models.IssueApproval{},
		&models.Approval{},
		&models.ApprovalComment{},
		&models.IssueReadState{},
		&models.IssueInboxArchive{},
		&models.InboxDismissal{},
		&models.Agent{},
		&models.AgentAPIKey{},
		&models.AgentConfigRevision{},
		&models.AgentRuntimeState{},
		&models.AgentTaskSession{},
		&models.AgentWakeupRequest{},
		&models.WorkspaceRuntimeService{},
		&models.WorkspaceOperation{},
		&models.ExecutionWorkspace{},
		&models.ProjectWorkspace{},
		&models.ActivityLog{},
		&models.Asset{},
		&models.CostEvent{},
		&models.FinanceEvent{},
		&models.Document{},
		&models.DocumentRevision{},
		&models.IssueDocument{},
		&models.IssueWorkProduct{},
		&models.Plugin{},
		&models.PluginConfig{},
		&models.PluginEntity{},
		&models.PluginState{},
		&models.PluginWebhookDelivery{},
		&models.PluginLog{},
		&models.Routine{},
		&models.RoutineRun{},
		&models.RoutineTrigger{},
		&models.HeartbeatRun{},
		&models.HeartbeatRunEvent{},
		&models.Invite{},
		&models.JoinRequest{},
		&models.BoardAPIKey{},
		&models.PrincipalPermissionGrant{},
		&models.FeedbackExport{},
		&models.FeedbackVote{},
	)
}
