package services

import (
	"fmt"
	"strings"
)

// PluginCapability mirrors the capability strings used in the manifest.
type PluginCapability string

const (
	CapCompaniesRead         PluginCapability = "companies.read"
	CapProjectsRead          PluginCapability = "projects.read"
	CapProjectWorkspacesRead PluginCapability = "project.workspaces.read"
	CapIssuesRead            PluginCapability = "issues.read"
	CapIssuesCreate          PluginCapability = "issues.create"
	CapIssuesUpdate          PluginCapability = "issues.update"
	CapIssueCommentsRead     PluginCapability = "issue.comments.read"
	CapIssueCommentsCreate   PluginCapability = "issue.comments.create"
	CapAgentsRead            PluginCapability = "agents.read"
	CapGoalsRead             PluginCapability = "goals.read"
	CapGoalsCreate           PluginCapability = "goals.create"
	CapActivityRead          PluginCapability = "activity.read"
	CapActivityLogWrite      PluginCapability = "activity.log.write"
	CapMetricsWrite          PluginCapability = "metrics.write"
	CapTelemetryTrack        PluginCapability = "telemetry.track"
	CapPluginStateRead       PluginCapability = "plugin.state.read"
	CapPluginStateWrite      PluginCapability = "plugin.state.write"
	CapEventsSubscribe       PluginCapability = "events.subscribe"
	CapEventsEmit            PluginCapability = "events.emit"
	CapJobsSchedule          PluginCapability = "jobs.schedule"
	CapWebhooksReceive       PluginCapability = "webhooks.receive"
	CapHttpOutbound          PluginCapability = "http.outbound"
	CapSecretsReadRef        PluginCapability = "secrets.read-ref"
	CapAgentToolsRegister    PluginCapability = "agent.tools.register"
	CapCostsRead             PluginCapability = "costs.read"
	CapUiSidebarRegister     PluginCapability = "ui.sidebar.register"
	CapUiPageRegister        PluginCapability = "ui.page.register"
	CapUiDetailTabRegister   PluginCapability = "ui.detailTab.register"
	CapUiDashboardWidgetReg  PluginCapability = "ui.dashboardWidget.register"
	CapUiActionRegister      PluginCapability = "ui.action.register"
	CapUiCommentAnnotateReg  PluginCapability = "ui.commentAnnotation.register"
	CapInstanceSettingsReg   PluginCapability = "instance.settings.register"
)

// operationCapabilities maps host RPC methods to their required capabilities.
var operationCapabilities = map[string][]PluginCapability{
	// Data read operations
	"companies.list":           {CapCompaniesRead},
	"companies.get":            {CapCompaniesRead},
	"projects.list":           {CapProjectsRead},
	"projects.get":            {CapProjectsRead},
	"project.workspaces.list": {CapProjectWorkspacesRead},
	"project.workspaces.get":  {CapProjectWorkspacesRead},
	"issues.list":             {CapIssuesRead},
	"issues.get":              {CapIssuesRead},
	"issue.comments.list":     {CapIssueCommentsRead},
	"issue.comments.get":      {CapIssueCommentsRead},
	"agents.list":             {CapAgentsRead},
	"agents.get":              {CapAgentsRead},
	"goals.list":              {CapGoalsRead},
	"goals.get":               {CapGoalsRead},
	"activity.list":           {CapActivityRead},
	"activity.get":            {CapActivityRead},
	"costs.list":              {CapCostsRead},
	"costs.get":               {CapCostsRead},

	// Data write operations
	"issues.create":          {CapIssuesCreate},
	"issues.update":          {CapIssuesUpdate},
	"issue.comments.create":  {CapIssueCommentsCreate},
	"activity.log":           {CapActivityLogWrite},
	"metrics.write":          {CapMetricsWrite},
	"telemetry.track":        {CapTelemetryTrack},
	"goals.create":           {CapGoalsCreate},

	// Plugin state operations
	"state.get":           {CapPluginStateRead},
	"state.list":          {CapPluginStateRead},
	"state.set":           {CapPluginStateWrite},
	"state.delete":        {CapPluginStateWrite},
	"entities.upsert":     {CapPluginStateWrite},
	"entities.list":       {CapPluginStateRead},

	// Runtime / Integration operations
	"events.subscribe": {CapEventsSubscribe},
	"events.emit":      {CapEventsEmit},
	"jobs.schedule":    {CapJobsSchedule},
	"jobs.cancel":      {CapJobsSchedule},
	"webhooks.receive": {CapWebhooksReceive},
	"http.fetch":       {CapHttpOutbound},
	"secrets.resolve":  {CapSecretsReadRef},

	// Agent tools
	"agent.tools.register": {CapAgentToolsRegister},
	"agent.tools.execute":  {CapAgentToolsRegister},
}

var uiSlotCapabilities = map[string]PluginCapability{
	"sidebar":                 CapUiSidebarRegister,
	"sidebarPanel":            CapUiSidebarRegister,
	"projectSidebarItem":      CapUiSidebarRegister,
	"page":                    CapUiPageRegister,
	"detailTab":               CapUiDetailTabRegister,
	"taskDetailView":          CapUiDetailTabRegister,
	"dashboardWidget":         CapUiDashboardWidgetReg,
	"globalToolbarButton":     CapUiActionRegister,
	"toolbarButton":           CapUiActionRegister,
	"contextMenuItem":         CapUiActionRegister,
	"commentAnnotation":       CapUiCommentAnnotateReg,
	"commentContextMenuItem":  CapUiActionRegister,
	"settingsPage":            CapInstanceSettingsReg,
}

