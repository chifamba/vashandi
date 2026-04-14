package mcp

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/chifamba/vashandi/openbrain/internal/brain"
)

func TestMCPServer(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	service := brain.NewService(db, brain.NewStubProvider(1536))
	require.NoError(t, service.AutoMigrate())
	server := NewServer(service)

	// stdio callers must include trustTier in params (no HTTP auth middleware available)
	noteReq, _ := json.Marshal(Request{Method: "memory_note", ID: "1", Params: json.RawMessage(`{"content":"test memory","type":"fact","agentId":"agent1","namespaceId":"ns1","trustTier":2}`)})
	res := server.HandleLine(string(noteReq))
	assert.Equal(t, "1", res.ID)
	assert.Nil(t, res.Error)

	searchReq, _ := json.Marshal(Request{Method: "memory_search", ID: "2", Params: json.RawMessage(`{"query":"test","topK":5,"agentId":"agent1","namespaceId":"ns1","trustTier":1}`)})
	res = server.HandleLine(string(searchReq))
	assert.Equal(t, "2", res.ID)
	assert.Nil(t, res.Error)

	browseReq, _ := json.Marshal(Request{Method: "memory_browse", ID: "3", Params: json.RawMessage(`{"namespaceId":"ns1","agentId":"agent1","limit":10,"trustTier":1}`)})
	res = server.HandleLine(string(browseReq))
	assert.Equal(t, "3", res.ID)
	assert.Nil(t, res.Error)
}
