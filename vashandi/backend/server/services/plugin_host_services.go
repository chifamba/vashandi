package services

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/chifamba/vashandi/vashandi/backend/db/models"
	"github.com/chifamba/vashandi/vashandi/backend/shared/telemetry"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

const (
	PluginFetchTimeout = 30 * time.Second
	DNSLookupTimeout   = 5 * time.Second
)

var allowedProtocols = map[string]bool{
	"http":  true,
	"https": true,
}

// PluginHostServices provides the bridge between plugin workers and the host platform.
type PluginHostServices struct {
	DB        *gorm.DB
	EventBus  *PluginEventBus
	Issues    *IssueService
	Goals     *GoalService
	Heartbeat *HeartbeatService
	Activity  *ActivityService
	Costs     *CostService
	Secrets   *SecretService
	Registry  *PluginRegistryService
	State     *PluginStateStore
	Telemetry *telemetry.Client
	Validator *PluginCapabilityValidator
	SecretsHandler *PluginSecretsHandler
	// Optional callback to notify the worker of an asynchronous event (e.g., onEvent)
	NotifyWorker func(pluginID string, method string, params interface{})
}

func NewPluginHostServices(opts PluginHostServicesOptions) *PluginHostServices {
	return &PluginHostServices{
		DB:           opts.DB,
		EventBus:     opts.EventBus,
		Issues:       opts.Issues,
		Goals:        opts.Goals,
		Heartbeat:    opts.Heartbeat,
		Activity:     opts.Activity,
		Costs:        opts.Costs,
		Secrets:      opts.Secrets,
		Registry:     opts.Registry,
		State:        opts.State,
		Telemetry:    opts.Telemetry,
		Validator:    opts.Validator,
		SecretsHandler: opts.SecretsHandler,
		NotifyWorker: opts.NotifyWorker,
	}
}

type PluginHostServicesOptions struct {
	DB           *gorm.DB
	EventBus     *PluginEventBus
	Issues       *IssueService
	Goals        *GoalService
	Heartbeat    *HeartbeatService
	Activity     *ActivityService
	Costs        *CostService
	Secrets      *SecretService
	Registry     *PluginRegistryService
	State        *PluginStateStore
	Telemetry    *telemetry.Client
	Validator    *PluginCapabilityValidator
	SecretsHandler *PluginSecretsHandler
	NotifyWorker func(pluginID string, method string, params interface{})
}

// --- SSRF Helpers ---

func isPrivateIP(ipStr string) bool {
	ip := net.ParseIP(ipStr)
	if ip == nil {
		return false
	}

	if ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsPrivate() || ip.IsUnspecified() {
		return true
	}

	if ip4 := ip.To4(); ip4 != nil {
		return ip4.IsPrivate() || ip4.IsLoopback() || ip4.IsLinkLocalUnicast() || ip4.IsUnspecified()
	}

	if len(ip) == net.IPv6len && (ip[0]&0xfe) == 0xfc {
		return true
	}

	return false
}

type ValidatedFetchTarget struct {
	ParsedURL       *url.URL
	ResolvedAddress string
	HostHeader      string
	TLSServerName   string
	UseTLS          bool
}

func ValidateAndResolveFetchURL(ctx context.Context, urlString string) (*ValidatedFetchTarget, error) {
	parsed, err := url.Parse(urlString)
	if err != nil {
		return nil, fmt.Errorf("invalid URL: %s", urlString)
	}

	if !allowedProtocols[parsed.Scheme] {
		return nil, fmt.Errorf("disallowed protocol %q - only http and https are permitted", parsed.Scheme)
	}

	host := parsed.Hostname()
	hostHeader := parsed.Host

	resolver := &net.Resolver{}

	ctxWithTimeout, cancel := context.WithTimeout(ctx, DNSLookupTimeout)
	defer cancel()

	ips, err := resolver.LookupIPAddr(ctxWithTimeout, host)
	if err != nil {
		return nil, fmt.Errorf("DNS resolution failed for %s: %w", host, err)
	}

	if len(ips) == 0 {
		return nil, fmt.Errorf("DNS resolution returned no results for %s", host)
	}

	var resolvedIP string
	for _, ip := range ips {
		if !isPrivateIP(ip.String()) {
			resolvedIP = ip.String()
			break
		}
	}

	if resolvedIP == "" {
		return nil, fmt.Errorf("all resolved IPs for %s are in private/reserved ranges", host)
	}

	tlsServerName := ""
	if parsed.Scheme == "https" && net.ParseIP(host) == nil {
		tlsServerName = host
	}

	return &ValidatedFetchTarget{
		ParsedURL:       parsed,
		ResolvedAddress: resolvedIP,
		HostHeader:      hostHeader,
		TLSServerName:   tlsServerName,
		UseTLS:          parsed.Scheme == "https",
	}, nil
}

