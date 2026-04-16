// Package services provides the company portability service for export/import of company data.
package services

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"gopkg.in/yaml.v3"
	"gorm.io/datatypes"
	"gorm.io/gorm"

	"github.com/chifamba/vashandi/vashandi/backend/db/models"
)

// ──────────────────────────────────────────────────────────────────────────────
// Public request / response types (mirrors the TypeScript shared types).
// ──────────────────────────────────────────────────────────────────────────────

// PortabilityInclude controls which data is included in an export or import.
type PortabilityInclude struct {
	Company  bool `json:"company"`
	Agents   bool `json:"agents"`
	Projects bool `json:"projects"`
	Issues   bool `json:"issues"`
	Skills   bool `json:"skills"`
}

// PortabilityFileEntry is either a plain text string or a base64-encoded binary.
type PortabilityFileEntry = interface{} // string | PortabilityBinaryEntry

// PortabilityBinaryEntry represents a binary file in a portability package.
type PortabilityBinaryEntry struct {
	Encoding    string  `json:"encoding"`
	Data        string  `json:"data"`
	ContentType *string `json:"contentType,omitempty"`
}

// PortabilityManifest is the manifest embedded in a portability package.
type PortabilityManifest struct {
	SchemaVersion int                                `json:"schemaVersion"`
	GeneratedAt   string                             `json:"generatedAt"`
	Source        *PortabilityManifestSource         `json:"source"`
	Includes      PortabilityInclude                 `json:"includes"`
	Company       *PortabilityCompanyManifestEntry   `json:"company"`
	Sidebar       *PortabilitySidebarOrder           `json:"sidebar"`
	Agents        []PortabilityAgentManifestEntry    `json:"agents"`
	Skills        []PortabilitySkillManifestEntry    `json:"skills"`
	Projects      []PortabilityProjectManifestEntry  `json:"projects"`
	Issues        []PortabilityIssueManifestEntry    `json:"issues"`
	EnvInputs     []PortabilityEnvInput              `json:"envInputs"`
}

// PortabilityManifestSource identifies the originating company.
type PortabilityManifestSource struct {
	CompanyID   string `json:"companyId"`
	CompanyName string `json:"companyName"`
}

// PortabilityCompanyManifestEntry holds company metadata.
type PortabilityCompanyManifestEntry struct {
	Path                               string  `json:"path"`
	Name                               string  `json:"name"`
	Description                        *string `json:"description"`
	BrandColor                         *string `json:"brandColor"`
	LogoPath                           *string `json:"logoPath"`
	RequireBoardApprovalForNewAgents   bool    `json:"requireBoardApprovalForNewAgents"`
	FeedbackDataSharingEnabled         bool    `json:"feedbackDataSharingEnabled"`
	FeedbackDataSharingConsentAt       *string `json:"feedbackDataSharingConsentAt"`
	FeedbackDataSharingConsentByUserID *string `json:"feedbackDataSharingConsentByUserId"`
	FeedbackDataSharingTermsVersion    *string `json:"feedbackDataSharingTermsVersion"`
}

// PortabilitySidebarOrder represents the sidebar agent/project ordering.
type PortabilitySidebarOrder struct {
	Agents   []string `json:"agents"`
	Projects []string `json:"projects"`
}

// PortabilityAgentManifestEntry holds agent metadata in the manifest.
type PortabilityAgentManifestEntry struct {
	Slug               string                 `json:"slug"`
	Name               string                 `json:"name"`
	Path               string                 `json:"path"`
	Skills             []string               `json:"skills"`
	Role               string                 `json:"role"`
	Title              *string                `json:"title"`
	Icon               *string                `json:"icon"`
	Capabilities       *string                `json:"capabilities"`
	ReportsToSlug      *string                `json:"reportsToSlug"`
	AdapterType        string                 `json:"adapterType"`
	AdapterConfig      map[string]interface{} `json:"adapterConfig"`
	RuntimeConfig      map[string]interface{} `json:"runtimeConfig"`
	Permissions        map[string]interface{} `json:"permissions"`
	BudgetMonthlyCents int                    `json:"budgetMonthlyCents"`
	Metadata           map[string]interface{} `json:"metadata"`
}

// PortabilitySkillManifestEntry holds skill metadata in the manifest.
type PortabilitySkillManifestEntry struct {
	Key           string                             `json:"key"`
	Slug          string                             `json:"slug"`
	Name          string                             `json:"name"`
	Path          string                             `json:"path"`
	Description   *string                            `json:"description"`
	SourceType    string                             `json:"sourceType"`
	SourceLocator *string                            `json:"sourceLocator"`
	SourceRef     *string                            `json:"sourceRef"`
	TrustLevel    *string                            `json:"trustLevel"`
	Compatibility *string                            `json:"compatibility"`
	Metadata      map[string]interface{}             `json:"metadata"`
	FileInventory []PortabilitySkillFileInventory    `json:"fileInventory"`
}

// PortabilitySkillFileInventory is an item in a skill file inventory.
type PortabilitySkillFileInventory struct {
	Path string `json:"path"`
	Kind string `json:"kind"`
}

// PortabilityProjectManifestEntry holds project metadata in the manifest.
type PortabilityProjectManifestEntry struct {
	Slug                     string                     `json:"slug"`
	Name                     string                     `json:"name"`
	Path                     string                     `json:"path"`
	Description              *string                    `json:"description"`
	OwnerAgentSlug           *string                    `json:"ownerAgentSlug"`
	LeadAgentSlug            *string                    `json:"leadAgentSlug"`
	TargetDate               *string                    `json:"targetDate"`
	Color                    *string                    `json:"color"`
	Status                   *string                    `json:"status"`
	Env                      map[string]interface{}     `json:"env"`
	ExecutionWorkspacePolicy map[string]interface{}     `json:"executionWorkspacePolicy"`
	Workspaces               []PortabilityWorkspaceEntry `json:"workspaces"`
	Metadata                 map[string]interface{}     `json:"metadata"`
}

// PortabilityWorkspaceEntry holds project workspace metadata.
type PortabilityWorkspaceEntry struct {
	Key            string                 `json:"key"`
	Name           string                 `json:"name"`
	SourceType     *string                `json:"sourceType"`
	RepoURL        *string                `json:"repoUrl"`
	RepoRef        *string                `json:"repoRef"`
	DefaultRef     *string                `json:"defaultRef"`
	Visibility     *string                `json:"visibility"`
	SetupCommand   *string                `json:"setupCommand"`
	CleanupCommand *string                `json:"cleanupCommand"`
	Metadata       map[string]interface{} `json:"metadata"`
	IsPrimary      bool                   `json:"isPrimary"`
}

// PortabilityIssueRoutineTrigger holds trigger data for a recurring issue.
type PortabilityIssueRoutineTrigger struct {
	Kind           string  `json:"kind"`
	Label          *string `json:"label"`
	Enabled        bool    `json:"enabled"`
	CronExpression *string `json:"cronExpression"`
	Timezone       *string `json:"timezone"`
	SigningMode     *string `json:"signingMode"`
	ReplayWindowSec *int   `json:"replayWindowSec"`
}

// PortabilityIssueRoutine holds routine configuration for a recurring issue.
type PortabilityIssueRoutine struct {
	ConcurrencyPolicy *string                          `json:"concurrencyPolicy"`
	CatchUpPolicy     *string                          `json:"catchUpPolicy"`
	Variables         []map[string]interface{}         `json:"variables,omitempty"`
	Triggers          []PortabilityIssueRoutineTrigger `json:"triggers"`
}

// PortabilityIssueManifestEntry holds issue/task metadata in the manifest.
type PortabilityIssueManifestEntry struct {
	Slug                     string                   `json:"slug"`
	Identifier               *string                  `json:"identifier"`
	Title                    string                   `json:"title"`
	Path                     string                   `json:"path"`
	ProjectSlug              *string                  `json:"projectSlug"`
	ProjectWorkspaceKey      *string                  `json:"projectWorkspaceKey"`
	AssigneeAgentSlug        *string                  `json:"assigneeAgentSlug"`
	Description              *string                  `json:"description"`
	Recurring                bool                     `json:"recurring"`
	Routine                  *PortabilityIssueRoutine `json:"routine"`
	LegacyRecurrence         map[string]interface{}   `json:"legacyRecurrence"`
	Status                   *string                  `json:"status"`
	Priority                 *string                  `json:"priority"`
	LabelIds                 []string                 `json:"labelIds"`
	BillingCode              *string                  `json:"billingCode"`
	ExecutionWorkspaceSettings map[string]interface{} `json:"executionWorkspaceSettings"`
	AssigneeAdapterOverrides   map[string]interface{} `json:"assigneeAdapterOverrides"`
	Metadata                   map[string]interface{} `json:"metadata"`
}

// PortabilityEnvInput describes an environment variable needed by the package.
type PortabilityEnvInput struct {
	Key          string  `json:"key"`
	Description  *string `json:"description"`
	AgentSlug    *string `json:"agentSlug"`
	ProjectSlug  *string `json:"projectSlug"`
	Kind         string  `json:"kind"`
	Requirement  string  `json:"requirement"`
	DefaultValue *string `json:"defaultValue"`
	Portability  string  `json:"portability"`
}

// ExportRequest is the body of a POST .../exports request.
type ExportRequest struct {
	Include            map[string]bool `json:"include"`
	Agents             []string        `json:"agents"`
	Skills             []string        `json:"skills"`
	Projects           []string        `json:"projects"`
	Issues             []string        `json:"issues"`
	ProjectIssues      []string        `json:"projectIssues"`
	SelectedFiles      []string        `json:"selectedFiles"`
	ExpandReferencedSkills bool        `json:"expandReferencedSkills"`
	SidebarOrder       *PortabilitySidebarOrder `json:"sidebarOrder"`
}

// ExportResult is returned by ExportBundle.
type ExportResult struct {
	RootPath              string                            `json:"rootPath"`
	Manifest              PortabilityManifest               `json:"manifest"`
	Files                 map[string]interface{}            `json:"files"`
	Warnings              []string                          `json:"warnings"`
	PaperclipExtensionPath string                           `json:"paperclipExtensionPath"`
}

// ExportPreviewResult extends ExportResult with a file inventory.
type ExportPreviewResult struct {
	ExportResult
	FileInventory []ExportPreviewFile `json:"fileInventory"`
	Counts        ExportPreviewCounts `json:"counts"`
}

// ExportPreviewFile is a single file listed in the preview inventory.
type ExportPreviewFile struct {
	Path string `json:"path"`
	Kind string `json:"kind"`
}

// ExportPreviewCounts holds counts of exported entities.
type ExportPreviewCounts struct {
	Files    int `json:"files"`
	Agents   int `json:"agents"`
	Skills   int `json:"skills"`
	Projects int `json:"projects"`
	Issues   int `json:"issues"`
}

// ImportSource describes the source of a portability package.
type ImportSource struct {
	Type     string                 `json:"type"` // "inline" | "github"
	RootPath *string                `json:"rootPath,omitempty"`
	Files    map[string]interface{} `json:"files,omitempty"`
	URL      string                 `json:"url,omitempty"`
}

// ImportTarget describes where to import the package.
type ImportTarget struct {
	Mode           string  `json:"mode"` // "new_company" | "existing_company"
	NewCompanyName *string `json:"newCompanyName,omitempty"`
	CompanyID      string  `json:"companyId,omitempty"`
}

// ImportRequest is the body of a POST .../imports/preview or .../imports/apply request.
type ImportRequest struct {
	Source            ImportSource            `json:"source"`
	Include           map[string]bool         `json:"include"`
	Target            ImportTarget            `json:"target"`
	Agents            interface{}             `json:"agents"` // "all" | []string
	CollisionStrategy string                  `json:"collisionStrategy"` // "rename"|"skip"|"replace"
	NameOverrides     map[string]string       `json:"nameOverrides"`
	SelectedFiles     []string                `json:"selectedFiles"`
	AdapterOverrides  map[string]AdapterOverride `json:"adapterOverrides"`
}

// AdapterOverride allows overriding the adapter type/config during import.
type AdapterOverride struct {
	AdapterType   string                 `json:"adapterType"`
	AdapterConfig map[string]interface{} `json:"adapterConfig"`
}

// AgentPlan describes the planned action for an agent during import.
type AgentPlan struct {
	Slug           string  `json:"slug"`
	Action         string  `json:"action"` // "create"|"update"|"skip"
	PlannedName    string  `json:"plannedName"`
	ExistingAgentID *string `json:"existingAgentId"`
	Reason         *string `json:"reason"`
}

// ProjectPlan describes the planned action for a project during import.
type ProjectPlan struct {
	Slug              string  `json:"slug"`
	Action            string  `json:"action"` // "create"|"update"|"skip"
	PlannedName       string  `json:"plannedName"`
	ExistingProjectID *string `json:"existingProjectId"`
	Reason            *string `json:"reason"`
}

// IssuePlan describes the planned action for an issue during import.
type IssuePlan struct {
	Slug         string  `json:"slug"`
	Action       string  `json:"action"` // "create"|"skip"
	PlannedTitle string  `json:"plannedTitle"`
	Reason       *string `json:"reason"`
}

// ImportPlan holds the full import plan.
type ImportPlan struct {
	CompanyAction string        `json:"companyAction"` // "none"|"create"|"update"
	AgentPlans    []AgentPlan   `json:"agentPlans"`
	ProjectPlans  []ProjectPlan `json:"projectPlans"`
	IssuePlans    []IssuePlan   `json:"issuePlans"`
}

// ImportCollision describes an import collision discovered during preview.
type ImportCollision struct {
	EntityType                  string   `json:"entityType"`
	Slug                        string   `json:"slug"`
	Name                        string   `json:"name"`
	ExistingID                  *string  `json:"existingId,omitempty"`
	ExistingName                *string  `json:"existingName,omitempty"`
	MatchTypes                  []string `json:"matchTypes"`
	RequestedCollisionStrategy  string   `json:"requestedCollisionStrategy"`
	RecommendedCollisionStrategy string  `json:"recommendedCollisionStrategy"`
	PlannedAction               string   `json:"plannedAction"`
	Reason                      string   `json:"reason"`
}

// PreviewResult is returned by PreviewImport.
type PreviewResult struct {
	Include             PortabilityInclude     `json:"include"`
	TargetCompanyID     *string                `json:"targetCompanyId"`
	TargetCompanyName   *string                `json:"targetCompanyName"`
	CollisionStrategy   string                 `json:"collisionStrategy"`
	SelectedAgentSlugs  []string               `json:"selectedAgentSlugs"`
	Plan                ImportPlan             `json:"plan"`
	Collisions          []ImportCollision      `json:"collisions"`
	Manifest            PortabilityManifest    `json:"manifest"`
	Files               map[string]interface{} `json:"files"`
	EnvInputs           []PortabilityEnvInput  `json:"envInputs"`
	Warnings            []string               `json:"warnings"`
	Errors              []string               `json:"errors"`
}

// ImportResult is returned by ImportBundle.
type ImportResult struct {
	Company   ImportResultCompany   `json:"company"`
	Agents    []ImportResultAgent   `json:"agents"`
	Projects  []ImportResultProject `json:"projects"`
	EnvInputs []PortabilityEnvInput `json:"envInputs"`
	Warnings  []string              `json:"warnings"`
}

// ImportResultCompany describes the company result after import.
type ImportResultCompany struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Action string `json:"action"` // "created"|"updated"|"unchanged"
}

// ImportResultAgent describes a single agent result after import.
type ImportResultAgent struct {
	Slug   string  `json:"slug"`
	ID     *string `json:"id"`
	Action string  `json:"action"` // "created"|"updated"|"skipped"
	Name   string  `json:"name"`
	Reason *string `json:"reason"`
}

// ImportResultProject describes a single project result after import.
type ImportResultProject struct {
	Slug   string  `json:"slug"`
	ID     *string `json:"id"`
	Action string  `json:"action"` // "created"|"updated"|"skipped"
	Name   string  `json:"name"`
	Reason *string `json:"reason"`
}

// ImportMode restricts what operations are allowed during import.
type ImportMode string

const (
	// ImportModeBoardFull allows all operations including replace.
	ImportModeBoardFull ImportMode = "board_full"
	// ImportModeAgentSafe only allows create or skip.
	ImportModeAgentSafe ImportMode = "agent_safe"
)

// ──────────────────────────────────────────────────────────────────────────────
// Internal resolution types.
// ──────────────────────────────────────────────────────────────────────────────

type resolvedSource struct {
	manifest PortabilityManifest
	files    map[string]interface{}
	warnings []string
}

// internalPlan holds intermediate data passed between buildPreview and importBundle.
type internalPlan struct {
	preview         PreviewResult
	source          resolvedSource
	include         PortabilityInclude
	collisionStrategy string
	selectedAgents  []PortabilityAgentManifestEntry
}

type existingCollisionEntity struct {
	ID   string
	Name string
}

// ──────────────────────────────────────────────────────────────────────────────
// Slug / URL-key helpers (mirrors normalizeAgentUrlKey from shared).
// ──────────────────────────────────────────────────────────────────────────────

var (
	slugDelimRE = regexp.MustCompile(`[^a-z0-9]+`)
	slugTrimRE  = regexp.MustCompile(`^-+|-+$`)
)

// normalizeSlug converts a string to a URL-safe slug.
func normalizeSlug(s string) string {
	n := strings.ToLower(strings.TrimSpace(s))
	n = slugDelimRE.ReplaceAllString(n, "-")
	n = slugTrimRE.ReplaceAllString(n, "")
	return n
}

// uniqueSlug returns a slug that is not already in used.
func uniqueSlug(base string, used map[string]bool) string {
	if !used[base] {
		used[base] = true
		return base
	}
	for i := 2; ; i++ {
		candidate := fmt.Sprintf("%s-%d", base, i)
		if !used[candidate] {
			used[candidate] = true
			return candidate
		}
	}
}

// uniqueName returns a name whose derived slug is not in usedSlugs.
func uniqueName(base string, usedSlugs map[string]bool) string {
	slug := normalizeSlug(base)
	if !usedSlugs[slug] {
		usedSlugs[slug] = true
		return base
	}
	for i := 2; ; i++ {
		candidate := fmt.Sprintf("%s %d", base, i)
		candidateSlug := normalizeSlug(candidate)
		if !usedSlugs[candidateSlug] {
			usedSlugs[candidateSlug] = true
			return candidate
		}
	}
}

func uniqueIssueTitle(base string, usedSlugs map[string]bool) string {
	return uniqueName(base, usedSlugs)
}

