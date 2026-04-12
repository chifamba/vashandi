package jobs

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/chifamba/vashandi/openbrain/internal/brain"
)

func TestHandleCuratorProposal(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	service := brain.NewService(db)
	require.NoError(t, service.AutoMigrate())
	_, err = service.CreateMemory(t.Context(), brain.Actor{Kind: "service", NamespaceID: "ns1", TrustTier: 4}, brain.MemoryPayload{NamespaceID: "ns1", EntityType: "fact", Text: "fact 1", Tier: 1})
	require.NoError(t, err)
	_, err = service.CreateMemory(t.Context(), brain.Actor{Kind: "service", NamespaceID: "ns1", TrustTier: 4}, brain.MemoryPayload{NamespaceID: "ns1", EntityType: "fact", Text: "fact 1", Tier: 1})
	require.NoError(t, err)
	job := &CuratorJob{Service: service}
	err = job.HandleCuratorProposal([]byte(`{"namespace_id":"ns1"}`))
	require.NoError(t, err)
	proposals, err := service.ListProposals(t.Context(), brain.Actor{Kind: "service", NamespaceID: "ns1", TrustTier: 4}, "ns1", "pending")
	require.NoError(t, err)
	assert.NotEmpty(t, proposals)
}
