package services

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

// PluginEvent mirrors the TypeScript PluginEvent interface.
type PluginEvent struct {
	EventID    string      `json:"eventId"`
	EventType  string      `json:"eventType"`
	CompanyID  string      `json:"companyId"`
	OccurredAt string      `json:"occurredAt"`
	ActorType  string      `json:"actorType"` // "user", "agent", "plugin", "system"
	ActorID    string      `json:"actorId"`
	EntityID   string      `json:"entityId,omitempty"`
	EntityType string      `json:"entityType,omitempty"`
	Payload    interface{} `json:"payload"`
}

// EventFilter mirrors the TypeScript EventFilter interface.
type EventFilter struct {
	ProjectID *string `json:"projectId,omitempty"`
	CompanyID *string `json:"companyId,omitempty"`
	AgentID   *string `json:"agentId,omitempty"`
}

type subscription struct {
	id      string
	pattern string
	filter  *EventFilter
	handler func(event PluginEvent)
}

type PluginEventBus struct {
	mu        sync.RWMutex
	registry  map[string][]subscription // pluginID -> subscriptions
}

func NewPluginEventBus() *PluginEventBus {
	return &PluginEventBus{
		registry: make(map[string][]subscription),
	}
}

// Publish emits an event to all matching subscribers across all plugins.
func (b *PluginEventBus) Publish(ctx context.Context, event PluginEvent) {
	type delivery struct {
		pluginID string
		sub      subscription
	}

	b.mu.RLock()
	deliveries := make([]delivery, 0)
	for pluginID, subs := range b.registry {
		for _, sub := range subs {
			if matchesPattern(event.EventType, sub.pattern) && passesFilter(event, sub.filter) {
				deliveries = append(deliveries, delivery{pluginID: pluginID, sub: sub})
			}
		}
	}
	b.mu.RUnlock()

	for _, delivery := range deliveries {
		if ctx != nil {
			select {
			case <-ctx.Done():
				return
			default:
			}
		}

		go func(pluginID string, sub subscription) {
			defer func() {
				if recovered := recover(); recovered != nil {
					slog.Error("[plugin-event-bus] subscription handler panicked",
						"pluginId", pluginID,
						"pattern", sub.pattern,
						"eventType", event.EventType,
						"panic", recovered,
					)
				}
			}()

			if ctx != nil {
				select {
				case <-ctx.Done():
					return
				default:
				}
			}

			sub.handler(event)
		}(delivery.pluginID, delivery.sub)
	}
}

// Subscribe adds a subscription for a plugin. Returns a cancel function.
func (b *PluginEventBus) Subscribe(pluginID string, pattern string, filter *EventFilter, handler func(event PluginEvent)) func() {
	b.mu.Lock()
	defer b.mu.Unlock()

	sub := subscription{
		id:      uuid.New().String(),
		pattern: pattern,
		filter:  filter,
		handler: handler,
	}

	b.registry[pluginID] = append(b.registry[pluginID], sub)

	// Return a closure to remove this specific subscription
	return func() {
		b.mu.Lock()
		defer b.mu.Unlock()
		subs := b.registry[pluginID]
		for i, s := range subs {
			if s.id == sub.id {
				// Remove by index
				b.registry[pluginID] = append(subs[:i], subs[i+1:]...)
				if len(b.registry[pluginID]) == 0 {
					delete(b.registry, pluginID)
				}
				break
			}
		}
	}
}

// Clear removes all subscriptions for a specific plugin.
func (b *PluginEventBus) Clear(pluginID string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	delete(b.registry, pluginID)
}

// ForPlugin returns a scoped handle for a specific plugin.
func (b *PluginEventBus) ForPlugin(pluginID string) *ScopedPluginEventBus {
	return &ScopedPluginEventBus{
		bus:      b,
		pluginID: pluginID,
	}
}

// ScopedPluginEventBus provides an isolated view of the bus for a plugin.
type ScopedPluginEventBus struct {
	bus      *PluginEventBus
	pluginID string
}

func (s *ScopedPluginEventBus) Subscribe(pattern string, filter *EventFilter, handler func(event PluginEvent)) func() {
	return s.bus.Subscribe(s.pluginID, pattern, filter, handler)
}

func (s *ScopedPluginEventBus) Emit(name string, companyId string, payload interface{}) {
	if name == "" || strings.TrimSpace(name) == "" {
		return
	}
	if strings.HasPrefix(name, "plugin.") {
		// Namespacing is handled automatically, plugins shouldn't provide the prefix.
		return
	}

	eventType := fmt.Sprintf("plugin.%s.%s", s.pluginID, name)
	event := PluginEvent{
		EventID:    uuid.New().String(),
		EventType:  eventType,
		CompanyID:  companyId,
		OccurredAt: time.Now().UTC().Format(time.RFC3339),
		ActorType:  "plugin",
		ActorID:    s.pluginID,
		Payload:    payload,
	}

	s.bus.Publish(context.Background(), event)
}

func (s *ScopedPluginEventBus) Clear() {
	s.bus.Clear(s.pluginID)
}

// --- Helpers ---

func matchesPattern(eventType, pattern string) bool {
	if eventType == pattern {
		return true
	}

	if strings.HasSuffix(pattern, ".*") {
		prefix := strings.TrimSuffix(pattern, "*") // keep the dot
		return strings.HasPrefix(eventType, prefix)
	}

	return false
}

func passesFilter(event PluginEvent, filter *EventFilter) bool {
	if filter == nil {
		return true
	}

	payload, _ := event.Payload.(map[string]interface{})

	if filter.ProjectID != nil {
		var projectID string
		if event.EntityType == "project" {
			projectID = event.EntityID
		} else if payload != nil {
			if pid, ok := payload["projectId"].(string); ok {
				projectID = pid
			}
		}
		if projectID != *filter.ProjectID {
			return false
		}
	}

	if filter.CompanyID != nil {
		if event.CompanyID != *filter.CompanyID {
			return false
		}
	}

	if filter.AgentID != nil {
		var agentID string
		if event.EntityType == "agent" {
			agentID = event.EntityID
		} else if payload != nil {
			if aid, ok := payload["agentId"].(string); ok {
				agentID = aid
			}
		}
		if agentID != *filter.AgentID {
			return false
		}
	}

	return true
}
