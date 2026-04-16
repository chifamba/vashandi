// Package services provides the agent instructions service for managing agent
// instruction bundles in both managed and external modes, mirroring the Node.js
// implementation in server/src/services/agent-instructions.ts.
package services

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/chifamba/vashandi/vashandi/backend/db/models"
	"gorm.io/gorm"
)

// Bundle mode constants
const (
	BundleModeManaged  = "managed"
	BundleModeExternal = "external"

	EntryFileDefault             = "AGENTS.md"
	ModeKey                      = "instructionsBundleMode"
	RootKey                      = "instructionsRootPath"
	EntryKey                     = "instructionsEntryFile"
	FileKey                      = "instructionsFilePath"
	PromptKey                    = "promptTemplate"
	BootstrapPromptKey           = "bootstrapPromptTemplate"
	LegacyPromptTemplatePath     = "promptTemplate.legacy.md"
)

// Ignored file/directory names for bundle listing
var (
	ignoredFileNames = map[string]bool{
		".DS_Store":   true,
		"Thumbs.db":   true,
		"Desktop.ini": true,
	}
	ignoredDirNames = map[string]bool{
		".git":            true,
		".nox":            true,
		".pytest_cache":   true,
		".ruff_cache":     true,
		".tox":            true,
		".venv":           true,
		"__pycache__":     true,
		"node_modules":    true,
		"venv":            true,
	}
)

// AgentInstructionsFileSummary represents summary information about an instruction file.
type AgentInstructionsFileSummary struct {
	Path        string `json:"path"`
	Size        int64  `json:"size"`
	Language    string `json:"language"`
	Markdown    bool   `json:"markdown"`
	IsEntryFile bool   `json:"isEntryFile"`
	Editable    bool   `json:"editable"`
	Deprecated  bool   `json:"deprecated"`
	Virtual     bool   `json:"virtual"`
}

// AgentInstructionsFileDetail contains full file details including content.
type AgentInstructionsFileDetail struct {
	AgentInstructionsFileSummary
	Content string `json:"content"`
}

// AgentInstructionsBundle represents the full bundle state.
type AgentInstructionsBundle struct {
	AgentID                            string                         `json:"agentId"`
	CompanyID                          string                         `json:"companyId"`
	Mode                               *string                        `json:"mode"`
	RootPath                           *string                        `json:"rootPath"`
	ManagedRootPath                    string                         `json:"managedRootPath"`
	EntryFile                          string                         `json:"entryFile"`
	ResolvedEntryPath                  *string                        `json:"resolvedEntryPath"`
	Editable                           bool                           `json:"editable"`
	Warnings                           []string                       `json:"warnings"`
	LegacyPromptTemplateActive         bool                           `json:"legacyPromptTemplateActive"`
	LegacyBootstrapPromptTemplateActive bool                          `json:"legacyBootstrapPromptTemplateActive"`
	Files                              []AgentInstructionsFileSummary `json:"files"`
}

// bundleState holds internal state while processing bundle configuration.
type bundleState struct {
	config                              map[string]interface{}
	mode                                *string
	rootPath                            *string
	entryFile                           string
	resolvedEntryPath                   *string
	warnings                            []string
	legacyPromptTemplateActive          bool
	legacyBootstrapPromptTemplateActive bool
}

// AgentInstructionsService manages agent instruction bundles.
type AgentInstructionsService struct {
	DB           *gorm.DB
	InstanceRoot string // Base path for managed instruction storage
}

// NewAgentInstructionsService creates a new service instance.
func NewAgentInstructionsService(db *gorm.DB, instanceRoot string) *AgentInstructionsService {
	if instanceRoot == "" {
		// Default to a data directory in the current working directory
		cwd, _ := os.Getwd()
		instanceRoot = filepath.Join(cwd, "data", "paperclip")
	}
	return &AgentInstructionsService{
		DB:           db,
		InstanceRoot: instanceRoot,
	}
}

