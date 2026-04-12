package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"google.golang.org/grpc"

	"github.com/chifamba/vashandi/openbrain/db/models"
	"github.com/chifamba/vashandi/openbrain/internal/brain"
	mcppkg "github.com/chifamba/vashandi/openbrain/internal/mcp"
	pb "github.com/chifamba/vashandi/openbrain/proto/v1"
)

type memoryServer struct {
	pb.UnimplementedMemoryServiceServer
	service *brain.Service
}

type application struct {
	service   *brain.Service
	mcpServer *mcppkg.Server
}

func (s *memoryServer) Ingest(ctx context.Context, req *pb.IngestRequest) (*pb.IngestResponse, error) {
	actor := brain.Actor{Kind: "service", NamespaceID: req.NamespaceId, TrustTier: 4}
	var count int32
	for _, record := range req.Records {
		_, err := s.service.CreateMemory(ctx, actor, brain.MemoryPayload{NamespaceID: req.NamespaceId, EntityType: record.Metadata["type"], Text: record.Text, Metadata: stringMapToAny(record.Metadata), Provenance: map[string]any{"kind": "grpc"}, Identity: map[string]any{"createdVia": "grpc"}})
		if err == nil {
			count++
		}
	}
	return &pb.IngestResponse{RecordsIngested: count}, nil
}

func (s *memoryServer) Query(ctx context.Context, req *pb.QueryRequest) (*pb.QueryResponse, error) {
	results, err := s.service.SearchMemories(ctx, brain.Actor{Kind: "service", NamespaceID: req.NamespaceId, TrustTier: 4}, brain.SearchRequest{NamespaceID: req.NamespaceId, Query: req.Query, Limit: int(req.Limit), Intent: "grpc_query"})
	if err != nil {
		return nil, err
	}
	out := make([]*pb.MemoryRecord, 0, len(results))
	for _, result := range results {
		out = append(out, &pb.MemoryRecord{Id: result.Memory.ID, Text: result.Memory.Text, Metadata: anyMapToString(result.Memory.Metadata)})
	}
	return &pb.QueryResponse{Records: out}, nil
}

func (s *memoryServer) Forget(ctx context.Context, req *pb.ForgetRequest) (*pb.ForgetResponse, error) {
	forgotten, err := s.service.ForgetMemories(ctx, brain.Actor{Kind: "service", NamespaceID: req.NamespaceId, TrustTier: 4}, req.NamespaceId, req.RecordIds)
	if err != nil {
		return nil, err
	}
	return &pb.ForgetResponse{RecordsForgotten: int32(forgotten)}, nil
}

func main() {
	if len(os.Args) > 1 {
		if err := Execute(runServer); err != nil {
			slog.Error("openbrain command failed", "error", err)
			os.Exit(1)
		}
		return
	}
	if err := runServer(); err != nil {
		slog.Error("openbrain server failed", "error", err)
		os.Exit(1)
	}
}

func runServer() error {
	db := InitDB()
	service := brain.NewService(db)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	service.StartBackgroundJobs(ctx)
	app := &application{service: service, mcpServer: mcppkg.NewServer(service)}

	router := app.routes()
	httpPort := envDefault("PORT", "3101")
	httpServer := &http.Server{Addr: ":" + httpPort, Handler: router, ReadHeaderTimeout: 10 * time.Second}
	httpErr := make(chan error, 1)
	go func() {
		slog.Info("starting REST server", "port", httpPort)
		httpErr <- httpServer.ListenAndServe()
	}()

	grpcPort := envDefault("GRPC_PORT", "50051")
	lis, err := net.Listen("tcp", ":"+grpcPort)
	if err != nil {
		return err
	}
	grpcServer := grpc.NewServer()
	pb.RegisterMemoryServiceServer(grpcServer, &memoryServer{service: service})
	grpcErr := make(chan error, 1)
	go func() {
		slog.Info("starting gRPC server", "port", grpcPort)
		grpcErr <- grpcServer.Serve(lis)
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = httpServer.Shutdown(shutdownCtx)
		grpcServer.GracefulStop()
		return nil
	case err := <-httpErr:
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
		return nil
	case err := <-grpcErr:
		return err
	}
}