var launcherCapabilities = map[string]PluginCapability{
	"page":                    CapUiPageRegister,
	"detailTab":               CapUiDetailTabRegister,
	"taskDetailView":          CapUiDetailTabRegister,
	"dashboardWidget":         CapUiDashboardWidgetReg,
	"sidebar":                 CapUiSidebarRegister,
	"sidebarPanel":            CapUiSidebarRegister,
	"projectSidebarItem":      CapUiSidebarRegister,
	"globalToolbarButton":     CapUiActionRegister,
	"toolbarButton":           CapUiActionRegister,
	"contextMenuItem":         CapUiActionRegister,
	"commentAnnotation":       CapUiCommentAnnotateReg,
	"commentContextMenuItem":  CapUiActionRegister,
	"settingsPage":            CapInstanceSettingsReg,
}

var featureCapabilities = map[string]PluginCapability{
	"tools":    CapAgentToolsRegister,
	"jobs":     CapJobsSchedule,
	"webhooks": CapWebhooksReceive,
}

// PluginManifestV1 represents the capability-relevant subset of a plugin manifest.
type PluginManifestV1 struct {
	ID           string           `json:"id"`
	Capabilities []string         `json:"capabilities"`
	Tools        []interface{}    `json:"tools,omitempty"`
	Jobs         []interface{}    `json:"jobs,omitempty"`
	Webhooks     []interface{}    `json:"webhooks,omitempty"`
	UI           *PluginManifestUI `json:"ui,omitempty"`
	Launchers            []PluginLauncher `json:"launchers,omitempty"` // Legacy top-level
	Sandbox              *PluginSandboxConfig `json:"sandbox,omitempty"`
	InstanceConfigSchema map[string]interface{} `json:"instanceConfigSchema,omitempty"`
}

type PluginSandboxConfig struct {
	TimeoutMs     int      `json:"timeoutMs,omitempty"`
	MemoryLimitMb int      `json:"memoryLimitMb,omitempty"`
	AllowedEnv    []string `json:"allowedEnv,omitempty"`
}

type PluginManifestUI struct {
	Slots     []PluginUiSlot   `json:"slots,omitempty"`
	Launchers []PluginLauncher `json:"launchers,omitempty"`
}

type PluginUiSlot struct {
	Type string `json:"type"`
}

type PluginLauncher struct {
	PlacementZone string `json:"placementZone"`
}

type PluginCapabilityValidator struct{}

func NewPluginCapabilityValidator() *PluginCapabilityValidator {
	return &PluginCapabilityValidator{}
}

// CheckOperation verifies that the plugin manifest permits the given operation.
func (v *PluginCapabilityValidator) CheckOperation(manifest *PluginManifestV1, operation string) error {
	if manifest == nil {
		return fmt.Errorf("plugin manifest is missing")
	}

	required, ok := operationCapabilities[operation]
	if !ok {
		return fmt.Errorf("unknown operation %q", operation)
	}

	declared := make(map[string]bool)
	for _, cap := range manifest.Capabilities {
		declared[cap] = true
	}

	for _, cap := range required {
		if !declared[string(cap)] {
			return fmt.Errorf("plugin %q lack's required capability %q for operation %q", manifest.ID, cap, operation)
		}
	}

	return nil
}

// ValidateManifestCapabilities ensures that a manifest's declared features are
// covered by its granted capabilities.
func (v *PluginCapabilityValidator) ValidateManifestCapabilities(manifest *PluginManifestV1) error {
	if manifest == nil {
		return nil
	}

	declared := make(map[string]bool)
	for _, cap := range manifest.Capabilities {
		declared[cap] = true
	}

	var missing []string

	// Check tools
	if len(manifest.Tools) > 0 {
		cap := featureCapabilities["tools"]
		if !declared[string(cap)] {
			missing = append(missing, string(cap))
		}
	}

	// Check jobs
	if len(manifest.Jobs) > 0 {
		cap := featureCapabilities["jobs"]
		if !declared[string(cap)] {
			missing = append(missing, string(cap))
		}
	}

	// Check webhooks
	if len(manifest.Webhooks) > 0 {
		cap := featureCapabilities["webhooks"]
		if !declared[string(cap)] {
			missing = append(missing, string(cap))
		}
	}

	// Check UI slots
	if manifest.UI != nil {
		for _, slot := range manifest.UI.Slots {
			cap, ok := uiSlotCapabilities[slot.Type]
			if ok && !declared[string(cap)] {
				if !containsString(missing, string(cap)) {
					missing = append(missing, string(cap))
				}
			}
		}
		for _, launcher := range manifest.UI.Launchers {
			cap, ok := launcherCapabilities[launcher.PlacementZone]
			if ok && !declared[string(cap)] {
				if !containsString(missing, string(cap)) {
					missing = append(missing, string(cap))
				}
			}
		}
	}

	// Legacy launchers
	for _, launcher := range manifest.Launchers {
		cap, ok := launcherCapabilities[launcher.PlacementZone]
		if ok && !declared[string(cap)] {
			if !containsString(missing, string(cap)) {
				missing = append(missing, string(cap))
			}
		}
	}

	if len(missing) > 0 {
		return fmt.Errorf("missing required capabilities for declared features: %s", strings.Join(missing, ", "))
	}

	return nil
}

func containsString(slice []string, val string) bool {
	for _, item := range slice {
		if item == val {
			return true
		}
	}
	return false
}
