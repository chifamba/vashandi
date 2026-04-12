package redis

import "fmt"

// KeyBuilder helps construct namespaced Redis keys
type KeyBuilder struct {
	NamespaceID string
}

// NewKeyBuilder creates a new KeyBuilder for a namespace
func NewKeyBuilder(namespaceID string) *KeyBuilder {
	return &KeyBuilder{
		NamespaceID: namespaceID,
	}
}

// Build returns a fully prefixed key
func (b *KeyBuilder) Build(key string) string {
	return fmt.Sprintf("namespace:%s:%s", b.NamespaceID, key)
}
