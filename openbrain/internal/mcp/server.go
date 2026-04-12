package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/chifamba/vashandi/openbrain/internal/brain"
)

type Server struct {
	service *brain.Service
	in      io.Reader
	out     io.Writer
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

func NewServer(service *brain.Service) *Server {
	return &Server{service: service, in: os.Stdin, out: os.Stdout}
}

func (s *Server) Start() {
	scanner := bufio.NewScanner(s.in)
	for scanner.Scan() {
		response := s.HandleLine(scanner.Text())
		body, _ := json.Marshal(response)
		fmt.Fprintln(s.out, string(body))
	}
}

func (s *Server) HandleLine(line string) Response {
	var req Request
	if err := json.Unmarshal([]byte(line), &req); err != nil {
		return Response{Error: &Error{Code: -32700, Message: "Parse error"}}
	}
	return s.HandleRequest(req)
}

func (s *Server) HandleRequest(req Request) Response {
	var (
		result any
		err    error
	)
	switch req.Method {
	case "memory_search":
		result, err = s.handleSearch(req.Params)
	case "memory_note":
		result, err = s.handleNote(req.Params)
	case "memory_forget":
		result, err = s.handleForget(req.Params)
	case "memory_correct":
		result, err = s.handleCorrect(req.Params)
	case "memory_browse":
		result, err = s.handleBrowse(req.Params)
	case "context_compile":
		result, err = s.handleContextCompile(req.Params)
	default:
		return Response{ID: req.ID, Error: &Error{Code: -32601, Message: "Method not found"}}
	}
	if err != nil {
		return Response{ID: req.ID, Error: &Error{Code: -32000, Message: err.Error()}}
	}
	return Response{ID: req.ID, Result: result}
}

func (s *Server) HTTPHandler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/mcp", s.handleHTTPMessage)
	mux.HandleFunc("/mcp/message", s.handleHTTPMessage)
	mux.HandleFunc("/mcp/sse", s.handleSSE)
	return mux
}

func (s *Server) handleHTTPMessage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var req Request
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(Response{Error: &Error{Code: -32700, Message: err.Error()}})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(s.HandleRequest(req))
}

func (s *Server) handleSSE(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	announce := map[string]any{"tools": []string{"memory_search", "memory_note", "memory_forget", "memory_correct", "memory_browse", "context_compile"}, "transport": "http+sse"}
	body, _ := json.Marshal(announce)
	fmt.Fprintf(w, "event: ready\ndata: %s\n\n", body)
	flusher.Flush()
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case now := <-ticker.C:
			fmt.Fprintf(w, "event: ping\ndata: {\"ts\":%q}\n\n", now.Format(time.RFC3339Nano))
			flusher.Flush()
		}
	}
}

func (s *Server) handleSearch(params json.RawMessage) (any, error) {
	var p struct {
		Query        string   `json:"query"`
		TopK         int      `json:"topK"`
		NamespaceID  string   `json:"namespaceId"`
		AgentID      string   `json:"agentId"`
		IncludeTypes []string `json:"includeTypes"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, err
	}
	actor := brain.Actor{Kind: "agent", NamespaceID: p.NamespaceID, AgentID: p.AgentID, TrustTier: 4}
	return s.service.SearchMemories(context.Background(), actor, brain.SearchRequest{NamespaceID: p.NamespaceID, Query: p.Query, Limit: p.TopK, IncludeTypes: p.IncludeTypes, AgentID: p.AgentID, Intent: "mcp_search"})
}

func (s *Server) handleNote(params json.RawMessage) (any, error) {
	var p struct {
		Content     string         `json:"content"`
		Type        string         `json:"type"`
		AgentID     string         `json:"agentId"`
		NamespaceID string         `json:"namespaceId"`
		Tier        int            `json:"tier"`
		Metadata    map[string]any `json:"metadata"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, err
	}
	actor := brain.Actor{Kind: "agent", NamespaceID: p.NamespaceID, AgentID: p.AgentID, TrustTier: 4}
	memory, err := s.service.CreateMemory(context.Background(), actor, brain.MemoryPayload{NamespaceID: p.NamespaceID, EntityType: p.Type, Text: p.Content, Tier: p.Tier, Metadata: p.Metadata, Provenance: map[string]any{"kind": "mcp"}, Identity: map[string]any{"createdVia": "mcp", "createdByAgentId": p.AgentID}})
	if err != nil {
		return nil, err
	}
	return brain.MemoryPayload{ID: memory.ID, NamespaceID: memory.NamespaceID, EntityType: memory.EntityType, Text: memory.Text, Metadata: map[string]any{"tier": memory.Tier}}, nil
}