func BuildSafeHTTPClient(target *ValidatedFetchTarget) *http.Client {
	dialer := &net.Dialer{
		Timeout:   30 * time.Second,
		KeepAlive: 30 * time.Second,
	}

	port := target.ParsedURL.Port()
	if port == "" {
		if target.UseTLS {
			port = "443"
		} else {
			port = "80"
		}
	}

	addr := net.JoinHostPort(target.ResolvedAddress, port)

	transport := &http.Transport{
		DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
			return dialer.DialContext(ctx, network, addr)
		},
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}

	return &http.Client{
		Transport: transport,
		Timeout:   PluginFetchTimeout,
	}
}

func SafeFetch(ctx context.Context, method, urlStr string, body io.Reader, headers map[string]string) (*http.Response, error) {
	target, err := ValidateAndResolveFetchURL(ctx, urlStr)
	if err != nil {
		return nil, err
	}

	client := BuildSafeHTTPClient(target)

	reqURL := *target.ParsedURL

	req, err := http.NewRequestWithContext(ctx, method, reqURL.String(), body)
	if err != nil {
		return nil, err
	}

	req.Host = target.HostHeader

	for k, v := range headers {
		if strings.ToLower(k) == "host" {
			continue
		}
		req.Header.Set(k, v)
	}

	return client.Do(req)
}

// --- Config ---

func (s *PluginHostServices) ConfigGet(ctx context.Context, pluginID string) (map[string]interface{}, error) {
	configRow, err := s.Registry.GetConfig(ctx, pluginID)
	if err != nil {
		return nil, err
	}
	if configRow == nil {
		return map[string]interface{}{}, nil
	}
	var config map[string]interface{}
	if err := json.Unmarshal(configRow.ConfigJSON, &config); err != nil {
		return nil, err
	}
	return config, nil
}

// --- State ---

func (s *PluginHostServices) StateGet(ctx context.Context, pluginID string, params PluginStateParams) (interface{}, error) {
	if params.Namespace == "" {
		params.Namespace = "default"
	}
	return s.State.Get(ctx, pluginID, params)
}

func (s *PluginHostServices) StateSet(ctx context.Context, pluginID string, params struct {
	PluginStateParams
	Value interface{} `json:"value"`
}) error {
	if params.Namespace == "" {
		params.Namespace = "default"
	}
	return s.State.Set(ctx, pluginID, params.PluginStateParams, params.Value)
}

func (s *PluginHostServices) StateDelete(ctx context.Context, pluginID string, params PluginStateParams) error {
	if params.Namespace == "" {
		params.Namespace = "default"
	}
	return s.State.Delete(ctx, pluginID, params)
}

// --- Entities ---

func (s *PluginHostServices) EntitiesUpsert(ctx context.Context, pluginID string, params struct {
	EntityType string                 `json:"entityType"`
	ScopeKind  string                 `json:"scopeKind"`
	ScopeID    *string                `json:"scopeId"`
	ExternalID *string                `json:"externalId"`
	Title      *string                `json:"title"`
	Status     *string                `json:"status"`
	Data       map[string]interface{} `json:"data"`
}) (*models.PluginEntity, error) {
	dataJSON, _ := json.Marshal(params.Data)
	entity := &models.PluginEntity{
		EntityType: params.EntityType,
		ScopeKind:  params.ScopeKind,
		ScopeID:    params.ScopeID,
		ExternalID: params.ExternalID,
		Title:      params.Title,
		Status:     params.Status,
		Data:       datatypes.JSON(dataJSON),
	}
	return s.Registry.UpsertEntity(ctx, pluginID, entity)
}

