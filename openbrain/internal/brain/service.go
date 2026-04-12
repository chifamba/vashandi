package brain

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"hash/fnv"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	_ "github.com/mattn/go-sqlite3"
	"gorm.io/gorm"

	"github.com/chifamba/vashandi/openbrain/db/models"
)

const (
	DefaultAuthSecret        = "dev_secret_token"
	DefaultQueryLimit        = 10
	DefaultContextCandidateK = 50
)

type Service struct {
	DB             *gorm.DB
	Now            func() time.Time
	mu             sync.Mutex
	repoSyncHashes map[string]string
}

type Actor struct {
	Kind        string         `json:"kind"`
	NamespaceID string         `json:"namespaceId,omitempty"`
	AgentID     string         `json:"agentId,omitempty"`
	TrustTier   int            `json:"trustTier,omitempty"`
	Name        string         `json:"name,omitempty"`
	RequestMeta map[string]any `json:"requestMeta,omitempty"`
}

type ScopedTokenClaims struct {
	NamespaceID string `json:"namespaceId"`
	AgentID     string `json:"agentId,omitempty"`
	TrustTier   int    `json:"trustTier,omitempty"`
	ActorKind   string `json:"actorKind,omitempty"`
	Name        string `json:"name,omitempty"`
}

type RecallProfile struct {
	Verbosity      string   `json:"verbosity,omitempty"`
	Format         string   `json:"format,omitempty"`
	TokenLimit     int      `json:"tokenLimit,omitempty"`
	PreferredTypes []string `json:"preferredTypes,omitempty"`
}

type MemoryPayload struct {
	ID          string         `json:"id,omitempty"`
	NamespaceID string         `json:"namespaceId,omitempty"`
	TeamID      string         `json:"teamId,omitempty"`
	EntityType  string         `json:"entityType,omitempty"`
	Title       string         `json:"title,omitempty"`
	Text        string         `json:"text"`
	Metadata    map[string]any `json:"metadata,omitempty"`
	Provenance  map[string]any `json:"provenance,omitempty"`
	Identity    map[string]any `json:"identity,omitempty"`
	Tier        int            `json:"tier,omitempty"`
	Version     int            `json:"version"`
	AccessCount int            `json:"accessCount"`
	CreatedAt   time.Time      `json:"createdAt"`
	UpdatedAt   time.Time      `json:"updatedAt"`
}

type SearchRequest struct {
	NamespaceID  string
	Query        string
	Limit        int
	IncludeTypes []string
	AgentID      string
	Intent       string
}

type SearchResult struct {
	Memory MemoryPayload `json:"memory"`
	Score  float64       `json:"score"`
}

type ContextRequest struct {
	NamespaceID  string   `json:"namespaceId"`
	AgentID      string   `json:"agentId"`
	TaskQuery    string   `json:"taskQuery"`
	Intent       string   `json:"intent"`
	TokenBudget  int      `json:"tokenBudget"`
	IncludeTypes []string `json:"includeTypes,omitempty"`
}

type ContextSnippet struct {
	ID         string         `json:"id"`
	EntityType string         `json:"entityType"`
	Tier       int            `json:"tier"`
	Score      float64        `json:"score"`
	Text       string         `json:"text"`
	Summary    string         `json:"summary,omitempty"`
	Metadata   map[string]any `json:"metadata,omitempty"`
}

type ContextResponse struct {
	Snippets       []ContextSnippet `json:"snippets"`
	ProfileSummary string           `json:"profileSummary,omitempty"`
	TokenCount     int              `json:"tokenCount"`
	LatencyMs      int64            `json:"latencyMs"`
	Usage          []map[string]any `json:"usage,omitempty"`
	Rendered       string           `json:"rendered,omitempty"`
}

type ProposalPayload struct {
	ID            string         `json:"id"`
	NamespaceID   string         `json:"namespaceId"`
	ProposalType  string         `json:"proposalType"`
	MemoryIDs     []string       `json:"memoryIds"`
	Summary       string         `json:"summary"`
	SuggestedText string         `json:"suggestedText"`
	SuggestedTier *int           `json:"suggestedTier,omitempty"`
	Status        string         `json:"status"`
	Details       map[string]any `json:"details,omitempty"`
	ReviewedBy    string         `json:"reviewedBy,omitempty"`
	CreatedAt     time.Time      `json:"createdAt"`
	UpdatedAt     time.Time      `json:"updatedAt"`
}

type DashboardMetrics struct {
	Thoughts               int64           `json:"thoughts"`
	Memories               int64           `json:"memories"`
	PendingProposals       int64           `json:"pendingProposals"`
	TierDistribution       map[int]int64   `json:"tierDistribution"`
	StaleMemoryRatio       float64         `json:"staleMemoryRatio"`
	ProposalAcceptanceRate float64         `json:"proposalAcceptanceRate"`
	KnowledgeGapCount      int64           `json:"knowledgeGapCount"`
	TopAccessed            []MemoryPayload `json:"topAccessed"`
}

type TriggerRequest struct {
	AgentID     string         `json:"agentId,omitempty"`
	TaskQuery   string         `json:"taskQuery,omitempty"`
	Intent      string         `json:"intent,omitempty"`
	TokenBudget int            `json:"tokenBudget,omitempty"`
	Content     string         `json:"content,omitempty"`
	Summary     string         `json:"summary,omitempty"`
	Metadata    map[string]any `json:"metadata,omitempty"`
	ErrorText   string         `json:"errorText,omitempty"`
}

type TriggerResponse struct {
	Status      string           `json:"status"`
	PacketID    string           `json:"packetId,omitempty"`
	Context     *ContextResponse `json:"context,omitempty"`
	CreatedIDs  []string         `json:"createdIds,omitempty"`
	ProposalIDs []string         `json:"proposalIds,omitempty"`
}

func NewService(db *gorm.DB) *Service {
	return &Service{DB: db, Now: time.Now, repoSyncHashes: map[string]string{}}
}

func (s *Service) AutoMigrate() error {
	if err := s.DB.AutoMigrate(
		&models.Namespace{},
		&models.Memory{},
		&models.MemoryVersion{},
		&models.Edge{},
		&models.RegisteredAgent{},
		&models.Proposal{},
		&models.AuditLog{},
		&models.ContextPacket{},
	); err != nil {
		return err
	}
	stmts := []string{
		"CREATE UNIQUE INDEX IF NOT EXISTS idx_registered_agents_namespace_agent_active ON registered_agents(namespace_id, vashandi_agent_id) WHERE is_active = true",
		"CREATE INDEX IF NOT EXISTS idx_memory_entities_namespace_created ON memory_entities(namespace_id, created_at DESC)",
		"CREATE INDEX IF NOT EXISTS idx_curator_proposals_namespace_status ON curator_proposals(namespace_id, status)",
	}
	for _, stmt := range stmts {
		if err := s.DB.Exec(stmt).Error; err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) StartBackgroundJobs(ctx context.Context) {
	promotionTicker := time.NewTicker(6 * time.Hour)
	decayTicker := time.NewTicker(24 * time.Hour)
	curatorTicker := time.NewTicker(7 * 24 * time.Hour)
	go func() {
		defer promotionTicker.Stop()
		defer decayTicker.Stop()
		defer curatorTicker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-promotionTicker.C:
				_, _ = s.RunPromotionScan(ctx, Actor{Kind: "system"})
			case <-decayTicker.C:
				_, _ = s.RunDecayScan(ctx, Actor{Kind: "system"})
			case <-curatorTicker.C:
				_ = s.GenerateHealthReport(ctx)
			}
		}
	}()
}

func AuthSecret() string {
	if secret := os.Getenv("OPENBRAIN_SIGNING_SECRET"); secret != "" {
		return secret
	}
	if secret := os.Getenv("OPENBRAIN_API_KEY"); secret != "" {
		return secret
	}
	return DefaultAuthSecret
}

func SignScopedToken(claims ScopedTokenClaims) (string, error) {
	payload, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}
	enc := base64.RawURLEncoding.EncodeToString(payload)
	mac := hmac.New(sha256.New, []byte(AuthSecret()))
	mac.Write([]byte(enc))
	sig := hex.EncodeToString(mac.Sum(nil))
	return "openbrain." + enc + "." + sig, nil
}