// GetBundle returns the full instruction bundle for an agent.
func (s *AgentInstructionsService) GetBundle(agent *models.Agent) (*AgentInstructionsBundle, error) {
	state := s.deriveBundleState(agent)
	state = s.recoverManagedBundleState(agent, state)

	if state.rootPath == nil {
		return s.toBundle(agent, state, nil), nil
	}

	info, err := os.Stat(*state.rootPath)
	if err != nil || !info.IsDir() {
		state.warnings = append(state.warnings, fmt.Sprintf("Instructions root does not exist: %s", *state.rootPath))
		return s.toBundle(agent, state, nil), nil
	}

	files, err := s.listFilesRecursive(*state.rootPath)
	if err != nil {
		return nil, fmt.Errorf("failed to list files: %w", err)
	}

	summaries := make([]AgentInstructionsFileSummary, 0, len(files))
	for _, relPath := range files {
		summary, err := s.readFileSummary(*state.rootPath, relPath, state.entryFile)
		if err != nil {
			continue
		}
		summaries = append(summaries, summary)
	}

	return s.toBundle(agent, state, summaries), nil
}

// ReadFile reads a specific file from the agent's instruction bundle.
func (s *AgentInstructionsService) ReadFile(agent *models.Agent, relativePath string) (*AgentInstructionsFileDetail, error) {
	state := s.deriveBundleState(agent)
	state = s.recoverManagedBundleState(agent, state)

	// Handle legacy prompt template pseudo-file
	if relativePath == LegacyPromptTemplatePath {
		content := asString(state.config[PromptKey])
		if content == nil {
			return nil, errors.New("instructions file not found")
		}
		return &AgentInstructionsFileDetail{
			AgentInstructionsFileSummary: AgentInstructionsFileSummary{
				Path:        LegacyPromptTemplatePath,
				Size:        int64(len(*content)),
				Language:    "markdown",
				Markdown:    true,
				IsEntryFile: false,
				Editable:    true,
				Deprecated:  true,
				Virtual:     true,
			},
			Content: *content,
		}, nil
	}

	if state.rootPath == nil {
		return nil, errors.New("agent instructions bundle is not configured")
	}

	absPath, err := s.resolvePathWithinRoot(*state.rootPath, relativePath)
	if err != nil {
		return nil, err
	}

	content, err := os.ReadFile(absPath)
	if err != nil {
		return nil, errors.New("instructions file not found")
	}

	info, err := os.Stat(absPath)
	if err != nil || info.IsDir() {
		return nil, errors.New("instructions file not found")
	}

	normalizedPath := normalizeRelativeFilePath(relativePath)
	return &AgentInstructionsFileDetail{
		AgentInstructionsFileSummary: AgentInstructionsFileSummary{
			Path:        normalizedPath,
			Size:        info.Size(),
			Language:    inferLanguage(normalizedPath),
			Markdown:    isMarkdown(normalizedPath),
			IsEntryFile: normalizedPath == state.entryFile,
			Editable:    true,
			Deprecated:  false,
			Virtual:     false,
		},
		Content: string(content),
	}, nil
}

