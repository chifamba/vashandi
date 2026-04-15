package services

import (
	"sync"
	"sync/atomic"
)

// StreamEventType represents the SSE event type for plugin streams.
type StreamEventType string

const (
	StreamEventMessage StreamEventType = "message"
	StreamEventOpen    StreamEventType = "open"
	StreamEventClose   StreamEventType = "close"
	StreamEventError   StreamEventType = "error"
)

// StreamSubscriber is a callback invoked when a stream event arrives.
type StreamSubscriber func(event interface{}, eventType StreamEventType)

// PluginStreamBus is an in-memory pub/sub bus for plugin SSE streams.
//
// Workers push events via JSON-RPC notifications (streams.open / streams.emit /
// streams.close). The bus fans those out to all SSE clients subscribed to the
// matching (pluginId, channel, companyId) tuple.
type PluginStreamBus struct {
	mu     sync.RWMutex
	nextID atomic.Int64
	subs   map[string]map[int64]StreamSubscriber // key → {id → subscriber}
}

// NewPluginStreamBus returns an initialised PluginStreamBus.
func NewPluginStreamBus() *PluginStreamBus {
	return &PluginStreamBus{
		subs: make(map[string]map[int64]StreamSubscriber),
	}
}

func pluginStreamKey(pluginID, channel, companyID string) string {
	return pluginID + ":" + channel + ":" + companyID
}

// Subscribe registers a listener for events on the given (pluginID, channel,
// companyID) tuple. It returns an unsubscribe function that the caller must
// invoke when the SSE connection closes.
func (b *PluginStreamBus) Subscribe(pluginID, channel, companyID string, listener StreamSubscriber) func() {
	key := pluginStreamKey(pluginID, channel, companyID)
	id := b.nextID.Add(1)

	b.mu.Lock()
	if b.subs[key] == nil {
		b.subs[key] = make(map[int64]StreamSubscriber)
	}
	b.subs[key][id] = listener
	b.mu.Unlock()

	return func() {
		b.mu.Lock()
		if subs, ok := b.subs[key]; ok {
			delete(subs, id)
			if len(subs) == 0 {
				delete(b.subs, key)
			}
		}
		b.mu.Unlock()
	}
}

// Publish delivers an event to all subscribers of the given tuple.
func (b *PluginStreamBus) Publish(pluginID, channel, companyID string, event interface{}, eventType StreamEventType) {
	key := pluginStreamKey(pluginID, channel, companyID)

	b.mu.RLock()
	subs := b.subs[key]
	listeners := make([]StreamSubscriber, 0, len(subs))
	for _, s := range subs {
		listeners = append(listeners, s)
	}
	b.mu.RUnlock()

	for _, l := range listeners {
		l(event, eventType)
	}
}