func (s *PluginHostServices) EntitiesList(ctx context.Context, pluginID string, params struct {
	EntityType *string `json:"entityType"`
	ExternalID *string `json:"externalId"`
	Limit      int     `json:"limit"`
	Offset     int     `json:"offset"`
}) ([]models.PluginEntity, error) {
	return s.Registry.ListEntities(ctx, pluginID, PluginEntityQuery{
		EntityType: params.EntityType,
		ExternalID: params.ExternalID,
		Limit:      params.Limit,
		Offset:     params.Offset,
	})
}

// --- Events ---

func (s *PluginHostServices) EventsEmit(ctx context.Context, pluginID string, params struct {
	Name      string      `json:"name"`
	CompanyID string      `json:"companyId"`
	Payload   interface{} `json:"payload"`
}) error {
	s.EventBus.ForPlugin(pluginID).Emit(params.Name, params.CompanyID, params.Payload)
	return nil
}

// --- Events (continued) ---

func (s *PluginHostServices) EventsSubscribe(ctx context.Context, pluginID string, params struct {
	EventPattern string       `json:"eventPattern"`
	Filter       *EventFilter `json:"filter"`
}) error {
	if s.NotifyWorker == nil {
		return nil
	}
	s.EventBus.Subscribe(pluginID, params.EventPattern, params.Filter, func(event PluginEvent) {
		s.NotifyWorker(pluginID, "onEvent", map[string]interface{}{
			"event": event,
		})
	})
	return nil
}

// --- HTTP ---