// WriteFile writes content to a file in the agent's instruction bundle.
func (s *AgentInstructionsService) WriteFile(agent *models.Agent, relativePath, content string, clearLegacy bool) (*AgentInstructionsBundle, *AgentInstructionsFileDetail, map[string]interface{}, error) {
	state := s.deriveBundleState(agent)

	// Handle legacy prompt template
	if relativePath == LegacyPromptTemplatePath {
		adapterConfig := copyConfig(state.config)
		adapterConfig[PromptKey] = content
		updatedAgent := *agent
		if err := setAdapterConfig(&updatedAgent, adapterConfig); err != nil {
			return nil, nil, nil, err
		}
		bundle, err := s.GetBundle(&updatedAgent)
		if err != nil {
			return nil, nil, nil, err
		}
		file, err := s.ReadFile(&updatedAgent, LegacyPromptTemplatePath)
		if err != nil {
			return nil, nil, nil, err
		}
		return bundle, file, adapterConfig, nil
	}

	// Ensure writable bundle
	adapterConfig, updatedState, err := s.ensureWritableBundle(agent, clearLegacy)
	if err != nil {
		return nil, nil, nil, err
	}

	absPath, err := s.resolvePathWithinRoot(*updatedState.rootPath, relativePath)
	if err != nil {
		return nil, nil, nil, err
	}

	// Create parent directories
	if err := os.MkdirAll(filepath.Dir(absPath), 0755); err != nil {
		return nil, nil, nil, fmt.Errorf("failed to create directory: %w", err)
	}

	// Write file
	if err := os.WriteFile(absPath, []byte(content), 0644); err != nil {
		return nil, nil, nil, fmt.Errorf("failed to write file: %w", err)
	}

	// Return updated bundle and file
	updatedAgent := *agent
	if err := setAdapterConfig(&updatedAgent, adapterConfig); err != nil {
		return nil, nil, nil, err
	}
	bundle, err := s.GetBundle(&updatedAgent)
	if err != nil {
		return nil, nil, nil, err
	}
	file, err := s.ReadFile(&updatedAgent, relativePath)
	if err != nil {
		return nil, nil, nil, err
	}
	return bundle, file, adapterConfig, nil
}

// DeleteFile removes a file from the agent's instruction bundle.
func (s *AgentInstructionsService) DeleteFile(agent *models.Agent, relativePath string) (*AgentInstructionsBundle, map[string]interface{}, error) {
	state := s.deriveBundleState(agent)
	state = s.recoverManagedBundleState(agent, state)

	if relativePath == LegacyPromptTemplatePath {
		return nil, nil, errors.New("cannot delete the legacy promptTemplate pseudo-file")
	}

	if state.rootPath == nil {
		return nil, nil, errors.New("agent instructions bundle is not configured")
	}

	normalizedPath := normalizeRelativeFilePath(relativePath)
	if normalizedPath == state.entryFile {
		return nil, nil, errors.New("cannot delete the bundle entry file")
	}

	absPath, err := s.resolvePathWithinRoot(*state.rootPath, normalizedPath)
	if err != nil {
		return nil, nil, err
	}

	// Remove file
	_ = os.Remove(absPath)

	adapterConfig := s.buildPersistedBundleConfig(s.deriveBundleState(agent), state, false)
	updatedAgent := *agent
	if err := setAdapterConfig(&updatedAgent, adapterConfig); err != nil {
		return nil, nil, err
	}
	bundle, err := s.GetBundle(&updatedAgent)
	if err != nil {
		return nil, nil, err
	}
	return bundle, adapterConfig, nil
}

// UpdateBundle updates the bundle configuration (mode, root path, entry file).
type UpdateBundleInput struct {
	Mode                     *string `json:"mode"`
	RootPath                 *string `json:"rootPath"`
	EntryFile                *string `json:"entryFile"`
	ClearLegacyPromptTemplate bool   `json:"clearLegacyPromptTemplate"`
}

