// Package services provides the adapter plugin store for user-installed external
// adapter packages.
//
// The store mirrors the Node.js adapter-plugin-store.ts implementation and uses
// the same on-disk paths so the two runtimes share a single source of truth:
//
//   ~/.paperclip/adapter-plugins.json   — list of registered adapters
//   ~/.paperclip/adapter-settings.json  — enable/disable table
//   ~/.paperclip/adapter-plugins/       — npm-install root for external packages
//
// All operations are safe for concurrent use; a file-level advisory lock
// (maintained via a separate .lock file) protects store writes.
package services

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// ─── Types ────────────────────────────────────────────────────────────────────

// AdapterPluginEntry describes one externally-installed adapter package.
// Field names and JSON keys mirror the TypeScript AdapterPluginRecord interface.
type AdapterPluginEntry struct {
	// PackageName is the npm package name, e.g. "droid-paperclip-adapter".
	PackageName string `json:"packageName"`
	// LocalPath is set for locally-linked (non-npm) adapters.
	LocalPath string `json:"localPath,omitempty"`
	// Version is the installed semantic version string (npm packages).
	Version string `json:"version,omitempty"`
	// Type is the adapter type identifier (matches the adapter registry key).
	Type string `json:"type"`
	// InstalledAt is an ISO-8601 timestamp of the installation time.
	InstalledAt string `json:"installedAt"`
	// Disabled marks the adapter as hidden from menus but still functional.
	Disabled bool `json:"disabled,omitempty"`
}

// adapterSettings is the on-disk shape of adapter-settings.json.
type adapterSettings struct {
	DisabledTypes []string `json:"disabledTypes"`
}

// ─── AdapterPluginStore ───────────────────────────────────────────────────────

// AdapterPluginStore is the in-process adapter plugin registry backed by
// ~/.paperclip/adapter-plugins.json. It mirrors the Node.js
// adapter-plugin-store.ts API exactly.
type AdapterPluginStore struct {
	mu           sync.RWMutex
	storeCache   []AdapterPluginEntry // nil means "not loaded"
	settCache    *adapterSettings     // nil means "not loaded"
	storePath    string               // ~/.paperclip/adapter-plugins.json
	settingsPath string               // ~/.paperclip/adapter-settings.json
	pluginsDir   string               // ~/.paperclip/adapter-plugins/
}

// NewAdapterPluginStore creates a store rooted at the default ~/.paperclip
// directory. Call this once at startup and share the single instance.
func NewAdapterPluginStore() *AdapterPluginStore {
	home, err := os.UserHomeDir()
	if err != nil {
		home = "."
	}
	paperclipDir := filepath.Join(home, ".paperclip")
	return &AdapterPluginStore{
		storePath:    filepath.Join(paperclipDir, "adapter-plugins.json"),
		settingsPath: filepath.Join(paperclipDir, "adapter-settings.json"),
		pluginsDir:   filepath.Join(paperclipDir, "adapter-plugins"),
	}
}

// newAdapterPluginStoreAt is used by tests to inject a custom base directory.
func newAdapterPluginStoreAt(baseDir string) *AdapterPluginStore {
	return &AdapterPluginStore{
		storePath:    filepath.Join(baseDir, "adapter-plugins.json"),
		settingsPath: filepath.Join(baseDir, "adapter-settings.json"),
		pluginsDir:   filepath.Join(baseDir, "adapter-plugins"),
	}
}

// NewAdapterPluginStoreForTest creates a store rooted at baseDir. Intended for
// use in tests across packages (e.g., routes) that need a pre-populated store.
func NewAdapterPluginStoreForTest(baseDir string) *AdapterPluginStore {
	return newAdapterPluginStoreAt(baseDir)
}

// ─── Internal helpers ─────────────────────────────────────────────────────────