func (app *application) routes() http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Get("/healthz", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"status": "ok"})
	})
	r.Group(func(protected chi.Router) {
		protected.Use(AuthMiddleware)
		protected.Mount("/", app.mcpServer.HTTPHandler())
		protected.Route("/api/v1", func(api chi.Router) {
			api.Get("/health", app.handleHealth)
			api.Post("/memories", app.handleCreateMemory)
			api.Get("/memories", app.handleBrowseMemories)
			api.Post("/memories/search", app.handleSearchMemories)
			api.Get("/memories/{memoryId}", app.handleGetMemory)
			api.Patch("/memories/{memoryId}", app.handleUpdateMemory)
			api.Delete("/memories/{memoryId}", app.handleDeleteMemory)
			api.Get("/memories/{memoryId}/versions", app.handleListVersions)
			api.Post("/memories/{memoryId}/rollback", app.handleRollbackMemory)
			api.Post("/memories/edges", app.handleCreateEdge)
			api.Get("/memories/{memoryId}/edges", app.handleGetEdges)
			api.Post("/context/compile", app.handleCompileContext)
			api.Get("/context/pending", app.handlePendingContext)
			api.Get("/audit/log", app.handleAuditLog)
			api.Get("/audit/export", app.handleAuditExport)
			api.Get("/admin/dashboard", app.handleDashboard)
			api.Post("/admin/daydream", app.handleDaydream)
			api.Get("/admin/proposals", app.handleProposalList)
			api.Get("/admin/memories", app.handleBrowseMemories)
			api.Route("/namespaces/{namespaceId}", func(ns chi.Router) {
				ns.Post("/memories/ingest", app.handleLegacyIngest)
				ns.Post("/memories/query", app.handleLegacyQuery)
				ns.Post("/memories/forget", app.handleLegacyForget)
				ns.Delete("/memories", app.handleLegacyForget)
				ns.Post("/context/compile", app.handleLegacyCompileContext)
				ns.Get("/proposals", app.handleProposalList)
				ns.Post("/proposals/{proposalId}/resolve", app.handleResolveProposal)
			})
		})
		protected.Route("/internal/v1/namespaces/{namespaceId}", func(internal chi.Router) {
			internal.Post("/agents", app.handleRegisterAgent)
			internal.Delete("/agents/{agentId}", app.handleDeregisterAgent)
			internal.Post("/triggers/{triggerType}", app.handleTrigger)
			internal.Post("/sync", app.handleSync)
		})
		protected.Route("/v1/namespaces/{namespaceId}", func(legacy chi.Router) {
			legacy.Post("/memories", app.handleLegacyIngest)
			legacy.Post("/memories/query", app.handleLegacyQuery)
			legacy.Delete("/memories", app.handleLegacyForget)
			legacy.Post("/memories/forget", app.handleLegacyForget)
			legacy.Post("/context/compile", app.handleLegacyCompileContext)
			legacy.Post("/triggers/{triggerType}", app.handleTrigger)
			legacy.Get("/proposals", app.handleProposalList)
			legacy.Post("/proposals/{proposalId}/resolve", app.handleResolveProposal)
		})
		protected.Get("/admin", app.handleAdminHTML)
	})
	return r
}

func (app *application) handleCreateMemory(w http.ResponseWriter, r *http.Request) {
	actor := actorFromRequest(r)
	var payload brain.MemoryPayload
	if err := decodeJSON(r, &payload); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	payload.NamespaceID = namespaceFromPayload(payload.NamespaceID, actor.NamespaceID, r.URL.Query().Get("namespaceId"))
	if !maybeNamespaceAuthorized(w, r, payload.NamespaceID) {
		return
	}
	memory, err := app.service.CreateMemory(r.Context(), actor, payload)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}
	writeJSON(w, http.StatusCreated, brain.MemoryPayload{ID: memory.ID, NamespaceID: memory.NamespaceID, EntityType: memory.EntityType, Title: memory.Title, Text: memory.Text, Tier: memory.Tier, Version: memory.Version, CreatedAt: memory.CreatedAt, UpdatedAt: memory.UpdatedAt})
}