func ParseScopedToken(token string) (*ScopedTokenClaims, bool) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 || parts[0] != "openbrain" {
		return nil, false
	}
	mac := hmac.New(sha256.New, []byte(AuthSecret()))
	mac.Write([]byte(parts[1]))
	wantSig := hex.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(wantSig), []byte(parts[2])) {
		return nil, false
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, false
	}
	var claims ScopedTokenClaims
	if err := json.Unmarshal(payload, &claims); err != nil {
		return nil, false
	}
	if claims.ActorKind == "" {
		claims.ActorKind = "service"
	}
	return &claims, true
}

func LegacyTokenValid(token string) bool {
	return token == AuthSecret()
}

func (s *Service) EnsureNamespace(ctx context.Context, namespaceID string) error {
	if namespaceID == "" {
		return errors.New("namespace id is required")
	}
	ns := models.Namespace{ID: namespaceID, CompanyID: namespaceID, Settings: "{}"}
	return s.DB.WithContext(ctx).FirstOrCreate(&ns, models.Namespace{ID: namespaceID}).Error
}

func (s *Service) RegisterAgent(ctx context.Context, actor Actor, namespaceID, agentID, name string, trustTier int, profile RecallProfile) (models.RegisteredAgent, error) {
	if err := s.EnsureNamespace(ctx, namespaceID); err != nil {
		return models.RegisteredAgent{}, err
	}
	if trustTier < 1 || trustTier > 4 {
		trustTier = 1
	}
	id := uuid.NewString()
	if agentID == "" {
		agentID = id
	}
	profileJSON := mustJSON(profile, "{}")
	now := s.Now().UTC()
	reg := models.RegisteredAgent{ID: id, NamespaceID: namespaceID, VashandiAgentID: agentID, Name: firstNonEmpty(name, agentID), TrustTier: trustTier, RecallProfile: profileJSON, IsActive: true, RegisteredAt: now}
	var existing models.RegisteredAgent
	err := s.DB.WithContext(ctx).Where("namespace_id = ? AND vashandi_agent_id = ? AND is_active = ?", namespaceID, agentID, true).First(&existing).Error
	if err == nil {
		existing.Name = reg.Name
		existing.TrustTier = trustTier
		existing.RecallProfile = profileJSON
		if err := s.DB.WithContext(ctx).Save(&existing).Error; err != nil {
			return models.RegisteredAgent{}, err
		}
		_ = s.appendAudit(ctx, actor, namespaceID, "register_agent", agentID, "registered_agent", "", hashJSON(existing), map[string]any{"name": name, "trustTier": trustTier})
		return existing, nil
	}
	if err := s.DB.WithContext(ctx).Create(&reg).Error; err != nil {
		return models.RegisteredAgent{}, err
	}
	_ = s.appendAudit(ctx, actor, namespaceID, "register_agent", agentID, "registered_agent", "", hashJSON(reg), map[string]any{"name": name, "trustTier": trustTier})
	return reg, nil
}

func (s *Service) DeregisterAgent(ctx context.Context, actor Actor, namespaceID, agentID string) error {
	now := s.Now().UTC()
	updates := map[string]any{"is_active": false, "deregistered_at": &now}
	if err := s.DB.WithContext(ctx).Model(&models.RegisteredAgent{}).Where("namespace_id = ? AND vashandi_agent_id = ? AND is_active = ?", namespaceID, agentID, true).Updates(updates).Error; err != nil {
		return err
	}
	return s.appendAudit(ctx, actor, namespaceID, "deregister_agent", agentID, "registered_agent", "", "", nil)
}

func (s *Service) GetRegisteredAgent(ctx context.Context, namespaceID, agentID string) (*models.RegisteredAgent, error) {
	var agent models.RegisteredAgent
	if err := s.DB.WithContext(ctx).Where("namespace_id = ? AND vashandi_agent_id = ? AND is_active = ?", namespaceID, agentID, true).First(&agent).Error; err != nil {
		return nil, err
	}
	return &agent, nil
}

func (s *Service) LoadRecallProfile(ctx context.Context, namespaceID, agentID string) RecallProfile {
	if agentID == "" {
		return RecallProfile{Format: "markdown", TokenLimit: 800, Verbosity: "balanced"}
	}
	agent, err := s.GetRegisteredAgent(ctx, namespaceID, agentID)
	if err != nil {
		return RecallProfile{Format: "markdown", TokenLimit: 800, Verbosity: "balanced"}
	}
	var profile RecallProfile
	_ = json.Unmarshal([]byte(agent.RecallProfile), &profile)
	if profile.Format == "" {
		profile.Format = "markdown"
	}
	if profile.TokenLimit <= 0 {
		profile.TokenLimit = 800
	}
	if profile.Verbosity == "" {
		profile.Verbosity = "balanced"
	}
	return profile
}

func (s *Service) CreateMemory(ctx context.Context, actor Actor, input MemoryPayload) (models.Memory, error) {
	if input.NamespaceID == "" {
		input.NamespaceID = actor.NamespaceID
	}
	if err := s.EnsureNamespace(ctx, input.NamespaceID); err != nil {
		return models.Memory{}, err
	}
	if input.EntityType == "" {
		input.EntityType = entityTypeFromMetadata(input.Metadata)
	}
	input.Tier = clampTier(input.Tier)
	if err := s.requireWriteTrust(actor, input.Tier); err != nil {
		return models.Memory{}, err
	}
	now := s.Now().UTC()
	if input.ID == "" {
		input.ID = uuid.NewString()
	}
	identity := input.Identity
	if identity == nil {
		identity = map[string]any{}
	}
	if identity["createdByAgentId"] == nil && actor.AgentID != "" {
		identity["createdByAgentId"] = actor.AgentID
	}
	if identity["createdVia"] == nil {
		identity["createdVia"] = firstNonEmpty(actor.Kind, "api")
	}
	embedding := encodeEmbedding(generateEmbedding(strings.TrimSpace(input.Title + " " + input.Text)))
	memory := models.Memory{
		ID:             input.ID,
		NamespaceID:    input.NamespaceID,
		TeamID:         input.TeamID,
		EntityType:     firstNonEmpty(input.EntityType, "note"),
		Title:          input.Title,
		Text:           strings.TrimSpace(input.Text),
		Embedding:      embedding,
		Metadata:       mustJSON(input.Metadata, "{}"),
		Provenance:     mustJSON(defaultMap(input.Provenance), "{}"),
		Identity:       mustJSON(identity, "{}"),
		Tier:           input.Tier,
		Version:        1,
		CreatedAt:      now,
		UpdatedAt:      now,
		LastAccessedAt: nil,
	}
	if memory.Text == "" {
		return models.Memory{}, errors.New("text is required")
	}
	if decayAt := computeDecayAt(memory.Tier, now); !decayAt.IsZero() {
		memory.DecayAt = &decayAt
	}
	if err := s.DB.WithContext(ctx).Create(&memory).Error; err != nil {
		return models.Memory{}, err
	}
	if err := s.saveVersion(ctx, memory, actor, "create"); err != nil {
		return models.Memory{}, err
	}
	_ = s.appendAudit(ctx, actor, memory.NamespaceID, "write", memory.ID, memory.EntityType, "", hashMemory(memory), input.Metadata)
	return memory, nil
}

func (s *Service) GetMemory(ctx context.Context, actor Actor, namespaceID, memoryID string) (models.Memory, error) {
	var memory models.Memory
	if err := s.DB.WithContext(ctx).Where("namespace_id = ? AND id = ?", namespaceID, memoryID).First(&memory).Error; err != nil {
		return models.Memory{}, err
	}
	if memory.IsDeleted {
		return models.Memory{}, gorm.ErrRecordNotFound
	}
	_ = s.touchMemories(ctx, actor, []models.Memory{memory})
	_ = s.appendAudit(ctx, actor, namespaceID, "read", memory.ID, memory.EntityType, "", hashMemory(memory), nil)
	return s.redactMemoryForActor(memory, actor), nil
}