func (s *AgentInstructionsService) UpdateBundle(agent *models.Agent, input UpdateBundleInput) (*AgentInstructionsBundle, map[string]interface{}, error) {
	state := s.deriveBundleState(agent)
	state = s.recoverManagedBundleState(agent, state)

	nextMode := BundleModeManaged
	if input.Mode != nil {
		nextMode = *input.Mode
	} else if state.mode != nil {
		nextMode = *state.mode
	}

	nextEntryFile := state.entryFile
	if input.EntryFile != nil {
		nextEntryFile = normalizeRelativeFilePath(*input.EntryFile)
	}

	var nextRootPath string
	if nextMode == BundleModeManaged {
		nextRootPath = s.resolveManagedInstructionsRoot(agent)
	} else {
		if input.RootPath != nil {
			nextRootPath = resolveHomeAwarePath(*input.RootPath)
		} else if state.rootPath != nil {
			nextRootPath = *state.rootPath
		}
		if nextRootPath == "" || !filepath.IsAbs(nextRootPath) {
			return nil, nil, errors.New("external instructions bundles require an absolute rootPath")
		}
	}

	// Ensure directory exists
	if err := os.MkdirAll(nextRootPath, 0755); err != nil {
		return nil, nil, fmt.Errorf("failed to create bundle root: %w", err)
	}

	// Export existing files if new directory is empty
	existingFiles, _ := s.listFilesRecursive(nextRootPath)
	if len(existingFiles) == 0 {
		exported, entryFile, _ := s.ExportFiles(agent)
		for relPath, content := range exported {
			absPath := filepath.Join(nextRootPath, relPath)
			_ = os.MkdirAll(filepath.Dir(absPath), 0755)
			_ = os.WriteFile(absPath, []byte(content), 0644)
		}
		if nextEntryFile == "" && entryFile != "" {
			nextEntryFile = entryFile
		}
	}

	// Ensure entry file exists
	refreshedFiles, _ := s.listFilesRecursive(nextRootPath)
	hasEntryFile := false
	for _, f := range refreshedFiles {
		if f == nextEntryFile {
			hasEntryFile = true
			break
		}
	}
	if !hasEntryFile {
		entryPath := filepath.Join(nextRootPath, nextEntryFile)
		_ = os.MkdirAll(filepath.Dir(entryPath), 0755)
		_ = os.WriteFile(entryPath, []byte(""), 0644)
	}

	// Build new config
	adapterConfig := s.applyBundleConfig(state.config, nextMode, nextRootPath, nextEntryFile, input.ClearLegacyPromptTemplate)
	updatedAgent := *agent
	if err := setAdapterConfig(&updatedAgent, adapterConfig); err != nil {
		return nil, nil, err
	}
	bundle, err := s.GetBundle(&updatedAgent)
	if err != nil {
		return nil, nil, err
	}
	return bundle, adapterConfig, nil
}

// ExportFiles exports all instruction files from the bundle.
func (s *AgentInstructionsService) ExportFiles(agent *models.Agent) (map[string]string, string, []string) {
	state := s.deriveBundleState(agent)
	state = s.recoverManagedBundleState(agent, state)

	if state.rootPath != nil {
		info, err := os.Stat(*state.rootPath)
		if err == nil && info.IsDir() {
			files, err := s.listFilesRecursive(*state.rootPath)
			if err == nil && len(files) > 0 {
				exported := make(map[string]string)
				for _, relPath := range files {
					absPath := filepath.Join(*state.rootPath, relPath)
					content, err := os.ReadFile(absPath)
					if err == nil {
						exported[relPath] = string(content)
					}
				}
				if len(exported) > 0 {
					return exported, state.entryFile, state.warnings
				}
			}
		}
	}

	// Fall back to legacy instructions
	legacyBody, _ := s.readLegacyInstructions(agent, state.config)
	if legacyBody == "" {
		legacyBody = "_No AGENTS instructions were resolved from current agent config._"
	}
	return map[string]string{state.entryFile: legacyBody}, state.entryFile, state.warnings
}

// SyncInstructionsBundleConfigFromFilePath syncs the bundle config from an instructions file path.
func SyncInstructionsBundleConfigFromFilePath(agent *models.Agent, adapterConfig map[string]interface{}, instanceRoot string) map[string]interface{} {
	instructionsFilePath := asString(adapterConfig[FileKey])
	next := copyConfig(adapterConfig)

	if instructionsFilePath == nil {
		delete(next, ModeKey)
		delete(next, RootKey)
		delete(next, EntryKey)
		return next
	}

	resolvedPath := resolveLegacyInstructionsPath(*instructionsFilePath, adapterConfig)
	rootPath := filepath.Dir(resolvedPath)
	entryFile := filepath.Base(resolvedPath)

	svc := &AgentInstructionsService{InstanceRoot: instanceRoot}
	managedRoot := svc.resolveManagedInstructionsRoot(agent)
	mode := BundleModeExternal
	if strings.HasPrefix(resolvedPath, managedRoot+string(filepath.Separator)) || resolvedPath == filepath.Join(managedRoot, entryFile) {
		mode = BundleModeManaged
	}

	return svc.applyBundleConfig(next, mode, rootPath, entryFile, false)
}