func (s *Server) handleForget(params json.RawMessage) (any, error) {
	var p struct {
		EntityID    string `json:"entityId"`
		NamespaceID string `json:"namespaceId"`
		AgentID     string `json:"agentId"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, err
	}
	actor := brain.Actor{Kind: "agent", NamespaceID: p.NamespaceID, AgentID: p.AgentID, TrustTier: 4}
	forgotten, err := s.service.ForgetMemories(context.Background(), actor, p.NamespaceID, []string{p.EntityID})
	if err != nil {
		return nil, err
	}
	return map[string]int64{"forgotten": forgotten}, nil
}

func (s *Server) handleCorrect(params json.RawMessage) (any, error) {
	var p struct {
		EntityID    string `json:"entityId"`
		Correction  string `json:"correction"`
		NamespaceID string `json:"namespaceId"`
		AgentID     string `json:"agentId"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, err
	}
	actor := brain.Actor{Kind: "agent", NamespaceID: p.NamespaceID, AgentID: p.AgentID, TrustTier: 4}
	memory, err := s.service.UpdateMemory(context.Background(), actor, p.NamespaceID, p.EntityID, map[string]any{"text": p.Correction})
	if err != nil {
		return nil, err
	}
	return brain.MemoryPayload{ID: memory.ID, NamespaceID: memory.NamespaceID, EntityType: memory.EntityType, Text: memory.Text}, nil
}

func (s *Server) handleBrowse(params json.RawMessage) (any, error) {
	var p struct {
		NamespaceID    string `json:"namespaceId"`
		AgentID        string `json:"agentId"`
		EntityType     string `json:"entityType"`
		Tier           *int   `json:"tier"`
		IncludeDeleted bool   `json:"includeDeleted"`
		Limit          int    `json:"limit"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, err
	}
	actor := brain.Actor{Kind: "agent", NamespaceID: p.NamespaceID, AgentID: p.AgentID, TrustTier: 4}
	memories, err := s.service.BrowseMemories(context.Background(), actor, p.NamespaceID, p.EntityType, p.Tier, p.IncludeDeleted, p.Limit)
	if err != nil {
		return nil, err
	}
	out := make([]brain.MemoryPayload, 0, len(memories))
	for _, memory := range memories {
		out = append(out, brain.MemoryPayload{ID: memory.ID, NamespaceID: memory.NamespaceID, EntityType: memory.EntityType, Title: memory.Title, Text: memory.Text, Tier: memory.Tier, Version: memory.Version, CreatedAt: memory.CreatedAt, UpdatedAt: memory.UpdatedAt})
	}
	return out, nil
}

func (s *Server) handleContextCompile(params json.RawMessage) (any, error) {
	var p struct {
		TaskQuery    string   `json:"taskQuery"`
		TokenBudget  int      `json:"tokenBudget"`
		AgentID      string   `json:"agentId"`
		NamespaceID  string   `json:"namespaceId"`
		IncludeTypes []string `json:"includeTypes"`
		Intent       string   `json:"intent"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, err
	}
	actor := brain.Actor{Kind: "agent", NamespaceID: p.NamespaceID, AgentID: p.AgentID, TrustTier: 4}
	return s.service.CompileContext(context.Background(), actor, brain.ContextRequest{NamespaceID: p.NamespaceID, AgentID: p.AgentID, TaskQuery: p.TaskQuery, TokenBudget: p.TokenBudget, IncludeTypes: p.IncludeTypes, Intent: p.Intent})
}
