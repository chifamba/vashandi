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
	service        *brain.Service
	in             io.Reader
	out            io.Writer
	actorExtractor func(r *http.Request) (brain.Actor, bool)
}

var supportedTools = []string{
	"memory_search",
	"memory_note",
	"memory_forget",
	"memory_correct",
	"memory_browse",
	"context_compile",
}

type Request struct {
	Method string          `json:"method"`
	Params json.RawMessage `json:"params"`
	ID     string          `json:"id"`
	// Actor is populated by the HTTP handler from the authenticated request context.
	// It is not part of the wire format; stdio callers embed trust info in params.
	Actor brain.Actor `json:"-"`
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

// NewServerWithActorExtractor creates a new MCP server with a custom actor extractor
// that allows the HTTP path to propagate the authenticated actor into MCP tool calls.
func NewServerWithActorExtractor(service *brain.Service, extractor func(r *http.Request) (brain.Actor, bool)) *Server {
	return &Server{service: service, in: os.Stdin, out: os.Stdout, actorExtractor: extractor}
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
		result, err = s.handleSearch(req)
	case "memory_note":
		result, err = s.handleNote(req)
	case "memory_forget":
		result, err = s.handleForget(req)
	case "memory_correct":
		result, err = s.handleCorrect(req)
	case "memory_browse":
		result, err = s.handleBrowse(req)
	case "context_compile":
		result, err = s.handleContextCompile(req)
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
	// Propagate the authenticated actor from request context to MCP params if not overridden.
	if s.actorExtractor != nil {
		if actor, ok := s.actorExtractor(r); ok {
			req.Actor = actor
		}
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
	announce := map[string]any{"tools": supportedTools, "transport": "http+sse"}
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

func (s *Server) handleSearch(req Request) (any, error) {
	var p struct {
		Query        string   `json:"query"`
		TopK         int      `json:"topK"`
		NamespaceID  string   `json:"namespaceId"`
		AgentID      string   `json:"agentId"`
		TrustTier    int      `json:"trustTier"`
		IncludeTypes []string `json:"includeTypes"`
	}
	if err := json.Unmarshal(req.Params, &p); err != nil {
		return nil, err
	}
	actor := mergeActor(req.Actor, p.AgentID, p.NamespaceID, p.TrustTier, "agent")
	return s.service.SearchMemories(context.Background(), actor, brain.SearchRequest{NamespaceID: p.NamespaceID, Query: p.Query, Limit: p.TopK, IncludeTypes: p.IncludeTypes, AgentID: p.AgentID, Intent: "mcp_search"})
}

func (s *Server) handleNote(req Request) (any, error) {
	var p struct {
		Content     string         `json:"content"`
		Type        string         `json:"type"`
		AgentID     string         `json:"agentId"`
		NamespaceID string         `json:"namespaceId"`
		TrustTier   int            `json:"trustTier"`
		Tier        int            `json:"tier"`
		Metadata    map[string]any `json:"metadata"`
	}
	if err := json.Unmarshal(req.Params, &p); err != nil {
		return nil, err
	}
	actor := mergeActor(req.Actor, p.AgentID, p.NamespaceID, p.TrustTier, "agent")
	memory, err := s.service.CreateMemory(context.Background(), actor, brain.MemoryPayload{NamespaceID: p.NamespaceID, EntityType: p.Type, Text: p.Content, Tier: p.Tier, Metadata: p.Metadata, Provenance: map[string]any{"kind": "mcp"}, Identity: map[string]any{"createdVia": "mcp", "createdByAgentId": p.AgentID}})
	if err != nil {
		return nil, err
	}
	return brain.MemoryPayload{ID: memory.ID, NamespaceID: memory.NamespaceID, EntityType: memory.EntityType, Text: memory.Text, Metadata: map[string]any{"tier": memory.Tier}}, nil
}

func (s *Server) handleForget(req Request) (any, error) {
	var p struct {
		EntityID    string `json:"entityId"`
		NamespaceID string `json:"namespaceId"`
		AgentID     string `json:"agentId"`
		TrustTier   int    `json:"trustTier"`
	}
	if err := json.Unmarshal(req.Params, &p); err != nil {
		return nil, err
	}
	actor := mergeActor(req.Actor, p.AgentID, p.NamespaceID, p.TrustTier, "agent")
	forgotten, err := s.service.ForgetMemories(context.Background(), actor, p.NamespaceID, []string{p.EntityID})
	if err != nil {
		return nil, err
	}
	return map[string]int64{"forgotten": forgotten}, nil
}

func (s *Server) handleCorrect(req Request) (any, error) {
	var p struct {
		EntityID    string `json:"entityId"`
		Correction  string `json:"correction"`
		NamespaceID string `json:"namespaceId"`
		AgentID     string `json:"agentId"`
		TrustTier   int    `json:"trustTier"`
	}
	if err := json.Unmarshal(req.Params, &p); err != nil {
		return nil, err
	}
	actor := mergeActor(req.Actor, p.AgentID, p.NamespaceID, p.TrustTier, "agent")
	memory, err := s.service.UpdateMemory(context.Background(), actor, p.NamespaceID, p.EntityID, map[string]any{"text": p.Correction})
	if err != nil {
		return nil, err
	}
	return brain.MemoryPayload{ID: memory.ID, NamespaceID: memory.NamespaceID, EntityType: memory.EntityType, Text: memory.Text}, nil
}

func (s *Server) handleBrowse(req Request) (any, error) {
	var p struct {
		NamespaceID    string `json:"namespaceId"`
		AgentID        string `json:"agentId"`
		TrustTier      int    `json:"trustTier"`
		EntityType     string `json:"entityType"`
		Tier           *int   `json:"tier"`
		IncludeDeleted bool   `json:"includeDeleted"`
		Limit          int    `json:"limit"`
	}
	if err := json.Unmarshal(req.Params, &p); err != nil {
		return nil, err
	}
	actor := mergeActor(req.Actor, p.AgentID, p.NamespaceID, p.TrustTier, "agent")
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

func (s *Server) handleContextCompile(req Request) (any, error) {
	var p struct {
		TaskQuery    string   `json:"taskQuery"`
		TokenBudget  int      `json:"tokenBudget"`
		AgentID      string   `json:"agentId"`
		NamespaceID  string   `json:"namespaceId"`
		TrustTier    int      `json:"trustTier"`
		IncludeTypes []string `json:"includeTypes"`
		Intent       string   `json:"intent"`
	}
	if err := json.Unmarshal(req.Params, &p); err != nil {
		return nil, err
	}
	actor := mergeActor(req.Actor, p.AgentID, p.NamespaceID, p.TrustTier, "agent")
	return s.service.CompileContext(context.Background(), actor, brain.ContextRequest{NamespaceID: p.NamespaceID, AgentID: p.AgentID, TaskQuery: p.TaskQuery, TokenBudget: p.TokenBudget, IncludeTypes: p.IncludeTypes, Intent: p.Intent})
}

// mergeActor builds an actor from the authenticated request actor (HTTP path) or from
// explicit params (stdio path). The authenticated actor always takes precedence for
// trust tier to prevent privilege escalation.
func mergeActor(base brain.Actor, agentID, namespaceID string, paramTrustTier int, kind string) brain.Actor {
	actor := base
	if actor.Kind == "" {
		actor.Kind = kind
	}
	if actor.AgentID == "" && agentID != "" {
		actor.AgentID = agentID
	}
	if actor.NamespaceID == "" && namespaceID != "" {
		actor.NamespaceID = namespaceID
	}
	// Only use param trust tier when no authenticated actor provided it (stdio path).
	if base.TrustTier == 0 {
		if paramTrustTier >= 1 && paramTrustTier <= 4 {
			actor.TrustTier = paramTrustTier
		} else if paramTrustTier != 0 {
			// Invalid trust tier provided; log and default to read-only.
			actor.TrustTier = 1
		} else {
			actor.TrustTier = 1 // default to read-only for unauthenticated MCP callers
		}
	}
	return actor
}