// deriveProjectSlug computes the slug for a project name (same logic as deriveProjectUrlKey).
func deriveProjectSlug(name string) string {
	s := normalizeSlug(name)
	if s != "" {
		return s
	}
	return "project"
}

func recommendedCollisionStrategy(mode ImportMode, requested string, supportsReplace bool) string {
	if requested == "" {
		requested = "rename"
	}
	if mode == ImportModeAgentSafe && requested == "replace" {
		requested = "rename"
	}
	if requested == "replace" && !supportsReplace {
		return "rename"
	}
	return requested
}

func buildCollisionMatchTypes(slugMatched, nameMatched bool, identifierMatched bool) []string {
	matchTypes := []string{}
	if slugMatched {
		matchTypes = append(matchTypes, "slug")
	}
	if nameMatched {
		matchTypes = append(matchTypes, "name")
	}
	if identifierMatched {
		matchTypes = append(matchTypes, "identifier")
	}
	if len(matchTypes) == 0 {
		matchTypes = append(matchTypes, "name")
	}
	return matchTypes
}

func deriveManifestSkillKey(frontmatter map[string]interface{}, fallbackSlug string, metadata map[string]interface{}, sourceType string, sourceLocator *string) string {
	if key := normalizeSkillKey(strVal(frontmatter, "key")); key != "" {
		return key
	}
	if key := normalizeSkillKey(strVal(frontmatter, "skillKey")); key != "" {
		return key
	}
	if metadata != nil {
		if key := normalizeSkillKey(strVal(metadata, "skillKey")); key != "" {
			return key
		}
		if key := normalizeSkillKey(strVal(metadata, "paperclipSkillKey")); key != "" {
			return key
		}
		if paperclip, ok := metadata["paperclip"].(map[string]interface{}); ok {
			if key := normalizeSkillKey(strVal(paperclip, "skillKey")); key != "" {
				return key
			}
		}
	}

	slug := normalizeSlug(strVal(frontmatter, "slug"))
	if slug == "" {
		slug = normalizeSlug(fallbackSlug)
	}
	if slug == "" {
		slug = "skill"
	}

	sourceKind := ""
	owner := ""
	repo := ""
	if metadata != nil {
		sourceKind = strVal(metadata, "sourceKind")
		owner = normalizeSlug(strVal(metadata, "owner"))
		repo = normalizeSlug(strVal(metadata, "repo"))
	}

	if (sourceType == "github" || sourceType == "skills_sh" || sourceKind == "github" || sourceKind == "skills_sh") && owner != "" && repo != "" {
		return owner + "/" + repo + "/" + slug
	}
	if sourceKind == "paperclip_bundled" {
		return "paperclipai/paperclip/" + slug
	}
	if sourceType == "url" || sourceKind == "url" {
		host := "unknown"
		if sourceLocator != nil && *sourceLocator != "" {
			if u, err := url.Parse(*sourceLocator); err == nil && u.Hostname() != "" {
				host = normalizeSlug(u.Hostname())
			}
		}
		if host == "" {
			host = "unknown"
		}
		return "url/" + host + "/" + slug
	}
	return slug
}

func normalizeSkillKey(value string) string {
	if value == "" {
		return ""
	}
	segments := strings.Split(value, "/")
	out := make([]string, 0, len(segments))
	for _, segment := range segments {
		if normalized := normalizeSlug(segment); normalized != "" {
			out = append(out, normalized)
		}
	}
	return strings.Join(out, "/")
}

// ──────────────────────────────────────────────────────────────────────────────
// Frontmatter markdown parsing and building.
// ──────────────────────────────────────────────────────────────────────────────

// parseFrontmatter parses a markdown document that may start with a YAML frontmatter block.
// Returns the parsed frontmatter (as a generic map) and the body text.
func parseFrontmatter(raw string) (fm map[string]interface{}, body string) {
	s := strings.ReplaceAll(raw, "\r\n", "\n")
	if !strings.HasPrefix(s, "---\n") {
		return map[string]interface{}{}, strings.TrimSpace(s)
	}
	closing := strings.Index(s[4:], "\n---\n")
	if closing < 0 {
		return map[string]interface{}{}, strings.TrimSpace(s)
	}
	fmRaw := strings.TrimSpace(s[4 : 4+closing])
	bodyRaw := strings.TrimSpace(s[4+closing+5:])
	out := map[string]interface{}{}
	if err := yaml.Unmarshal([]byte(fmRaw), &out); err != nil {
		return map[string]interface{}{}, bodyRaw
	}
	return out, bodyRaw
}

// buildMarkdown constructs a frontmatter markdown document.
func buildMarkdown(fm map[string]interface{}, body string) string {
	body = strings.TrimSpace(strings.ReplaceAll(body, "\r\n", "\n"))
	yamlBytes, _ := yaml.Marshal(fm)
	frontmatter := "---\n" + string(yamlBytes) + "---\n"
	if body == "" {
		return frontmatter + "\n"
	}
	return frontmatter + "\n" + body + "\n"
}

// buildYAMLFile serialises a value to a YAML string.
func buildYAMLFile(v interface{}) string {
	b, err := yaml.Marshal(v)
	if err != nil {
		return "{}\n"
	}
	return string(b)
}

// ──────────────────────────────────────────────────────────────────────────────
// File-kind classification.
// ──────────────────────────────────────────────────────────────────────────────

func classifyFileKind(p string) string {
	p = strings.ReplaceAll(p, "\\", "/")
	switch {
	case p == "COMPANY.md":
		return "company"
	case p == ".paperclip.yaml" || p == ".paperclip.yml":
		return "extension"
	case p == "README.md":
		return "readme"
	case strings.HasPrefix(p, "agents/"):
		return "agent"
	case strings.HasPrefix(p, "skills/"):
		return "skill"
	case strings.HasPrefix(p, "projects/"):
		return "project"
	case strings.HasPrefix(p, "tasks/"):
		return "issue"
	default:
		return "other"
	}
}