func (s *PluginHostServices) HTTPFetch(ctx context.Context, pluginID string, params struct {
	URL  string `json:"url"`
	Init interface{} `json:"init"` // Simplified for now
}) (interface{}, error) {
	// Re-use logic from skeleton but wrap it for RPC response
	// Note: Node.js implementation handles RequestInit headers, method, body.
	// For now, I'll implement a basic version of this.
	// In a full implementation, I'd parse 'init' to extract method, headers, etc.
	resp, err := SafeFetch(ctx, "GET", params.URL, nil, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	headers := make(map[string]string)
	for k, v := range resp.Header {
		headers[k] = strings.Join(v, ", ")
	}

	return map[string]interface{}{
		"status":     resp.StatusCode,
		"statusText": resp.Status,
		"headers":    headers,
		"body":       string(body),
	}, nil
}

// --- Secrets ---

func (s *PluginHostServices) SecretsResolve(ctx context.Context, pluginID string, params struct {
	SecretRef string `json:"secretRef"`
	CompanyID string `json:"companyId"`
}) (string, error) {
	return s.SecretsHandler.ResolveSecret(ctx, pluginID, params.CompanyID, params.SecretRef)
}

func (s *PluginHostServices) SecretsList(ctx context.Context, pluginID string, params struct {
	CompanyID string `json:"companyId"`
}) ([]SecretMeta, error) {
	return s.SecretsHandler.ListAvailableSecrets(ctx, pluginID, params.CompanyID)
}

// --- Activity ---

func (s *PluginHostServices) ActivityLog(ctx context.Context, pluginID string, params struct {
	CompanyID  string                 `json:"companyId"`
	Message    string                 `json:"message"`
	EntityType string                 `json:"entityType"`
	EntityID   string                 `json:"entityId"`
	Metadata   map[string]interface{} `json:"metadata"`
}) error {
	if params.CompanyID == "" {
		return fmt.Errorf("companyId is required")
	}
	_, err := s.Activity.Log(ctx, LogEntry{
		CompanyID:  params.CompanyID,
		ActorType:  "plugin",
		ActorID:    pluginID,
		Action:     params.Message,
		EntityType: params.EntityType,
		EntityID:   params.EntityID,
		Details:    params.Metadata,
	})
	return err
}

// --- Metrics ---

func (s *PluginHostServices) MetricsWrite(ctx context.Context, pluginID string, params struct {
	Name  string                 `json:"name"`
	Value float64                `json:"value"`
	Tags  map[string]interface{} `json:"tags"`
}) error {
	s.appendLogBuffer(bufferedLogEntry{
		pluginID: pluginID,
		level:    "metric",
		message:  params.Name,
		meta:     map[string]interface{}{"value": params.Value, "tags": params.Tags},
	})
	return nil
}

// --- Telemetry ---

func (s *PluginHostServices) TelemetryTrack(ctx context.Context, pluginID string, params struct {
	EventName  string                 `json:"eventName"`
	Dimensions map[string]interface{} `json:"dimensions"`
}) error {
	if s.Telemetry == nil {
		return nil
	}
	dims := make(map[string]interface{})
	for k, v := range params.Dimensions {
		dims[k] = v
	}
	s.Telemetry.Track("plugin."+params.EventName, dims)
	return nil
}

// --- Logger ---

func (s *PluginHostServices) LoggerLog(ctx context.Context, pluginID string, params struct {
	Level   string                 `json:"level"`
	Message string                 `json:"message"`
	Meta    map[string]interface{} `json:"meta"`
}) error {
	s.appendLogBuffer(bufferedLogEntry{
		pluginID: pluginID,
		level:    params.Level,
		message:  params.Message,
		meta:     params.Meta,
	})
	return nil
}

// --- Internal Log Buffering ---

const (
	logBufferFlushSize     = 100
	logBufferFlushInterval = 5 * time.Second
)

type bufferedLogEntry struct {
	pluginID  string
	level     string
	message   string
	meta      interface{}
	createdAt time.Time
}

var (
	logBuffer   []bufferedLogEntry
	logBufferMu sync.Mutex
)

func (s *PluginHostServices) appendLogBuffer(entry bufferedLogEntry) {
	if entry.createdAt.IsZero() {
		entry.createdAt = time.Now()
	}
	logBufferMu.Lock()
	logBuffer = append(logBuffer, entry)
	shouldFlush := len(logBuffer) >= logBufferFlushSize
	logBufferMu.Unlock()

	if shouldFlush {
		go s.FlushLogs(context.Background())
	}
}

func (s *PluginHostServices) StartLogFlusher(ctx context.Context) {
	ticker := time.NewTicker(logBufferFlushInterval)
	go func() {
		for {
			select {
			case <-ticker.C:
				s.FlushLogs(context.Background())
			case <-ctx.Done():
				ticker.Stop()
				s.FlushLogs(context.Background()) // final flush
				return
			}
		}
	}()
}

func (s *PluginHostServices) FlushLogs(ctx context.Context) {
	logBufferMu.Lock()
	if len(logBuffer) == 0 {
		logBufferMu.Unlock()
		return
	}
	entries := logBuffer
	logBuffer = nil
	logBufferMu.Unlock()

	var modelsLogs []models.PluginLog
	for _, e := range entries {
		metaJSON, _ := json.Marshal(e.meta)
		modelsLogs = append(modelsLogs, models.PluginLog{
			PluginID:  e.pluginID,
			Level:     e.level,
			Message:   e.message,
			Meta:      datatypes.JSON(metaJSON),
			CreatedAt: e.createdAt,
		})
	}

	if err := s.DB.Create(&modelsLogs).Error; err != nil {
		fmt.Printf("failed to flush plugin logs: %v\n", err)
	}
}

// --- Internal Helpers ---

func (s *PluginHostServices) ensureCompanyID(companyID string) (string, error) {
	if companyID == "" {
		return "", fmt.Errorf("companyId is required for this operation")
	}
	return companyID, nil
}

func (s *PluginHostServices) requireInCompany(ctx context.Context, companyID string, entity interface{}, model interface{}) error {
	// Simple generic check using GORM
	err := s.DB.WithContext(ctx).Where("id = ? AND company_id = ?", entity, companyID).First(model).Error
	if err != nil {
		return fmt.Errorf("entity not found in company")
	}
	return nil
}

func (s *PluginHostServices) applyWindow(query *gorm.DB, params map[string]interface{}) *gorm.DB {
	if limit, ok := params["limit"].(float64); ok && limit > 0 {
		query = query.Limit(int(limit))
	}
	if offset, ok := params["offset"].(float64); ok && offset > 0 {
		query = query.Offset(int(offset))
	}
	return query
}

// --- Companies ---

func (s *PluginHostServices) CompaniesList(ctx context.Context, pluginID string, params map[string]interface{}) ([]models.Company, error) {
	var results []models.Company
	query := s.DB.WithContext(ctx)
	query = s.applyWindow(query, params)
	err := query.Find(&results).Error
	return results, err
}

func (s *PluginHostServices) CompaniesGet(ctx context.Context, pluginID string, params struct {
	CompanyID string `json:"companyId"`
}) (*models.Company, error) {
	var company models.Company
	err := s.DB.WithContext(ctx).First(&company, "id = ?", params.CompanyID).Error
	return &company, err
}

// --- Projects ---

func (s *PluginHostServices) ProjectsList(ctx context.Context, pluginID string, params map[string]interface{}) ([]models.Project, error) {
	cid, ok := params["companyId"].(string)
	if !ok || cid == "" {
		return nil, fmt.Errorf("companyId is required")
	}
	var results []models.Project
	query := s.DB.WithContext(ctx).Where("company_id = ?", cid)
	query = s.applyWindow(query, params)
	err := query.Find(&results).Error
	return results, err
}

func (s *PluginHostServices) ProjectsGet(ctx context.Context, pluginID string, params struct {
	CompanyID string `json:"companyId"`
	ProjectID string `json:"projectId"`
}) (*models.Project, error) {
	var project models.Project
	err := s.DB.WithContext(ctx).Where("id = ? AND company_id = ?", params.ProjectID, params.CompanyID).First(&project).Error
	return &project, err
}

// --- Workspaces ---

func (s *PluginHostServices) ProjectsListWorkspaces(ctx context.Context, pluginID string, params struct {
	CompanyID string `json:"companyId"`
	ProjectID string `json:"projectId"`
}) ([]models.ProjectWorkspace, error) {
	var results []models.ProjectWorkspace
	err := s.DB.WithContext(ctx).Where("project_id = ?", params.ProjectID).Find(&results).Error
	return results, err
}

// --- Issues ---

func (s *PluginHostServices) IssuesList(ctx context.Context, pluginID string, params map[string]interface{}) ([]models.Issue, error) {
	cid, ok := params["companyId"].(string)
	if !ok || cid == "" {
		return nil, fmt.Errorf("companyId is required")
	}
	return s.Issues.ListIssues(ctx, cid, params)
}

func (s *PluginHostServices) IssuesGet(ctx context.Context, pluginID string, params struct {
	CompanyID string `json:"companyId"`
	IssueID   string `json:"issueId"`
}) (*models.Issue, error) {
	var issue models.Issue
	err := s.DB.WithContext(ctx).Where("id = ? AND company_id = ?", params.IssueID, params.CompanyID).First(&issue).Error
	return &issue, err
}

func (s *PluginHostServices) IssuesCreate(ctx context.Context, pluginID string, params struct {
	CompanyID string `json:"companyId"`
	Issue     models.Issue `json:"issue"`
}) (*models.Issue, error) {
	params.Issue.CompanyID = params.CompanyID
	return s.Issues.CreateIssue(ctx, &params.Issue)
}

func (s *PluginHostServices) IssuesUpdate(ctx context.Context, pluginID string, params struct {
	CompanyID string `json:"companyId"`
	IssueID   string `json:"issueId"`
	Patch     map[string]interface{} `json:"patch"`
}) (*models.Issue, error) {
	var issue models.Issue
	if err := s.requireInCompany(ctx, params.CompanyID, params.IssueID, &issue); err != nil {
		return nil, err
	}
	if err := s.DB.WithContext(ctx).Model(&issue).Updates(params.Patch).Error; err != nil {
		return nil, err
	}
	return &issue, nil
}

// --- Issue Documents ---

func (s *PluginHostServices) IssueDocumentsList(ctx context.Context, pluginID string, params struct {
	CompanyID string `json:"companyId"`
	IssueID   string `json:"issueId"`
}) ([]models.IssueDocument, error) {
	var results []models.IssueDocument
	err := s.DB.WithContext(ctx).Where("issue_id = ?", params.IssueID).Find(&results).Error
	return results, err
}

func (s *PluginHostServices) IssueDocumentsUpsert(ctx context.Context, pluginID string, params struct {
	CompanyID string `json:"companyId"`
	IssueID   string `json:"issueId"`
	Document  models.IssueDocument `json:"document"`
}) (*models.IssueDocument, error) {
	params.Document.IssueID = params.IssueID
	err := s.DB.WithContext(ctx).Save(&params.Document).Error
	return &params.Document, err
}

// --- Agents ---

func (s *PluginHostServices) AgentsList(ctx context.Context, pluginID string, params map[string]interface{}) ([]models.Agent, error) {
	cid, ok := params["companyId"].(string)
	if !ok || cid == "" {
		return nil, fmt.Errorf("companyId is required")
	}
	var results []models.Agent
	query := s.DB.WithContext(ctx).Where("company_id = ?", cid)
	query = s.applyWindow(query, params)
	err := query.Find(&results).Error
	return results, err
}

func (s *PluginHostServices) AgentsGet(ctx context.Context, pluginID string, params struct {
	CompanyID string `json:"companyId"`
	AgentID   string `json:"agentId"`
}) (*models.Agent, error) {
	var agent models.Agent
	err := s.DB.WithContext(ctx).Where("id = ? AND company_id = ?", params.AgentID, params.CompanyID).First(&agent).Error
	return &agent, err
}

func (s *PluginHostServices) AgentsInvoke(ctx context.Context, pluginID string, params struct {
	CompanyID string      `json:"companyId"`
	AgentID   string      `json:"agentId"`
	Prompt    string      `json:"prompt"`
	Reason    string      `json:"reason"`
}) (interface{}, error) {
	run, err := s.Heartbeat.Wakeup(ctx, params.CompanyID, params.AgentID, WakeupOptions{
		Source:        "automation",
		TriggerDetail: "plugin_invocation",
		Context: map[string]interface{}{
			"prompt": params.Prompt,
			"reason": params.Reason,
		},
	})
	if err != nil {
		return nil, err
	}
	return map[string]interface{}{"runId": run.ID}, nil
}

// --- Goals ---

func (s *PluginHostServices) GoalsList(ctx context.Context, pluginID string, params map[string]interface{}) ([]models.Goal, error) {
	cid, ok := params["companyId"].(string)
	if !ok || cid == "" {
		return nil, fmt.Errorf("companyId is required")
	}
	var results []models.Goal
	query := s.DB.WithContext(ctx).Where("company_id = ?", cid)
	query = s.applyWindow(query, params)
	err := query.Find(&results).Error
	return results, err
}

func (s *PluginHostServices) GoalsCreate(ctx context.Context, pluginID string, params struct {
	CompanyID string      `json:"companyId"`
	Goal      models.Goal `json:"goal"`
}) (*models.Goal, error) {
	params.Goal.CompanyID = params.CompanyID
	return s.Goals.CreateGoal(ctx, params.CompanyID, &params.Goal)
}

// --- Agent Sessions ---

func (s *PluginHostServices) AgentSessionsCreate(ctx context.Context, pluginID string, params struct {
	CompanyID string `json:"companyId"`
	AgentID   string `json:"agentId"`
	TaskKey   string `json:"taskKey"`
}) (interface{}, error) {
	session := models.AgentTaskSession{
		CompanyID:   params.CompanyID,
		AgentID:     params.AgentID,
		TaskKey:     params.TaskKey,
		// ... more fields
	}
	if err := s.DB.WithContext(ctx).Create(&session).Error; err != nil {
		return nil, err
	}
	return map[string]interface{}{"sessionId": session.ID}, nil
}

func (s *PluginHostServices) AgentSessionsSendMessage(ctx context.Context, pluginID string, params struct {
	CompanyID string `json:"companyId"`
	SessionID string `json:"sessionId"`
	Prompt    string `json:"prompt"`
}) (interface{}, error) {
	var session models.AgentTaskSession
	if err := s.DB.WithContext(ctx).Where("id = ? AND company_id = ?", params.SessionID, params.CompanyID).First(&session).Error; err != nil {
		return nil, err
	}

	run, err := s.Heartbeat.Wakeup(ctx, params.CompanyID, session.AgentID, WakeupOptions{
		Source:        "automation",
		TriggerDetail: "session_message",
		Context: map[string]interface{}{
			"prompt":    params.Prompt,
			"sessionId": session.ID,
			"taskKey":   session.TaskKey,
		},
	})
	if err != nil {
		return nil, err
	}

	return map[string]interface{}{"runId": run.ID}, nil
}

// Dispose cleans up all active subscriptions and flushes the log buffer.
// Should be called when the plugin worker is stopped.
func (s *PluginHostServices) Dispose(pluginID string) {
	// 1. Clear event bus subscriptions for this plugin.
	if s.EventBus != nil {
		s.EventBus.Clear(pluginID)
	}

	// 2. Flush logs for this plugin.
	s.FlushLogs(context.Background())
}