func (s *Service) BrowseMemories(ctx context.Context, actor Actor, namespaceID, entityType string, tier *int, includeDeleted bool, limit int) ([]models.Memory, error) {
	if limit <= 0 || limit > 200 {
		limit = 100
	}
	q := s.DB.WithContext(ctx).Model(&models.Memory{}).Where("namespace_id = ?", namespaceID)
	if entityType != "" {
		q = q.Where("entity_type = ?", entityType)
	}
	if tier != nil {
		q = q.Where("tier = ?", *tier)
	}
	if !includeDeleted {
		q = q.Where("is_deleted = ?", false)
	}
	var memories []models.Memory
	if err := q.Order("updated_at desc").Limit(limit).Find(&memories).Error; err != nil {
		return nil, err
	}
	_ = s.touchMemories(ctx, actor, memories)
	for i := range memories {
		memories[i] = s.redactMemoryForActor(memories[i], actor)
	}
	_ = s.appendAudit(ctx, actor, namespaceID, "browse", "", "memory", "", "", map[string]any{"count": len(memories), "entityType": entityType})
	return memories, nil
}

func (s *Service) UpdateMemory(ctx context.Context, actor Actor, namespaceID, memoryID string, patch map[string]any) (models.Memory, error) {
	var memory models.Memory
	if err := s.DB.WithContext(ctx).Where("namespace_id = ? AND id = ?", namespaceID, memoryID).First(&memory).Error; err != nil {
		return models.Memory{}, err
	}
	if err := s.requireWriteTrust(actor, memory.Tier); err != nil {
		return models.Memory{}, err
	}
	beforeHash := hashMemory(memory)
	if v, ok := patch["title"].(string); ok {
		memory.Title = strings.TrimSpace(v)
	}
	if v, ok := patch["text"].(string); ok && strings.TrimSpace(v) != "" {
		memory.Text = strings.TrimSpace(v)
		memory.Embedding = encodeEmbedding(generateEmbedding(memory.Title + " " + memory.Text))
	}
	if v, ok := patch["entityType"].(string); ok && v != "" {
		memory.EntityType = v
	}
	if v, ok := patch["tier"].(float64); ok {
		tier := clampTier(int(v))
		if err := s.requireWriteTrust(actor, tier); err != nil {
			return models.Memory{}, err
		}
		memory.Tier = tier
	}
	if v, ok := patch["metadata"].(map[string]any); ok {
		memory.Metadata = mustJSON(v, "{}")
	}
	if v, ok := patch["provenance"].(map[string]any); ok {
		memory.Provenance = mustJSON(v, "{}")
	}
	if v, ok := patch["identity"].(map[string]any); ok {
		memory.Identity = mustJSON(v, "{}")
	}
	memory.Version++
	memory.UpdatedAt = s.Now().UTC()
	if decayAt := computeDecayAt(memory.Tier, memory.UpdatedAt); !decayAt.IsZero() {
		memory.DecayAt = &decayAt
	}
	if err := s.DB.WithContext(ctx).Save(&memory).Error; err != nil {
		return models.Memory{}, err
	}
	if err := s.saveVersion(ctx, memory, actor, "update"); err != nil {
		return models.Memory{}, err
	}
	_ = s.appendAudit(ctx, actor, namespaceID, "update", memory.ID, memory.EntityType, beforeHash, hashMemory(memory), patch)
	return s.redactMemoryForActor(memory, actor), nil
}

func (s *Service) RollbackMemory(ctx context.Context, actor Actor, namespaceID, memoryID string, version int) (models.Memory, error) {
	var snapshot models.MemoryVersion
	if err := s.DB.WithContext(ctx).Where("namespace_id = ? AND entity_id = ? AND version = ?", namespaceID, memoryID, version).First(&snapshot).Error; err != nil {
		return models.Memory{}, err
	}
	var memory models.Memory
	if err := s.DB.WithContext(ctx).Where("namespace_id = ? AND id = ?", namespaceID, memoryID).First(&memory).Error; err != nil {
		return models.Memory{}, err
	}
	beforeHash := hashMemory(memory)
	memory.Title = snapshot.Title
	memory.Text = snapshot.Text
	memory.Metadata = snapshot.Metadata
	memory.Provenance = snapshot.Provenance
	memory.Identity = snapshot.Identity
	memory.Embedding = snapshot.Embedding
	memory.Tier = snapshot.Tier
	memory.Version++
	memory.IsDeleted = false
	memory.UpdatedAt = s.Now().UTC()
	if err := s.DB.WithContext(ctx).Save(&memory).Error; err != nil {
		return models.Memory{}, err
	}
	if err := s.saveVersion(ctx, memory, actor, fmt.Sprintf("rollback:%d", version)); err != nil {
		return models.Memory{}, err
	}
	_ = s.appendAudit(ctx, actor, namespaceID, "rollback", memory.ID, memory.EntityType, beforeHash, hashMemory(memory), map[string]any{"rolledBackToVersion": version})
	return s.redactMemoryForActor(memory, actor), nil
}

func (s *Service) SoftDeleteMemory(ctx context.Context, actor Actor, namespaceID, memoryID string) error {
	var memory models.Memory
	if err := s.DB.WithContext(ctx).Where("namespace_id = ? AND id = ?", namespaceID, memoryID).First(&memory).Error; err != nil {
		return err
	}
	if err := s.requireDeleteTrust(actor); err != nil {
		return err
	}
	memory.IsDeleted = true
	memory.Version++
	memory.UpdatedAt = s.Now().UTC()
	if err := s.DB.WithContext(ctx).Save(&memory).Error; err != nil {
		return err
	}
	if err := s.saveVersion(ctx, memory, actor, "delete"); err != nil {
		return err
	}
	return s.appendAudit(ctx, actor, namespaceID, "delete", memory.ID, memory.EntityType, hashMemory(memory), "", nil)
}

func (s *Service) ForgetMemories(ctx context.Context, actor Actor, namespaceID string, ids []string) (int64, error) {
	var affected int64
	for _, id := range ids {
		if err := s.SoftDeleteMemory(ctx, actor, namespaceID, id); err == nil {
			affected++
		}
	}
	_ = s.appendAudit(ctx, actor, namespaceID, "forget", "", "memory", "", "", map[string]any{"ids": ids, "count": affected})
	return affected, nil
}

func (s *Service) CreateEdge(ctx context.Context, actor Actor, namespaceID, fromID, toID, edgeType string, weight float64, metadata map[string]any) (models.Edge, error) {
	if weight == 0 {
		weight = 1
	}
	edge := models.Edge{ID: uuid.NewString(), NamespaceID: namespaceID, FromEntityID: fromID, ToEntityID: toID, EdgeType: firstNonEmpty(edgeType, "relates_to"), Weight: weight, Metadata: mustJSON(metadata, "{}"), CreatedAt: s.Now().UTC()}
	if err := s.DB.WithContext(ctx).Create(&edge).Error; err != nil {
		return models.Edge{}, err
	}
	_ = s.appendAudit(ctx, actor, namespaceID, "link", edge.ID, edge.EdgeType, "", hashJSON(edge), metadata)
	return edge, nil
}

func (s *Service) GetEdges(ctx context.Context, actor Actor, namespaceID, memoryID string) ([]models.Edge, error) {
	var edges []models.Edge
	if err := s.DB.WithContext(ctx).Where("namespace_id = ? AND (from_entity_id = ? OR to_entity_id = ?)", namespaceID, memoryID, memoryID).Order("created_at desc").Find(&edges).Error; err != nil {
		return nil, err
	}
	_ = s.appendAudit(ctx, actor, namespaceID, "read_edges", memoryID, "edge", "", "", map[string]any{"count": len(edges)})
	return edges, nil
}