func (app *application) handleBrowseMemories(w http.ResponseWriter, r *http.Request) {
	actor := actorFromRequest(r)
	namespaceID := namespaceFromPayload(r.URL.Query().Get("namespaceId"), actor.NamespaceID)
	if !maybeNamespaceAuthorized(w, r, namespaceID) {
		return
	}
	var tierPtr *int
	if tierRaw := r.URL.Query().Get("tier"); tierRaw != "" {
		if tier, err := strconv.Atoi(tierRaw); err == nil {
			tierPtr = &tier
		}
	}
	includeDeleted := r.URL.Query().Get("includeDeleted") == "true"
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	memories, err := app.service.BrowseMemories(r.Context(), actor, namespaceID, r.URL.Query().Get("entityType"), tierPtr, includeDeleted, limit)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	payloads := make([]brain.MemoryPayload, 0, len(memories))
	for _, memory := range memories {
		payloads = append(payloads, brain.MemoryPayload{ID: memory.ID, NamespaceID: memory.NamespaceID, EntityType: memory.EntityType, Title: memory.Title, Text: memory.Text, Tier: memory.Tier, Version: memory.Version, AccessCount: memory.AccessCount, CreatedAt: memory.CreatedAt, UpdatedAt: memory.UpdatedAt})
	}
	writeJSON(w, http.StatusOK, payloads)
}

func (app *application) handleGetMemory(w http.ResponseWriter, r *http.Request) {
	actor := actorFromRequest(r)
	namespaceID := namespaceFromPayload(r.URL.Query().Get("namespaceId"), actor.NamespaceID)
	if !maybeNamespaceAuthorized(w, r, namespaceID) {
		return
	}
	memory, err := app.service.GetMemory(r.Context(), actor, namespaceID, chi.URLParam(r, "memoryId"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	writeJSON(w, http.StatusOK, brain.MemoryPayload{ID: memory.ID, NamespaceID: memory.NamespaceID, EntityType: memory.EntityType, Title: memory.Title, Text: memory.Text, Tier: memory.Tier, Version: memory.Version, AccessCount: memory.AccessCount, CreatedAt: memory.CreatedAt, UpdatedAt: memory.UpdatedAt})
}

func (app *application) handleUpdateMemory(w http.ResponseWriter, r *http.Request) {
	actor := actorFromRequest(r)
	namespaceID := namespaceFromPayload(r.URL.Query().Get("namespaceId"), actor.NamespaceID)
	if !maybeNamespaceAuthorized(w, r, namespaceID) {
		return
	}
	var patch map[string]any
	if err := decodeJSON(r, &patch); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	memory, err := app.service.UpdateMemory(r.Context(), actor, namespaceID, chi.URLParam(r, "memoryId"), patch)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}
	writeJSON(w, http.StatusOK, brain.MemoryPayload{ID: memory.ID, NamespaceID: memory.NamespaceID, EntityType: memory.EntityType, Title: memory.Title, Text: memory.Text, Tier: memory.Tier, Version: memory.Version, AccessCount: memory.AccessCount, CreatedAt: memory.CreatedAt, UpdatedAt: memory.UpdatedAt})
}

func (app *application) handleDeleteMemory(w http.ResponseWriter, r *http.Request) {
	actor := actorFromRequest(r)
	namespaceID := namespaceFromPayload(r.URL.Query().Get("namespaceId"), actor.NamespaceID)
	if !maybeNamespaceAuthorized(w, r, namespaceID) {
		return
	}
	if err := app.service.SoftDeleteMemory(r.Context(), actor, namespaceID, chi.URLParam(r, "memoryId")); err != nil {
		http.Error(w, err.Error(), http.StatusForbidden)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "deleted"})
}

