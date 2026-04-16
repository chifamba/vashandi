package client

import "time"

type Agent struct {
	ID                 string     `json:"id"`
	CompanyID          string     `json:"companyId"`
	Name               string     `json:"name"`
	Role               string     `json:"role"`
	Status             string     `json:"status"`
	ReportsTo          *string    `json:"reportsTo"`
	AdapterType        string     `json:"adapterType"`
	BudgetMonthlyCents int        `json:"budgetMonthlyCents"`
	SpentMonthlyCents  int        `json:"spentMonthlyCents"`
	LastHeartbeatAt    *time.Time `json:"lastHeartbeatAt"`
}

type Approval struct {
	ID                 string     `json:"id"`
	CompanyID          string     `json:"companyId"`
	Type               string     `json:"type"`
	RequestedByAgentID *string    `json:"requestedByAgentId"`
	RequestedByUserID  *string    `json:"requestedByUserId"`
	Status             string     `json:"status"`
	DecisionNote       *string    `json:"decisionNote"`
	DecidedByUserID    *string    `json:"decidedByUserId"`
	DecidedAt          *time.Time `json:"decidedAt"`
	CreatedAt          time.Time  `json:"createdAt"`
	UpdatedAt          time.Time  `json:"updatedAt"`
}

type ActivityLog struct {
	ID         string                 `json:"id"`
	CompanyID  string                 `json:"companyId"`
	ActorType  string                 `json:"actorType"`
	ActorID    string                 `json:"actorId"`
	Action     string                 `json:"action"`
	EntityType string                 `json:"entityType"`
	EntityID   string                 `json:"entityId"`
	AgentID    *string                `json:"agentId"`
	RunID      *string                `json:"runId"`
	Details    map[string]interface{} `json:"details"`
	CreatedAt  time.Time              `json:"createdAt"`
}

type Plugin struct {
	ID                 string `json:"id"`
	PluginKey          string `json:"pluginKey"`
	PackageName        string `json:"packageName"`
	Version            string `json:"version"`
	Status             string `json:"status"`
	SupportsConfigTest bool   `json:"supportsConfigTest"`
}

type DashboardSummary struct {
	TotalAgents          int64   `json:"totalAgents"`
	ActiveAgents         int64   `json:"activeAgents"`
	RunningAgents        int64   `json:"runningAgents"`
	PausedAgents         int64   `json:"pausedAgents"`
	ErrorAgents          int64   `json:"errorAgents"`
	TotalIssues          int64   `json:"totalIssues"`
	OpenIssues           int64   `json:"openIssues"`
	InProgressIssues     int64   `json:"inProgressIssues"`
	BlockedIssues        int64   `json:"blockedIssues"`
	DoneIssues           int64   `json:"doneIssues"`
	PendingApprovals     int64   `json:"pendingApprovals"`
	MTDSpend             float64 `json:"mtdSpend"`
	BudgetUtilization    float64 `json:"budgetUtilization"`
	MemoryOperationCount int64   `json:"memoryOperationCount"`
	MemoryHitRate        float64 `json:"memoryHitRate"`
	MCPInvocationCount   int64   `json:"mcpInvocationCount"`
}

type PlatformMetrics struct {
	TotalAgents   int64   `json:"totalAgents"`
	ActiveRuns    int64   `json:"activeRuns"`
	TotalSpendMTD float64 `json:"totalSpendMTD"`
	ErrorRate     float64 `json:"errorRate"`
}

type ContextOperation struct {
	Name     string `json:"name"`
	Method   string `json:"method"`
	Endpoint string `json:"endpoint"`
	Status   string `json:"status"`
}

type HeartbeatRun struct {
	ID               string                 `json:"id"`
	CompanyID        string                 `json:"companyId"`
	AgentID          string                 `json:"agentId"`
	Status           string                 `json:"status"`
	Error            *string                `json:"error"`
	InvocationSource string                 `json:"invocationSource"`
	TriggerDetail    *string                `json:"triggerDetail"`
	ResultJSON       map[string]interface{} `json:"resultJson"`
	StdoutExcerpt    *string                `json:"stdoutExcerpt"`
	StderrExcerpt    *string                `json:"stderrExcerpt"`
	CreatedAt        time.Time              `json:"createdAt"`
	UpdatedAt        time.Time              `json:"updatedAt"`
}

type HeartbeatRunEvent struct {
	ID        int64                  `json:"id"`
	CompanyID string                 `json:"companyId"`
	RunID     string                 `json:"runId"`
	AgentID   string                 `json:"agentId"`
	Seq       int                    `json:"seq"`
	EventType string                 `json:"eventType"`
	Stream    *string                `json:"stream"`
	Level     *string                `json:"level"`
	Color     *string                `json:"color"`
	Message   *string                `json:"message"`
	Payload   map[string]interface{} `json:"payload"`
	CreatedAt time.Time              `json:"createdAt"`
}

type HeartbeatWakeupRequest struct {
	CompanyID     string                 `json:"companyId"`
	AgentID       string                 `json:"agentId"`
	Source        string                 `json:"source"`
	TriggerDetail string                 `json:"triggerDetail,omitempty"`
	Context       map[string]interface{} `json:"context,omitempty"`
}