func classifySkillInventoryKind(path string) string {
	path = strings.ReplaceAll(path, "\\", "/")
	switch {
	case path == "SKILL.md":
		return "skill"
	case strings.HasPrefix(path, "references/"):
		return "reference"
	case strings.HasPrefix(path, "scripts/"):
		return "script"
	case strings.HasPrefix(path, "assets/"):
		return "asset"
	case strings.HasSuffix(strings.ToLower(path), ".md"):
		return "markdown"
	default:
		return "other"
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// Inline file helpers.
// ──────────────────────────────────────────────────────────────────────────────

// readFileAsText returns the text content of an entry, or "" if it is binary/missing.
func readFileAsText(files map[string]interface{}, path string) (string, bool) {
	entry, ok := files[path]
	if !ok {
		return "", false
	}
	switch v := entry.(type) {
	case string:
		return v, true
	default:
		return "", false
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// JSON helpers for GORM datatypes.JSON.
// ──────────────────────────────────────────────────────────────────────────────

func jsonToMap(data datatypes.JSON) map[string]interface{} {
	if len(data) == 0 {
		return nil
	}
	m := map[string]interface{}{}
	_ = json.Unmarshal(data, &m)
	return m
}

func mapToJSON(m map[string]interface{}) datatypes.JSON {
	if m == nil {
		return datatypes.JSON("{}")
	}
	b, _ := json.Marshal(m)
	return datatypes.JSON(b)
}

func strSliceToJSON(ss []string) datatypes.JSON {
	if ss == nil {
		ss = []string{}
	}
	b, _ := json.Marshal(ss)
	return datatypes.JSON(b)
}

// strVal safely extracts a string value from a generic map.
func strVal(m map[string]interface{}, key string) string {
	if v, ok := m[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

func portStrPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func derefStr(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

// ──────────────────────────────────────────────────────────────────────────────
// Env-input helpers (mirrors TS isSensitiveEnvKey / extractPortableEnvInputs).
// ──────────────────────────────────────────────────────────────────────────────

// isSensitiveEnvKey returns true for API keys, tokens, secrets, passwords, etc.
func isSensitiveEnvKey(key string) bool {
	n := strings.ToLower(strings.TrimSpace(key))
	for _, pattern := range []string{
		"token", "_token", "-token",
		"apikey", "api_key", "api-key",
		"access_token", "access-token",
		"auth", "auth_token", "auth-token",
		"authorization", "bearer",
		"secret", "passwd", "password",
		"credential", "jwt",
		"privatekey", "private_key", "private-key",
		"cookie", "connectionstring",
	} {
		if n == pattern || strings.HasSuffix(n, pattern) || strings.Contains(n, pattern) {
			return true
		}
	}
	return false
}

// isAbsoluteCommand returns true for paths like /usr/bin/foo or C:\foo.
func isAbsoluteCommand(val string) bool {
	if val == "" {
		return false
	}
	// Unix-style absolute path
	if strings.HasPrefix(val, "/") {
		return true
	}
	// Windows-style C:\ or C:/
	if len(val) >= 3 && val[1] == ':' && (val[2] == '\\' || val[2] == '/') {
		return true
	}
	return false
}

type scopedEnvInputContext struct {
	label         string
	warningPrefix string
	agentSlug     *string
	projectSlug   *string
}

// extractPortableScopedEnvInputs converts an env map (plain/secret_ref bindings)
// into PortabilityEnvInput entries, mirroring the TS implementation exactly.
func extractPortableScopedEnvInputs(scope scopedEnvInputContext, env map[string]interface{}, warnings *[]string) []PortabilityEnvInput {
	out := []PortabilityEnvInput{}
	for key, binding := range env {
		if strings.ToUpper(key) == "PATH" {
			if warnings != nil {
				*warnings = append(*warnings, fmt.Sprintf("%s PATH override was omitted from export because it is system-dependent.", scope.warningPrefix))
			}
			continue
		}

		desc := func(d string) *string { return &d }(fmt.Sprintf("Optional default for %s on %s", key, scope.label))

		// secret_ref binding
		if m, ok := binding.(map[string]interface{}); ok && m["type"] == "secret_ref" {
			d := fmt.Sprintf("Provide %s for %s", key, scope.label)
			dv := ""
			out = append(out, PortabilityEnvInput{
				Key:          key,
				Description:  &d,
				AgentSlug:    scope.agentSlug,
				ProjectSlug:  scope.projectSlug,
				Kind:         "secret",
				Requirement:  "optional",
				DefaultValue: &dv,
				Portability:  "portable",
			})
			continue
		}

		// plain binding: { type: "plain", value: "..." }
		if m, ok := binding.(map[string]interface{}); ok && m["type"] == "plain" {
			raw := ""
			if v, ok := m["value"].(string); ok {
				raw = strings.TrimSpace(v)
			}
			sensitive := isSensitiveEnvKey(key)
			portability := "portable"
			if raw != "" && isAbsoluteCommand(raw) {
				portability = "system_dependent"
				if warnings != nil {
					*warnings = append(*warnings, fmt.Sprintf("%s env %s default was exported as system-dependent.", scope.warningPrefix, key))
				}
			}
			dv := ""
			if !sensitive {
				dv = raw
			}
			kind := "plain"
			if sensitive {
				kind = "secret"
			}
			out = append(out, PortabilityEnvInput{
				Key:          key,
				Description:  desc,
				AgentSlug:    scope.agentSlug,
				ProjectSlug:  scope.projectSlug,
				Kind:         kind,
				Requirement:  "optional",
				DefaultValue: &dv,
				Portability:  portability,
			})
			continue
		}

		// bare string binding
		if raw, ok := binding.(string); ok {
			raw = strings.TrimSpace(raw)
			portability := "portable"
			if isAbsoluteCommand(raw) {
				portability = "system_dependent"
				if warnings != nil {
					*warnings = append(*warnings, fmt.Sprintf("%s env %s default was exported as system-dependent.", scope.warningPrefix, key))
				}
			}
			sensitive := isSensitiveEnvKey(key)
			dv := ""
			if !sensitive {
				dv = raw
			}
			kind := "plain"
			if sensitive {
				kind = "secret"
			}
			out = append(out, PortabilityEnvInput{
				Key:          key,
				Description:  desc,
				AgentSlug:    scope.agentSlug,
				ProjectSlug:  scope.projectSlug,
				Kind:         kind,
				Requirement:  "optional",
				DefaultValue: &dv,
				Portability:  portability,
			})
		}
	}
	return out
}

func extractPortableEnvInputs(agentSlug string, adapterCfg map[string]interface{}, warnings *[]string) []PortabilityEnvInput {
	env, _ := adapterCfg["env"].(map[string]interface{})
	if env == nil {
		return nil
	}
	return extractPortableScopedEnvInputs(scopedEnvInputContext{
		label:         "agent " + agentSlug,
		warningPrefix: "Agent " + agentSlug,
		agentSlug:     &agentSlug,
	}, env, warnings)
}

func extractPortableProjectEnvInputs(projectSlug string, projectEnv interface{}, warnings *[]string) []PortabilityEnvInput {
	env, _ := projectEnv.(map[string]interface{})
	if env == nil {
		return nil
	}
	return extractPortableScopedEnvInputs(scopedEnvInputContext{
		label:         "project " + projectSlug,
		warningPrefix: "Project " + projectSlug,
		projectSlug:   &projectSlug,
	}, env, warnings)
}

// dedupeEnvInputs removes duplicate env inputs by (agentSlug, projectSlug, KEY).
func dedupeEnvInputs(inputs []PortabilityEnvInput) []PortabilityEnvInput {
	seen := map[string]bool{}
	out := []PortabilityEnvInput{}
	for _, v := range inputs {
		ag := ""
		if v.AgentSlug != nil {
			ag = *v.AgentSlug
		}
		pr := ""
		if v.ProjectSlug != nil {
			pr = *v.ProjectSlug
		}
		k := ag + ":" + pr + ":" + strings.ToUpper(v.Key)
		if seen[k] {
			continue
		}
		seen[k] = true
		out = append(out, v)
	}
	return out
}

// buildEnvInputMap converts a slice of PortabilityEnvInput into the YAML map
// shape written under a per-agent or per-project `inputs.env` key.
func buildEnvInputMap(inputs []PortabilityEnvInput) map[string]interface{} {
	m := map[string]interface{}{}
	for _, v := range inputs {
		entry := map[string]interface{}{
			"kind":        v.Kind,
			"requirement": v.Requirement,
		}
		if v.DefaultValue != nil {
			entry["default"] = *v.DefaultValue
		}
		if v.Description != nil && *v.Description != "" {
			entry["description"] = *v.Description
		}
		if v.Portability == "system_dependent" {
			entry["portability"] = "system_dependent"
		}
		m[v.Key] = entry
	}
	return m
}

// ──────────────────────────────────────────────────────────────────────────────
// Adapter-default pruning (mirrors TS pruneDefaultLikeValue).
// ──────────────────────────────────────────────────────────────────────────────

type defaultRule struct {
	path  []string
	value interface{}
}

var adapterDefaultRulesByType = map[string][]defaultRule{
	"codex_local": {
		{path: []string{"timeoutSec"}, value: float64(0)},
		{path: []string{"graceSec"}, value: float64(15)},
	},
	"gemini_local": {
		{path: []string{"timeoutSec"}, value: float64(0)},
		{path: []string{"graceSec"}, value: float64(15)},
	},
	"opencode_local": {
		{path: []string{"timeoutSec"}, value: float64(0)},
		{path: []string{"graceSec"}, value: float64(15)},
	},
	"cursor": {
		{path: []string{"timeoutSec"}, value: float64(0)},
		{path: []string{"graceSec"}, value: float64(15)},
	},
	"claude_local": {
		{path: []string{"timeoutSec"}, value: float64(0)},
		{path: []string{"graceSec"}, value: float64(15)},
		{path: []string{"maxTurnsPerRun"}, value: float64(1000)},
	},
	"openclaw_gateway": {
		{path: []string{"timeoutSec"}, value: float64(120)},
		{path: []string{"waitTimeoutMs"}, value: float64(120000)},
		{path: []string{"sessionKeyStrategy"}, value: "fixed"},
		{path: []string{"sessionKey"}, value: "paperclip"},
		{path: []string{"role"}, value: "operator"},
		{path: []string{"scopes"}, value: []interface{}{"operator.admin"}},
	},
}

var runtimeDefaultRules = []defaultRule{
	{path: []string{"heartbeat", "cooldownSec"}, value: float64(10)},
	{path: []string{"heartbeat", "intervalSec"}, value: float64(3600)},
	{path: []string{"heartbeat", "wakeOnOnDemand"}, value: true},
	{path: []string{"heartbeat", "wakeOnAssignment"}, value: true},
	{path: []string{"heartbeat", "wakeOnAutomation"}, value: true},
	{path: []string{"heartbeat", "wakeOnDemand"}, value: true},
	{path: []string{"heartbeat", "maxConcurrentRuns"}, value: float64(3)},
}

func jsonValEqual(a, b interface{}) bool {
	// Use JSON marshalling for deep equality (same as TS JSON.stringify comparison).
	aj, _ := json.Marshal(a)
	bj, _ := json.Marshal(b)
	return string(aj) == string(bj)
}

func isPathDefault(pathSegments []string, val interface{}, rules []defaultRule) bool {
	for _, r := range rules {
		if jsonValEqual(r.path, pathSegments) && jsonValEqual(r.value, val) {
			return true
		}
	}
	return false
}

// pruneDefaultLikeValue recursively strips values that match known defaults and
// false boolean values (when dropFalseBools=true), mirroring TS behaviour.
// Returns nil to signal the value should be omitted entirely.
func pruneDefaultLikeValue(val interface{}, path []string, dropFalseBools bool, rules []defaultRule) (interface{}, bool) {
	if rules != nil && isPathDefault(path, val, rules) {
		return nil, false
	}
	switch v := val.(type) {
	case bool:
		if dropFalseBools && !v {
			return nil, false
		}
		return v, true
	case map[string]interface{}:
		out := map[string]interface{}{}
		for k, entry := range v {
			childPath := append(append([]string{}, path...), k)
			pruned, keep := pruneDefaultLikeValue(entry, childPath, dropFalseBools, rules)
			if keep {
				out[k] = pruned
			}
		}
		return out, true
	case []interface{}:
		out := make([]interface{}, 0, len(v))
		for _, entry := range v {
			pruned, keep := pruneDefaultLikeValue(entry, path, dropFalseBools, rules)
			if keep {
				out = append(out, pruned)
			}
		}
		return out, true
	case nil:
		return nil, false
	}
	return val, true
}

func pruneAdapterConfig(adapterType string, cfg map[string]interface{}) map[string]interface{} {
	rules := adapterDefaultRulesByType[adapterType]
	pruned, _ := pruneDefaultLikeValue(cfg, []string{}, true, rules)
	if m, ok := pruned.(map[string]interface{}); ok {
		return m
	}
	return map[string]interface{}{}
}

func pruneRuntimeConfig(cfg map[string]interface{}) map[string]interface{} {
	pruned, _ := pruneDefaultLikeValue(cfg, []string{}, true, runtimeDefaultRules)
	if m, ok := pruned.(map[string]interface{}); ok {
		return m
	}
	return map[string]interface{}{}
}

func prunePermissions(cfg map[string]interface{}) map[string]interface{} {
	pruned, _ := pruneDefaultLikeValue(cfg, []string{}, true, nil)
	if m, ok := pruned.(map[string]interface{}); ok {
		return m
	}
	return map[string]interface{}{}
}

// ──────────────────────────────────────────────────────────────────────────────
// README generation and export helpers.
// ──────────────────────────────────────────────────────────────────────────────

var portabilityRoleLabels = map[string]string{
	"ceo":      "CEO",
	"cto":      "CTO",
	"cmo":      "CMO",
	"cfo":      "CFO",
	"coo":      "COO",
	"vp":       "VP",
	"manager":  "Manager",
	"engineer": "Engineer",
	"agent":    "Agent",
}

func portabilityRoleLabel(role string) string {
	if label, ok := portabilityRoleLabels[strings.ToLower(strings.TrimSpace(role))]; ok {
		return label
	}
	if role == "" {
		return "Agent"
	}
	return role
}

func portabilitySkillSourceLabel(skill PortabilitySkillManifestEntry) string {
	if skill.SourceLocator != nil && *skill.SourceLocator != "" {
		switch skill.SourceType {
		case "github", "skills_sh", "url":
			return fmt.Sprintf("[%s](%s)", skill.SourceType, *skill.SourceLocator)
		default:
			return *skill.SourceLocator
		}
	}
	if skill.SourceType == "local" || skill.SourceType == "local_path" {
		return "local"
	}
	if skill.SourceType == "" {
		return "—"
	}
	return skill.SourceType
}

func generateExportReadme(manifest PortabilityManifest) string {
	var sb strings.Builder
	companyName := "Company Package"
	companyDescription := ""
	if manifest.Company != nil && manifest.Company.Name != "" {
		companyName = manifest.Company.Name
	}
	if manifest.Company != nil && manifest.Company.Description != nil {
		companyDescription = strings.TrimSpace(*manifest.Company.Description)
	}

	sb.WriteString("# " + companyName + "\n\n")
	if companyDescription != "" {
		sb.WriteString("> " + companyDescription + "\n\n")
	}

	if len(manifest.Agents) > 0 {
		sb.WriteString("![Org Chart](images/org-chart.svg)\n\n")
	}

	sb.WriteString("## What's Inside\n\n")
	sb.WriteString("> This is an [Agent Company](https://agentcompanies.io) package from [Paperclip](https://paperclip.ing)\n\n")

	counts := [][2]interface{}{}
	if len(manifest.Agents) > 0 {
		counts = append(counts, [2]interface{}{"Agents", len(manifest.Agents)})
	}
	if len(manifest.Projects) > 0 {
		counts = append(counts, [2]interface{}{"Projects", len(manifest.Projects)})
	}
	if len(manifest.Skills) > 0 {
		counts = append(counts, [2]interface{}{"Skills", len(manifest.Skills)})
	}
	if len(manifest.Issues) > 0 {
		counts = append(counts, [2]interface{}{"Tasks", len(manifest.Issues)})
	}
	if len(counts) > 0 {
		sb.WriteString("| Content | Count |\n|---------|-------|\n")
		for _, entry := range counts {
			sb.WriteString(fmt.Sprintf("| %s | %d |\n", entry[0], entry[1]))
		}
		sb.WriteString("\n")
	}

	if len(manifest.Agents) > 0 {
		sb.WriteString("### Agents\n\n")
		sb.WriteString("| Agent | Role | Reports To |\n|-------|------|------------|\n")
		for _, a := range manifest.Agents {
			reportsTo := "—"
			if a.ReportsToSlug != nil && *a.ReportsToSlug != "" {
				reportsTo = *a.ReportsToSlug
			}
			sb.WriteString(fmt.Sprintf("| %s | %s | %s |\n", a.Name, portabilityRoleLabel(a.Role), reportsTo))
		}
		sb.WriteString("\n")
	}

	if len(manifest.Projects) > 0 {
		sb.WriteString("### Projects\n\n")
		for _, p := range manifest.Projects {
			sb.WriteString("- **" + p.Name + "**")
			if p.Description != nil && *p.Description != "" {
				sb.WriteString(" — " + *p.Description)
			}
			sb.WriteString("\n")
		}
		sb.WriteString("\n")
	}
	if len(manifest.Skills) > 0 {
		sb.WriteString("### Skills\n\n")
		sb.WriteString("| Skill | Description | Source |\n|-------|-------------|--------|\n")
		for _, sk := range manifest.Skills {
			description := "—"
			if sk.Description != nil && *sk.Description != "" {
				description = *sk.Description
			}
			sb.WriteString(fmt.Sprintf("| %s | %s | %s |\n", sk.Name, description, portabilitySkillSourceLabel(sk)))
		}
		sb.WriteString("\n")
	}

	sb.WriteString("## Getting Started\n\n```bash\npnpm paperclipai company import this-github-url-or-folder\n```\n\n")
	sb.WriteString("See [Paperclip](https://paperclip.ing) for more information.\n\n")
	sb.WriteString("---\n")
	sb.WriteString(fmt.Sprintf("Exported from [Paperclip](https://paperclip.ing) on %s\n", strings.Split(time.Now().UTC().Format(time.RFC3339), "T")[0]))
	return sb.String()
}

func htmlEscapePortable(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, `"`, "&quot;")
	return s
}

func generateOrgChartSVG(agents []PortabilityAgentManifestEntry) string {
	if len(agents) == 0 {
		return ""
	}

	type nodePos struct{ x, y int }
	childMap := map[string][]string{}
	agentBySlug := map[string]PortabilityAgentManifestEntry{}
	roots := []string{}
	for _, agent := range agents {
		agentBySlug[agent.Slug] = agent
		if agent.ReportsToSlug == nil || *agent.ReportsToSlug == "" {
			roots = append(roots, agent.Slug)
			continue
		}
		childMap[*agent.ReportsToSlug] = append(childMap[*agent.ReportsToSlug], agent.Slug)
	}
	if len(roots) == 0 {
		for _, agent := range agents {
			roots = append(roots, agent.Slug)
		}
	}
	sort.Strings(roots)

	nodeW, nodeH := 220, 80
	hGap, vGap := 20, 60
	colIdx := map[int]int{}
	positions := map[string]nodePos{}
	visited := map[string]bool{}
	var place func(string, int)
	place = func(slug string, level int) {
		if visited[slug] {
			return
		}
		visited[slug] = true
		col := colIdx[level]
		colIdx[level]++
		positions[slug] = nodePos{x: col*(nodeW+hGap) + 10, y: level*(nodeH+vGap) + 10}
		children := append([]string{}, childMap[slug]...)
		sort.Strings(children)
		for _, child := range children {
			place(child, level+1)
		}
	}
	for _, root := range roots {
		place(root, 0)
	}
	for _, agent := range agents {
		place(agent.Slug, 0)
	}

	maxX, maxY := 420, 220
	for _, pos := range positions {
		if pos.x+nodeW+10 > maxX {
			maxX = pos.x + nodeW + 10
		}
		if pos.y+nodeH+10 > maxY {
			maxY = pos.y + nodeH + 10
		}
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" width="%d" height="%d">`, maxX, maxY))
	sb.WriteString(`<rect width="100%" height="100%" fill="white"/>`)
	for slug, pos := range positions {
		for _, child := range childMap[slug] {
			childPos, ok := positions[child]
			if !ok {
				continue
			}
			sb.WriteString(fmt.Sprintf(`<line x1="%d" y1="%d" x2="%d" y2="%d" stroke="#1e1b4b" stroke-width="1.5"/>`, pos.x+nodeW/2, pos.y+nodeH, childPos.x+nodeW/2, childPos.y))
		}
	}
	for slug, pos := range positions {
		agent := agentBySlug[slug]
		name := agent.Name
		if len(name) > 20 {
			name = name[:20] + "..."
		}
		role := portabilityRoleLabel(agent.Role)
		sb.WriteString(fmt.Sprintf(`<rect x="%d" y="%d" width="%d" height="%d" rx="8" fill="#e0e7ff" stroke="#1e1b4b"/>`, pos.x, pos.y, nodeW, nodeH))
		sb.WriteString(fmt.Sprintf(`<text x="%d" y="%d" fill="#1e1b4b" font-size="13" text-anchor="middle">%s</text>`, pos.x+nodeW/2, pos.y+35, htmlEscapePortable(name)))
		sb.WriteString(fmt.Sprintf(`<text x="%d" y="%d" fill="#4338ca" font-size="11" text-anchor="middle">%s</text>`, pos.x+nodeW/2, pos.y+55, htmlEscapePortable(role)))
	}
	sb.WriteString(`</svg>`)
	return sb.String()
}

func buildSkillSourceMetadata(skill models.CompanySkill) map[string]interface{} {
	metadata := jsonToMap(skill.Metadata)
	if metadata == nil {
		metadata = map[string]interface{}{}
	}

	source := map[string]interface{}{}
	sourceKind := strVal(metadata, "sourceKind")
	switch {
	case sourceKind == "paperclip_bundled":
		source["kind"] = "github-dir"
		source["repo"] = "paperclipai/paperclip"
		source["path"] = "skills/" + skill.Slug
		source["trackingRef"] = "master"
		source["url"] = fmt.Sprintf("https://github.com/paperclipai/paperclip/tree/master/skills/%s", skill.Slug)
	case (skill.SourceType == "github" || skill.SourceType == "skills_sh") && strVal(metadata, "owner") != "" && strVal(metadata, "repo") != "":
		source["kind"] = "github-dir"
		source["repo"] = fmt.Sprintf("%s/%s", strVal(metadata, "owner"), strVal(metadata, "repo"))
		if repoSkillDir := strVal(metadata, "repoSkillDir"); repoSkillDir != "" {
			source["path"] = repoSkillDir
		}
		if skill.SourceRef != nil && *skill.SourceRef != "" {
			source["commit"] = *skill.SourceRef
		}
		if trackingRef := strVal(metadata, "trackingRef"); trackingRef != "" {
			source["trackingRef"] = trackingRef
		}
		if skill.SourceLocator != nil && *skill.SourceLocator != "" {
			source["url"] = *skill.SourceLocator
		}
	case skill.SourceType == "url" && skill.SourceLocator != nil && *skill.SourceLocator != "":
		source["kind"] = "url"
		source["url"] = *skill.SourceLocator
	}
	if len(source) == 0 {
		source = nil
	}

	normalized := map[string]interface{}{
		"skillKey":          skill.Key,
		"paperclipSkillKey": skill.Key,
		"paperclip": map[string]interface{}{
			"skillKey": skill.Key,
			"slug":     skill.Slug,
		},
	}
	for key, value := range metadata {
		normalized[key] = value
	}
	if source != nil {
		normalized["sources"] = []interface{}{source}
	}
	return normalized
}

// ──────────────────────────────────────────────────────────────────────────────
// Import helpers (Gap 6).
// ──────────────────────────────────────────────────────────────────────────────

// disableImportedTimerHeartbeat clones the runtimeConfig and sets
// heartbeat.enabled = false so imported agents don't auto-start heartbeat
// loops immediately after import, mirroring the TS implementation.
func disableImportedTimerHeartbeat(runtimeConfig map[string]interface{}) map[string]interface{} {
	next := cloneMap(runtimeConfig)
	if next == nil {
		next = map[string]interface{}{}
	}
	hb, _ := next["heartbeat"].(map[string]interface{})
	if hb == nil {
		hb = map[string]interface{}{}
	} else {
		// shallow copy
		hbCopy := make(map[string]interface{}, len(hb))
		for k, v := range hb {
			hbCopy[k] = v
		}
		hb = hbCopy
	}
	hb["enabled"] = false
	next["heartbeat"] = hb
	return next
}

// ──────────────────────────────────────────────────────────────────────────────
// GitHub source fetching.
// ──────────────────────────────────────────────────────────────────────────────


type ghParsedURL struct {
	hostname    string
	owner       string
	repo        string
	ref         string
	basePath    string
	companyPath string
}

func parseGitHubSourceURL(rawURL string) (*ghParsedURL, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("invalid URL: %v", err)
	}
	hostname := u.Hostname()
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) < 2 {
		return nil, fmt.Errorf("invalid GitHub URL: need at least owner/repo")
	}
	owner := parts[0]
	repo := strings.TrimSuffix(parts[1], ".git")

	queryRef := strings.TrimSpace(u.Query().Get("ref"))
	queryPath := strings.Trim(u.Query().Get("path"), "/")
	queryCompanyPath := strings.Trim(u.Query().Get("companyPath"), "/")

	if queryRef != "" || queryPath != "" || queryCompanyPath != "" {
		companyPath := queryCompanyPath
		if companyPath == "" {
			if queryPath != "" {
				companyPath = queryPath + "/COMPANY.md"
			} else {
				companyPath = "COMPANY.md"
			}
		}
		if queryRef == "" {
			queryRef = "main"
		}
		return &ghParsedURL{hostname: hostname, owner: owner, repo: repo, ref: queryRef, basePath: queryPath, companyPath: companyPath}, nil
	}

	ref := "main"
	basePath := ""
	companyPath := "COMPANY.md"

	if len(parts) >= 3 {
		switch parts[2] {
		case "tree":
			if len(parts) >= 4 {
				ref = parts[3]
			}
			basePath = strings.Join(parts[4:], "/")
		case "blob":
			if len(parts) >= 4 {
				ref = parts[3]
			}
			blobPath := strings.Join(parts[4:], "/")
			if blobPath == "" {
				return nil, fmt.Errorf("invalid GitHub blob URL")
			}
			companyPath = blobPath
			idx := strings.LastIndex(blobPath, "/")
			if idx >= 0 {
				basePath = blobPath[:idx]
			}
		}
	}

	return &ghParsedURL{hostname: hostname, owner: owner, repo: repo, ref: ref, basePath: basePath, companyPath: companyPath}, nil
}

func rawGitHubURL(p *ghParsedURL, filePath string) string {
	return ResolveRawGitHubURL(p.hostname, p.owner, p.repo, p.ref, filePath)
}

func ghGet(ctx context.Context, rawURL string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", rawURL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := GHFetch(ctx, rawURL, req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil, nil
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d fetching %s", resp.StatusCode, rawURL)
	}
	return io.ReadAll(resp.Body)
}

func ghGetText(ctx context.Context, rawURL string) (string, error) {
	b, err := ghGet(ctx, rawURL)
	if err != nil || b == nil {
		return "", err
	}
	return string(b), nil
}

type ghTree struct {
	Tree []struct {
		Path string `json:"path"`
		Type string `json:"type"`
	} `json:"tree"`
}

func fetchGitHubTree(ctx context.Context, p *ghParsedURL) ([]string, error) {
	apiBase := GitHubAPIBase(p.hostname)
	u := fmt.Sprintf("%s/repos/%s/%s/git/trees/%s?recursive=1", apiBase, p.owner, p.repo, p.ref)
	req, _ := http.NewRequestWithContext(ctx, "GET", u, nil)
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := GHFetch(ctx, u, req)
	if err != nil {
		return nil, nil // non-fatal
	}
	defer resp.Body.Close()
	var tree ghTree
	if err := json.NewDecoder(resp.Body).Decode(&tree); err != nil {
		return nil, nil
	}
	paths := make([]string, 0, len(tree.Tree))
	for _, e := range tree.Tree {
		if e.Type == "blob" {
			paths = append(paths, e.Path)
		}
	}
	return paths, nil
}

// ──────────────────────────────────────────────────────────────────────────────
// Manifest building from inline files.
// ──────────────────────────────────────────────────────────────────────────────

// buildManifestFromFiles parses a file map and constructs a PortabilityManifest.
func buildManifestFromFiles(files map[string]interface{}, sourceLabel *PortabilityManifestSource) (resolvedSource, error) {
	warnings := []string{}

	// Find COMPANY.md
	companyPath := ""
	if _, ok := files["COMPANY.md"]; ok {
		companyPath = "COMPANY.md"
	} else {
		for k := range files {
			if strings.HasSuffix(k, "/COMPANY.md") {
				companyPath = k
				break
			}
		}
	}
	if companyPath == "" {
		return resolvedSource{}, fmt.Errorf("company package is missing COMPANY.md")
	}
	companyMarkdown, ok := readFileAsText(files, companyPath)
	if !ok {
		return resolvedSource{}, fmt.Errorf("company package file is not readable as text: %s", companyPath)
	}

	companyFM, _ := parseFrontmatter(companyMarkdown)

	// Find .paperclip.yaml extension
	var paperclipExt map[string]interface{}
	for _, extPath := range []string{".paperclip.yaml", ".paperclip.yml"} {
		if text, ok := readFileAsText(files, extPath); ok {
			_ = yaml.Unmarshal([]byte(text), &paperclipExt)
			break
		}
	}
	if paperclipExt == nil {
		paperclipExt = map[string]interface{}{}
	}
	pcCompany, _ := paperclipExt["company"].(map[string]interface{})
	if pcCompany == nil {
		pcCompany = map[string]interface{}{}
	}
	var sidebar *PortabilitySidebarOrder
	if pcSidebar, ok := paperclipExt["sidebar"].(map[string]interface{}); ok {
		sidebar = &PortabilitySidebarOrder{}
		if agents, ok := pcSidebar["agents"].([]interface{}); ok {
			for _, a := range agents {
				if s, ok := a.(string); ok {
					sidebar.Agents = append(sidebar.Agents, s)
				}
			}
		}
		if projects, ok := pcSidebar["projects"].([]interface{}); ok {
			for _, p := range projects {
				if s, ok := p.(string); ok {
					sidebar.Projects = append(sidebar.Projects, s)
				}
			}
		}
	}

	// Build company manifest entry
	companyName := strVal(companyFM, "name")
	if companyName == "" && sourceLabel != nil {
		companyName = sourceLabel.CompanyName
	}
	if companyName == "" {
		companyName = "Imported Company"
	}

	var companyDesc *string
	if d := strVal(companyFM, "description"); d != "" {
		companyDesc = &d
	}
	brandColor := portStrPtr(strVal(pcCompany, "brandColor"))
	logoPath := portStrPtr(strVal(pcCompany, "logoPath"))
	if logoPath == nil {
		logoPath = portStrPtr(strVal(pcCompany, "logo"))
	}
	requireApproval := true
	if v, ok := pcCompany["requireBoardApprovalForNewAgents"].(bool); ok {
		requireApproval = v
	}
	feedbackEnabled := false
	if v, ok := pcCompany["feedbackDataSharingEnabled"].(bool); ok {
		feedbackEnabled = v
	}
	var feedbackConsentAt *string
	if v, ok := pcCompany["feedbackDataSharingConsentAt"].(string); ok && v != "" {
		feedbackConsentAt = &v
	}
	var feedbackConsentByUserID *string
	if v, ok := pcCompany["feedbackDataSharingConsentByUserId"].(string); ok && v != "" {
		feedbackConsentByUserID = &v
	}
	var feedbackTermsVersion *string
	if v, ok := pcCompany["feedbackDataSharingTermsVersion"].(string); ok && v != "" {
		feedbackTermsVersion = &v
	}

	companyEntry := &PortabilityCompanyManifestEntry{
		Path:                               companyPath,
		Name:                               companyName,
		Description:                        companyDesc,
		BrandColor:                         brandColor,
		LogoPath:                           logoPath,
		RequireBoardApprovalForNewAgents:   requireApproval,
		FeedbackDataSharingEnabled:         feedbackEnabled,
		FeedbackDataSharingConsentAt:       feedbackConsentAt,
		FeedbackDataSharingConsentByUserID: feedbackConsentByUserID,
		FeedbackDataSharingTermsVersion:    feedbackTermsVersion,
	}

	if logoPath != nil && *logoPath != "" {
		if _, exists := files[*logoPath]; !exists {
			warnings = append(warnings, fmt.Sprintf("Referenced company logo file is missing from package: %s", *logoPath))
		}
	}

	// Discover agent/project/task/skill files
	var agentPaths, projectPaths, taskPaths, skillPaths []string
	for k := range files {
		switch {
		case strings.HasSuffix(k, "/AGENTS.md") || k == "AGENTS.md":
			agentPaths = append(agentPaths, k)
		case strings.HasSuffix(k, "/PROJECT.md") || k == "PROJECT.md":
			projectPaths = append(projectPaths, k)
		case strings.HasSuffix(k, "/TASK.md") || k == "TASK.md":
			taskPaths = append(taskPaths, k)
		case strings.HasSuffix(k, "/SKILL.md") || k == "SKILL.md":
			skillPaths = append(skillPaths, k)
		}
	}
	sort.Strings(agentPaths)
	sort.Strings(projectPaths)
	sort.Strings(taskPaths)
	sort.Strings(skillPaths)

	// Parse paperclip extension sections
	pcAgents, _ := paperclipExt["agents"].(map[string]interface{})
	pcProjects, _ := paperclipExt["projects"].(map[string]interface{})
	pcTasks, _ := paperclipExt["tasks"].(map[string]interface{})
	pcRoutines, _ := paperclipExt["routines"].(map[string]interface{})

	// Build agent manifest entries
	agentEntries := make([]PortabilityAgentManifestEntry, 0, len(agentPaths))
	for _, agPath := range agentPaths {
		text, ok := readFileAsText(files, agPath)
		if !ok {
			warnings = append(warnings, fmt.Sprintf("Referenced agent file is missing from package: %s", agPath))
			continue
		}
		fm, _ := parseFrontmatter(text)

		// Slug is derived from the directory name (agents/<slug>/AGENTS.md)
		slug := ""
		parts := strings.Split(agPath, "/")
		if len(parts) >= 2 {
			slug = parts[len(parts)-2]
		} else {
			slug = normalizeSlug(strVal(fm, "name"))
		}
		if slug == "" {
			slug = "agent"
		}

		name := strVal(fm, "name")
		if name == "" {
			name = slug
		}

		reportsTo := portStrPtr(strVal(fm, "reportsTo"))
		if reportsTo == nil {
			reportsTo = portStrPtr(strVal(fm, "reportsToSlug"))
		}

		var skills []string
		if rawSkills, ok := fm["skills"].([]interface{}); ok {
			for _, s := range rawSkills {
				if sv, ok := s.(string); ok && sv != "" {
					skills = append(skills, sv)
				}
			}
		}

		// Merge extension from .paperclip.yaml agents section
		ext := map[string]interface{}{}
		if pcAgents != nil {
			if agExt, ok := pcAgents[slug].(map[string]interface{}); ok {
				ext = agExt
			}
		}

		role := strVal(ext, "role")
		if role == "" {
			role = strVal(fm, "role")
		}
		if role == "" {
			role = "agent"
		}
		icon := portStrPtr(strVal(ext, "icon"))
		capabilities := portStrPtr(strVal(ext, "capabilities"))
		title := portStrPtr(strVal(ext, "title"))
		if title == nil {
			title = portStrPtr(strVal(fm, "title"))
		}

		adapterConfig := map[string]interface{}{}
		if adapter, ok := ext["adapter"].(map[string]interface{}); ok {
			if cfg, ok := adapter["config"].(map[string]interface{}); ok {
				adapterConfig = cfg
			}
		}
		adapterType := "process"
		if adapter, ok := ext["adapter"].(map[string]interface{}); ok {
			if t := strVal(adapter, "type"); t != "" {
				adapterType = t
			}
		}
		runtimeConfig := map[string]interface{}{}
		if rc, ok := ext["runtime"].(map[string]interface{}); ok {
			runtimeConfig = rc
		}
		permissions := map[string]interface{}{}
		if perm, ok := ext["permissions"].(map[string]interface{}); ok {
			permissions = perm
		}
		budgetMonthlyCents := 0
		if b, ok := ext["budgetMonthlyCents"].(int); ok {
			budgetMonthlyCents = b
		} else if b, ok := ext["budgetMonthlyCents"].(float64); ok {
			budgetMonthlyCents = int(b)
		}
		var metadata map[string]interface{}
		if md, ok := ext["metadata"].(map[string]interface{}); ok {
			metadata = md
		}

		agentEntries = append(agentEntries, PortabilityAgentManifestEntry{
			Slug:               slug,
			Name:               name,
			Path:               agPath,
			Skills:             skills,
			Role:               role,
			Title:              title,
			Icon:               icon,
			Capabilities:       capabilities,
			ReportsToSlug:      reportsTo,
			AdapterType:        adapterType,
			AdapterConfig:      adapterConfig,
			RuntimeConfig:      runtimeConfig,
			Permissions:        permissions,
			BudgetMonthlyCents: budgetMonthlyCents,
			Metadata:           metadata,
		})
	}

	// Build project manifest entries
	projectEntries := make([]PortabilityProjectManifestEntry, 0, len(projectPaths))
	for _, prPath := range projectPaths {
		text, ok := readFileAsText(files, prPath)
		if !ok {
			warnings = append(warnings, fmt.Sprintf("Referenced project file is missing from package: %s", prPath))
			continue
		}
		fm, _ := parseFrontmatter(text)

		slug := ""
		parts := strings.Split(prPath, "/")
		if len(parts) >= 2 {
			slug = parts[len(parts)-2]
		}
		if slug == "" {
			slug = deriveProjectSlug(strVal(fm, "name"))
		}

		name := strVal(fm, "name")
		if name == "" {
			name = slug
		}
		desc := portStrPtr(strVal(fm, "description"))

		ext := map[string]interface{}{}
		if pcProjects != nil {
			if prExt, ok := pcProjects[slug].(map[string]interface{}); ok {
				ext = prExt
			}
		}

		ownerSlug := portStrPtr(strVal(fm, "owner"))
		leadSlug := portStrPtr(strVal(ext, "leadAgentSlug"))
		if leadSlug == nil {
			leadSlug = ownerSlug
		}
		targetDate := portStrPtr(strVal(ext, "targetDate"))
		color := portStrPtr(strVal(ext, "color"))
		status := portStrPtr(strVal(ext, "status"))
		var execPolicy map[string]interface{}
		if ep, ok := ext["executionWorkspacePolicy"].(map[string]interface{}); ok {
			execPolicy = ep
		}

		// Parse workspaces
		var workspaces []PortabilityWorkspaceEntry
		if wsList, ok := ext["workspaces"].([]interface{}); ok {
			for i, wsRaw := range wsList {
				ws, ok := wsRaw.(map[string]interface{})
				if !ok {
					continue
				}
				key := strVal(ws, "key")
				if key == "" {
					key = fmt.Sprintf("workspace-%d", i+1)
				}
				wsName := strVal(ws, "name")
				if wsName == "" {
					wsName = key
				}
				workspaces = append(workspaces, PortabilityWorkspaceEntry{
					Key:            key,
					Name:           wsName,
					SourceType:     portStrPtr(strVal(ws, "sourceType")),
					RepoURL:        portStrPtr(strVal(ws, "repoUrl")),
					RepoRef:        portStrPtr(strVal(ws, "repoRef")),
					DefaultRef:     portStrPtr(strVal(ws, "defaultRef")),
					Visibility:     portStrPtr(strVal(ws, "visibility")),
					SetupCommand:   portStrPtr(strVal(ws, "setupCommand")),
					CleanupCommand: portStrPtr(strVal(ws, "cleanupCommand")),
					IsPrimary:      i == 0,
				})
			}
		}
		if len(workspaces) == 0 {
			workspaces = []PortabilityWorkspaceEntry{}
		}

		projectEntries = append(projectEntries, PortabilityProjectManifestEntry{
			Slug:                     slug,
			Name:                     name,
			Path:                     prPath,
			Description:              desc,
			OwnerAgentSlug:           ownerSlug,
			LeadAgentSlug:            leadSlug,
			TargetDate:               targetDate,
			Color:                    color,
			Status:                   status,
			ExecutionWorkspacePolicy: execPolicy,
			Workspaces:               workspaces,
		})
	}

	// Build issue/task manifest entries
	issueEntries := make([]PortabilityIssueManifestEntry, 0, len(taskPaths))
	for _, taskPath := range taskPaths {
		text, ok := readFileAsText(files, taskPath)
		if !ok {
			warnings = append(warnings, fmt.Sprintf("Referenced task file is missing from package: %s", taskPath))
			continue
		}
		fm, _ := parseFrontmatter(text)

		slug := ""
		parts := strings.Split(taskPath, "/")
		if len(parts) >= 2 {
			slug = parts[len(parts)-2]
		}
		if slug == "" {
			slug = normalizeSlug(strVal(fm, "name"))
		}
		if slug == "" {
			slug = "task"
		}

		title := strVal(fm, "name")
		if title == "" {
			title = slug
		}
		desc := portStrPtr(strVal(fm, "description"))
		if *desc == "" {
			// Use body as description for markdown-only format
			_, body := parseFrontmatter(text)
			if body != "" {
				desc = &body
			} else {
				desc = nil
			}
		}
		projectSlug := portStrPtr(strVal(fm, "project"))
		assigneeSlug := portStrPtr(strVal(fm, "assignee"))
		recurring := false
		if v, ok := fm["recurring"].(bool); ok {
			recurring = v
		}

		identifier := portStrPtr(strVal(fm, "identifier"))

		// Merge extension from .paperclip.yaml tasks section
		ext := map[string]interface{}{}
		if pcTasks != nil {
			if taskExt, ok := pcTasks[slug].(map[string]interface{}); ok {
				ext = taskExt
			}
		}
		status := portStrPtr(strVal(ext, "status"))
		priority := portStrPtr(strVal(ext, "priority"))
		billingCode := portStrPtr(strVal(ext, "billingCode"))

		// Routine from .paperclip.yaml routines section
		var routine *PortabilityIssueRoutine
		if pcRoutines != nil {
			if routineExt, ok := pcRoutines[slug].(map[string]interface{}); ok {
				r := &PortabilityIssueRoutine{
					ConcurrencyPolicy: portStrPtr(strVal(routineExt, "concurrencyPolicy")),
					CatchUpPolicy:     portStrPtr(strVal(routineExt, "catchUpPolicy")),
				}
				if triggers, ok := routineExt["triggers"].([]interface{}); ok {
					for _, tRaw := range triggers {
						t, ok := tRaw.(map[string]interface{})
						if !ok {
							continue
						}
						enabled := true
						if v, ok := t["enabled"].(bool); ok {
							enabled = v
						}
						var replayWindow *int
						if v, ok := t["replayWindowSec"].(int); ok {
							replayWindow = &v
						} else if v, ok := t["replayWindowSec"].(float64); ok {
							vi := int(v)
							replayWindow = &vi
						}
						r.Triggers = append(r.Triggers, PortabilityIssueRoutineTrigger{
							Kind:            strVal(t, "kind"),
							Label:           portStrPtr(strVal(t, "label")),
							Enabled:         enabled,
							CronExpression:  portStrPtr(strVal(t, "cronExpression")),
							Timezone:        portStrPtr(strVal(t, "timezone")),
							SigningMode:     portStrPtr(strVal(t, "signingMode")),
							ReplayWindowSec: replayWindow,
						})
					}
				}
				if len(r.Triggers) == 0 {
					r.Triggers = []PortabilityIssueRoutineTrigger{}
				}
				routine = r
				recurring = true
			}
		}

		issueEntries = append(issueEntries, PortabilityIssueManifestEntry{
			Slug:              slug,
			Identifier:        identifier,
			Title:             title,
			Path:              taskPath,
			ProjectSlug:       projectSlug,
			AssigneeAgentSlug: assigneeSlug,
			Description:       desc,
			Recurring:         recurring,
			Routine:           routine,
			Status:            status,
			Priority:          priority,
			BillingCode:       billingCode,
			LabelIds:          []string{},
		})
	}

	// Build skill manifest entries
	skillEntries := make([]PortabilitySkillManifestEntry, 0, len(skillPaths))
	for _, skillPath := range skillPaths {
		text, ok := readFileAsText(files, skillPath)
		if !ok {
			warnings = append(warnings, fmt.Sprintf("Referenced skill file is missing from package: %s", skillPath))
			continue
		}
		fm, _ := parseFrontmatter(text)

		slug := ""
		parts := strings.Split(skillPath, "/")
		if len(parts) >= 2 {
			slug = parts[len(parts)-2]
		}
		if slug == "" {
			slug = normalizeSlug(strVal(fm, "name"))
		}
		if slug == "" {
			slug = "skill"
		}

		skillDir := skillPath
		if idx := strings.LastIndex(skillPath, "/"); idx >= 0 {
			skillDir = skillPath[:idx]
		}
		fileInventory := make([]PortabilitySkillFileInventory, 0)
		for entry := range files {
			if entry != skillPath && !strings.HasPrefix(entry, skillDir+"/") {
				continue
			}
			relativePath := "SKILL.md"
			if entry != skillPath {
				relativePath = strings.TrimPrefix(entry, skillDir+"/")
			}
			fileInventory = append(fileInventory, PortabilitySkillFileInventory{
				Path: relativePath,
				Kind: classifySkillInventoryKind(relativePath),
			})
		}
		sort.Slice(fileInventory, func(i, j int) bool {
			return fileInventory[i].Path < fileInventory[j].Path
		})

		metadata, _ := fm["metadata"].(map[string]interface{})
		var primarySource map[string]interface{}
		if metadata != nil {
			if sources, ok := metadata["sources"].([]interface{}); ok {
				for _, raw := range sources {
					if source, ok := raw.(map[string]interface{}); ok {
						primarySource = source
						break
					}
				}
			}
		}
		sourceType := "catalog"
		var sourceLocator *string
		var sourceRef *string
		normalizedMetadata := map[string]interface{}{}
		for key, value := range metadata {
			normalizedMetadata[key] = value
		}
		if len(normalizedMetadata) == 0 {
			normalizedMetadata = nil
		}
		if primarySource != nil {
			switch kind := strVal(primarySource, "kind"); kind {
			case "github-dir", "github-file":
				sourceType = "github"
				sourceLocator = portStrPtr(strVal(primarySource, "url"))
				sourceRef = portStrPtr(strVal(primarySource, "commit"))
				if sourceRef == nil {
					sourceRef = portStrPtr(strVal(primarySource, "ref"))
				}
			case "url":
				sourceType = "url"
				sourceLocator = portStrPtr(strVal(primarySource, "url"))
			}
		}
		if sourceType == "catalog" && metadata != nil {
			switch strVal(metadata, "sourceKind") {
			case "github", "skills_sh", "paperclip_bundled":
				sourceType = "github"
			case "url":
				sourceType = "url"
			}
		}

		key := deriveManifestSkillKey(fm, slug, metadata, sourceType, sourceLocator)
		if key == "" {
			key = slug
		}
		name := strVal(fm, "name")
		if name == "" {
			name = slug
		}
		desc := portStrPtr(strVal(fm, "description"))

		skillEntries = append(skillEntries, PortabilitySkillManifestEntry{
			Key:           key,
			Slug:          slug,
			Name:          name,
			Path:          skillPath,
			Description:   desc,
			SourceType:    sourceType,
			SourceLocator: sourceLocator,
			SourceRef:     sourceRef,
			Metadata:      normalizedMetadata,
			FileInventory: fileInventory,
		})
	}

	manifest := PortabilityManifest{
		SchemaVersion: 5,
		GeneratedAt:   time.Now().UTC().Format(time.RFC3339),
		Source:        sourceLabel,
		Includes: PortabilityInclude{
			Company:  true,
			Agents:   len(agentEntries) > 0,
			Projects: len(projectEntries) > 0,
			Issues:   len(issueEntries) > 0,
			Skills:   len(skillEntries) > 0,
		},
		Company:   companyEntry,
		Sidebar:   sidebar,
		Agents:    agentEntries,
		Skills:    skillEntries,
		Projects:  projectEntries,
		Issues:    issueEntries,
		EnvInputs: []PortabilityEnvInput{},
	}

	return resolvedSource{
		manifest: manifest,
		files:    files,
		warnings: warnings,
	}, nil
}

// ──────────────────────────────────────────────────────────────────────────────
// PortabilityService provides company export and import operations.
// ──────────────────────────────────────────────────────────────────────────────

// PortabilityService handles company export and import.
type PortabilityService struct {
	db *gorm.DB
}

// NewPortabilityService creates a new PortabilityService.
func NewPortabilityService(db *gorm.DB) *PortabilityService {
	return &PortabilityService{db: db}
}

// resolveInclude merges the request include flags with sensible defaults.
func resolveInclude(req map[string]bool) PortabilityInclude {
	get := func(key string, defaultVal bool) bool {
		if v, ok := req[key]; ok {
			return v
		}
		return defaultVal
	}
	return PortabilityInclude{
		Company:  get("company", true),
		Agents:   get("agents", true),
		Projects: get("projects", false),
		Issues:   get("issues", false),
		Skills:   get("skills", false),
	}
}

// ExportBundle builds a portability package for the given company.
func (s *PortabilityService) ExportBundle(ctx context.Context, companyID string, req ExportRequest) (*ExportResult, error) {
	include := resolveInclude(req.Include)
	// Promote include flags if explicit selectors provided
	if len(req.Agents) > 0 {
		include.Agents = true
	}
	if len(req.Projects) > 0 || len(req.ProjectIssues) > 0 {
		include.Projects = true
	}
	if len(req.Issues) > 0 || len(req.ProjectIssues) > 0 {
		include.Issues = true
	}
	if len(req.Skills) > 0 {
		include.Skills = true
	}

	// Fetch company
	var company models.Company
	if err := s.db.WithContext(ctx).First(&company, "id = ?", companyID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("company not found")
		}
		return nil, err
	}

	rootPath := normalizeSlug(company.Name)
	if rootPath == "" {
		rootPath = "company-package"
	}

	files := map[string]interface{}{}
	warnings := []string{}
	envInputs := []PortabilityEnvInput{}

	// ── Agents ───────────────────────────────────────────────────────────────
	var agentRows []models.Agent
	agentIDToSlug := map[string]string{}
	if include.Agents || include.Skills {
		if err := s.db.WithContext(ctx).
			Where("company_id = ? AND status != ?", companyID, "terminated").
			Order("name ASC").
			Find(&agentRows).Error; err != nil {
			return nil, err
		}

		// Filter to selected agents if specified
		if len(req.Agents) > 0 {
			selected := map[string]bool{}
			for _, sel := range req.Agents {
				selected[sel] = true
				selected[normalizeSlug(sel)] = true
			}
			filtered := []models.Agent{}
			for _, ag := range agentRows {
				if selected[ag.ID] || selected[ag.Name] || selected[normalizeSlug(ag.Name)] {
					filtered = append(filtered, ag)
				} else {
					warnings = append(warnings, fmt.Sprintf("Agent selector %q was not found and was skipped.", ag.Name))
				}
			}
			agentRows = filtered
		}

		// Build slug map
		usedSlugs := map[string]bool{}
		for _, ag := range agentRows {
			base := normalizeSlug(ag.Name)
			if base == "" {
				base = "agent"
			}
			slug := uniqueSlug(base, usedSlugs)
			agentIDToSlug[ag.ID] = slug
		}
	}

	// ── Skills ───────────────────────────────────────────────────────────────
	var skillRows []models.CompanySkill
	if include.Skills || include.Agents {
		if err := s.db.WithContext(ctx).
			Where("company_id = ?", companyID).
			Order("key ASC").
			Find(&skillRows).Error; err != nil {
			return nil, err
		}

		// Export skill files
		if include.Skills {
			for _, skill := range skillRows {
				skillDir := "skills/" + normalizeSlug(skill.Slug)
				if skillDir == "skills/" {
					skillDir = "skills/" + normalizeSlug(skill.Key)
				}
				skillPath := skillDir + "/SKILL.md"
				fm := map[string]interface{}{
					"name": skill.Name,
					"key":  skill.Key,
					"slug": skill.Slug,
				}
				if skill.Description != nil && *skill.Description != "" {
					fm["description"] = *skill.Description
				}
				fm["metadata"] = buildSkillSourceMetadata(skill)
				files[skillPath] = buildMarkdown(fm, skill.Markdown)
			}
		}
	}

	// ── Build agent files ─────────────────────────────────────────────────────
	paperclipAgents := map[string]interface{}{}
	if include.Agents {
		for _, ag := range agentRows {
			slug := agentIDToSlug[ag.ID]
			agPath := "agents/" + slug + "/AGENTS.md"

			// Extract agent instructions from adapterConfig.promptTemplate
			adapterCfg := jsonToMap(ag.AdapterConfig)
			if adapterCfg == nil {
				adapterCfg = map[string]interface{}{}
			}
			instructions := ""
			if pt, ok := adapterCfg["promptTemplate"].(string); ok {
				instructions = pt
			}

			reportsToSlug := ""
			if ag.ReportsTo != nil {
				reportsToSlug = agentIDToSlug[*ag.ReportsTo]
			}

			fm := map[string]interface{}{}
			fm["name"] = ag.Name
			if ag.Title != nil && *ag.Title != "" {
				fm["title"] = *ag.Title
			}
			if reportsToSlug != "" {
				fm["reportsTo"] = reportsToSlug
			}

			// Collect skill refs from adapterConfig
			var desiredSkills []string
			if skills, ok := adapterCfg["skills"]; ok {
				switch sv := skills.(type) {
				case []interface{}:
					for _, s := range sv {
						if str, ok := s.(string); ok {
							desiredSkills = append(desiredSkills, str)
						}
					}
				}
			}
			if len(desiredSkills) > 0 {
				fm["skills"] = desiredSkills
			}

			files[agPath] = buildMarkdown(fm, instructions)

			// Build portable adapterConfig (strip prompt-related keys)
			portableAdapterCfg := make(map[string]interface{})
			for k, v := range adapterCfg {
				switch k {
				case "promptTemplate", "bootstrapPromptTemplate", "instructionsFilePath",
					"instructionsBundleMode", "instructionsRootPath", "instructionsEntryFile", "env":
					// skip — env extracted separately below
				default:
					portableAdapterCfg[k] = v
				}
			}
			delete(portableAdapterCfg, "skills")

			// Gap 4: warn and drop absolute command values.
			if cmd := strVal(portableAdapterCfg, "command"); isAbsoluteCommand(cmd) {
				warnings = append(warnings, fmt.Sprintf("Agent %s command %s was omitted from export because it is system-dependent.", slug, cmd))
				delete(portableAdapterCfg, "command")
			}

			// Gap 3: prune adapter-type-specific defaults and false-boolean permissions.
			portableAdapterCfg = pruneAdapterConfig(ag.AdapterType, portableAdapterCfg)

			runtimeCfg := jsonToMap(ag.RuntimeConfig)
			if runtimeCfg == nil {
				runtimeCfg = map[string]interface{}{}
			}
			runtimeCfg = pruneRuntimeConfig(runtimeCfg)

			permissions := jsonToMap(ag.Permissions)
			if permissions == nil {
				permissions = map[string]interface{}{}
			}
			permissions = prunePermissions(permissions)

			// Gap 1: extract env inputs from adapterConfig.env.
			envInputsStart := len(envInputs)
			agentEnvAdded := extractPortableEnvInputs(slug, adapterCfg, &warnings)
			envInputs = append(envInputs, agentEnvAdded...)
			agentEnvInputs := dedupeEnvInputs(envInputs[envInputsStart:])

			ext := map[string]interface{}{}
			if ag.Role != "" && ag.Role != "agent" {
				ext["role"] = ag.Role
			}
			if ag.Icon != nil && *ag.Icon != "" {
				ext["icon"] = *ag.Icon
			}
			if ag.Capabilities != nil && *ag.Capabilities != "" {
				ext["capabilities"] = *ag.Capabilities
			}
			if ag.AdapterType != "" {
				ext["adapter"] = map[string]interface{}{
					"type":   ag.AdapterType,
					"config": portableAdapterCfg,
				}
			}
			if len(runtimeCfg) > 0 {
				ext["runtime"] = runtimeCfg
			}
			if len(permissions) > 0 {
				ext["permissions"] = permissions
			}
			if ag.BudgetMonthlyCents > 0 {
				ext["budgetMonthlyCents"] = ag.BudgetMonthlyCents
			}
			if len(agentEnvInputs) > 0 {
				ext["inputs"] = map[string]interface{}{
					"env": buildEnvInputMap(agentEnvInputs),
				}
			}
			paperclipAgents[slug] = ext
		}
	}

	// ── Projects ──────────────────────────────────────────────────────────────
	var projectRows []models.Project
	projectIDToSlug := map[string]string{}
	paperclipProjects := map[string]interface{}{}
	if include.Projects || include.Issues {
		if err := s.db.WithContext(ctx).
			Where("company_id = ? AND archived_at IS NULL", companyID).
			Order("created_at ASC").
			Find(&projectRows).Error; err != nil {
			return nil, err
		}

		// Filter to selected projects if specified
		if len(req.Projects) > 0 && include.Projects {
			selected := map[string]bool{}
			for _, sel := range req.Projects {
				selected[sel] = true
				selected[normalizeSlug(sel)] = true
			}
			filtered := []models.Project{}
			for _, p := range projectRows {
				if selected[p.ID] || selected[p.Name] || selected[normalizeSlug(p.Name)] {
					filtered = append(filtered, p)
				}
			}
			projectRows = filtered
		}

		// Build slug map
		usedProjectSlugs := map[string]bool{}
		for _, p := range projectRows {
			base := deriveProjectSlug(p.Name)
			slug := uniqueSlug(base, usedProjectSlugs)
			projectIDToSlug[p.ID] = slug
		}

		if include.Projects {
			for _, p := range projectRows {
				slug := projectIDToSlug[p.ID]
				projectPath := "projects/" + slug + "/PROJECT.md"
				leadSlug := ""
				if p.LeadAgentID != nil {
					leadSlug = agentIDToSlug[*p.LeadAgentID]
				}

				fm := map[string]interface{}{}
				fm["name"] = p.Name
				if p.Description != nil && *p.Description != "" {
					fm["description"] = *p.Description
				}
				if leadSlug != "" {
					fm["owner"] = leadSlug
				}
				files[projectPath] = buildMarkdown(fm, derefStr(p.Description))

				ext := map[string]interface{}{}
				if p.Status != "" && p.Status != "backlog" {
					ext["status"] = p.Status
				}
				if p.Color != nil && *p.Color != "" {
					ext["color"] = *p.Color
				}
				if p.TargetDate != nil {
					ext["targetDate"] = p.TargetDate.Format("2006-01-02")
				}
				if leadSlug != "" {
					ext["leadAgentSlug"] = leadSlug
				}
				// Gap 2: extract env inputs from project.env.
				// Note: models.Project does not carry an Env column; project env inputs
				// are only available via the full service layer. We leave this as a
				// no-op in the basic export path — no data loss, just no env inputs.
				_ = extractPortableProjectEnvInputs // referenced to avoid unused-import lint
				paperclipProjects[slug] = ext
			}
		}
	}

	// ── Issues / Tasks ────────────────────────────────────────────────────────
	paperclipTasks := map[string]interface{}{}
	paperclipRoutines := map[string]interface{}{}
	if include.Issues {
		// Build selected issue rows
		var issueRows []models.Issue
		query := s.db.WithContext(ctx).Where("company_id = ?", companyID)
		if err := query.Find(&issueRows).Error; err != nil {
			return nil, err
		}

		// Filter to selected issues/projects
		selected := map[string]bool{}
		for _, sel := range req.Issues {
			selected[sel] = true
		}
		for _, projectSel := range req.ProjectIssues {
			// Mark all issues from this project
			for _, p := range projectRows {
				if p.ID == projectSel || p.Name == projectSel || normalizeSlug(p.Name) == normalizeSlug(projectSel) {
					for _, issue := range issueRows {
						if issue.ProjectID != nil && *issue.ProjectID == p.ID {
							selected[issue.ID] = true
						}
					}
				}
			}
		}

		filteredIssues := issueRows
		if len(selected) > 0 {
			filteredIssues = []models.Issue{}
			for _, issue := range issueRows {
				if selected[issue.ID] {
					filteredIssues = append(filteredIssues, issue)
				}
			}
		}

		// Build task slug map
		usedTaskSlugs := map[string]bool{}
		taskIDToSlug := map[string]string{}
		for _, issue := range filteredIssues {
			base := normalizeSlug(issue.Title)
			if base == "" {
				base = "task"
			}
			slug := uniqueSlug(base, usedTaskSlugs)
			taskIDToSlug[issue.ID] = slug
		}

		for _, issue := range filteredIssues {
			slug := taskIDToSlug[issue.ID]
			taskPath := "tasks/" + slug + "/TASK.md"

			projectSlug := ""
			if issue.ProjectID != nil {
				projectSlug = projectIDToSlug[*issue.ProjectID]
			}
			assigneeSlug := ""
			if issue.AssigneeAgentID != nil {
				assigneeSlug = agentIDToSlug[*issue.AssigneeAgentID]
			}

			fm := map[string]interface{}{}
			fm["name"] = issue.Title
			if projectSlug != "" {
				fm["project"] = projectSlug
			}
			if assigneeSlug != "" {
				fm["assignee"] = assigneeSlug
			}

			desc := ""
			if issue.Description != nil {
				desc = *issue.Description
			}
			files[taskPath] = buildMarkdown(fm, desc)

			ext := map[string]interface{}{}
			if issue.Identifier != nil && *issue.Identifier != "" {
				ext["identifier"] = *issue.Identifier
			}
			if issue.Status != "" && issue.Status != "backlog" {
				ext["status"] = issue.Status
			}
			if issue.Priority != "" && issue.Priority != "medium" {
				ext["priority"] = issue.Priority
			}
			if issue.BillingCode != nil && *issue.BillingCode != "" {
				ext["billingCode"] = *issue.BillingCode
			}
			if len(ext) > 0 {
				paperclipTasks[slug] = ext
			}
		}

		// Build routine files
		var routineRows []models.Routine
		if err := s.db.WithContext(ctx).
			Where("company_id = ?", companyID).
			Find(&routineRows).Error; err != nil {
			return nil, err
		}

		var triggerRows []models.RoutineTrigger
		if len(routineRows) > 0 {
			routineIDs := make([]string, len(routineRows))
			for i, r := range routineRows {
				routineIDs[i] = r.ID
			}
			if err := s.db.WithContext(ctx).
				Where("routine_id IN ?", routineIDs).
				Find(&triggerRows).Error; err != nil {
				return nil, err
			}
		}
		triggersByRoutine := map[string][]models.RoutineTrigger{}
		for _, t := range triggerRows {
			triggersByRoutine[t.RoutineID] = append(triggersByRoutine[t.RoutineID], t)
		}

		usedRoutineSlugs := map[string]bool{}
		for _, routine := range routineRows {
			base := normalizeSlug(routine.Title)
			if base == "" {
				base = "routine"
			}
			slug := uniqueSlug(base, usedRoutineSlugs)

			taskPath := "tasks/" + slug + "/TASK.md"
			projectSlug := projectIDToSlug[routine.ProjectID]
			assigneeSlug := agentIDToSlug[routine.AssigneeAgentID]

			fm := map[string]interface{}{
				"name":      routine.Title,
				"recurring": true,
			}
			if projectSlug != "" {
				fm["project"] = projectSlug
			}
			if assigneeSlug != "" {
				fm["assignee"] = assigneeSlug
			}
			desc := ""
			if routine.Description != nil {
				desc = *routine.Description
			}
			files[taskPath] = buildMarkdown(fm, desc)

			// Build routine extension
			triggers := triggersByRoutine[routine.ID]
			triggersOut := []map[string]interface{}{}
			for _, t := range triggers {
				te := map[string]interface{}{"kind": t.Kind}
				if t.Label != nil && *t.Label != "" {
					te["label"] = *t.Label
				}
				if !t.Enabled {
					te["enabled"] = false
				}
				if t.Kind == "schedule" {
					if t.CronExpression != nil {
						te["cronExpression"] = *t.CronExpression
					}
					if t.Timezone != nil {
						te["timezone"] = *t.Timezone
					}
				}
				if t.Kind == "webhook" {
					if t.SigningMode != nil && *t.SigningMode != "bearer" {
						te["signingMode"] = *t.SigningMode
					}
					if t.ReplayWindowSec != nil && *t.ReplayWindowSec != 300 {
						te["replayWindowSec"] = *t.ReplayWindowSec
					}
				}
				triggersOut = append(triggersOut, te)
			}

			routineExt := map[string]interface{}{
				"triggers": triggersOut,
			}
			if routine.ConcurrencyPolicy != "coalesce_if_active" {
				routineExt["concurrencyPolicy"] = routine.ConcurrencyPolicy
			}
			if routine.CatchUpPolicy != "skip_missed" {
				routineExt["catchUpPolicy"] = routine.CatchUpPolicy
			}
			paperclipRoutines[slug] = routineExt
		}
	}

	// ── COMPANY.md ────────────────────────────────────────────────────────────
	companyFM := map[string]interface{}{
		"name":   company.Name,
		"schema": "agentcompanies/v1",
		"slug":   rootPath,
	}
	if company.Description != nil && *company.Description != "" {
		companyFM["description"] = *company.Description
	}
	files["COMPANY.md"] = buildMarkdown(companyFM, "")

	// ── .paperclip.yaml ────────────────────────────────────────────────────────
	paperclipExt := map[string]interface{}{
		"schema": "paperclip/v1",
	}
	companySettings := map[string]interface{}{}
	if company.BrandColor != nil && *company.BrandColor != "" {
		companySettings["brandColor"] = *company.BrandColor
	}
	if !company.RequireBoardApprovalForNewAgents {
		companySettings["requireBoardApprovalForNewAgents"] = false
	}
	if company.FeedbackDataSharingEnabled {
		companySettings["feedbackDataSharingEnabled"] = true
	}
	if company.FeedbackDataSharingConsentAt != nil {
		companySettings["feedbackDataSharingConsentAt"] = company.FeedbackDataSharingConsentAt.UTC().Format(time.RFC3339)
	}
	if company.FeedbackDataSharingConsentByUserID != nil {
		companySettings["feedbackDataSharingConsentByUserId"] = *company.FeedbackDataSharingConsentByUserID
	}
	if company.FeedbackDataSharingTermsVersion != nil {
		companySettings["feedbackDataSharingTermsVersion"] = *company.FeedbackDataSharingTermsVersion
	}
	if len(companySettings) > 0 {
		paperclipExt["company"] = companySettings
	}

	if req.SidebarOrder != nil {
		if len(req.SidebarOrder.Agents) > 0 || len(req.SidebarOrder.Projects) > 0 {
			paperclipExt["sidebar"] = req.SidebarOrder
		}
	}

	if len(paperclipAgents) > 0 {
		paperclipExt["agents"] = paperclipAgents
	}
	if len(paperclipProjects) > 0 {
		paperclipExt["projects"] = paperclipProjects
	}
	if len(paperclipTasks) > 0 {
		paperclipExt["tasks"] = paperclipTasks
	}
	if len(paperclipRoutines) > 0 {
		paperclipExt["routines"] = paperclipRoutines
	}

	paperclipExtPath := ".paperclip.yaml"
	files[paperclipExtPath] = buildYAMLFile(paperclipExt)

	// ── Build manifest from the generated files ───────────────────────────────
	sourceLabel := &PortabilityManifestSource{
		CompanyID:   company.ID,
		CompanyName: company.Name,
	}
	resolved, err := buildManifestFromFiles(files, sourceLabel)
	if err != nil {
		return nil, err
	}
	resolved.warnings = append(warnings, resolved.warnings...)

	// Gap 1+2: Assign deduplicated env inputs to manifest.
	resolved.manifest.EnvInputs = dedupeEnvInputs(envInputs)

	if orgChartSVG := generateOrgChartSVG(resolved.manifest.Agents); orgChartSVG != "" {
		files["images/org-chart.svg"] = orgChartSVG
	}
	files["README.md"] = generateExportReadme(resolved.manifest)

	return &ExportResult{
		RootPath:               rootPath,
		Manifest:               resolved.manifest,
		Files:                  files,
		Warnings:               resolved.warnings,
		PaperclipExtensionPath: paperclipExtPath,
	}, nil
}

// PreviewExport returns an export preview (same as ExportBundle plus inventory).
func (s *PortabilityService) PreviewExport(ctx context.Context, companyID string, req ExportRequest) (*ExportPreviewResult, error) {
	result, err := s.ExportBundle(ctx, companyID, req)
	if err != nil {
		return nil, err
	}

	// Build file inventory sorted alphabetically
	var inventory []ExportPreviewFile
	for p := range result.Files {
		inventory = append(inventory, ExportPreviewFile{
			Path: p,
			Kind: classifyFileKind(p),
		})
	}
	sort.Slice(inventory, func(i, j int) bool {
		return inventory[i].Path < inventory[j].Path
	})

	return &ExportPreviewResult{
		ExportResult:  *result,
		FileInventory: inventory,
		Counts: ExportPreviewCounts{
			Files:    len(result.Files),
			Agents:   len(result.Manifest.Agents),
			Skills:   len(result.Manifest.Skills),
			Projects: len(result.Manifest.Projects),
			Issues:   len(result.Manifest.Issues),
		},
	}, nil
}

// ──────────────────────────────────────────────────────────────────────────────
// Import – resolve source.
// ──────────────────────────────────────────────────────────────────────────────

func (s *PortabilityService) resolveSource(ctx context.Context, src ImportSource) (resolvedSource, error) {
	if src.Type == "inline" {
		files := src.Files
		if files == nil {
			files = map[string]interface{}{}
		}
		// Apply rootPath prefix stripping if provided
		if src.RootPath != nil && *src.RootPath != "" {
			prefix := strings.TrimSuffix(*src.RootPath, "/") + "/"
			stripped := map[string]interface{}{}
			for k, v := range files {
				if strings.HasPrefix(k, prefix) {
					stripped[strings.TrimPrefix(k, prefix)] = v
				} else {
					stripped[k] = v
				}
			}
			files = stripped
		}
		return buildManifestFromFiles(files, nil)
	}

	if src.Type == "github" {
		return s.resolveGitHubSource(ctx, src.URL)
	}

	return resolvedSource{}, fmt.Errorf("unsupported source type: %s", src.Type)
}

func (s *PortabilityService) resolveGitHubSource(ctx context.Context, rawURL string) (resolvedSource, error) {
	parsed, err := parseGitHubSourceURL(rawURL)
	if err != nil {
		return resolvedSource{}, err
	}

	warnings := []string{}
	ref := parsed.ref

	// Fetch COMPANY.md
	companyRelPath := parsed.companyPath
	if parsed.basePath != "" && parsed.companyPath == "COMPANY.md" {
		companyRelPath = parsed.basePath + "/COMPANY.md"
	}
	companyMarkdown, fetchErr := ghGetText(ctx, rawGitHubURL(parsed, companyRelPath))
	if fetchErr != nil || companyMarkdown == "" {
		if ref == "main" {
			// Fall back to master
			ref = "master"
			parsed.ref = ref
			warnings = append(warnings, "GitHub ref main not found; falling back to master.")
			companyMarkdown, fetchErr = ghGetText(ctx, rawGitHubURL(parsed, companyRelPath))
		}
		if fetchErr != nil || companyMarkdown == "" {
			return resolvedSource{}, fmt.Errorf("GitHub company package is missing COMPANY.md")
		}
	}

	normalizedCompanyPath := parsed.companyPath
	if parsed.basePath != "" {
		normalizedCompanyPath = "COMPANY.md"
	}
	files := map[string]interface{}{
		normalizedCompanyPath: companyMarkdown,
	}

	// Fetch file tree
	treePaths, _ := fetchGitHubTree(ctx, parsed)
	basePrefix := ""
	if parsed.basePath != "" {
		basePrefix = strings.TrimSuffix(parsed.basePath, "/") + "/"
	}
	for _, repoPath := range treePaths {
		relativePath := repoPath
		if basePrefix != "" {
			if !strings.HasPrefix(repoPath, basePrefix) {
				continue
			}
			relativePath = repoPath[len(basePrefix):]
		}
		// Only fetch relevant files
		if !strings.HasSuffix(relativePath, ".md") &&
			!strings.HasPrefix(relativePath, "skills/") &&
			relativePath != ".paperclip.yaml" && relativePath != ".paperclip.yml" {
			continue
		}
		if _, exists := files[relativePath]; exists {
			continue
		}
		text, err := ghGetText(ctx, rawGitHubURL(parsed, repoPath))
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("Failed to fetch %s: %v", repoPath, err))
			continue
		}
		files[relativePath] = text
	}

	resolved, err := buildManifestFromFiles(files, nil)
	if err != nil {
		return resolvedSource{}, err
	}
	resolved.warnings = append(warnings, resolved.warnings...)
	return resolved, nil
}

// ──────────────────────────────────────────────────────────────────────────────
// Import – build plan.
// ──────────────────────────────────────────────────────────────────────────────

func (s *PortabilityService) buildPreview(ctx context.Context, req ImportRequest, mode ImportMode) (*internalPlan, error) {
	if mode == "" {
		mode = ImportModeBoardFull
	}
	collisionStrategy := req.CollisionStrategy
	if collisionStrategy == "" {
		collisionStrategy = "rename"
	}
	if mode == ImportModeAgentSafe && collisionStrategy == "replace" {
		return nil, fmt.Errorf("safe import routes do not allow replace collision strategy")
	}

	source, err := s.resolveSource(ctx, req.Source)
	if err != nil {
		return nil, err
	}

	manifest := source.manifest
	warnings := source.warnings[:]
	errors := []string{}
	collisions := []ImportCollision{}

	// Resolve include
	include := resolveInclude(req.Include)
	include.Company = include.Company && manifest.Company != nil
	include.Agents = include.Agents && len(manifest.Agents) > 0
	include.Projects = include.Projects && len(manifest.Projects) > 0
	include.Issues = include.Issues && len(manifest.Issues) > 0
	include.Skills = include.Skills && len(manifest.Skills) > 0
	if req.Include["company"] && manifest.Company == nil {
		errors = append(errors, "Manifest does not include company metadata.")
	}

	// Select agents
	var selectedAgents []PortabilityAgentManifestEntry
	if include.Agents {
		if req.Agents == nil || req.Agents == "all" {
			selectedAgents = manifest.Agents
		} else if slugs, ok := req.Agents.([]interface{}); ok {
			slugSet := map[string]bool{}
			for _, s := range slugs {
				if str, ok := s.(string); ok {
					slugSet[str] = true
				}
			}
			for _, ag := range manifest.Agents {
				if slugSet[ag.Slug] {
					selectedAgents = append(selectedAgents, ag)
				}
			}
		}
	}
	if selectedAgents == nil {
		selectedAgents = []PortabilityAgentManifestEntry{}
	}
	selectedAgentSet := map[string]bool{}
	for _, agent := range manifest.Agents {
		selectedAgentSet[agent.Slug] = true
	}
	if include.Agents {
		if requested, ok := req.Agents.([]interface{}); ok {
			for _, raw := range requested {
				slug, ok := raw.(string)
				if !ok || slug == "" {
					continue
				}
				if !selectedAgentSet[slug] {
					errors = append(errors, fmt.Sprintf("Selected agent slug not found in manifest: %s", slug))
				}
			}
		}
		if len(selectedAgents) == 0 {
			warnings = append(warnings, "No agents selected for import.")
		}
	}

	// Determine target company
	var targetCompanyID *string
	var targetCompanyName *string
	if req.Target.Mode == "existing_company" {
		var c models.Company
		if err := s.db.WithContext(ctx).First(&c, "id = ?", req.Target.CompanyID).Error; err != nil {
			return nil, fmt.Errorf("target company not found")
		}
		targetCompanyID = &c.ID
		targetCompanyName = &c.Name
	}

	// Build agent plans
	agentPlans := []AgentPlan{}
	existingSlugToAgent := map[string]existingCollisionEntity{}
	usedSlugsForRename := map[string]bool{}
	existingAgentNameToAgent := map[string]existingCollisionEntity{}

	if req.Target.Mode == "existing_company" && len(selectedAgents) > 0 {
		var existingAgents []models.Agent
		if err := s.db.WithContext(ctx).
			Where("company_id = ? AND status != ?", req.Target.CompanyID, "terminated").
			Find(&existingAgents).Error; err != nil {
			return nil, err
		}
		for _, ag := range existingAgents {
			slug := normalizeSlug(ag.Name)
			if slug == "" {
				slug = ag.ID
			}
			if _, already := existingSlugToAgent[slug]; !already {
				existingSlugToAgent[slug] = existingCollisionEntity{ID: ag.ID, Name: ag.Name}
			}
			usedSlugsForRename[slug] = true
			if normalizedName := normalizeSlug(ag.Name); normalizedName != "" {
				if _, already := existingAgentNameToAgent[normalizedName]; !already {
					existingAgentNameToAgent[normalizedName] = existingCollisionEntity{ID: ag.ID, Name: ag.Name}
				}
			}
		}
	}

	for _, manifestAgent := range selectedAgents {
		existing, exists := existingSlugToAgent[manifestAgent.Slug]
		nameMatch := false
		if !exists {
			if candidate, ok := existingAgentNameToAgent[normalizeSlug(manifestAgent.Name)]; ok {
				existing = candidate
				exists = true
				nameMatch = true
			}
		}
		if !exists {
			agentPlans = append(agentPlans, AgentPlan{
				Slug:            manifestAgent.Slug,
				Action:          "create",
				PlannedName:     manifestAgent.Name,
				ExistingAgentID: nil,
				Reason:          nil,
			})
			continue
		}
		recommendedStrategy := recommendedCollisionStrategy(mode, collisionStrategy, true)
		matchTypes := buildCollisionMatchTypes(!nameMatch, nameMatch, false)
		switch collisionStrategy {
		case "replace":
			if mode == ImportModeBoardFull {
				reason := "Existing slug matched; replace strategy."
				agentPlans = append(agentPlans, AgentPlan{
					Slug:            manifestAgent.Slug,
					Action:          "update",
					PlannedName:     existing.Name,
					ExistingAgentID: &existing.ID,
					Reason:          &reason,
				})
				existingID, existingName := existing.ID, existing.Name
				collisions = append(collisions, ImportCollision{
					EntityType:                   "agent",
					Slug:                         manifestAgent.Slug,
					Name:                         manifestAgent.Name,
					ExistingID:                   &existingID,
					ExistingName:                 &existingName,
					MatchTypes:                   matchTypes,
					RequestedCollisionStrategy:   collisionStrategy,
					RecommendedCollisionStrategy: recommendedStrategy,
					PlannedAction:                "update",
					Reason:                       reason,
				})
			} else {
				reason := "Existing slug matched; rename strategy."
				renamed := uniqueName(manifestAgent.Name, usedSlugsForRename)
				agentPlans = append(agentPlans, AgentPlan{
					Slug:            manifestAgent.Slug,
					Action:          "create",
					PlannedName:     renamed,
					ExistingAgentID: &existing.ID,
					Reason:          &reason,
				})
				existingID, existingName := existing.ID, existing.Name
				collisions = append(collisions, ImportCollision{
					EntityType:                   "agent",
					Slug:                         manifestAgent.Slug,
					Name:                         manifestAgent.Name,
					ExistingID:                   &existingID,
					ExistingName:                 &existingName,
					MatchTypes:                   matchTypes,
					RequestedCollisionStrategy:   collisionStrategy,
					RecommendedCollisionStrategy: recommendedStrategy,
					PlannedAction:                "create",
					Reason:                       reason,
				})
			}
		case "skip":
			reason := "Existing slug matched; skip strategy."
			agentPlans = append(agentPlans, AgentPlan{
				Slug:            manifestAgent.Slug,
				Action:          "skip",
				PlannedName:     existing.Name,
				ExistingAgentID: &existing.ID,
				Reason:          &reason,
			})
			existingID, existingName := existing.ID, existing.Name
			collisions = append(collisions, ImportCollision{
				EntityType:                   "agent",
				Slug:                         manifestAgent.Slug,
				Name:                         manifestAgent.Name,
				ExistingID:                   &existingID,
				ExistingName:                 &existingName,
				MatchTypes:                   matchTypes,
				RequestedCollisionStrategy:   collisionStrategy,
				RecommendedCollisionStrategy: recommendedStrategy,
				PlannedAction:                "skip",
				Reason:                       reason,
			})
		default: // "rename"
			reason := "Existing slug matched; rename strategy."
			renamed := uniqueName(manifestAgent.Name, usedSlugsForRename)
			agentPlans = append(agentPlans, AgentPlan{
				Slug:            manifestAgent.Slug,
				Action:          "create",
				PlannedName:     renamed,
				ExistingAgentID: &existing.ID,
				Reason:          &reason,
			})
			existingID, existingName := existing.ID, existing.Name
			collisions = append(collisions, ImportCollision{
				EntityType:                   "agent",
				Slug:                         manifestAgent.Slug,
				Name:                         manifestAgent.Name,
				ExistingID:                   &existingID,
				ExistingName:                 &existingName,
				MatchTypes:                   matchTypes,
				RequestedCollisionStrategy:   collisionStrategy,
				RecommendedCollisionStrategy: recommendedStrategy,
				PlannedAction:                "create",
				Reason:                       reason,
			})
		}
	}

	// Apply name overrides
	for i := range agentPlans {
		if override, ok := req.NameOverrides[agentPlans[i].Slug]; ok {
			agentPlans[i].PlannedName = override
		}
	}

	// Warn about agent updates
	for _, ap := range agentPlans {
		if ap.Action == "update" {
			warnings = append(warnings, fmt.Sprintf("Existing agent %q (%s) will be overwritten by import.", ap.PlannedName, ap.Slug))
		}
	}

	// Build project plans
	projectPlans := []ProjectPlan{}
	existingProjectSlugToProject := map[string]existingCollisionEntity{}
	usedProjectSlugsForRename := map[string]bool{}
	existingProjectNameToProject := map[string]existingCollisionEntity{}

	if include.Projects && req.Target.Mode == "existing_company" {
		var existingProjects []models.Project
		if err := s.db.WithContext(ctx).
			Where("company_id = ? AND archived_at IS NULL", req.Target.CompanyID).
			Find(&existingProjects).Error; err != nil {
			return nil, err
		}
		for _, p := range existingProjects {
			slug := deriveProjectSlug(p.Name)
			if _, already := existingProjectSlugToProject[slug]; !already {
				existingProjectSlugToProject[slug] = existingCollisionEntity{ID: p.ID, Name: p.Name}
			}
			usedProjectSlugsForRename[slug] = true
			if normalizedName := normalizeSlug(p.Name); normalizedName != "" {
				if _, already := existingProjectNameToProject[normalizedName]; !already {
					existingProjectNameToProject[normalizedName] = existingCollisionEntity{ID: p.ID, Name: p.Name}
				}
			}
		}
	}

	if include.Projects {
		for _, manifestProject := range manifest.Projects {
			existing, exists := existingProjectSlugToProject[manifestProject.Slug]
			nameMatch := false
			if !exists {
				if candidate, ok := existingProjectNameToProject[normalizeSlug(manifestProject.Name)]; ok {
					existing = candidate
					exists = true
					nameMatch = true
				}
			}
			if !exists {
				projectPlans = append(projectPlans, ProjectPlan{
					Slug:              manifestProject.Slug,
					Action:            "create",
					PlannedName:       manifestProject.Name,
					ExistingProjectID: nil,
					Reason:            nil,
				})
				continue
			}
			recommendedStrategy := recommendedCollisionStrategy(mode, collisionStrategy, true)
			matchTypes := buildCollisionMatchTypes(!nameMatch, nameMatch, false)
			switch collisionStrategy {
			case "replace":
				if mode == ImportModeBoardFull {
					reason := "Existing slug matched; replace strategy."
					projectPlans = append(projectPlans, ProjectPlan{
						Slug:              manifestProject.Slug,
						Action:            "update",
						PlannedName:       existing.Name,
						ExistingProjectID: &existing.ID,
						Reason:            &reason,
					})
					existingID, existingName := existing.ID, existing.Name
					collisions = append(collisions, ImportCollision{
						EntityType:                   "project",
						Slug:                         manifestProject.Slug,
						Name:                         manifestProject.Name,
						ExistingID:                   &existingID,
						ExistingName:                 &existingName,
						MatchTypes:                   matchTypes,
						RequestedCollisionStrategy:   collisionStrategy,
						RecommendedCollisionStrategy: recommendedStrategy,
						PlannedAction:                "update",
						Reason:                       reason,
					})
				} else {
					reason := "Existing slug matched; rename strategy."
					renamed := uniqueName(manifestProject.Name, usedProjectSlugsForRename)
					projectPlans = append(projectPlans, ProjectPlan{
						Slug:              manifestProject.Slug,
						Action:            "create",
						PlannedName:       renamed,
						ExistingProjectID: &existing.ID,
						Reason:            &reason,
					})
					existingID, existingName := existing.ID, existing.Name
					collisions = append(collisions, ImportCollision{
						EntityType:                   "project",
						Slug:                         manifestProject.Slug,
						Name:                         manifestProject.Name,
						ExistingID:                   &existingID,
						ExistingName:                 &existingName,
						MatchTypes:                   matchTypes,
						RequestedCollisionStrategy:   collisionStrategy,
						RecommendedCollisionStrategy: recommendedStrategy,
						PlannedAction:                "create",
						Reason:                       reason,
					})
				}
			case "skip":
				reason := "Existing slug matched; skip strategy."
				projectPlans = append(projectPlans, ProjectPlan{
					Slug:              manifestProject.Slug,
					Action:            "skip",
					PlannedName:       existing.Name,
					ExistingProjectID: &existing.ID,
					Reason:            &reason,
				})
				existingID, existingName := existing.ID, existing.Name
				collisions = append(collisions, ImportCollision{
					EntityType:                   "project",
					Slug:                         manifestProject.Slug,
					Name:                         manifestProject.Name,
					ExistingID:                   &existingID,
					ExistingName:                 &existingName,
					MatchTypes:                   matchTypes,
					RequestedCollisionStrategy:   collisionStrategy,
					RecommendedCollisionStrategy: recommendedStrategy,
					PlannedAction:                "skip",
					Reason:                       reason,
				})
			default: // "rename"
				reason := "Existing slug matched; rename strategy."
				renamed := uniqueName(manifestProject.Name, usedProjectSlugsForRename)
				projectPlans = append(projectPlans, ProjectPlan{
					Slug:              manifestProject.Slug,
					Action:            "create",
					PlannedName:       renamed,
					ExistingProjectID: &existing.ID,
					Reason:            &reason,
				})
				existingID, existingName := existing.ID, existing.Name
				collisions = append(collisions, ImportCollision{
					EntityType:                   "project",
					Slug:                         manifestProject.Slug,
					Name:                         manifestProject.Name,
					ExistingID:                   &existingID,
					ExistingName:                 &existingName,
					MatchTypes:                   matchTypes,
					RequestedCollisionStrategy:   collisionStrategy,
					RecommendedCollisionStrategy: recommendedStrategy,
					PlannedAction:                "create",
					Reason:                       reason,
				})
			}
		}
		for i := range projectPlans {
			if override, ok := req.NameOverrides[projectPlans[i].Slug]; ok {
				projectPlans[i].PlannedName = override
			}
		}
		for _, pp := range projectPlans {
			if pp.Action == "update" {
				warnings = append(warnings, fmt.Sprintf("Existing project %q (%s) will be overwritten by import.", pp.PlannedName, pp.Slug))
			}
		}
	}

	if req.Target.Mode == "existing_company" && (include.Skills || include.Agents) {
		var existingSkills []models.CompanySkill
		if err := s.db.WithContext(ctx).
			Where("company_id = ?", req.Target.CompanyID).
			Find(&existingSkills).Error; err != nil {
			return nil, err
		}
		existingSkillKeyToSkill := map[string]existingCollisionEntity{}
		existingSkillSlugToSkill := map[string]existingCollisionEntity{}
		existingSkillNameToSkill := map[string]existingCollisionEntity{}
		for _, skill := range existingSkills {
			existingSkillKeyToSkill[skill.Key] = existingCollisionEntity{ID: skill.ID, Name: skill.Name}
			if normalizedSlug := normalizeSlug(skill.Slug); normalizedSlug != "" {
				if _, ok := existingSkillSlugToSkill[normalizedSlug]; !ok {
					existingSkillSlugToSkill[normalizedSlug] = existingCollisionEntity{ID: skill.ID, Name: skill.Name}
				}
			}
			if normalizedName := normalizeSlug(skill.Name); normalizedName != "" {
				if _, ok := existingSkillNameToSkill[normalizedName]; !ok {
					existingSkillNameToSkill[normalizedName] = existingCollisionEntity{ID: skill.ID, Name: skill.Name}
				}
			}
		}
		for _, manifestSkill := range manifest.Skills {
			existing, exists := existingSkillKeyToSkill[manifestSkill.Key]
			keyMatch := exists
			slugMatch := false
			nameMatch := false
			if !exists {
				if candidate, ok := existingSkillSlugToSkill[normalizeSlug(manifestSkill.Slug)]; ok {
					existing = candidate
					exists = true
					slugMatch = true
				}
			}
			if !exists {
				if candidate, ok := existingSkillNameToSkill[normalizeSlug(manifestSkill.Name)]; ok {
					existing = candidate
					exists = true
					nameMatch = true
				}
			}
			if !exists {
				continue
			}
			recommendedStrategy := recommendedCollisionStrategy(mode, collisionStrategy, true)
			reason := "Existing skill matched by key or slug."
			plannedAction := "rename"
			matchTypes := []string{}
			if keyMatch {
				matchTypes = append(matchTypes, "key")
			}
			if slugMatch {
				matchTypes = append(matchTypes, "slug")
			}
			if nameMatch {
				matchTypes = append(matchTypes, "name")
			}
			if len(matchTypes) == 0 {
				matchTypes = append(matchTypes, "key")
			}
			switch collisionStrategy {
			case "skip":
				reason = "Existing skill matched; skip strategy."
				plannedAction = "skip"
			case "replace":
				if mode == ImportModeBoardFull {
					reason = "Existing skill matched; replace strategy."
					plannedAction = "update"
				} else {
					reason = "Existing skill matched; rename strategy."
				}
			default:
				reason = "Existing skill matched; rename strategy."
			}
			existingID, existingName := existing.ID, existing.Name
			collisions = append(collisions, ImportCollision{
				EntityType:                   "skill",
				Slug:                         manifestSkill.Slug,
				Name:                         manifestSkill.Name,
				ExistingID:                   &existingID,
				ExistingName:                 &existingName,
				MatchTypes:                   matchTypes,
				RequestedCollisionStrategy:   collisionStrategy,
				RecommendedCollisionStrategy: recommendedStrategy,
				PlannedAction:                plannedAction,
				Reason:                       reason,
			})
			if mode == ImportModeAgentSafe {
				warnings = append(warnings, fmt.Sprintf("Existing skill %q matched during safe import and will %s instead of overwritten.", manifestSkill.Slug, map[string]string{"skip": "be skipped", "rename": "be renamed"}[recommendedStrategy]))
			} else if collisionStrategy == "replace" {
				warnings = append(warnings, fmt.Sprintf("Existing skill %q (%s) will be overwritten by import.", manifestSkill.Slug, manifestSkill.Key))
			}
		}
	}

	// Build issue plans
	issuePlans := []IssuePlan{}
	if include.Issues {
		existingIssueSlugToIssue := map[string]existingCollisionEntity{}
		existingIssueNameToIssue := map[string]existingCollisionEntity{}
		existingIssueIdentifierToIssue := map[string]existingCollisionEntity{}
		usedIssueSlugsForRename := map[string]bool{}
		if req.Target.Mode == "existing_company" {
			var existingIssues []models.Issue
			if err := s.db.WithContext(ctx).
				Where("company_id = ? AND hidden_at IS NULL", req.Target.CompanyID).
				Find(&existingIssues).Error; err != nil {
				return nil, err
			}
			for _, issue := range existingIssues {
				slug := normalizeSlug(issue.Title)
				if slug == "" {
					slug = issue.ID
				}
				if _, exists := existingIssueSlugToIssue[slug]; !exists {
					existingIssueSlugToIssue[slug] = existingCollisionEntity{ID: issue.ID, Name: issue.Title}
				}
				if normalizedTitle := normalizeSlug(issue.Title); normalizedTitle != "" {
					if _, exists := existingIssueNameToIssue[normalizedTitle]; !exists {
						existingIssueNameToIssue[normalizedTitle] = existingCollisionEntity{ID: issue.ID, Name: issue.Title}
					}
				}
				if issue.Identifier != nil && *issue.Identifier != "" {
					existingIssueIdentifierToIssue[*issue.Identifier] = existingCollisionEntity{ID: issue.ID, Name: issue.Title}
				}
				usedIssueSlugsForRename[slug] = true
			}
		}
		for _, manifestIssue := range manifest.Issues {
			reason := (*string)(nil)
			if manifestIssue.Recurring {
				r := "Recurring task will be imported as a routine."
				reason = &r
			}
			action := "create"
			plannedTitle := manifestIssue.Title
			existing, exists := existingIssueSlugToIssue[manifestIssue.Slug]
			slugMatch := exists
			nameMatch := false
			identifierMatch := false
			if !exists {
				if candidate, ok := existingIssueNameToIssue[normalizeSlug(manifestIssue.Title)]; ok {
					existing = candidate
					exists = true
					nameMatch = true
				}
			}
			if !exists && manifestIssue.Identifier != nil && *manifestIssue.Identifier != "" {
				if candidate, ok := existingIssueIdentifierToIssue[*manifestIssue.Identifier]; ok {
					existing = candidate
					exists = true
					identifierMatch = true
				}
			}
			if exists {
				recommendedStrategy := recommendedCollisionStrategy(mode, collisionStrategy, false)
				var reasonText string
				switch recommendedStrategy {
				case "skip":
					action = "skip"
					reasonText = "Existing task matched; skip strategy."
				default:
					plannedTitle = uniqueIssueTitle(manifestIssue.Title, usedIssueSlugsForRename)
					reasonText = "Existing task matched; rename strategy."
					if collisionStrategy == "replace" {
						warnings = append(warnings, fmt.Sprintf("Task %q matched an existing issue and will be renamed because replace is not supported for tasks.", manifestIssue.Slug))
					}
				}
				reason = &reasonText
				existingID, existingName := existing.ID, existing.Name
				collisions = append(collisions, ImportCollision{
					EntityType:                   "issue",
					Slug:                         manifestIssue.Slug,
					Name:                         manifestIssue.Title,
					ExistingID:                   &existingID,
					ExistingName:                 &existingName,
					MatchTypes:                   buildCollisionMatchTypes(slugMatch, nameMatch, identifierMatch),
					RequestedCollisionStrategy:   collisionStrategy,
					RecommendedCollisionStrategy: recommendedStrategy,
					PlannedAction:                action,
					Reason:                       reasonText,
				})
			} else {
				usedIssueSlugsForRename[normalizeSlug(plannedTitle)] = true
			}
			issuePlans = append(issuePlans, IssuePlan{
				Slug:         manifestIssue.Slug,
				Action:       action,
				PlannedTitle: plannedTitle,
				Reason:       reason,
			})
		}
	}

	// Determine company action
	companyAction := "none"
	if req.Target.Mode == "new_company" {
		companyAction = "create"
	} else if include.Company && mode == ImportModeBoardFull {
		companyAction = "update"
	}

	selectedSlugs := make([]string, 0, len(selectedAgents))
	for _, ag := range selectedAgents {
		selectedSlugs = append(selectedSlugs, ag.Slug)
	}

	preview := PreviewResult{
		Include:            include,
		TargetCompanyID:    targetCompanyID,
		TargetCompanyName:  targetCompanyName,
		CollisionStrategy:  collisionStrategy,
		SelectedAgentSlugs: selectedSlugs,
		Plan: ImportPlan{
			CompanyAction: companyAction,
			AgentPlans:    agentPlans,
			ProjectPlans:  projectPlans,
			IssuePlans:    issuePlans,
		},
		Collisions: collisions,
		Manifest:   manifest,
		Files:      source.files,
		EnvInputs:  manifest.EnvInputs,
		Warnings:   warnings,
		Errors:     errors,
	}

	return &internalPlan{
		preview:           preview,
		source:            source,
		include:           include,
		collisionStrategy: collisionStrategy,
		selectedAgents:    selectedAgents,
	}, nil
}

// PreviewImport returns an import preview without making any changes.
func (s *PortabilityService) PreviewImport(ctx context.Context, req ImportRequest, mode ImportMode) (*PreviewResult, error) {
	plan, err := s.buildPreview(ctx, req, mode)
	if err != nil {
		return nil, err
	}
	return &plan.preview, nil
}

// ──────────────────────────────────────────────────────────────────────────────
// Import – execute.
// ──────────────────────────────────────────────────────────────────────────────

// ImportBundle executes a portability import.
func (s *PortabilityService) ImportBundle(ctx context.Context, req ImportRequest, actorUserID string, mode ImportMode) (*ImportResult, error) {
	if mode == "" {
		mode = ImportModeBoardFull
	}

	plan, err := s.buildPreview(ctx, req, mode)
	if err != nil {
		return nil, err
	}

	if len(plan.preview.Errors) > 0 {
		return nil, fmt.Errorf("import preview has errors: %s", strings.Join(plan.preview.Errors, "; "))
	}
	if mode == ImportModeAgentSafe {
		if plan.preview.Plan.CompanyAction == "update" {
			return nil, fmt.Errorf("safe import routes only allow create or skip actions")
		}
		for _, ap := range plan.preview.Plan.AgentPlans {
			if ap.Action == "update" {
				return nil, fmt.Errorf("safe import routes only allow create or skip actions")
			}
		}
		for _, pp := range plan.preview.Plan.ProjectPlans {
			if pp.Action == "update" {
				return nil, fmt.Errorf("safe import routes only allow create or skip actions")
			}
		}
	}

	manifest := plan.source.manifest
	warnings := plan.preview.Warnings[:]
	include := plan.include

	// ── Resolve or create target company ─────────────────────────────────────
	var targetCompany models.Company
	companyAction := "unchanged"

	if req.Target.Mode == "new_company" {
		companyName := "Imported Company"
		if req.Target.NewCompanyName != nil && *req.Target.NewCompanyName != "" {
			companyName = *req.Target.NewCompanyName
		} else if manifest.Company != nil {
			companyName = manifest.Company.Name
		} else if manifest.Source != nil {
			companyName = manifest.Source.CompanyName
		}

		newCompany := models.Company{
			ID:     uuid.New().String(),
			Name:   companyName,
			Status: "active",
			RequireBoardApprovalForNewAgents: true,
		}
		if include.Company && manifest.Company != nil {
			if manifest.Company.Description != nil {
				newCompany.Description = manifest.Company.Description
			}
			newCompany.BrandColor = manifest.Company.BrandColor
			newCompany.RequireBoardApprovalForNewAgents = manifest.Company.RequireBoardApprovalForNewAgents
			newCompany.FeedbackDataSharingEnabled = manifest.Company.FeedbackDataSharingEnabled
			if manifest.Company.FeedbackDataSharingConsentAt != nil {
				t, _ := time.Parse(time.RFC3339, *manifest.Company.FeedbackDataSharingConsentAt)
				if !t.IsZero() {
					newCompany.FeedbackDataSharingConsentAt = &t
				}
			}
			newCompany.FeedbackDataSharingConsentByUserID = manifest.Company.FeedbackDataSharingConsentByUserID
			newCompany.FeedbackDataSharingTermsVersion = manifest.Company.FeedbackDataSharingTermsVersion
		}
		if err := s.db.WithContext(ctx).Create(&newCompany).Error; err != nil {
			return nil, fmt.Errorf("failed to create company: %w", err)
		}

		// Ensure the actor is a member
		if actorUserID != "" {
			membership := models.CompanyMembership{
				ID:            uuid.New().String(),
				CompanyID:     newCompany.ID,
				PrincipalType: "user",
				PrincipalID:   actorUserID,
				Status:        "active",
			}
			_ = s.db.WithContext(ctx).
				Where("company_id = ? AND principal_type = ? AND principal_id = ?", newCompany.ID, "user", actorUserID).
				FirstOrCreate(&membership).Error
		}

		// Copy memberships for agent_safe mode
		if mode == ImportModeAgentSafe && req.Target.Mode == "existing_company" {
			var sourceMemberships []models.CompanyMembership
			_ = s.db.WithContext(ctx).
				Where("company_id = ? AND status = ? AND principal_type = ?", req.Target.CompanyID, "active", "user").
				Find(&sourceMemberships).Error
			for _, m := range sourceMemberships {
				newM := models.CompanyMembership{
					ID:            uuid.New().String(),
					CompanyID:     newCompany.ID,
					PrincipalType: m.PrincipalType,
					PrincipalID:   m.PrincipalID,
					Status:        "active",
					MembershipRole: m.MembershipRole,
				}
				_ = s.db.WithContext(ctx).Create(&newM).Error
			}
		}

		targetCompany = newCompany
		companyAction = "created"
	} else {
		if err := s.db.WithContext(ctx).First(&targetCompany, "id = ?", req.Target.CompanyID).Error; err != nil {
			return nil, fmt.Errorf("target company not found")
		}
		if include.Company && manifest.Company != nil && mode == ImportModeBoardFull {
			updates := map[string]interface{}{
				"name":                                  manifest.Company.Name,
				"require_board_approval_for_new_agents": manifest.Company.RequireBoardApprovalForNewAgents,
				"feedback_data_sharing_enabled":         manifest.Company.FeedbackDataSharingEnabled,
			}
			if manifest.Company.Description != nil {
				updates["description"] = *manifest.Company.Description
			}
			if manifest.Company.BrandColor != nil {
				updates["brand_color"] = *manifest.Company.BrandColor
			}
			if manifest.Company.FeedbackDataSharingConsentAt != nil {
				t, err := time.Parse(time.RFC3339, *manifest.Company.FeedbackDataSharingConsentAt)
				if err == nil {
					updates["feedback_data_sharing_consent_at"] = t
				}
			}
			if manifest.Company.FeedbackDataSharingConsentByUserID != nil {
				updates["feedback_data_sharing_consent_by_user_id"] = *manifest.Company.FeedbackDataSharingConsentByUserID
			}
			if manifest.Company.FeedbackDataSharingTermsVersion != nil {
				updates["feedback_data_sharing_terms_version"] = *manifest.Company.FeedbackDataSharingTermsVersion
			}
			if err := s.db.WithContext(ctx).Model(&targetCompany).Updates(updates).Error; err != nil {
				warnings = append(warnings, fmt.Sprintf("Failed to update company metadata: %v", err))
			} else {
				companyAction = "updated"
			}
			// Reload
			_ = s.db.WithContext(ctx).First(&targetCompany, "id = ?", req.Target.CompanyID).Error
		}
	}

	// ── Import agents ─────────────────────────────────────────────────────────
	resultAgents := []ImportResultAgent{}
	importedSlugToAgentID := map[string]string{}
	existingSlugToAgentID := map[string]string{}

	if include.Agents {
		var existingAgents []models.Agent
		_ = s.db.WithContext(ctx).
			Where("company_id = ? AND status != ?", targetCompany.ID, "terminated").
			Find(&existingAgents).Error
		for _, ag := range existingAgents {
			slug := normalizeSlug(ag.Name)
			if slug != "" {
				existingSlugToAgentID[slug] = ag.ID
			}
		}

		for _, agentPlan := range plan.preview.Plan.AgentPlans {
			manifestAgent := findAgentBySlug(plan.selectedAgents, agentPlan.Slug)
			if manifestAgent == nil {
				continue
			}

			if agentPlan.Action == "skip" {
				resultAgents = append(resultAgents, ImportResultAgent{
					Slug:   agentPlan.Slug,
					ID:     agentPlan.ExistingAgentID,
					Action: "skipped",
					Name:   agentPlan.PlannedName,
					Reason: agentPlan.Reason,
				})
				continue
			}

			// Get instructions from AGENTS.md
			agentMDPath := manifestAgent.Path
			instructions, _ := readFileAsText(plan.source.files, agentMDPath)
			_, instructionBody := parseFrontmatter(instructions)

			// Apply adapter override if provided
			adapterType := manifestAgent.AdapterType
			adapterCfg := cloneMap(manifestAgent.AdapterConfig)
			if override, ok := req.AdapterOverrides[agentPlan.Slug]; ok && req.AdapterOverrides != nil {
				if override.AdapterType != "" {
					adapterType = override.AdapterType
				}
				if override.AdapterConfig != nil {
					adapterCfg = override.AdapterConfig
				}
			}

			// Store instructions as promptTemplate
			if instructionBody != "" {
				adapterCfg["promptTemplate"] = instructionBody
			}

			// Persist skill references
			if len(manifestAgent.Skills) > 0 {
				adapterCfg["skills"] = manifestAgent.Skills
			}

			adapterCfgJSON := mapToJSON(adapterCfg)
			// Gap 6: disable heartbeat on imported agents to prevent auto-starts.
			importedRuntimeConfig := disableImportedTimerHeartbeat(manifestAgent.RuntimeConfig)
			runtimeCfgJSON := mapToJSON(importedRuntimeConfig)
			permissionsJSON := mapToJSON(manifestAgent.Permissions)
			if len(permissionsJSON) == 0 {
				permissionsJSON = datatypes.JSON("{}")
			}

			if agentPlan.Action == "update" && agentPlan.ExistingAgentID != nil {
				// Update existing agent
				updates := map[string]interface{}{
					"name":                 agentPlan.PlannedName,
					"role":                 manifestAgent.Role,
					"adapter_type":         adapterType,
					"adapter_config":       adapterCfgJSON,
					"runtime_config":       runtimeCfgJSON,
					"permissions":          permissionsJSON,
					"budget_monthly_cents": manifestAgent.BudgetMonthlyCents,
				}
				if manifestAgent.Title != nil {
					updates["title"] = *manifestAgent.Title
				}
				if manifestAgent.Icon != nil {
					updates["icon"] = *manifestAgent.Icon
				}
				if manifestAgent.Capabilities != nil {
					updates["capabilities"] = *manifestAgent.Capabilities
				}
				if err := s.db.WithContext(ctx).
					Model(&models.Agent{}).
					Where("id = ?", *agentPlan.ExistingAgentID).
					Updates(updates).Error; err != nil {
					warnings = append(warnings, fmt.Sprintf("Failed to update agent %s: %v", agentPlan.Slug, err))
					resultAgents = append(resultAgents, ImportResultAgent{
						Slug:   agentPlan.Slug,
						ID:     nil,
						Action: "skipped",
						Name:   agentPlan.PlannedName,
						Reason: portStrPtr(fmt.Sprintf("Update failed: %v", err)),
					})
					continue
				}
				importedSlugToAgentID[agentPlan.Slug] = *agentPlan.ExistingAgentID
				existingSlugToAgentID[normalizeSlug(agentPlan.PlannedName)] = *agentPlan.ExistingAgentID
				resultAgents = append(resultAgents, ImportResultAgent{
					Slug:   agentPlan.Slug,
					ID:     agentPlan.ExistingAgentID,
					Action: "updated",
					Name:   agentPlan.PlannedName,
					Reason: agentPlan.Reason,
				})
				continue
			}

			// Create new agent
			newAgent := models.Agent{
				ID:                 uuid.New().String(),
				CompanyID:          targetCompany.ID,
				Name:               agentPlan.PlannedName,
				Role:               manifestAgent.Role,
				AdapterType:        adapterType,
				AdapterConfig:      adapterCfgJSON,
				RuntimeConfig:      runtimeCfgJSON,
				Permissions:        permissionsJSON,
				BudgetMonthlyCents: manifestAgent.BudgetMonthlyCents,
				Status:             "idle",
			}
			if manifestAgent.Title != nil {
				newAgent.Title = manifestAgent.Title
			}
			if manifestAgent.Icon != nil {
				newAgent.Icon = manifestAgent.Icon
			}
			if manifestAgent.Capabilities != nil {
				newAgent.Capabilities = manifestAgent.Capabilities
			}
			if err := s.db.WithContext(ctx).Create(&newAgent).Error; err != nil {
				warnings = append(warnings, fmt.Sprintf("Failed to create agent %s: %v", agentPlan.Slug, err))
				resultAgents = append(resultAgents, ImportResultAgent{
					Slug:   agentPlan.Slug,
					ID:     nil,
					Action: "skipped",
					Name:   agentPlan.PlannedName,
					Reason: portStrPtr(fmt.Sprintf("Create failed: %v", err)),
				})
				continue
			}

			// Ensure membership
			membership := models.CompanyMembership{
				ID:            uuid.New().String(),
				CompanyID:     targetCompany.ID,
				PrincipalType: "agent",
				PrincipalID:   newAgent.ID,
				Status:        "active",
			}
			_ = s.db.WithContext(ctx).Create(&membership).Error

			agentIDRef := newAgent.ID
			importedSlugToAgentID[agentPlan.Slug] = newAgent.ID
			existingSlugToAgentID[normalizeSlug(newAgent.Name)] = newAgent.ID
			resultAgents = append(resultAgents, ImportResultAgent{
				Slug:   agentPlan.Slug,
				ID:     &agentIDRef,
				Action: "created",
				Name:   newAgent.Name,
				Reason: agentPlan.Reason,
			})
		}

		// Apply reportsTo links
		for _, manifestAgent := range plan.selectedAgents {
			agentID, ok := importedSlugToAgentID[manifestAgent.Slug]
			if !ok {
				continue
			}
			if manifestAgent.ReportsToSlug == nil || *manifestAgent.ReportsToSlug == "" {
				continue
			}
			managerID, ok := importedSlugToAgentID[*manifestAgent.ReportsToSlug]
			if !ok {
				managerID = existingSlugToAgentID[*manifestAgent.ReportsToSlug]
			}
			if managerID == "" || managerID == agentID {
				continue
			}
			if err := s.db.WithContext(ctx).
				Model(&models.Agent{}).
				Where("id = ?", agentID).
				Update("reports_to", managerID).Error; err != nil {
				warnings = append(warnings, fmt.Sprintf("Could not assign manager %s for imported agent %s.", *manifestAgent.ReportsToSlug, manifestAgent.Slug))
			}
		}
	}

	// ── Import projects ────────────────────────────────────────────────────────
	resultProjects := []ImportResultProject{}
	importedProjectSlugToID := map[string]string{}

	if include.Projects {
		for _, projectPlan := range plan.preview.Plan.ProjectPlans {
			manifestProject := findProjectBySlug(manifest.Projects, projectPlan.Slug)
			if manifestProject == nil {
				continue
			}

			if projectPlan.Action == "skip" {
				resultProjects = append(resultProjects, ImportResultProject{
					Slug:   projectPlan.Slug,
					ID:     projectPlan.ExistingProjectID,
					Action: "skipped",
					Name:   projectPlan.PlannedName,
					Reason: projectPlan.Reason,
				})
				continue
			}

			leadAgentID := (*string)(nil)
			if manifestProject.LeadAgentSlug != nil {
				if id, ok := importedSlugToAgentID[*manifestProject.LeadAgentSlug]; ok {
					leadAgentID = &id
				} else if id, ok := existingSlugToAgentID[*manifestProject.LeadAgentSlug]; ok {
					leadAgentID = &id
				}
			}

			status := "backlog"
			if manifestProject.Status != nil && *manifestProject.Status != "" {
				status = *manifestProject.Status
			}

			if projectPlan.Action == "update" && projectPlan.ExistingProjectID != nil {
				updates := map[string]interface{}{
					"name":   projectPlan.PlannedName,
					"status": status,
				}
				if manifestProject.Description != nil {
					updates["description"] = *manifestProject.Description
				}
				if leadAgentID != nil {
					updates["lead_agent_id"] = *leadAgentID
				}
				if manifestProject.Color != nil {
					updates["color"] = *manifestProject.Color
				}
				if manifestProject.TargetDate != nil {
					t, err := time.Parse("2006-01-02", *manifestProject.TargetDate)
					if err == nil {
						updates["target_date"] = t
					}
				}
				if err := s.db.WithContext(ctx).
					Model(&models.Project{}).
					Where("id = ?", *projectPlan.ExistingProjectID).
					Updates(updates).Error; err != nil {
					warnings = append(warnings, fmt.Sprintf("Failed to update project %s: %v", projectPlan.Slug, err))
					resultProjects = append(resultProjects, ImportResultProject{
						Slug:   projectPlan.Slug,
						ID:     nil,
						Action: "skipped",
						Name:   projectPlan.PlannedName,
						Reason: portStrPtr(fmt.Sprintf("Update failed: %v", err)),
					})
					continue
				}
				importedProjectSlugToID[projectPlan.Slug] = *projectPlan.ExistingProjectID
				resultProjects = append(resultProjects, ImportResultProject{
					Slug:   projectPlan.Slug,
					ID:     projectPlan.ExistingProjectID,
					Action: "updated",
					Name:   projectPlan.PlannedName,
					Reason: projectPlan.Reason,
				})
				continue
			}

			// Create new project
			newProject := models.Project{
				ID:        uuid.New().String(),
				CompanyID: targetCompany.ID,
				Name:      projectPlan.PlannedName,
				Status:    status,
			}
			if manifestProject.Description != nil {
				newProject.Description = manifestProject.Description
			}
			if leadAgentID != nil {
				newProject.LeadAgentID = leadAgentID
			}
			if manifestProject.Color != nil {
				newProject.Color = manifestProject.Color
			}
			if manifestProject.TargetDate != nil {
				t, err := time.Parse("2006-01-02", *manifestProject.TargetDate)
				if err == nil {
					newProject.TargetDate = &t
				}
			}
			if err := s.db.WithContext(ctx).Create(&newProject).Error; err != nil {
				warnings = append(warnings, fmt.Sprintf("Failed to create project %s: %v", projectPlan.Slug, err))
				resultProjects = append(resultProjects, ImportResultProject{
					Slug:   projectPlan.Slug,
					ID:     nil,
					Action: "skipped",
					Name:   projectPlan.PlannedName,
					Reason: portStrPtr(fmt.Sprintf("Create failed: %v", err)),
				})
				continue
			}
			projectIDRef := newProject.ID
			importedProjectSlugToID[projectPlan.Slug] = newProject.ID
			resultProjects = append(resultProjects, ImportResultProject{
				Slug:   projectPlan.Slug,
				ID:     &projectIDRef,
				Action: "created",
				Name:   newProject.Name,
				Reason: projectPlan.Reason,
			})
		}
	}

	// ── Import issues/tasks ────────────────────────────────────────────────────
	if include.Issues {
		for _, issuePlan := range plan.preview.Plan.IssuePlans {
			manifestIssue := findIssueBySlug(manifest.Issues, issuePlan.Slug)
			if manifestIssue == nil || issuePlan.Action == "skip" {
				continue
			}

			if manifestIssue.Recurring && manifestIssue.Routine != nil {
				// Import as a routine
				if manifestIssue.ProjectSlug == nil || *manifestIssue.ProjectSlug == "" {
					warnings = append(warnings, fmt.Sprintf("Recurring task %s skipped: no project slug.", issuePlan.Slug))
					continue
				}
				projectID, ok := importedProjectSlugToID[*manifestIssue.ProjectSlug]
				if !ok {
					warnings = append(warnings, fmt.Sprintf("Recurring task %s skipped: project %s not imported.", issuePlan.Slug, *manifestIssue.ProjectSlug))
					continue
				}
				assigneeID := ""
				if manifestIssue.AssigneeAgentSlug != nil {
					if id, ok := importedSlugToAgentID[*manifestIssue.AssigneeAgentSlug]; ok {
						assigneeID = id
					} else if id, ok := existingSlugToAgentID[*manifestIssue.AssigneeAgentSlug]; ok {
						assigneeID = id
					}
				}
				if assigneeID == "" {
					warnings = append(warnings, fmt.Sprintf("Recurring task %s skipped: assignee agent not found.", issuePlan.Slug))
					continue
				}

				routine := models.Routine{
					ID:              uuid.New().String(),
					CompanyID:       targetCompany.ID,
					ProjectID:       projectID,
					Title:           issuePlan.PlannedTitle,
					AssigneeAgentID: assigneeID,
					Status:          "active",
					ConcurrencyPolicy: "coalesce_if_active",
					CatchUpPolicy:     "skip_missed",
					Variables:         datatypes.JSON("[]"),
				}
				if manifestIssue.Description != nil {
					routine.Description = manifestIssue.Description
				}
				if manifestIssue.Routine.ConcurrencyPolicy != nil {
					routine.ConcurrencyPolicy = *manifestIssue.Routine.ConcurrencyPolicy
				}
				if manifestIssue.Routine.CatchUpPolicy != nil {
					routine.CatchUpPolicy = *manifestIssue.Routine.CatchUpPolicy
				}
				if manifestIssue.Priority != nil {
					routine.Priority = *manifestIssue.Priority
				} else {
					routine.Priority = "medium"
				}
				if err := s.db.WithContext(ctx).Create(&routine).Error; err != nil {
					warnings = append(warnings, fmt.Sprintf("Failed to create routine %s: %v", issuePlan.Slug, err))
					continue
				}

				// Create triggers
				for _, trigger := range manifestIssue.Routine.Triggers {
					t := models.RoutineTrigger{
						ID:        uuid.New().String(),
						CompanyID: targetCompany.ID,
						RoutineID: routine.ID,
						Kind:      trigger.Kind,
						Label:     trigger.Label,
						Enabled:   trigger.Enabled,
					}
					if trigger.CronExpression != nil {
						t.CronExpression = trigger.CronExpression
					}
					if trigger.Timezone != nil {
						t.Timezone = trigger.Timezone
					}
					if trigger.SigningMode != nil {
						t.SigningMode = trigger.SigningMode
					}
					if trigger.ReplayWindowSec != nil {
						t.ReplayWindowSec = trigger.ReplayWindowSec
					}
					_ = s.db.WithContext(ctx).Create(&t).Error
				}
				continue
			}

			// Import as a regular issue
			projectID := (*string)(nil)
			if manifestIssue.ProjectSlug != nil && *manifestIssue.ProjectSlug != "" {
				if id, ok := importedProjectSlugToID[*manifestIssue.ProjectSlug]; ok {
					projectID = &id
				}
			}
			assigneeID := (*string)(nil)
			if manifestIssue.AssigneeAgentSlug != nil {
				if id, ok := importedSlugToAgentID[*manifestIssue.AssigneeAgentSlug]; ok {
					assigneeID = &id
				} else if id, ok := existingSlugToAgentID[*manifestIssue.AssigneeAgentSlug]; ok {
					assigneeID = &id
				}
			}

			status := "backlog"
			if manifestIssue.Status != nil && *manifestIssue.Status != "" {
				status = *manifestIssue.Status
			}
			priority := "medium"
			if manifestIssue.Priority != nil && *manifestIssue.Priority != "" {
				priority = *manifestIssue.Priority
			}

			newIssue := models.Issue{
				ID:              uuid.New().String(),
				CompanyID:       targetCompany.ID,
				Title:           issuePlan.PlannedTitle,
				Status:          status,
				Priority:        priority,
				OriginKind:      "manual",
				ProjectID:       projectID,
				AssigneeAgentID: assigneeID,
			}
			if manifestIssue.Description != nil {
				newIssue.Description = manifestIssue.Description
			}
			if manifestIssue.BillingCode != nil {
				newIssue.BillingCode = manifestIssue.BillingCode
			}
			if err := s.db.WithContext(ctx).Create(&newIssue).Error; err != nil {
				warnings = append(warnings, fmt.Sprintf("Failed to create issue %s: %v", issuePlan.Slug, err))
			}
		}
	}

	// Gap 7: warn about system_dependent env inputs that may need manual adjustment.
	for _, envInput := range manifest.EnvInputs {
		if envInput.Portability == "system_dependent" {
			scope := ""
			if envInput.AgentSlug != nil {
				scope = " (agent " + *envInput.AgentSlug + ")"
			} else if envInput.ProjectSlug != nil {
				scope = " (project " + *envInput.ProjectSlug + ")"
			}
			warnings = append(warnings, fmt.Sprintf("Environment input %s%s is system-dependent and may need manual adjustment after import.", envInput.Key, scope))
		}
	}

	return &ImportResult{
		Company: ImportResultCompany{
			ID:     targetCompany.ID,
			Name:   targetCompany.Name,
			Action: companyAction,
		},
		Agents:    resultAgents,
		Projects:  resultProjects,
		EnvInputs: manifest.EnvInputs,
		Warnings:  warnings,
	}, nil
}

// ──────────────────────────────────────────────────────────────────────────────
// Helpers.
// ──────────────────────────────────────────────────────────────────────────────

func findAgentBySlug(agents []PortabilityAgentManifestEntry, slug string) *PortabilityAgentManifestEntry {
	for i := range agents {
		if agents[i].Slug == slug {
			return &agents[i]
		}
	}
	return nil
}

func findProjectBySlug(projects []PortabilityProjectManifestEntry, slug string) *PortabilityProjectManifestEntry {
	for i := range projects {
		if projects[i].Slug == slug {
			return &projects[i]
		}
	}
	return nil
}

func findIssueBySlug(issues []PortabilityIssueManifestEntry, slug string) *PortabilityIssueManifestEntry {
	for i := range issues {
		if issues[i].Slug == slug {
			return &issues[i]
		}
	}
	return nil
}

func cloneMap(m map[string]interface{}) map[string]interface{} {
	if m == nil {
		return map[string]interface{}{}
	}
	out := make(map[string]interface{}, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

// base64Decode decodes a base64 string; returns nil on error.
func base64Decode(s string) []byte {
	b, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		b, _ = base64.RawStdEncoding.DecodeString(s)
	}
	return b
}

// unused at runtime but prevents import error if the compiler complains
var _ = base64Decode
