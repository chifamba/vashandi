package services

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

// PluginDevReloader is the subset of the plugin lifecycle needed by the watcher.
// Callers should pass in a concrete implementation (e.g. the PluginWorkerManager).
type PluginDevReloader interface {
	// RestartWorker shuts down and restarts the worker process for the given pluginId.
	RestartWorker(pluginId string) error
}

// PluginDevWatch represents a single active directory watch for one plugin.
type pluginDevWatch struct {
	pluginId   string
	absPath    string
	timer      *time.Timer
	timerMu    sync.Mutex
}

func (w *pluginDevWatch) resetDebounce(debounce time.Duration, action func()) {
	w.timerMu.Lock()
	defer w.timerMu.Unlock()
	if w.timer != nil {
		w.timer.Stop()
	}
	w.timer = time.AfterFunc(debounce, action)
}

func (w *pluginDevWatch) cancel() {
	w.timerMu.Lock()
	defer w.timerMu.Unlock()
	if w.timer != nil {
		w.timer.Stop()
		w.timer = nil
	}
}

// pluginPackageJson is the minimal shape we care about in package.json.
type pluginPackageJson struct {
	PaperclipPlugin *struct {
		Manifest string `json:"manifest"`
		Worker   string `json:"worker"`
		UI       string `json:"ui"`
	} `json:"paperclipPlugin"`
}

const pluginDevDebounce = 500 * time.Millisecond

// shouldIgnorePath returns true for paths inside node_modules, .git, .vite,
// .paperclip-sdk, or any hidden directory — mirroring the TS implementation.
func pluginDevShouldIgnore(rel string) bool {
	rel = filepath.ToSlash(rel)
	for _, seg := range strings.Split(rel, "/") {
		if seg == "" {
			continue
		}
		if seg == "node_modules" || seg == ".git" || seg == ".vite" || seg == ".paperclip-sdk" {
			return true
		}
		if strings.HasPrefix(seg, ".") {
			return true
		}
	}
	return false
}

// resolvePluginWatchTargets returns the set of absolute paths to watch for a
// local-path plugin package.  It reads package.json to find the declared
// entrypoints (manifest, worker, ui); when none are declared it falls back to
// the dist/ subdirectory.  Paths that do not exist on disk are skipped.
func resolvePluginWatchTargets(packagePath string) []string {
	abs, err := filepath.Abs(packagePath)
	if err != nil {
		return nil
	}

	targets := map[string]struct{}{}

	addIfExists := func(p string) {
		p = filepath.Clean(p)
		if _, err := os.Stat(p); err == nil {
			targets[p] = struct{}{}
		}
	}

	// Always watch package.json itself.
	addIfExists(filepath.Join(abs, "package.json"))

	// Try to read package.json to discover entrypoints.
	data, err := os.ReadFile(filepath.Join(abs, "package.json"))
	if err != nil {
		// No package.json — nothing more to watch.
		var out []string
		for p := range targets {
			out = append(out, p)
		}
		return out
	}

	var pkgJSON pluginPackageJson
	_ = json.Unmarshal(data, &pkgJSON)

	entrypoints := []string{}
	if pkgJSON.PaperclipPlugin != nil {
		for _, ep := range []string{
			pkgJSON.PaperclipPlugin.Manifest,
			pkgJSON.PaperclipPlugin.Worker,
			pkgJSON.PaperclipPlugin.UI,
		} {
			if ep != "" {
				entrypoints = append(entrypoints, ep)
			}
		}
	}

	if len(entrypoints) == 0 {
		// Fall back: watch the dist/ directory recursively.
		addIfExists(filepath.Join(abs, "dist"))
	} else {
		for _, ep := range entrypoints {
			resolved := ep
			if !filepath.IsAbs(ep) {
				resolved = filepath.Join(abs, ep)
			}
			addIfExists(resolved)
		}
	}

	out := make([]string, 0, len(targets))
	for p := range targets {
		out = append(out, p)
	}
	return out
}

// PluginDevWatcher watches local-path plugin directories and restarts the
// plugin worker on file changes.  Only active in development mode.
type PluginDevWatcher struct {
	lifecycle PluginDevReloader

	watcher  *fsnotify.Watcher
	watches  map[string]*pluginDevWatch // pluginId -> watch state
	watchesMu sync.Mutex

	done chan struct{}
}

// NewPluginDevWatcher creates a PluginDevWatcher.  Call Start() to begin
// watching.
func NewPluginDevWatcher(lifecycle PluginDevReloader) (*PluginDevWatcher, error) {
	fw, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}
	return &PluginDevWatcher{
		lifecycle: lifecycle,
		watcher:   fw,
		watches:   make(map[string]*pluginDevWatch),
		done:      make(chan struct{}),
	}, nil
}