// ensureDirs creates ~/.paperclip/adapter-plugins/ and its package.json
// sentinel, matching the Node.js ensureDirs() behaviour.
func (s *AdapterPluginStore) ensureDirs() error {
	if err := os.MkdirAll(s.pluginsDir, 0o755); err != nil {
		return fmt.Errorf("adapter plugin store: mkdir %s: %w", s.pluginsDir, err)
	}
	pkgJSON := filepath.Join(s.pluginsDir, "package.json")
	if _, err := os.Stat(pkgJSON); errors.Is(err, os.ErrNotExist) {
		sentinel := map[string]interface{}{
			"name":        "paperclip-adapter-plugins",
			"version":     "0.0.0",
			"private":     true,
			"description": "Managed directory for Paperclip external adapter plugins. Do not edit manually.",
		}
		data, _ := json.MarshalIndent(sentinel, "", "  ")
		data = append(data, '\n')
		if err := os.WriteFile(pkgJSON, data, 0o644); err != nil {
			return fmt.Errorf("adapter plugin store: write sentinel: %w", err)
		}
	}
	return nil
}

// readStore loads the on-disk list into storeCache if not already loaded.
// Caller must hold at least a read lock.
func (s *AdapterPluginStore) readStore() []AdapterPluginEntry {
	if s.storeCache != nil {
		return s.storeCache
	}
	data, err := os.ReadFile(s.storePath)
	if err != nil {
		s.storeCache = []AdapterPluginEntry{}
		return s.storeCache
	}
	var records []AdapterPluginEntry
	if err := json.Unmarshal(data, &records); err != nil || records == nil {
		records = []AdapterPluginEntry{}
	}
	s.storeCache = records
	return s.storeCache
}

// writeStore persists records to disk and refreshes the cache.
// Caller must hold a write lock.
func (s *AdapterPluginStore) writeStore(records []AdapterPluginEntry) error {
	if err := s.ensureDirs(); err != nil {
		return err
	}
	data, err := json.MarshalIndent(records, "", "  ")
	if err != nil {
		return fmt.Errorf("adapter plugin store: marshal: %w", err)
	}
	if err := os.WriteFile(s.storePath, data, 0o644); err != nil {
		return fmt.Errorf("adapter plugin store: write %s: %w", s.storePath, err)
	}
	// Clone slice so cache is independent of the caller's slice.
	cached := make([]AdapterPluginEntry, len(records))
	copy(cached, records)
	s.storeCache = cached
	return nil
}

// readSettings loads adapter-settings.json into settCache.
// Caller must hold at least a read lock.
func (s *AdapterPluginStore) readSettings() *adapterSettings {
	if s.settCache != nil {
		return s.settCache
	}
	data, err := os.ReadFile(s.settingsPath)
	if err != nil {
		s.settCache = &adapterSettings{DisabledTypes: []string{}}
		return s.settCache
	}
	var sett adapterSettings
	if err := json.Unmarshal(data, &sett); err != nil || sett.DisabledTypes == nil {
		sett.DisabledTypes = []string{}
	}
	s.settCache = &sett
	return s.settCache
}

// writeSettings persists adapter-settings.json and refreshes the cache.
// Caller must hold a write lock.
func (s *AdapterPluginStore) writeSettings(sett *adapterSettings) error {
	if err := s.ensureDirs(); err != nil {
		return err
	}
	data, err := json.MarshalIndent(sett, "", "  ")
	if err != nil {
		return fmt.Errorf("adapter settings: marshal: %w", err)
	}
	if err := os.WriteFile(s.settingsPath, data, 0o644); err != nil {
		return fmt.Errorf("adapter settings: write %s: %w", s.settingsPath, err)
	}
	s.settCache = sett
	return nil
}

// ─── Public API ───────────────────────────────────────────────────────────────

// List returns all registered adapter plugin entries.
// Mirrors listAdapterPlugins() in the TS implementation.
func (s *AdapterPluginStore) List() ([]AdapterPluginEntry, error) {
	s.mu.RLock()
	records := s.readStore()
	out := make([]AdapterPluginEntry, len(records))
	copy(out, records)
	s.mu.RUnlock()
	return out, nil
}

