package services

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestPluginEventBus_MatchesPattern(t *testing.T) {
	tests := []struct {
		eventType string
		pattern   string
		want      bool
	}{
		{"issue.created", "issue.created", true},
		{"issue.created", "issue.updated", false},
		{"plugin.acme.linear.sync", "plugin.acme.linear.sync", true},
		{"plugin.acme.linear.sync", "plugin.acme.linear.*", true},
		{"plugin.acme.linear.sync", "plugin.*", true},
		{"plugin.acme.linear.sync", "plugin.acme.*", true},
		{"plugin.acme.linear.sync", "plugin.other.*", false},
		{"issue.created", "plugin.*", false},
	}

	for _, tt := range tests {
		if got := matchesPattern(tt.eventType, tt.pattern); got != tt.want {
			t.Errorf("matchesPattern(%q, %q) = %v, want %v", tt.eventType, tt.pattern, got, tt.want)
		}
	}
}

func TestPluginEventBus_PassesFilter(t *testing.T) {
	project1 := "proj-1"
	project2 := "proj-2"
	company1 := "comp-1"
	agent1 := "agent-1"

	tests := []struct {
		name   string
		event  PluginEvent
		filter *EventFilter
		want   bool
	}{
		{
			"no filter",
			PluginEvent{CompanyID: company1},
			nil,
			true,
		},
		{
			"company match",
			PluginEvent{CompanyID: company1},
			&EventFilter{CompanyID: &company1},
			true,
		},
		{
			"company mismatch",
			PluginEvent{CompanyID: "other"},
			&EventFilter{CompanyID: &company1},
			false,
		},
		{
			"project match (entity)",
			PluginEvent{EntityType: "project", EntityID: project1},
			&EventFilter{ProjectID: &project1},
			true,
		},
		{
			"project match (payload)",
			PluginEvent{Payload: map[string]interface{}{"projectId": project1}},
			&EventFilter{ProjectID: &project1},
			true,
		},
		{
			"project mismatch",
			PluginEvent{EntityID: project2, EntityType: "project"},
			&EventFilter{ProjectID: &project1},
			false,
		},
		{
			"agent match (entity)",
			PluginEvent{EntityType: "agent", EntityID: agent1},
			&EventFilter{AgentID: &agent1},
			true,
		},
		{
			"multiple filters (AND)",
			PluginEvent{CompanyID: company1, EntityType: "project", EntityID: project1},
			&EventFilter{CompanyID: &company1, ProjectID: &project1},
			true,
		},
		{
			"multiple filters mismatch",
			PluginEvent{CompanyID: company1, EntityType: "project", EntityID: project2},
			&EventFilter{CompanyID: &company1, ProjectID: &project1},
			false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := passesFilter(tt.event, tt.filter); got != tt.want {
				t.Errorf("passesFilter() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestPluginEventBus_PublishSubscribe(t *testing.T) {
	bus := NewPluginEventBus()
	ctx := context.Background()

	var wg sync.WaitGroup
	wg.Add(2)

	received := make([]PluginEvent, 0)
	var mu sync.Mutex

	// Subscription 1: Exact match
	bus.Subscribe("plugin-1", "issue.created", nil, func(event PluginEvent) {
		mu.Lock()
		received = append(received, event)
		mu.Unlock()
		wg.Done()
	})

	// Subscription 2: Wildcard match
	bus.Subscribe("plugin-2", "plugin.foo.*", nil, func(event PluginEvent) {
		mu.Lock()
		received = append(received, event)
		mu.Unlock()
		wg.Done()
	})

	// Publish 1: Should match sub 1
	bus.Publish(ctx, PluginEvent{EventType: "issue.created", CompanyID: "comp-1"})

	// Publish 2: Should match sub 2
	bus.Publish(ctx, PluginEvent{EventType: "plugin.foo.bar", CompanyID: "comp-1"})

	// Publish 3: Should match nothing
	bus.Publish(ctx, PluginEvent{EventType: "other.event", CompanyID: "comp-1"})

	// Wait for delivery (goroutines)
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(1 * time.Second):
		t.Fatal("timed out waiting for events")
	}

	if len(received) != 2 {
		t.Errorf("expected 2 received events, got %d", len(received))
	}
}
