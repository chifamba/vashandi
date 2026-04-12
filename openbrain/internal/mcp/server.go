package mcp
import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"gorm.io/gorm"
)
type Server struct {
	db *gorm.DB
	in io.Reader
	out io.Writer
}
func NewServer(db *gorm.DB) *Server {
	return &Server{db: db, in: os.Stdin, out: os.Stdout}
}
type Request struct {
	Method string          `json:"method"`
	Params json.RawMessage `json:"params"`
	ID     string          `json:"id"`
}
type Response struct {
	ID     string      `json:"id"`
	Result interface{} `json:"result,omitempty"`
	Error  *Error      `json:"error,omitempty"`
}
type Error struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}
func (s *Server) Start() {
	scanner := bufio.NewScanner(s.in)
	for scanner.Scan() {
		s.handleLine(scanner.Text())
	}
}
func (s *Server) handleLine(line string) {
	var req Request
	if err := json.Unmarshal([]byte(line), &req); err != nil {
		s.sendError("", -32700, "Parse error")
		return
	}
	var res interface{}
	var err error
	switch req.Method {
	case "memory_search": res, err = s.handleSearch(req.Params)
	case "memory_note": res, err = s.handleNote(req.Params)
	case "memory_forget": res, err = s.handleForget(req.Params)
	case "memory_correct": res, err = s.handleCorrect(req.Params)
	default: s.sendError(req.ID, -32601, "Method not found"); return
	}
	if err != nil {
		s.sendError(req.ID, -32000, err.Error())
		return
	}
	s.sendResponse(req.ID, res)
}
type Memory struct {
	ID          string `json:"id" gorm:"primaryKey"`
	NamespaceID string `json:"namespaceId" gorm:"index;not null"`
	Text        string `json:"text" gorm:"type:text"`
	Metadata    string `json:"metadata" gorm:"type:jsonb"`
}
func (s *Server) handleSearch(params json.RawMessage) (interface{}, error) {
	var p struct { Query string `json:"query"`; TopK int `json:"topK"`; NamespaceID string `json:"namespaceId"` }
	json.Unmarshal(params, &p)
	limit := p.TopK
	if limit == 0 { limit = 10 }
	var memories []Memory
	if err := s.db.Where("namespace_id = ? AND text LIKE ?", p.NamespaceID, "%"+p.Query+"%").Limit(limit).Find(&memories).Error; err != nil {
		return nil, err
	}
	return memories, nil
}
func (s *Server) handleNote(params json.RawMessage) (interface{}, error) {
	var p struct { Content string `json:"content"`; Type string `json:"type"`; AgentID string `json:"agentId"`; NamespaceID string `json:"namespaceId"` }
	json.Unmarshal(params, &p)
	meta := map[string]string{"type": p.Type, "author_agent": p.AgentID}
	metaBytes, _ := json.Marshal(meta)
	mem := Memory{ID: fmt.Sprintf("mcp-note-%d", len(p.Content)), NamespaceID: p.NamespaceID, Text: p.Content, Metadata: string(metaBytes)}
	if err := s.db.Create(&mem).Error; err != nil { return nil, err }
	return mem, nil
}
func (s *Server) handleForget(params json.RawMessage) (interface{}, error) {
	var p struct { EntityID string `json:"entityId"`; NamespaceID string `json:"namespaceId"` }
	json.Unmarshal(params, &p)
	res := s.db.Where("namespace_id = ? AND id = ?", p.NamespaceID, p.EntityID).Delete(&Memory{})
	return map[string]int64{"forgotten": res.RowsAffected}, res.Error
}
func (s *Server) handleCorrect(params json.RawMessage) (interface{}, error) {
	var p struct { EntityID string `json:"entityId"`; Correction string `json:"correction"`; NamespaceID string `json:"namespaceId"` }
	json.Unmarshal(params, &p)
	res := s.db.Model(&Memory{}).Where("namespace_id = ? AND id = ?", p.NamespaceID, p.EntityID).Update("text", p.Correction)
	return map[string]int64{"updated": res.RowsAffected}, res.Error
}
func (s *Server) sendResponse(id string, result interface{}) {
	b, _ := json.Marshal(Response{ID: id, Result: result})
	fmt.Fprintln(s.out, string(b))
}
func (s *Server) sendError(id string, code int, message string) {
	b, _ := json.Marshal(Response{ID: id, Error: &Error{Code: code, Message: message}})
	fmt.Fprintln(s.out, string(b))
}