// Internal helper methods

func (s *AgentInstructionsService) resolveManagedInstructionsRoot(agent *models.Agent) string {
	return filepath.Join(s.InstanceRoot, "companies", agent.CompanyID, "agents", agent.ID, "instructions")
}

func (s *AgentInstructionsService) deriveBundleState(agent *models.Agent) *bundleState {
	config := jsonAsRecord(agent.AdapterConfig)
	warnings := make([]string, 0)

	storedModeRaw := config[ModeKey]
	storedRootRaw := asString(config[RootKey])
	legacyInstructionsPath := asString(config[FileKey])

	var mode *string
	if m, ok := storedModeRaw.(string); ok && (m == BundleModeManaged || m == BundleModeExternal) {
		mode = &m
	}

	var rootPath *string
	if storedRootRaw != nil {
		resolved := resolveHomeAwarePath(*storedRootRaw)
		rootPath = &resolved
	}

	entryFile := EntryFileDefault
	if storedEntryRaw := asString(config[EntryKey]); storedEntryRaw != nil {
		entryFile = normalizeRelativeFilePath(*storedEntryRaw)
	}

	// Handle legacy instructions file path
	if rootPath == nil && legacyInstructionsPath != nil {
		resolvedLegacyPath := resolveLegacyInstructionsPath(*legacyInstructionsPath, config)
		dir := filepath.Dir(resolvedLegacyPath)
		rootPath = &dir
		entryFile = filepath.Base(resolvedLegacyPath)

		managedRoot := s.resolveManagedInstructionsRoot(agent)
		if strings.HasPrefix(resolvedLegacyPath, managedRoot+string(filepath.Separator)) ||
			resolvedLegacyPath == filepath.Join(managedRoot, entryFile) {
			m := BundleModeManaged
			mode = &m
		} else {
			m := BundleModeExternal
			mode = &m
		}

		if !filepath.IsAbs(*legacyInstructionsPath) {
			warnings = append(warnings, "Using legacy relative instructionsFilePath; migrate this agent to a managed or absolute external bundle.")
		}
	}

	var resolvedEntryPath *string
	if rootPath != nil {
		p := filepath.Join(*rootPath, entryFile)
		resolvedEntryPath = &p
	}

	return &bundleState{
		config:                              config,
		mode:                                mode,
		rootPath:                            rootPath,
		entryFile:                           entryFile,
		resolvedEntryPath:                   resolvedEntryPath,
		warnings:                            warnings,
		legacyPromptTemplateActive:          asString(config[PromptKey]) != nil,
		legacyBootstrapPromptTemplateActive: asString(config[BootstrapPromptKey]) != nil,
	}
}