func (s *Service) SearchMemories(ctx context.Context, actor Actor, req SearchRequest) ([]SearchResult, error) {
	if req.Limit <= 0 || req.Limit > 100 {
		req.Limit = DefaultQueryLimit
	}
	q := s.DB.WithContext(ctx).Model(&models.Memory{}).Where("namespace_id = ? AND is_deleted = ?", req.NamespaceID, false)
	if len(req.IncludeTypes) > 0 {
		q = q.Where("entity_type IN ?", req.IncludeTypes)
	}
	var memories []models.Memory
	if err := q.Find(&memories).Error; err != nil {
		return nil, err
	}
	queryEmbedding := generateEmbedding(req.Query)
	tokens := tokenize(req.Query)
	results := make([]SearchResult, 0, len(memories))
	for _, memory := range memories {
		semantic := cosine(queryEmbedding, decodeEmbedding(memory.Embedding))
		lexical := lexicalScore(tokens, memory.Text+" "+memory.Title)
		recency := recencyWeight(memory.LastAccessedAt, memory.UpdatedAt, s.Now())
		bonus := tierWeight(memory.Tier)
		score := semantic*0.55 + lexical*0.25 + recency*0.1 + bonus*0.1
		if strings.TrimSpace(req.Query) == "" {
			score = recency + bonus
		}
		if score <= 0 && strings.TrimSpace(req.Query) != "" {
			continue
		}
		results = append(results, SearchResult{Memory: toPayload(s.redactMemoryForActor(memory, actor)), Score: score})
	}
	sort.Slice(results, func(i, j int) bool {
		if results[i].Score == results[j].Score {
			return results[i].Memory.UpdatedAt.After(results[j].Memory.UpdatedAt)
		}
		return results[i].Score > results[j].Score
	})
	if len(results) > req.Limit {
		results = results[:req.Limit]
	}
	returned := make([]models.Memory, 0, len(results))
	for _, result := range results {
		var memory models.Memory
		if err := s.DB.WithContext(ctx).Where("namespace_id = ? AND id = ?", req.NamespaceID, result.Memory.ID).First(&memory).Error; err == nil {
			returned = append(returned, memory)
		}
	}
	_ = s.touchMemories(ctx, actor, returned)
	meta := map[string]any{"query": req.Query, "count": len(results), "includeTypes": req.IncludeTypes, "intent": req.Intent}
	afterHash := fmt.Sprintf("results:%d", len(results))
	_ = s.appendAudit(ctx, actor, req.NamespaceID, "search", "", "memory", "", afterHash, meta)
	return results, nil
}

func (s *Service) CompileContext(ctx context.Context, actor Actor, req ContextRequest) (ContextResponse, error) {
	start := time.Now()
	profile := s.LoadRecallProfile(ctx, req.NamespaceID, req.AgentID)
	limit := DefaultContextCandidateK
	results, err := s.SearchMemories(ctx, actor, SearchRequest{NamespaceID: req.NamespaceID, Query: req.TaskQuery, Limit: limit, IncludeTypes: combinePreferredTypes(req.IncludeTypes, profile.PreferredTypes), AgentID: req.AgentID, Intent: req.Intent})
	if err != nil {
		return ContextResponse{}, err
	}
	budget := req.TokenBudget
	if budget <= 0 {
		budget = profile.TokenLimit
	}
	var snippets []ContextSnippet
	tokenCount := 0
	for _, result := range results {
		cost := estimateTokens(result.Memory.Text)
		if budget > 0 && tokenCount+cost > budget {
			continue
		}
		snippets = append(snippets, ContextSnippet{ID: result.Memory.ID, EntityType: result.Memory.EntityType, Tier: result.Memory.Tier, Score: result.Score, Text: result.Memory.Text, Summary: summarizeText(result.Memory.Text, profile.Verbosity), Metadata: result.Memory.Metadata})
		tokenCount += cost
	}
	response := ContextResponse{Snippets: snippets, ProfileSummary: fmt.Sprintf("format=%s verbosity=%s preferred_types=%s", profile.Format, profile.Verbosity, strings.Join(profile.PreferredTypes, ",")), TokenCount: tokenCount, LatencyMs: time.Since(start).Milliseconds(), Usage: []map[string]any{{"provider": "openbrain", "model": "local-semantic-hash", "inputTokens": estimateTokens(req.TaskQuery), "outputTokens": tokenCount}}}
	response.Rendered = renderContext(profile.Format, snippets)
	return response, nil
}

func (s *Service) GetPendingContext(ctx context.Context, actor Actor, namespaceID, agentID string) (*models.ContextPacket, error) {
	var packet models.ContextPacket
	if err := s.DB.WithContext(ctx).Where("namespace_id = ? AND agent_id = ? AND delivered_at IS NULL", namespaceID, agentID).Order("created_at desc").First(&packet).Error; err != nil {
		return nil, err
	}
	now := s.Now().UTC()
	packet.DeliveredAt = &now
	if err := s.DB.WithContext(ctx).Save(&packet).Error; err != nil {
		return nil, err
	}
	_ = s.appendAudit(ctx, actor, namespaceID, "read_pending_context", packet.ID, "context_packet", "", "", map[string]any{"agentId": agentID})
	return &packet, nil
}

func (s *Service) HandleTrigger(ctx context.Context, actor Actor, namespaceID, triggerType string, req TriggerRequest) (TriggerResponse, error) {
	switch triggerType {
	case "run_start", "checkout", "branch_creation", "test_failure":
		query := firstNonEmpty(req.TaskQuery, req.ErrorText, req.Content, req.Summary)
		includeTypes := []string(nil)
		if triggerType == "branch_creation" {
			includeTypes = []string{"adr", "constraint", "decision"}
		}
		contextResp, err := s.CompileContext(ctx, actor, ContextRequest{NamespaceID: namespaceID, AgentID: req.AgentID, TaskQuery: query, Intent: firstNonEmpty(req.Intent, triggerType), TokenBudget: req.TokenBudget, IncludeTypes: includeTypes})
		if err != nil {
			return TriggerResponse{}, err
		}
		packetID, err := s.storeContextPacket(ctx, actor, namespaceID, req.AgentID, triggerType, contextResp)
		if err != nil {
			return TriggerResponse{}, err
		}
		return TriggerResponse{Status: triggerType + "_triggered", PacketID: packetID, Context: &contextResp}, nil
	case "run_complete":
		content := strings.TrimSpace(firstNonEmpty(req.Content, req.Summary))
		if content == "" {
			return TriggerResponse{Status: "run_complete_triggered"}, nil
		}
		memory, err := s.CreateMemory(ctx, actor, MemoryPayload{NamespaceID: namespaceID, EntityType: "note", Text: content, Metadata: mergeMap(req.Metadata, map[string]any{"triggerType": triggerType}), Provenance: map[string]any{"kind": "trigger", "triggerType": triggerType}, Tier: 0})
		if err != nil {
			return TriggerResponse{}, err
		}
		proposals, _ := s.GenerateCuratorProposals(ctx, namespaceID, actor)
		ids := []string{}
		for _, proposal := range proposals {
			ids = append(ids, proposal.ID)
		}
		return TriggerResponse{Status: "run_complete_triggered", CreatedIDs: []string{memory.ID}, ProposalIDs: ids}, nil
	default:
		return TriggerResponse{}, fmt.Errorf("unsupported trigger type: %s", triggerType)
	}
}

func (s *Service) ListProposals(ctx context.Context, actor Actor, namespaceID, status string) ([]ProposalPayload, error) {
	q := s.DB.WithContext(ctx).Model(&models.Proposal{}).Where("namespace_id = ?", namespaceID)
	if status != "" {
		q = q.Where("status = ?", status)
	}
	var proposals []models.Proposal
	if err := q.Order("created_at desc").Find(&proposals).Error; err != nil {
		return nil, err
	}
	result := make([]ProposalPayload, 0, len(proposals))
	for _, proposal := range proposals {
		result = append(result, toProposalPayload(proposal))
	}
	_ = s.appendAudit(ctx, actor, namespaceID, "browse_proposals", "", "proposal", "", "", map[string]any{"count": len(result)})
	return result, nil
}

func (s *Service) GenerateCuratorProposals(ctx context.Context, namespaceID string, actor Actor) ([]models.Proposal, error) {
	var memories []models.Memory
	if err := s.DB.WithContext(ctx).Where("namespace_id = ? AND is_deleted = ?", namespaceID, false).Find(&memories).Error; err != nil {
		return nil, err
	}
	var proposals []models.Proposal
	proposals = append(proposals, s.dedupProposals(ctx, namespaceID, memories)...)
	proposals = append(proposals, s.synthesisProposals(ctx, namespaceID, memories)...)
	proposals = append(proposals, s.demotionProposals(ctx, namespaceID, memories)...)
	proposals = append(proposals, s.gapProposals(ctx, namespaceID)...)
	_ = s.appendAudit(ctx, actor, namespaceID, "curator_generate", "", "proposal", "", fmt.Sprintf("proposals:%d", len(proposals)), nil)
	return proposals, nil
}