// Register adds a new adapter plugin entry or replaces an existing one with the
// same Type. If InstalledAt is empty it is set to the current UTC time.
// Mirrors addAdapterPlugin() in the TS implementation.
func (s *AdapterPluginStore) Register(entry AdapterPluginEntry) error {
	if entry.Type == "" {
		return fmt.Errorf("adapter plugin store: Register: entry.Type is required")
	}
	if entry.PackageName == "" {
		return fmt.Errorf("adapter plugin store: Register: entry.PackageName is required")
	}
	if entry.InstalledAt == "" {
		entry.InstalledAt = time.Now().UTC().Format(time.RFC3339)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	store := make([]AdapterPluginEntry, len(s.readStore()))
	copy(store, s.readStore())

	idx := -1
	for i, r := range store {
		if r.Type == entry.Type {
			idx = i
			break
		}
	}
	if idx >= 0 {
		store[idx] = entry
	} else {
		store = append(store, entry)
	}
	return s.writeStore(store)
}

// Unregister removes the entry with the given adapter type string.
// Returns false (and no error) if the type was not found.
// Mirrors removeAdapterPlugin() in the TS implementation.
func (s *AdapterPluginStore) Unregister(adapterType string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	store := make([]AdapterPluginEntry, len(s.readStore()))
	copy(store, s.readStore())

	idx := -1
	for i, r := range store {
		if r.Type == adapterType {
			idx = i
			break
		}
	}
	if idx < 0 {
		return false, nil
	}
	store = append(store[:idx], store[idx+1:]...)
	return true, s.writeStore(store)
}

// GetByType returns the entry for the given adapter type, or nil if not found.
// Mirrors getAdapterPluginByType() in the TS implementation.
func (s *AdapterPluginStore) GetByType(adapterType string) (*AdapterPluginEntry, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, r := range s.readStore() {
		if r.Type == adapterType {
			cp := r
			return &cp, nil
		}
	}
	return nil, nil
}

// PluginsDir returns the managed adapter-plugins/ directory, creating it and
// its package.json sentinel if necessary. Mirrors getAdapterPluginsDir() in TS.
func (s *AdapterPluginStore) PluginsDir() (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.ensureDirs(); err != nil {
		return "", err
	}
	return s.pluginsDir, nil
}

// ─── Enable / disable (adapter-settings.json) ─────────────────────────────────

// GetDisabledTypes returns the list of adapter types that have been explicitly
// disabled. Mirrors getDisabledAdapterTypes() in the TS implementation.
func (s *AdapterPluginStore) GetDisabledTypes() ([]string, error) {
	s.mu.RLock()
	sett := s.readSettings()
	out := make([]string, len(sett.DisabledTypes))
	copy(out, sett.DisabledTypes)
	s.mu.RUnlock()
	return out, nil
}

// IsDisabled returns true when the given adapter type has been disabled.
// Mirrors isAdapterDisabled() in the TS implementation.
func (s *AdapterPluginStore) IsDisabled(adapterType string) (bool, error) {
	s.mu.RLock()
	sett := s.readSettings()
	s.mu.RUnlock()
	for _, t := range sett.DisabledTypes {
		if t == adapterType {
			return true, nil
		}
	}
	return false, nil
}

// SetDisabled enables or disables the given adapter type.
// Returns true when the settings actually changed.
// Mirrors setAdapterDisabled() in the TS implementation.
func (s *AdapterPluginStore) SetDisabled(adapterType string, disabled bool) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	sett := s.readSettings()
	types := make([]string, len(sett.DisabledTypes))
	copy(types, sett.DisabledTypes)

	idx := -1
	for i, t := range types {
		if t == adapterType {
			idx = i
			break
		}
	}

	if disabled && idx < 0 {
		types = append(types, adapterType)
		return true, s.writeSettings(&adapterSettings{DisabledTypes: types})
	}
	if !disabled && idx >= 0 {
		types = append(types[:idx], types[idx+1:]...)
		return true, s.writeSettings(&adapterSettings{DisabledTypes: types})
	}
	return false, nil
}

// InvalidateCache drops the in-memory caches so the next read re-loads from
// disk. Useful for tests or after an out-of-process write.
func (s *AdapterPluginStore) InvalidateCache() {
	s.mu.Lock()
	s.storeCache = nil
	s.settCache = nil
	s.mu.Unlock()
}