func (s *AgentInstructionsService) recoverManagedBundleState(agent *models.Agent, state *bundleState) *bundleState {
	managedRootPath := s.resolveManagedInstructionsRoot(agent)

	info, err := os.Stat(managedRootPath)
	if err != nil || !info.IsDir() {
		return state
	}

	files, err := s.listFilesRecursive(managedRootPath)
	if err != nil || len(files) == 0 {
		return state
	}

	recoveredEntryFile := state.entryFile
	hasEntry := false
	hasDefault := false
	for _, f := range files {
		if f == state.entryFile {
			hasEntry = true
		}
		if f == EntryFileDefault {
			hasDefault = true
		}
	}
	if !hasEntry {
		if hasDefault {
			recoveredEntryFile = EntryFileDefault
		} else if len(files) > 0 {
			recoveredEntryFile = files[0]
		}
	}

	if state.rootPath == nil {
		m := BundleModeManaged
		resolvedPath := filepath.Join(managedRootPath, recoveredEntryFile)
		return &bundleState{
			config:                              state.config,
			mode:                                &m,
			rootPath:                            &managedRootPath,
			entryFile:                           recoveredEntryFile,
			resolvedEntryPath:                   &resolvedPath,
			warnings:                            state.warnings,
			legacyPromptTemplateActive:          state.legacyPromptTemplateActive,
			legacyBootstrapPromptTemplateActive: state.legacyBootstrapPromptTemplateActive,
		}
	}

	if state.mode != nil && *state.mode == BundleModeExternal {
		return state
	}

	return state
}

func (s *AgentInstructionsService) toBundle(agent *models.Agent, state *bundleState, files []AgentInstructionsFileSummary) *AgentInstructionsBundle {
	nextFiles := make([]AgentInstructionsFileSummary, 0, len(files)+1)
	nextFiles = append(nextFiles, files...)

	// Add legacy prompt template as a virtual file if active
	if state.legacyPromptTemplateActive {
		hasLegacy := false
		for _, f := range nextFiles {
			if f.Path == LegacyPromptTemplatePath {
				hasLegacy = true
				break
			}
		}
		if !hasLegacy {
			legacyContent := asString(state.config[PromptKey])
			size := int64(0)
			if legacyContent != nil {
				size = int64(len(*legacyContent))
			}
			nextFiles = append(nextFiles, AgentInstructionsFileSummary{
				Path:        LegacyPromptTemplatePath,
				Size:        size,
				Language:    "markdown",
				Markdown:    true,
				IsEntryFile: false,
				Editable:    true,
				Deprecated:  true,
				Virtual:     true,
			})
		}
	}

	// Sort files by path
	sort.Slice(nextFiles, func(i, j int) bool {
		return nextFiles[i].Path < nextFiles[j].Path
	})

	return &AgentInstructionsBundle{
		AgentID:                             agent.ID,
		CompanyID:                           agent.CompanyID,
		Mode:                                state.mode,
		RootPath:                            state.rootPath,
		ManagedRootPath:                     s.resolveManagedInstructionsRoot(agent),
		EntryFile:                           state.entryFile,
		ResolvedEntryPath:                   state.resolvedEntryPath,
		Editable:                            state.rootPath != nil,
		Warnings:                            state.warnings,
		LegacyPromptTemplateActive:          state.legacyPromptTemplateActive,
		LegacyBootstrapPromptTemplateActive: state.legacyBootstrapPromptTemplateActive,
		Files:                               nextFiles,
	}
}

func (s *AgentInstructionsService) ensureWritableBundle(agent *models.Agent, clearLegacy bool) (map[string]interface{}, *bundleState, error) {
	derived := s.deriveBundleState(agent)
	current := s.recoverManagedBundleState(agent, derived)

	if current.rootPath != nil && current.mode != nil {
		adapterConfig := s.buildPersistedBundleConfig(derived, current, clearLegacy)
		newState := s.deriveBundleState(&models.Agent{
			ID:            agent.ID,
			CompanyID:     agent.CompanyID,
			AdapterConfig: mustMarshalJSON(adapterConfig),
		})
		return adapterConfig, newState, nil
	}

	managedRoot := s.resolveManagedInstructionsRoot(agent)
	entryFile := current.entryFile
	if entryFile == "" {
		entryFile = EntryFileDefault
	}

	nextConfig := s.applyBundleConfig(current.config, BundleModeManaged, managedRoot, entryFile, clearLegacy)
	if err := os.MkdirAll(managedRoot, 0755); err != nil {
		return nil, nil, fmt.Errorf("failed to create managed root: %w", err)
	}

	entryPath := filepath.Join(managedRoot, entryFile)
	if _, err := os.Stat(entryPath); os.IsNotExist(err) {
		legacyInstructions, _ := s.readLegacyInstructions(agent, current.config)
		if strings.TrimSpace(legacyInstructions) != "" {
			_ = os.MkdirAll(filepath.Dir(entryPath), 0755)
			_ = os.WriteFile(entryPath, []byte(legacyInstructions), 0644)
		}
	}

	newState := s.deriveBundleState(&models.Agent{
		ID:            agent.ID,
		CompanyID:     agent.CompanyID,
		AdapterConfig: mustMarshalJSON(nextConfig),
	})
	return nextConfig, newState, nil
}