func (s *Service) ResolveProposal(ctx context.Context, actor Actor, namespaceID, proposalID, action string) (ProposalPayload, error) {
	var proposal models.Proposal
	if err := s.DB.WithContext(ctx).Where("namespace_id = ? AND id = ?", namespaceID, proposalID).First(&proposal).Error; err != nil {
		return ProposalPayload{}, err
	}
	if action != "approve" && action != "reject" {
		return ProposalPayload{}, errors.New("action must be approve or reject")
	}
	now := s.Now().UTC()
	proposal.Status = action
	proposal.ReviewedBy = actor.AgentID
	proposal.ReviewedAt = &now
	if err := s.DB.WithContext(ctx).Save(&proposal).Error; err != nil {
		return ProposalPayload{}, err
	}
	if action == "approve" {
		if err := s.applyProposal(ctx, actor, proposal); err != nil {
			return ProposalPayload{}, err
		}
	}
	_ = s.appendAudit(ctx, actor, namespaceID, "proposal_"+action, proposal.ID, proposal.ProposalType, "", hashJSON(proposal), nil)
	return toProposalPayload(proposal), nil
}

func (s *Service) RunPromotionScan(ctx context.Context, actor Actor) ([]models.Memory, error) {
	var memories []models.Memory
	if err := s.DB.WithContext(ctx).Where("is_deleted = ?", false).Find(&memories).Error; err != nil {
		return nil, err
	}
	var promoted []models.Memory
	for _, memory := range memories {
		updated, ok, err := s.evaluatePromotion(ctx, actor, memory)
		if err != nil {
			return nil, err
		}
		if ok {
			promoted = append(promoted, updated)
		}
	}
	return promoted, nil
}

func (s *Service) RunDecayScan(ctx context.Context, actor Actor) ([]models.Memory, error) {
	var memories []models.Memory
	if err := s.DB.WithContext(ctx).Where("is_deleted = ?", false).Find(&memories).Error; err != nil {
		return nil, err
	}
	now := s.Now().UTC()
	var changed []models.Memory
	for _, memory := range memories {
		before := hashMemory(memory)
		modified := false
		switch memory.Tier {
		case 0:
			if memory.CreatedAt.Before(now.Add(-24 * time.Hour)) {
				memory.IsDeleted = true
				modified = true
			}
		case 1:
			last := memory.UpdatedAt
			if memory.LastAccessedAt != nil {
				last = *memory.LastAccessedAt
			}
			if last.Before(now.Add(-30 * 24 * time.Hour)) {
				memory.Tier = 0
				if decayAt := computeDecayAt(0, now); !decayAt.IsZero() {
					memory.DecayAt = &decayAt
				}
				modified = true
			}
		}
		if modified {
			memory.Version++
			memory.UpdatedAt = now
			if err := s.DB.WithContext(ctx).Save(&memory).Error; err != nil {
				return nil, err
			}
			if err := s.saveVersion(ctx, memory, actor, "decay"); err != nil {
				return nil, err
			}
			_ = s.appendAudit(ctx, actor, memory.NamespaceID, "decay", memory.ID, memory.EntityType, before, hashMemory(memory), nil)
			changed = append(changed, memory)
		}
	}
	return changed, nil
}

func (s *Service) SyncRepositoryDir(ctx context.Context, actor Actor, namespaceID, dir string) ([]models.Memory, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.EnsureNamespace(ctx, namespaceID); err != nil {
		return nil, err
	}
	var synced []models.Memory
	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			name := d.Name()
			if strings.HasPrefix(name, ".") && name != ".openbrain" && path != dir {
				return filepath.SkipDir
			}
			return nil
		}
		rel, _ := filepath.Rel(dir, path)
		rel = filepath.ToSlash(rel)
		if !strings.HasSuffix(rel, ".openbrain/brain.md") && !strings.HasSuffix(rel, ".openbrain/session.md") {
			return nil
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		hash := hashString(string(content))
		lastHash := s.repoSyncHashes[namespaceID+":"+rel]
		s.repoSyncHashes[namespaceID+":"+rel] = hash
		if lastHash == hash {
			return nil
		}
		kind := "session"
		tier := 1
		if strings.HasSuffix(rel, "brain.md") {
			kind = "file_sync"
			tier = 2
		}
		memory, err := s.upsertSyncedMemory(ctx, actor, namespaceID, rel, string(content), kind, tier)
		if err != nil {
			return err
		}
		synced = append(synced, memory)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return synced, nil
}

func (s *Service) WatchRepositoryDir(ctx context.Context, actor Actor, namespaceID, dir string, interval time.Duration, fn func([]models.Memory, error)) {
	if interval <= 0 {
		interval = 5 * time.Second
	}
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				memories, err := s.SyncRepositoryDir(ctx, actor, namespaceID, dir)
				if fn != nil && (err != nil || len(memories) > 0) {
					fn(memories, err)
				}
			}
		}
	}()
}

func (s *Service) Dashboard(ctx context.Context, namespaceID string) (DashboardMetrics, error) {
	metrics := DashboardMetrics{TierDistribution: map[int]int64{0: 0, 1: 0, 2: 0, 3: 0}}
	if err := s.DB.WithContext(ctx).Model(&models.Memory{}).Where("namespace_id = ? AND is_deleted = ?", namespaceID, false).Count(&metrics.Memories).Error; err != nil {
		return metrics, err
	}
	metrics.Thoughts = metrics.Memories
	if err := s.DB.WithContext(ctx).Model(&models.Proposal{}).Where("namespace_id = ? AND status = ?", namespaceID, "pending").Count(&metrics.PendingProposals).Error; err != nil {
		return metrics, err
	}
	var allProposals int64
	var approvedProposals int64
	_ = s.DB.WithContext(ctx).Model(&models.Proposal{}).Where("namespace_id = ?", namespaceID).Count(&allProposals).Error
	_ = s.DB.WithContext(ctx).Model(&models.Proposal{}).Where("namespace_id = ? AND status = ?", namespaceID, "approve").Count(&approvedProposals).Error
	if allProposals > 0 {
		metrics.ProposalAcceptanceRate = float64(approvedProposals) / float64(allProposals)
	}
	for tier := 0; tier <= 3; tier++ {
		var count int64
		_ = s.DB.WithContext(ctx).Model(&models.Memory{}).Where("namespace_id = ? AND is_deleted = ? AND tier = ?", namespaceID, false, tier).Count(&count).Error
		metrics.TierDistribution[tier] = count
	}
	var stale int64
	_ = s.DB.WithContext(ctx).Model(&models.Memory{}).Where("namespace_id = ? AND tier = ? AND is_deleted = ? AND created_at < ?", namespaceID, 0, false, s.Now().Add(-24*time.Hour)).Count(&stale).Error
	if metrics.Memories > 0 {
		metrics.StaleMemoryRatio = float64(stale) / float64(metrics.Memories)
	}
	_ = s.DB.WithContext(ctx).Model(&models.Proposal{}).Where("namespace_id = ? AND proposal_type = ?", namespaceID, "gap").Count(&metrics.KnowledgeGapCount).Error
	var top []models.Memory
	if err := s.DB.WithContext(ctx).Where("namespace_id = ? AND is_deleted = ?", namespaceID, false).Order("access_count desc, updated_at desc").Limit(5).Find(&top).Error; err == nil {
		for _, memory := range top {
			metrics.TopAccessed = append(metrics.TopAccessed, toPayload(memory))
		}
	}
	return metrics, nil
}

