package services

import (
	"fmt"
	"sync"
)

type StreamEventType string

const (
	StreamEventMessage StreamEventType = "message"
	StreamEventOpen    StreamEventType = "open"
	StreamEventClose   StreamEventType = "close"
	StreamEventError   StreamEventType = "error"
)

type StreamEvent struct {
	Type    StreamEventType `json:"type"`
	Data    interface{}     `json:"data"`
	Channel string          `json:"channel"`
}

type streamSubscriber chan StreamEvent

type PluginStreamBus struct {
	subscribers map[string]map[streamSubscriber]struct{}
	mu          sync.RWMutex
}

func NewPluginStreamBus() *PluginStreamBus {
	return &PluginStreamBus{
		subscribers: make(map[string]map[streamSubscriber]struct{}),
	}
}

func (b *PluginStreamBus) streamKey(pluginID, channel, companyID string) string {
	return fmt.Sprintf("%s:%s:%s", pluginID, channel, companyID)
}

func (b *PluginStreamBus) Subscribe(pluginID, channel, companyID string) (<-chan StreamEvent, func()) {
	key := b.streamKey(pluginID, channel, companyID)
	ch := make(streamSubscriber, 10) // Buffered to prevent blocking

	b.mu.Lock()
	defer b.mu.Unlock()

	if b.subscribers[key] == nil {
		b.subscribers[key] = make(map[streamSubscriber]struct{})
	}
	b.subscribers[key][ch] = struct{}{}

	cancel := func() {
		b.mu.Lock()
		defer b.mu.Unlock()
		if subs, ok := b.subscribers[key]; ok {
			delete(subs, ch)
			if len(subs) == 0 {
				delete(b.subscribers, key)
			}
		}
		close(ch)
	}

	return ch, cancel
}

func (b *PluginStreamBus) Publish(pluginID, channel, companyID string, data interface{}, eventType StreamEventType) {
	if eventType == "" {
		eventType = StreamEventMessage
	}

	key := b.streamKey(pluginID, channel, companyID)
	event := StreamEvent{
		Type:    eventType,
		Data:    data,
		Channel: channel,
	}

	b.mu.RLock()
	subs, ok := b.subscribers[key]
	if !ok || len(subs) == 0 {
		b.mu.RUnlock()
		return
	}

	// Copy subscribers list while holding RLock to avoid long-holding during sends
	targets := make([]streamSubscriber, 0, len(subs))
	for ch := range subs {
		targets = append(targets, ch)
	}
	b.mu.RUnlock()

	for _, ch := range targets {
		select {
		case ch <- event:
		default:
			// Subscriber slow or buffer full, skip to avoid blocking the whole bus
		}
	}
}