func (s *AgentInstructionsService) buildPersistedBundleConfig(derived, current *bundleState, clearLegacy bool) map[string]interface{} {
	if derived.rootPath != nil && current.rootPath != nil &&
		derived.mode != nil && current.mode != nil &&
		*derived.mode == *current.mode &&
		filepath.Clean(*derived.rootPath) == filepath.Clean(*current.rootPath) &&
		derived.entryFile == current.entryFile &&
		!clearLegacy {
		return current.config
	}

	if current.rootPath == nil || current.mode == nil {
		return current.config
	}

	return s.applyBundleConfig(current.config, *current.mode, *current.rootPath, current.entryFile, clearLegacy)
}

func (s *AgentInstructionsService) applyBundleConfig(config map[string]interface{}, mode, rootPath, entryFile string, clearLegacy bool) map[string]interface{} {
	next := copyConfig(config)
	next[ModeKey] = mode
	next[RootKey] = rootPath
	next[EntryKey] = entryFile
	next[FileKey] = filepath.Join(rootPath, entryFile)

	if clearLegacy {
		delete(next, PromptKey)
		delete(next, BootstrapPromptKey)
	}

	return next
}

func (s *AgentInstructionsService) listFilesRecursive(rootPath string) ([]string, error) {
	var output []string

	err := filepath.Walk(rootPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			// Skip permission errors and other file system errors gracefully.
			// These are expected in some environments (e.g., scanning workspace roots).
			return nil
		}

		name := info.Name()
		if name == "." || name == ".." {
			return nil
		}

		if info.IsDir() {
			if ignoredDirNames[name] {
				return filepath.SkipDir
			}
			return nil
		}

		if ignoredFileNames[name] || strings.HasPrefix(name, "._") ||
			strings.HasSuffix(name, ".pyc") || strings.HasSuffix(name, ".pyo") {
			return nil
		}

		relPath, err := filepath.Rel(rootPath, path)
		if err != nil {
			return nil
		}

		// Normalize to forward slashes
		relPath = filepath.ToSlash(relPath)
		output = append(output, relPath)
		return nil
	})

	if err != nil {
		return nil, err
	}

	sort.Strings(output)
	return output, nil
}

func (s *AgentInstructionsService) readFileSummary(rootPath, relativePath, entryFile string) (AgentInstructionsFileSummary, error) {
	absPath := filepath.Join(rootPath, relativePath)
	info, err := os.Stat(absPath)
	if err != nil {
		return AgentInstructionsFileSummary{}, err
	}

	return AgentInstructionsFileSummary{
		Path:        relativePath,
		Size:        info.Size(),
		Language:    inferLanguage(relativePath),
		Markdown:    isMarkdown(relativePath),
		IsEntryFile: relativePath == entryFile,
		Editable:    true,
		Deprecated:  false,
		Virtual:     false,
	}, nil
}

func (s *AgentInstructionsService) readLegacyInstructions(agent *models.Agent, config map[string]interface{}) (string, error) {
	instructionsFilePath := asString(config[FileKey])
	if instructionsFilePath != nil {
		resolvedPath := resolveLegacyInstructionsPath(*instructionsFilePath, config)
		content, err := os.ReadFile(resolvedPath)
		if err == nil {
			return string(content), nil
		}
	}

	if pt := asString(config[PromptKey]); pt != nil {
		return *pt, nil
	}

	return "", nil
}