func (app *application) handleListVersions(w http.ResponseWriter, r *http.Request) {
	actor := actorFromRequest(r)
	namespaceID := namespaceFromPayload(r.URL.Query().Get("namespaceId"), actor.NamespaceID)
	if !maybeNamespaceAuthorized(w, r, namespaceID) {
		return
	}
	var versions []models.MemoryVersion
	if err := app.service.DB.WithContext(r.Context()).Where("namespace_id = ? AND entity_id = ?", namespaceID, chi.URLParam(r, "memoryId")).Order("version desc").Find(&versions).Error; err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, versions)
}

func (app *application) handleRollbackMemory(w http.ResponseWriter, r *http.Request) {
	actor := actorFromRequest(r)
	var body struct {
		NamespaceID string `json:"namespaceId"`
		Version     int    `json:"version"`
	}
	if err := decodeJSON(r, &body); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	body.NamespaceID = namespaceFromPayload(body.NamespaceID, actor.NamespaceID, r.URL.Query().Get("namespaceId"))
	if !maybeNamespaceAuthorized(w, r, body.NamespaceID) {
		return
	}
	memory, err := app.service.RollbackMemory(r.Context(), actor, body.NamespaceID, chi.URLParam(r, "memoryId"), body.Version)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}
	writeJSON(w, http.StatusOK, memory)
}

func (app *application) handleSearchMemories(w http.ResponseWriter, r *http.Request) {
	actor := actorFromRequest(r)
	var body struct {
		NamespaceID  string   `json:"namespaceId"`
		Query        string   `json:"query"`
		TopK         int      `json:"topK"`
		Limit        int      `json:"limit"`
		IncludeTypes []string `json:"includeTypes"`
		AgentID      string   `json:"agentId"`
		Intent       string   `json:"intent"`
	}
	if err := decodeJSON(r, &body); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	body.NamespaceID = namespaceFromPayload(body.NamespaceID, actor.NamespaceID)
	if !maybeNamespaceAuthorized(w, r, body.NamespaceID) {
		return
	}
	limit := body.TopK
	if limit == 0 {
		limit = body.Limit
	}
	results, err := app.service.SearchMemories(r.Context(), actor, brain.SearchRequest{NamespaceID: body.NamespaceID, Query: body.Query, Limit: limit, IncludeTypes: body.IncludeTypes, AgentID: body.AgentID, Intent: body.Intent})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"records": results})
}

func (app *application) handleCreateEdge(w http.ResponseWriter, r *http.Request) {
	actor := actorFromRequest(r)
	var body struct {
		NamespaceID string         `json:"namespaceId"`
		FromID      string         `json:"fromEntityId"`
		ToID        string         `json:"toEntityId"`
		EdgeType    string         `json:"edgeType"`
		Weight      float64        `json:"weight"`
		Metadata    map[string]any `json:"metadata"`
	}
	if err := decodeJSON(r, &body); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	body.NamespaceID = namespaceFromPayload(body.NamespaceID, actor.NamespaceID)
	if !maybeNamespaceAuthorized(w, r, body.NamespaceID) {
		return
	}
	edge, err := app.service.CreateEdge(r.Context(), actor, body.NamespaceID, body.FromID, body.ToID, body.EdgeType, body.Weight, body.Metadata)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}
	writeJSON(w, http.StatusCreated, edge)
}

