package mcp
import (
	"bytes"
	"encoding/json"
	"testing"
	"github.com/stretchr/testify/assert"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)
type Namespace struct { ID string `gorm:"primaryKey"`; CompanyID string `gorm:"index;not null"` }
func TestMCPServer(t *testing.T) {
	db, _ := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	db.AutoMigrate(&Memory{}, &Namespace{})
	var in, out bytes.Buffer
	server := &Server{db: db, in: &in, out: &out}

	b, _ := json.Marshal(Request{Method: "memory_note", ID: "1", Params: json.RawMessage(`{"content": "test memory", "type": "fact", "agentId": "agent1", "namespaceId": "ns1"}`)})
	server.handleLine(string(b))
	var res Response
	json.Unmarshal(out.Bytes(), &res)
	assert.Equal(t, "1", res.ID)

	out.Reset()
	b, _ = json.Marshal(Request{Method: "memory_search", ID: "2", Params: json.RawMessage(`{"query": "test", "topK": 5, "agentId": "agent1", "namespaceId": "ns1"}`)})
	server.handleLine(string(b))
	json.Unmarshal(out.Bytes(), &res)
	assert.Equal(t, "2", res.ID)

	out.Reset()
	b, _ = json.Marshal(Request{Method: "memory_correct", ID: "3", Params: json.RawMessage(`{"entityId": "mcp-note-11", "correction": "updated memory", "agentId": "agent1", "namespaceId": "ns1"}`)})
	server.handleLine(string(b))
	json.Unmarshal(out.Bytes(), &res)
	assert.Equal(t, "3", res.ID)
}