func (s *Service) GenerateHealthReport(ctx context.Context) error {
	var namespaces []models.Namespace
	if err := s.DB.WithContext(ctx).Find(&namespaces).Error; err != nil {
		return err
	}
	for _, namespace := range namespaces {
		metrics, err := s.Dashboard(ctx, namespace.ID)
		if err != nil {
			return err
		}
		_, err = s.CreateMemory(ctx, Actor{Kind: "curator", NamespaceID: namespace.ID, TrustTier: 4}, MemoryPayload{NamespaceID: namespace.ID, EntityType: "document", Title: "Weekly Memory Health Report", Text: mustJSON(metrics, "{}"), Tier: 2, Metadata: map[string]any{"reportType": "weekly_memory_health"}, Provenance: map[string]any{"kind": "health_report"}})
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) ExportAudit(ctx context.Context, namespaceID, format string) ([]byte, string, error) {
	var logs []models.AuditLog
	if err := s.DB.WithContext(ctx).Where("namespace_id = ?", namespaceID).Order("id asc").Find(&logs).Error; err != nil {
		return nil, "", err
	}
	switch strings.ToLower(format) {
	case "", "jsonld":
		payload := map[string]any{"@context": "https://openbrain.local/audit", "namespaceId": namespaceID, "records": logs}
		body, err := json.MarshalIndent(payload, "", "  ")
		return body, "application/ld+json", err
	case "sqlite":
		return exportAuditSQLite(logs)
	default:
		return nil, "", fmt.Errorf("unsupported audit export format: %s", format)
	}
}

func exportAuditSQLite(logs []models.AuditLog) ([]byte, string, error) {
	file, err := os.CreateTemp("", "openbrain-audit-*.sqlite")
	if err != nil {
		return nil, "", err
	}
	path := file.Name()
	file.Close()
	defer os.Remove(path)
	db, err := sql.Open("sqlite3", path)
	if err != nil {
		return nil, "", err
	}
	defer db.Close()
	if _, err := db.Exec(`CREATE TABLE audit_log (id INTEGER PRIMARY KEY, namespace_id TEXT, agent_id TEXT, actor_kind TEXT, action TEXT, entity_id TEXT, entity_type TEXT, before_hash TEXT, after_hash TEXT, chain_hash TEXT, request_meta TEXT, created_at TEXT)`); err != nil {
		return nil, "", err
	}
	for _, log := range logs {
		if _, err := db.Exec(`INSERT INTO audit_log (id, namespace_id, agent_id, actor_kind, action, entity_id, entity_type, before_hash, after_hash, chain_hash, request_meta, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, log.ID, log.NamespaceID, log.AgentID, log.ActorKind, log.Action, log.EntityID, log.EntityType, log.BeforeHash, log.AfterHash, log.ChainHash, log.RequestMeta, log.CreatedAt.Format(time.RFC3339Nano)); err != nil {
			return nil, "", err
		}
	}
	body, err := os.ReadFile(path)
	return body, "application/vnd.sqlite3", err
}

func (s *Service) upsertSyncedMemory(ctx context.Context, actor Actor, namespaceID, path, content, kind string, tier int) (models.Memory, error) {
	var memories []models.Memory
	if err := s.DB.WithContext(ctx).Where("namespace_id = ? AND metadata LIKE ?", namespaceID, "%\"sync_path\":\""+strings.ReplaceAll(path, "\"", "")+"\"%").Find(&memories).Error; err != nil {
		return models.Memory{}, err
	}
	payload := MemoryPayload{NamespaceID: namespaceID, EntityType: "note", Title: filepath.Base(path), Text: content, Tier: tier, Metadata: map[string]any{"sync_path": path, "sync_kind": kind}, Provenance: map[string]any{"kind": kind, "path": path}, Identity: map[string]any{"createdVia": "file_sync", "createdByAgentId": actor.AgentID}}
	if len(memories) == 0 {
		return s.CreateMemory(ctx, actor, payload)
	}
	return s.UpdateMemory(ctx, actor, namespaceID, memories[0].ID, map[string]any{"text": content, "tier": float64(tier), "metadata": payload.Metadata, "provenance": payload.Provenance, "identity": payload.Identity, "title": payload.Title})
}

func (s *Service) storeContextPacket(ctx context.Context, actor Actor, namespaceID, agentID, triggerType string, response ContextResponse) (string, error) {
	if agentID == "" {
		agentID = "anonymous"
	}
	expires := s.Now().UTC().Add(6 * time.Hour)
	packet := models.ContextPacket{ID: uuid.NewString(), NamespaceID: namespaceID, AgentID: agentID, TriggerType: triggerType, Payload: mustJSON(response, "{}"), ExpiresAt: &expires, CreatedAt: s.Now().UTC()}
	if err := s.DB.WithContext(ctx).Create(&packet).Error; err != nil {
		return "", err
	}
	_ = s.appendAudit(ctx, actor, namespaceID, "store_context_packet", packet.ID, "context_packet", "", hashJSON(packet), map[string]any{"triggerType": triggerType})
	return packet.ID, nil
}

func (s *Service) saveVersion(ctx context.Context, memory models.Memory, actor Actor, reason string) error {
	version := models.MemoryVersion{ID: uuid.NewString(), NamespaceID: memory.NamespaceID, EntityID: memory.ID, Version: memory.Version, Title: memory.Title, Text: memory.Text, Embedding: memory.Embedding, Metadata: memory.Metadata, Provenance: memory.Provenance, Identity: memory.Identity, Tier: memory.Tier, ChangedBy: mustJSON(actor, "{}"), ChangeReason: reason, CreatedAt: s.Now().UTC()}
	return s.DB.WithContext(ctx).Create(&version).Error
}

func (s *Service) appendAudit(ctx context.Context, actor Actor, namespaceID, action, entityID, entityType, beforeHash, afterHash string, requestMeta map[string]any) error {
	var prev models.AuditLog
	chainBase := ""
	if err := s.DB.WithContext(ctx).Where("namespace_id = ?", namespaceID).Order("id desc").First(&prev).Error; err == nil {
		chainBase = prev.ChainHash
	}
	now := s.Now().UTC()
	metaJSON := mustJSON(requestMeta, "{}")
	payload := fmt.Sprintf("%s|%s|%s|%s", chainBase, namespaceID, now.Format(time.RFC3339Nano), afterHash)
	entry := models.AuditLog{NamespaceID: namespaceID, AgentID: actor.AgentID, ActorKind: firstNonEmpty(actor.Kind, "system"), Action: action, EntityID: entityID, EntityType: entityType, BeforeHash: beforeHash, AfterHash: afterHash, ChainHash: hashString(payload), RequestMeta: metaJSON, CreatedAt: now}
	return s.DB.WithContext(ctx).Create(&entry).Error
}

func (s *Service) touchMemories(ctx context.Context, actor Actor, memories []models.Memory) error {
	for _, memory := range memories {
		now := s.Now().UTC()
		updates := map[string]any{"access_count": memory.AccessCount + 1, "last_accessed_at": &now, "updated_at": now}
		if err := s.DB.WithContext(ctx).Model(&models.Memory{}).Where("id = ?", memory.ID).Updates(updates).Error; err != nil {
			return err
		}
		memory.AccessCount++
		memory.LastAccessedAt = &now
		memory.UpdatedAt = now
		_, _, _ = s.evaluatePromotion(ctx, actor, memory)
	}
	return nil
}

func (s *Service) evaluatePromotion(ctx context.Context, actor Actor, memory models.Memory) (models.Memory, bool, error) {
	beforeHash := hashMemory(memory)
	now := s.Now().UTC()
	promoted := false
	switch {
	case memory.Tier == 0 && (memory.AccessCount >= 3 || memory.ManualPromote) && memory.CreatedAt.After(now.Add(-24*time.Hour)):
		memory.Tier = 1
		promoted = true
	case memory.Tier == 1 && (memory.AccessCount >= 5 || memory.ManualPromote):
		memory.Tier = 2
		promoted = true
	}
	if !promoted {
		return memory, false, nil
	}
	memory.Version++
	memory.UpdatedAt = now
	if decayAt := computeDecayAt(memory.Tier, now); !decayAt.IsZero() {
		memory.DecayAt = &decayAt
	}
	if err := s.DB.WithContext(ctx).Save(&memory).Error; err != nil {
		return models.Memory{}, false, err
	}
	if err := s.saveVersion(ctx, memory, actor, "tier_promotion"); err != nil {
		return models.Memory{}, false, err
	}
	_ = s.appendAudit(ctx, actor, memory.NamespaceID, "promote", memory.ID, memory.EntityType, beforeHash, hashMemory(memory), map[string]any{"tier": memory.Tier})
	return memory, true, nil
}

func (s *Service) applyProposal(ctx context.Context, actor Actor, proposal models.Proposal) error {
	ids := decodeStringSlice(proposal.MemoryIDs)
	switch proposal.ProposalType {
	case "deduplicate", "synthesize":
		_, err := s.CreateMemory(ctx, Actor{Kind: "curator", NamespaceID: proposal.NamespaceID, TrustTier: 4, AgentID: actor.AgentID}, MemoryPayload{NamespaceID: proposal.NamespaceID, EntityType: "note", Title: proposal.Summary, Text: proposal.SuggestedText, Tier: valueOrDefault(proposal.SuggestedTier, 2), Metadata: mergeMap(decodeJSONMap(proposal.Details), map[string]any{"sourceProposal": proposal.ID}), Provenance: map[string]any{"kind": "proposal", "proposalType": proposal.ProposalType}})
		if err != nil {
			return err
		}
		if proposal.ProposalType == "deduplicate" {
			_, _ = s.ForgetMemories(ctx, Actor{Kind: "curator", NamespaceID: proposal.NamespaceID, TrustTier: 4, AgentID: actor.AgentID}, proposal.NamespaceID, ids)
		}
	case "demote":
		for _, id := range ids {
			_, err := s.UpdateMemory(ctx, Actor{Kind: "curator", NamespaceID: proposal.NamespaceID, TrustTier: 4, AgentID: actor.AgentID}, proposal.NamespaceID, id, map[string]any{"tier": float64(1)})
			if err != nil {
				return err
			}
		}
	case "gap":
		_, err := s.CreateMemory(ctx, Actor{Kind: "curator", NamespaceID: proposal.NamespaceID, TrustTier: 4, AgentID: actor.AgentID}, MemoryPayload{NamespaceID: proposal.NamespaceID, EntityType: "task", Title: proposal.Summary, Text: proposal.SuggestedText, Tier: 1, Metadata: mergeMap(decodeJSONMap(proposal.Details), map[string]any{"sourceProposal": proposal.ID}), Provenance: map[string]any{"kind": "knowledge_gap"}})
		return err
	}
	return nil
}

func (s *Service) dedupProposals(ctx context.Context, namespaceID string, memories []models.Memory) []models.Proposal {
	var created []models.Proposal
	seen := map[string]struct{}{}
	for i := 0; i < len(memories); i++ {
		for j := i + 1; j < len(memories); j++ {
			a, b := memories[i], memories[j]
			key := a.ID + ":" + b.ID
			if _, ok := seen[key]; ok {
				continue
			}
			if cosine(decodeEmbedding(a.Embedding), decodeEmbedding(b.Embedding)) < 0.95 {
				continue
			}
			summary := fmt.Sprintf("Deduplicate %s and %s", a.ID, b.ID)
			proposal, err := s.createProposalIfMissing(ctx, models.Proposal{NamespaceID: namespaceID, ProposalType: "deduplicate", MemoryIDs: mustJSON([]string{a.ID, b.ID}, "[]"), Summary: summary, SuggestedText: summarizeText(a.Text+"\n\n"+b.Text, "short"), Status: "pending", Details: mustJSON(map[string]any{"reason": "cosine_similarity_gt_0.95", "similarity": cosine(decodeEmbedding(a.Embedding), decodeEmbedding(b.Embedding))}, "{}")})
			if err == nil {
				created = append(created, proposal)
			}
			seen[key] = struct{}{}
		}
	}
	return created
}

func (s *Service) synthesisProposals(ctx context.Context, namespaceID string, memories []models.Memory) []models.Proposal {
	byType := map[string][]models.Memory{}
	for _, memory := range memories {
		if memory.Tier == 1 {
			byType[memory.EntityType] = append(byType[memory.EntityType], memory)
		}
	}
	var created []models.Proposal
	for entityType, group := range byType {
		if len(group) < 2 {
			continue
		}
		top := group
		if len(top) > 3 {
			top = top[:3]
		}
		ids := []string{}
		parts := make([]string, 0, len(top))
		for _, memory := range top {
			ids = append(ids, memory.ID)
			parts = append(parts, memory.Text)
		}
		tier := 2
		proposal, err := s.createProposalIfMissing(ctx, models.Proposal{NamespaceID: namespaceID, ProposalType: "synthesize", MemoryIDs: mustJSON(ids, "[]"), Summary: fmt.Sprintf("Synthesize %d %s memories", len(top), entityType), SuggestedText: summarizeText(strings.Join(parts, "\n"), "balanced"), SuggestedTier: &tier, Status: "pending", Details: mustJSON(map[string]any{"entityType": entityType}, "{}")})
		if err == nil {
			created = append(created, proposal)
		}
	}
	return created
}

func (s *Service) demotionProposals(ctx context.Context, namespaceID string, memories []models.Memory) []models.Proposal {
	now := s.Now().UTC()
	var created []models.Proposal
	for _, memory := range memories {
		if memory.Tier != 2 {
			continue
		}
		last := memory.UpdatedAt
		if memory.LastAccessedAt != nil {
			last = *memory.LastAccessedAt
		}
		if last.After(now.Add(-60 * 24 * time.Hour)) {
			continue
		}
		proposal, err := s.createProposalIfMissing(ctx, models.Proposal{NamespaceID: namespaceID, ProposalType: "demote", MemoryIDs: mustJSON([]string{memory.ID}, "[]"), Summary: fmt.Sprintf("Demote stale reference memory %s", memory.ID), SuggestedText: memory.Text, Status: "pending", Details: mustJSON(map[string]any{"reason": "unused_60_days"}, "{}")})
		if err == nil {
			created = append(created, proposal)
		}
	}
	return created
}

func (s *Service) gapProposals(ctx context.Context, namespaceID string) []models.Proposal {
	var logs []models.AuditLog
	if err := s.DB.WithContext(ctx).Where("namespace_id = ? AND action = ?", namespaceID, "search").Order("id desc").Limit(20).Find(&logs).Error; err != nil {
		return nil
	}
	counts := map[string]int{}
	for _, log := range logs {
		meta := decodeJSONMap(log.RequestMeta)
		query, _ := meta["query"].(string)
		count, _ := meta["count"].(float64)
		if strings.TrimSpace(query) == "" || count > 0 {
			continue
		}
		counts[query]++
	}
	var created []models.Proposal
	for query, count := range counts {
		if count < 2 {
			continue
		}
		proposal, err := s.createProposalIfMissing(ctx, models.Proposal{NamespaceID: namespaceID, ProposalType: "gap", MemoryIDs: "[]", Summary: fmt.Sprintf("Knowledge gap detected for query %q", query), SuggestedText: fmt.Sprintf("Agents repeatedly searched for %q but OpenBrain had no relevant memory. Capture guidance, ADRs, or troubleshooting notes for this topic.", query), Status: "pending", Details: mustJSON(map[string]any{"query": query, "occurrences": count}, "{}")})
		if err == nil {
			created = append(created, proposal)
		}
	}
	return created
}

func (s *Service) createProposalIfMissing(ctx context.Context, proposal models.Proposal) (models.Proposal, error) {
	var existing models.Proposal
	if err := s.DB.WithContext(ctx).Where("namespace_id = ? AND proposal_type = ? AND summary = ? AND status = ?", proposal.NamespaceID, proposal.ProposalType, proposal.Summary, "pending").First(&existing).Error; err == nil {
		return existing, nil
	}
	proposal.ID = uuid.NewString()
	proposal.CreatedAt = s.Now().UTC()
	proposal.UpdatedAt = proposal.CreatedAt
	if err := s.DB.WithContext(ctx).Create(&proposal).Error; err != nil {
		return models.Proposal{}, err
	}
	return proposal, nil
}

func (s *Service) redactMemoryForActor(memory models.Memory, actor Actor) models.Memory {
	if actor.Kind != "agent" {
		return memory
	}
	maxTier := actor.TrustTier
	if maxTier == 0 {
		maxTier = 1
	}
	if memory.Tier > maxTier {
		memory.Text = "[redacted: higher trust tier required]"
	}
	return memory
}

func (s *Service) requireWriteTrust(actor Actor, tier int) error {
	if actor.Kind == "agent" {
		if actor.TrustTier < 2 {
			return errors.New("agent trust tier does not permit writes")
		}
		if tier >= 2 && actor.TrustTier < 4 {
			return errors.New("agent trust tier does not permit direct L2/L3 writes")
		}
	}
	return nil
}

func (s *Service) requireDeleteTrust(actor Actor) error {
	if actor.Kind == "agent" && actor.TrustTier < 4 {
		return errors.New("agent trust tier does not permit delete")
	}
	return nil
}

func computeDecayAt(tier int, now time.Time) time.Time {
	switch tier {
	case 0:
		return now.Add(24 * time.Hour)
	case 1:
		return now.Add(30 * 24 * time.Hour)
	default:
		return time.Time{}
	}
}

func estimateTokens(text string) int {
	count := len(strings.Fields(text))
	if count == 0 {
		return 1
	}
	return count + int(math.Ceil(float64(len(text))/20))
}

func tierWeight(tier int) float64 {
	switch tier {
	case 3:
		return 4
	case 2:
		return 2
	case 1:
		return 1
	default:
		return 0.5
	}
}

func renderContext(format string, snippets []ContextSnippet) string {
	switch strings.ToLower(format) {
	case "json":
		body, _ := json.Marshal(snippets)
		return string(body)
	case "xml":
		var b strings.Builder
		b.WriteString("<context>")
		for _, snippet := range snippets {
			b.WriteString(fmt.Sprintf("<snippet id=%q tier=%q type=%q>%s</snippet>", snippet.ID, fmt.Sprint(snippet.Tier), snippet.EntityType, xmlEscape(snippet.Text)))
		}
		b.WriteString("</context>")
		return b.String()
	default:
		var b strings.Builder
		for _, snippet := range snippets {
			b.WriteString(fmt.Sprintf("- [%s/L%d] %s\n", snippet.EntityType, snippet.Tier, snippet.Text))
		}
		return strings.TrimSpace(b.String())
	}
}

func xmlEscape(s string) string {
	replacer := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;", "'", "&apos;")
	return replacer.Replace(s)
}

func summarizeText(text, verbosity string) string {
	words := strings.Fields(text)
	limit := 24
	switch verbosity {
	case "short":
		limit = 16
	case "detailed":
		limit = 40
	}
	if len(words) <= limit {
		return strings.Join(words, " ")
	}
	return strings.Join(words[:limit], " ") + "…"
}

func combinePreferredTypes(requested, preferred []string) []string {
	if len(requested) > 0 {
		return requested
	}
	return preferred
}

func recencyWeight(last *time.Time, updated, now time.Time) float64 {
	ref := updated
	if last != nil {
		ref = *last
	}
	age := now.Sub(ref)
	if age <= 0 {
		return 1
	}
	days := age.Hours() / 24
	return 1 / (1 + days/30)
}

func lexicalScore(tokens []string, text string) float64 {
	if len(tokens) == 0 {
		return 0
	}
	text = strings.ToLower(text)
	matched := 0
	for _, token := range tokens {
		if strings.Contains(text, token) {
			matched++
		}
	}
	return float64(matched) / float64(len(tokens))
}

func tokenize(text string) []string {
	fields := strings.Fields(strings.ToLower(text))
	out := make([]string, 0, len(fields))
	for _, field := range fields {
		field = strings.Trim(field, ".,;:!?()[]{}\"'`")
		if field != "" {
			out = append(out, field)
		}
	}
	return out
}

func generateEmbedding(text string) []float64 {
	const dims = 64
	vec := make([]float64, dims)
	for _, token := range tokenize(text) {
		h := fnv.New64a()
		_, _ = h.Write([]byte(token))
		sum := h.Sum64()
		idx := int(sum % dims)
		sign := 1.0
		if sum&1 == 1 {
			sign = -1.0
		}
		vec[idx] += sign
	}
	norm := 0.0
	for _, value := range vec {
		norm += value * value
	}
	if norm == 0 {
		return vec
	}
	norm = math.Sqrt(norm)
	for i := range vec {
		vec[i] /= norm
	}
	return vec
}

func encodeEmbedding(vec []float64) string { return mustJSON(vec, "[]") }

func decodeEmbedding(raw string) []float64 {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	var vec []float64
	if err := json.Unmarshal([]byte(raw), &vec); err != nil {
		return nil
	}
	return vec
}

func cosine(a, b []float64) float64 {
	if len(a) == 0 || len(b) == 0 {
		return 0
	}
	limit := len(a)
	if len(b) < limit {
		limit = len(b)
	}
	dot := 0.0
	normA := 0.0
	normB := 0.0
	for i := 0; i < limit; i++ {
		dot += a[i] * b[i]
		normA += a[i] * a[i]
		normB += b[i] * b[i]
	}
	if normA == 0 || normB == 0 {
		return 0
	}
	return dot / (math.Sqrt(normA) * math.Sqrt(normB))
}

func toPayload(memory models.Memory) MemoryPayload {
	return MemoryPayload{ID: memory.ID, NamespaceID: memory.NamespaceID, TeamID: memory.TeamID, EntityType: memory.EntityType, Title: memory.Title, Text: memory.Text, Metadata: decodeJSONMap(memory.Metadata), Provenance: decodeJSONMap(memory.Provenance), Identity: decodeJSONMap(memory.Identity), Tier: memory.Tier, Version: memory.Version, AccessCount: memory.AccessCount, CreatedAt: memory.CreatedAt, UpdatedAt: memory.UpdatedAt}
}

func toProposalPayload(proposal models.Proposal) ProposalPayload {
	return ProposalPayload{ID: proposal.ID, NamespaceID: proposal.NamespaceID, ProposalType: proposal.ProposalType, MemoryIDs: decodeStringSlice(proposal.MemoryIDs), Summary: proposal.Summary, SuggestedText: proposal.SuggestedText, SuggestedTier: proposal.SuggestedTier, Status: proposal.Status, Details: decodeJSONMap(proposal.Details), ReviewedBy: proposal.ReviewedBy, CreatedAt: proposal.CreatedAt, UpdatedAt: proposal.UpdatedAt}
}

func hashMemory(memory models.Memory) string {
	return hashString(memory.Title + "|" + memory.Text + "|" + memory.Metadata + "|" + memory.Provenance + "|" + fmt.Sprint(memory.Tier) + "|" + fmt.Sprint(memory.IsDeleted))
}

func hashJSON(v any) string {
	body, _ := json.Marshal(v)
	return hashString(string(body))
}

func hashString(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func mustJSON(v any, fallback string) string {
	if v == nil {
		return fallback
	}
	body, err := json.Marshal(v)
	if err != nil {
		return fallback
	}
	return string(body)
}

func decodeJSONMap(raw string) map[string]any {
	if strings.TrimSpace(raw) == "" {
		return map[string]any{}
	}
	var value map[string]any
	if err := json.Unmarshal([]byte(raw), &value); err != nil || value == nil {
		return map[string]any{}
	}
	return value
}

func decodeStringSlice(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	var value []string
	if err := json.Unmarshal([]byte(raw), &value); err != nil {
		return nil
	}
	return value
}

func clampTier(tier int) int {
	if tier < 0 {
		return 0
	}
	if tier > 3 {
		return 3
	}
	return tier
}

func entityTypeFromMetadata(metadata map[string]any) string {
	if metadata == nil {
		return "note"
	}
	if value, ok := metadata["type"].(string); ok && value != "" {
		return value
	}
	return "note"
}

func mergeMap(base, extra map[string]any) map[string]any {
	result := map[string]any{}
	for k, v := range base {
		result[k] = v
	}
	for k, v := range extra {
		result[k] = v
	}
	return result
}

func defaultMap(value map[string]any) map[string]any {
	if value == nil {
		return map[string]any{}
	}
	return value
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func valueOrDefault(ptr *int, fallback int) int {
	if ptr == nil {
		return fallback
	}
	return *ptr
}