func (app *application) handleGetEdges(w http.ResponseWriter, r *http.Request) {
	actor := actorFromRequest(r)
	namespaceID := namespaceFromPayload(r.URL.Query().Get("namespaceId"), actor.NamespaceID)
	if !maybeNamespaceAuthorized(w, r, namespaceID) {
		return
	}
	edges, err := app.service.GetEdges(r.Context(), actor, namespaceID, chi.URLParam(r, "memoryId"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, edges)
}

func (app *application) handleCompileContext(w http.ResponseWriter, r *http.Request) {
	actor := actorFromRequest(r)
	var req brain.ContextRequest
	if err := decodeJSON(r, &req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	req.NamespaceID = namespaceFromPayload(req.NamespaceID, actor.NamespaceID)
	if !maybeNamespaceAuthorized(w, r, req.NamespaceID) {
		return
	}
	resp, err := app.service.CompileContext(r.Context(), actor, req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (app *application) handlePendingContext(w http.ResponseWriter, r *http.Request) {
	actor := actorFromRequest(r)
	namespaceID := namespaceFromPayload(r.URL.Query().Get("namespaceId"), actor.NamespaceID)
	agentID := firstNonEmpty(r.URL.Query().Get("agentId"), actor.AgentID)
	if !maybeNamespaceAuthorized(w, r, namespaceID) {
		return
	}
	packet, err := app.service.GetPendingContext(r.Context(), actor, namespaceID, agentID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	writeJSON(w, http.StatusOK, packet)
}

func (app *application) handleRegisterAgent(w http.ResponseWriter, r *http.Request) {
	actor := actorFromRequest(r)
	namespaceID := chi.URLParam(r, "namespaceId")
	if !maybeNamespaceAuthorized(w, r, namespaceID) {
		return
	}
	var body struct {
		AgentID       string              `json:"agentId"`
		Name          string              `json:"name"`
		TrustTier     int                 `json:"trustTier"`
		RecallProfile brain.RecallProfile `json:"recallProfile"`
	}
	if err := decodeJSON(r, &body); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	agent, err := app.service.RegisterAgent(r.Context(), actor, namespaceID, body.AgentID, body.Name, body.TrustTier, body.RecallProfile)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}
	writeJSON(w, http.StatusCreated, agent)
}

func (app *application) handleDeregisterAgent(w http.ResponseWriter, r *http.Request) {
	actor := actorFromRequest(r)
	namespaceID := chi.URLParam(r, "namespaceId")
	if !maybeNamespaceAuthorized(w, r, namespaceID) {
		return
	}
	if err := app.service.DeregisterAgent(r.Context(), actor, namespaceID, chi.URLParam(r, "agentId")); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "deregistered"})
}

func (app *application) handleTrigger(w http.ResponseWriter, r *http.Request) {
	actor := actorFromRequest(r)
	namespaceID := chi.URLParam(r, "namespaceId")
	if !maybeNamespaceAuthorized(w, r, namespaceID) {
		return
	}
	var req brain.TriggerRequest
	_ = decodeJSON(r, &req)
	resp, err := app.service.HandleTrigger(r.Context(), actor, namespaceID, chi.URLParam(r, "triggerType"), req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (app *application) handleSync(w http.ResponseWriter, r *http.Request) {
	actor := actorFromRequest(r)
	namespaceID := chi.URLParam(r, "namespaceId")
	if !maybeNamespaceAuthorized(w, r, namespaceID) {
		return
	}
	var body struct {
		Dir string `json:"dir"`
	}
	if err := decodeJSON(r, &body); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	memories, err := app.service.SyncRepositoryDir(r.Context(), actor, namespaceID, firstNonEmpty(body.Dir, "."))
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"synced": len(memories), "memories": memories})
}

func (app *application) handleProposalList(w http.ResponseWriter, r *http.Request) {
	actor := actorFromRequest(r)
	namespaceID := namespaceFromPayload(chi.URLParam(r, "namespaceId"), r.URL.Query().Get("namespaceId"), actor.NamespaceID)
	if !maybeNamespaceAuthorized(w, r, namespaceID) {
		return
	}
	proposals, err := app.service.ListProposals(r.Context(), actor, namespaceID, r.URL.Query().Get("status"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, proposals)
}

func (app *application) handleResolveProposal(w http.ResponseWriter, r *http.Request) {
	actor := actorFromRequest(r)
	namespaceID := chi.URLParam(r, "namespaceId")
	if !maybeNamespaceAuthorized(w, r, namespaceID) {
		return
	}
	var body struct {
		Action string `json:"action"`
	}
	if err := decodeJSON(r, &body); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	proposal, err := app.service.ResolveProposal(r.Context(), actor, namespaceID, chi.URLParam(r, "proposalId"), body.Action)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}
	writeJSON(w, http.StatusOK, proposal)
}

func (app *application) handleAuditLog(w http.ResponseWriter, r *http.Request) {
	actor := actorFromRequest(r)
	namespaceID := namespaceFromPayload(r.URL.Query().Get("namespaceId"), actor.NamespaceID)
	if !maybeNamespaceAuthorized(w, r, namespaceID) {
		return
	}
	var logs []map[string]any
	query := app.service.DB.WithContext(r.Context()).Raw("SELECT id, namespace_id, agent_id, actor_kind, action, entity_id, entity_type, before_hash, after_hash, chain_hash, request_meta, created_at FROM memory_audit_log WHERE namespace_id = ? ORDER BY id DESC LIMIT 200", namespaceID)
	if err := query.Scan(&logs).Error; err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, logs)
}

func (app *application) handleAuditExport(w http.ResponseWriter, r *http.Request) {
	actor := actorFromRequest(r)
	namespaceID := namespaceFromPayload(r.URL.Query().Get("namespaceId"), actor.NamespaceID)
	if !maybeNamespaceAuthorized(w, r, namespaceID) {
		return
	}
	body, contentType, err := app.service.ExportAudit(r.Context(), namespaceID, r.URL.Query().Get("format"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}
	w.Header().Set("Content-Type", contentType)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body)
}

func (app *application) handleDashboard(w http.ResponseWriter, r *http.Request) {
	actor := actorFromRequest(r)
	namespaceID := namespaceFromPayload(r.URL.Query().Get("namespaceId"), actor.NamespaceID)
	if !maybeNamespaceAuthorized(w, r, namespaceID) {
		return
	}
	metrics, err := app.service.Dashboard(r.Context(), namespaceID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, metrics)
}

func (app *application) handleDaydream(w http.ResponseWriter, r *http.Request) {
	actor := actorFromRequest(r)
	var body struct {
		NamespaceID string `json:"namespaceId"`
	}
	if err := decodeJSON(r, &body); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	body.NamespaceID = namespaceFromPayload(body.NamespaceID, actor.NamespaceID)
	if !maybeNamespaceAuthorized(w, r, body.NamespaceID) {
		return
	}
	proposals, err := app.service.GenerateCuratorProposals(r.Context(), body.NamespaceID, actor)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"generated": len(proposals), "proposals": proposals})
}