// Watch begins watching packagePath for the given pluginId.
// Calling Watch for an already-watched pluginId is a no-op.
func (d *PluginDevWatcher) Watch(pluginId, packagePath string) {
	d.watchesMu.Lock()
	defer d.watchesMu.Unlock()

	if _, exists := d.watches[pluginId]; exists {
		return // already watching
	}

	abs, err := filepath.Abs(packagePath)
	if err != nil || abs == "" {
		slog.Warn("plugin-dev-watcher: invalid package path",
			"pluginId", pluginId, "packagePath", packagePath)
		return
	}
	if _, err := os.Stat(abs); err != nil {
		slog.Warn("plugin-dev-watcher: package path does not exist, skipping",
			"pluginId", pluginId, "packagePath", abs)
		return
	}

	targets := resolvePluginWatchTargets(abs)
	if len(targets) == 0 {
		slog.Warn("plugin-dev-watcher: no watch targets found, skipping",
			"pluginId", pluginId, "packagePath", abs)
		return
	}

	added := []string{}
	for _, t := range targets {
		if err := d.watcher.Add(t); err != nil {
			slog.Warn("plugin-dev-watcher: could not watch path",
				"pluginId", pluginId, "path", t, "error", err)
		} else {
			added = append(added, t)
		}
	}

	if len(added) == 0 {
		return
	}

	d.watches[pluginId] = &pluginDevWatch{
		pluginId: pluginId,
		absPath:  abs,
	}
	slog.Info("plugin-dev-watcher: watching local plugin",
		"pluginId", pluginId, "packagePath", abs, "watchTargets", added)
}

// Unwatch stops watching the directory for the given pluginId.
func (d *PluginDevWatcher) Unwatch(pluginId string) {
	d.watchesMu.Lock()
	w, exists := d.watches[pluginId]
	if exists {
		delete(d.watches, pluginId)
	}
	d.watchesMu.Unlock()

	if w != nil {
		w.cancel()
	}
}

// Close shuts down all watchers.
func (d *PluginDevWatcher) Close() {
	close(d.done)
	_ = d.watcher.Close()

	d.watchesMu.Lock()
	defer d.watchesMu.Unlock()
	for _, w := range d.watches {
		w.cancel()
	}
}

// findPluginForPath returns the pluginId whose absPath is a prefix of the
// changed file, or "" if no match.
func (d *PluginDevWatcher) findPluginForPath(changedPath string) string {
	d.watchesMu.Lock()
	defer d.watchesMu.Unlock()
	for id, w := range d.watches {
		rel, err := filepath.Rel(w.absPath, changedPath)
		if err == nil && !strings.HasPrefix(rel, "..") {
			return id
		}
		// Direct file match (package.json etc.)
		if filepath.Clean(changedPath) == filepath.Clean(w.absPath) {
			return id
		}
	}
	return ""
}

// run is the event loop; it should be called in a goroutine.
func (d *PluginDevWatcher) run() {
	for {
		select {
		case <-d.done:
			return
		case err, ok := <-d.watcher.Errors:
			if !ok {
				return
			}
			slog.Warn("plugin-dev-watcher: watcher error", "error", err)
		case event, ok := <-d.watcher.Events:
			if !ok {
				return
			}

			changedPath := filepath.Clean(event.Name)

			// Compute relative path for ignore check
			pluginId := d.findPluginForPath(changedPath)
			if pluginId == "" {
				continue
			}

			d.watchesMu.Lock()
			w := d.watches[pluginId]
			d.watchesMu.Unlock()
			if w == nil {
				continue
			}

			rel, _ := filepath.Rel(w.absPath, changedPath)
			if pluginDevShouldIgnore(rel) {
				continue
			}

			w.resetDebounce(pluginDevDebounce, func() {
				slog.Info("plugin-dev-watcher: file change detected, restarting worker",
					"pluginId", pluginId,
					"changedFile", rel,
				)
				if err := d.lifecycle.RestartWorker(pluginId); err != nil {
					slog.Warn("plugin-dev-watcher: failed to restart worker",
						"pluginId", pluginId, "error", err)
				}
			})
		}
	}
}

// StartPluginDevWatcher creates and starts a PluginDevWatcher that monitors
// the given pluginDirs (map of pluginId -> packagePath).  It is a no-op if
// PLUGIN_DEV_WATCH is not set to "true" or "1".
//
// The watcher is stopped when ctx is cancelled.
func StartPluginDevWatcher(ctx context.Context, pluginDirs map[string]string, lifecycle PluginDevReloader) {
	devWatch := strings.TrimSpace(os.Getenv("PLUGIN_DEV_WATCH"))
	if devWatch != "true" && devWatch != "1" {
		return
	}
	if len(pluginDirs) == 0 {
		slog.Info("plugin-dev-watcher: no local-path plugins to watch")
		return
	}

	dw, err := NewPluginDevWatcher(lifecycle)
	if err != nil {
		slog.Error("plugin-dev-watcher: failed to create watcher", "error", err)
		return
	}

	for id, path := range pluginDirs {
		dw.Watch(id, path)
	}

	go dw.run()

	go func() {
		<-ctx.Done()
		dw.Close()
		slog.Info("plugin-dev-watcher: stopped")
	}()

	slog.Info("plugin-dev-watcher: started", "plugins", len(pluginDirs))
}
