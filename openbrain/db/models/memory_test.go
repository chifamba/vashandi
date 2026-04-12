package models

import (
	"testing"
	"time"
)

func TestMemoryStruct(t *testing.T) {
	mem := Memory{
		ID:          "mem-123",
		NamespaceID: "ns-123",
		Text:        "hello world",
		CreatedAt:   time.Now(),
	}

	if mem.NamespaceID != "ns-123" {
		t.Errorf("expected NamespaceID to be ns-123, got %s", mem.NamespaceID)
	}
}

func TestNamespaceStruct(t *testing.T) {
	ns := Namespace{
		ID:        "ns-123",
		CompanyID: "co-123",
	}

	if ns.CompanyID != "co-123" {
		t.Errorf("expected CompanyID to be co-123, got %s", ns.CompanyID)
	}
}