func (app *application) handleHealth(w http.ResponseWriter, r *http.Request) {
	actor := actorFromRequest(r)
	namespaceID := namespaceFromPayload(r.URL.Query().Get("namespaceId"), actor.NamespaceID)
	if namespaceID == "" {
		writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "time": time.Now().UTC()})
		return
	}
	metrics, err := app.service.Dashboard(r.Context(), namespaceID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "metrics": metrics})
}

func (app *application) handleAdminHTML(w http.ResponseWriter, r *http.Request) {
	namespaceID := namespaceFromPayload(r.URL.Query().Get("namespaceId"), actorFromRequest(r).NamespaceID)
	if namespaceID == "" {
		namespaceID = "default"
	}
	metrics, _ := app.service.Dashboard(r.Context(), namespaceID)
	proposals, _ := app.service.ListProposals(r.Context(), actorFromRequest(r), namespaceID, "")
	metricsJSON, _ := json.MarshalIndent(metrics, "", "  ")
	proposalsJSON, _ := json.MarshalIndent(proposals, "", "  ")
	page := fmt.Sprintf(`<!doctype html><html><head><title>OpenBrain Admin</title><style>body{font-family:sans-serif;max-width:1000px;margin:2rem auto;padding:0 1rem}pre{background:#111;color:#eee;padding:1rem;overflow:auto;white-space:pre-wrap}code{background:#f4f4f4;padding:.1rem .3rem}</style></head><body><h1>OpenBrain Admin</h1><p>Namespace: <code>%s</code></p><p>This page is server-rendered to avoid exposing bearer tokens in client-side JavaScript. Use the JSON admin endpoints or CLI for mutating actions such as curator day-dreaming.</p><p>Trigger curator generation with: <code>openbrain --base-url http://localhost:3101 --token YOUR_TOKEN memory approve &lt;proposal-id&gt; --namespace %s</code> or call <code>POST /api/v1/admin/daydream</code>.</p><h2>Dashboard</h2><pre>%s</pre><h2>Proposals</h2><pre>%s</pre></body></html>`, namespaceID, namespaceID, string(metricsJSON), string(proposalsJSON))
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(page))
}