func (s *AgentInstructionsService) resolvePathWithinRoot(rootPath, relativePath string) (string, error) {
	normalizedRelPath := normalizeRelativeFilePath(relativePath)
	absRoot, _ := filepath.Abs(rootPath)
	absPath := filepath.Join(absRoot, normalizedRelPath)

	relToRoot, err := filepath.Rel(absRoot, absPath)
	if err != nil {
		return "", errors.New("instructions file path must stay within the bundle root")
	}
	if relToRoot == ".." || strings.HasPrefix(relToRoot, ".."+string(filepath.Separator)) {
		return "", errors.New("instructions file path must stay within the bundle root")
	}

	return absPath, nil
}

// Helper functions

func normalizeRelativeFilePath(candidatePath string) string {
	normalized := filepath.ToSlash(candidatePath)
	normalized = filepath.Clean(normalized)
	normalized = strings.TrimPrefix(normalized, "/")
	if normalized == "" || normalized == "." || normalized == ".." || strings.HasPrefix(normalized, "../") {
		return ""
	}
	return normalized
}

func inferLanguage(relativePath string) string {
	lower := strings.ToLower(relativePath)
	switch {
	case strings.HasSuffix(lower, ".md"):
		return "markdown"
	case strings.HasSuffix(lower, ".json"):
		return "json"
	case strings.HasSuffix(lower, ".yaml"), strings.HasSuffix(lower, ".yml"):
		return "yaml"
	case strings.HasSuffix(lower, ".ts"), strings.HasSuffix(lower, ".tsx"):
		return "typescript"
	case strings.HasSuffix(lower, ".js"), strings.HasSuffix(lower, ".jsx"),
		strings.HasSuffix(lower, ".mjs"), strings.HasSuffix(lower, ".cjs"):
		return "javascript"
	case strings.HasSuffix(lower, ".sh"):
		return "bash"
	case strings.HasSuffix(lower, ".py"):
		return "python"
	case strings.HasSuffix(lower, ".toml"):
		return "toml"
	case strings.HasSuffix(lower, ".txt"):
		return "text"
	default:
		return "text"
	}
}

func isMarkdown(relativePath string) bool {
	return strings.HasSuffix(strings.ToLower(relativePath), ".md")
}

func resolveHomeAwarePath(path string) string {
	if strings.HasPrefix(path, "~") {
		home, err := os.UserHomeDir()
		if err == nil {
			return filepath.Join(home, path[1:])
		}
	}
	return path
}

func resolveLegacyInstructionsPath(candidatePath string, config map[string]interface{}) string {
	if filepath.IsAbs(candidatePath) {
		return candidatePath
	}
	cwd := asString(config["cwd"])
	if cwd == nil || !filepath.IsAbs(*cwd) {
		return candidatePath
	}
	return filepath.Join(*cwd, candidatePath)
}

func asString(value interface{}) *string {
	if s, ok := value.(string); ok {
		trimmed := strings.TrimSpace(s)
		if trimmed != "" {
			return &trimmed
		}
	}
	return nil
}

func jsonAsRecord(data []byte) map[string]interface{} {
	if len(data) == 0 {
		return make(map[string]interface{})
	}
	var result map[string]interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		return make(map[string]interface{})
	}
	if result == nil {
		return make(map[string]interface{})
	}
	return result
}

func copyConfig(config map[string]interface{}) map[string]interface{} {
	copy := make(map[string]interface{})
	for k, v := range config {
		copy[k] = v
	}
	return copy
}

func setAdapterConfig(agent *models.Agent, config map[string]interface{}) error {
	data, err := json.Marshal(config)
	if err != nil {
		return err
	}
	agent.AdapterConfig = data
	return nil
}

func mustMarshalJSON(v interface{}) []byte {
	data, _ := json.Marshal(v)
	return data
}
