package redis

import "testing"

func TestKeyBuilder(t *testing.T) {
	kb := NewKeyBuilder("comp-123")
	key := kb.Build("rate_limit:agent-1")
	expected := "namespace:comp-123:rate_limit:agent-1"

	if key != expected {
		t.Errorf("expected %s, got %s", expected, key)
	}
}