func (app *application) handleLegacyIngest(w http.ResponseWriter, r *http.Request) {
	actor := actorFromRequest(r)
	namespaceID := chi.URLParam(r, "namespaceId")
	if !maybeNamespaceAuthorized(w, r, namespaceID) {
		return
	}
	var body struct {
		Records []struct {
			Text     string            `json:"text"`
			Metadata map[string]string `json:"metadata"`
		} `json:"records"`
	}
	if err := decodeJSON(r, &body); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	count := 0
	ids := []string{}
	for _, record := range body.Records {
		memory, err := app.service.CreateMemory(r.Context(), actor, brain.MemoryPayload{NamespaceID: namespaceID, EntityType: record.Metadata["type"], Text: record.Text, Metadata: stringMapToAny(record.Metadata), Provenance: map[string]any{"kind": "legacy_rest"}, Identity: map[string]any{"createdVia": "legacy_rest"}})
		if err == nil {
			count++
			ids = append(ids, memory.ID)
		}
	}
	writeJSON(w, http.StatusCreated, map[string]any{"status": "ingested", "recordsIngested": count, "ids": ids})
}

func (app *application) handleLegacyQuery(w http.ResponseWriter, r *http.Request) {
	actor := actorFromRequest(r)
	namespaceID := chi.URLParam(r, "namespaceId")
	var body struct {
		Query string `json:"query"`
		Limit int    `json:"limit"`
	}
	if err := decodeJSON(r, &body); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	results, err := app.service.SearchMemories(r.Context(), actor, brain.SearchRequest{NamespaceID: namespaceID, Query: body.Query, Limit: body.Limit, Intent: "legacy_query"})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"records": results})
}

func (app *application) handleLegacyForget(w http.ResponseWriter, r *http.Request) {
	actor := actorFromRequest(r)
	namespaceID := chi.URLParam(r, "namespaceId")
	var body struct {
		RecordIDs []string `json:"record_ids"`
	}
	if err := decodeJSON(r, &body); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	forgotten, err := app.service.ForgetMemories(r.Context(), actor, namespaceID, body.RecordIDs)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"records_forgotten": forgotten})
}

func (app *application) handleLegacyCompileContext(w http.ResponseWriter, r *http.Request) {
	actor := actorFromRequest(r)
	namespaceID := chi.URLParam(r, "namespaceId")
	var req brain.ContextRequest
	if err := decodeJSON(r, &req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	req.NamespaceID = namespaceID
	resp, err := app.service.CompileContext(r.Context(), actor, req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func namespaceFromPayload(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func envDefault(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func stringMapToAny(value map[string]string) map[string]any {
	out := map[string]any{}
	for k, v := range value {
		out[k] = v
	}
	return out
}

func anyMapToString(value map[string]any) map[string]string {
	out := map[string]string{}
	for k, v := range value {
		out[k] = fmt.Sprint(v)
	}
	return out
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
