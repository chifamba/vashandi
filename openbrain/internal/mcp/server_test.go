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
	service := brain.NewService(db)
	require.NoError(t, service.AutoMigrate())
	server := NewServer(service)

	noteReq, _ := json.Marshal(Request{Method: "memory_note", ID: "1", Params: json.RawMessage(`{"content":"test memory","type":"fact","agentId":"agent1","namespaceId":"ns1"}`)})
	res := server.HandleLine(string(noteReq))
	assert.Equal(t, "1", res.ID)
	assert.Nil(t, res.Error)

	searchReq, _ := json.Marshal(Request{Method: "memory_search", ID: "2", Params: json.RawMessage(`{"query":"test","topK":5,"agentId":"agent1","namespaceId":"ns1"}`)})
	res = server.HandleLine(string(searchReq))
	assert.Equal(t, "2", res.ID)
	assert.Nil(t, res.Error)

	browseReq, _ := json.Marshal(Request{Method: "memory_browse", ID: "3", Params: json.RawMessage(`{"namespaceId":"ns1","agentId":"agent1","limit":10}`)})
	res = server.HandleLine(string(browseReq))
	assert.Equal(t, "3", res.ID)
	assert.Nil(t, res.Error)
}
