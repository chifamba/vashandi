package main

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
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
	"google.golang.org/grpc/credentials"
	"gorm.io/gorm"

	"github.com/chifamba/paperclip/backend/shared/tls"
	"github.com/chifamba/vashandi/openbrain/db/models"
	"github.com/chifamba/vashandi/openbrain/internal/brain"
	mcppkg "github.com/chifamba/vashandi/openbrain/internal/mcp"
	pb "github.com/chifamba/vashandi/openbrain/proto/v1"
	adminui "github.com/chifamba/vashandi/openbrain/ui"
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
	// No subcommand means container/server mode; CLI actions are always invoked explicitly.
	if err := runServer(); err != nil {
		slog.Error("openbrain server failed", "error", err)
		os.Exit(1)
	}
}

func runServer() error {
	embedding := brain.InitEmbeddingProvider()
	service := brain.NewService(db, embedding)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	service.StartBackgroundJobs(ctx)
	app := &application{service: service, mcpServer: mcppkg.NewServerWithActorExtractor(service, func(r *http.Request) (brain.Actor, bool) {
		actor := actorFromRequest(r)
		return actor, true
	})}

	router := app.routes()
	tlsCfg := tls.LoadConfigFromEnv()
	tlsConfig, err := tls.GetServerConfig(ctx, tlsCfg)
	if err != nil {
		slog.Error("failed to load TLS configuration", "error", err)
		if tlsCfg.Enforced {
			return err
		}
	}

	httpServer := &http.Server{
		Addr:              ":" + httpPort,
		Handler:           router,
		ReadHeaderTimeout: 10 * time.Second,
		TLSConfig:         tlsConfig,
	}

	httpErr := make(chan error, 1)
	go func() {
		if tlsConfig != nil {
			slog.Info("starting HTTPS/mTLS server", "port", httpPort, "enforced", tlsCfg.Enforced)
			httpErr <- httpServer.ListenAndServeTLS("", "")
		} else {
			slog.Info("starting REST server (plain HTTP)", "port", httpPort)
			httpErr <- httpServer.ListenAndServe()
		}
	}()

	grpcPort := envDefault("GRPC_PORT", "50051")
	lis, err := net.Listen("tcp", ":"+grpcPort)
	if err != nil {
		return err
	}
	var opts []grpc.ServerOption
	if tlsConfig != nil {
		opts = append(opts, grpc.Creds(credentials.NewTLS(tlsConfig)))
	}
	grpcServer := grpc.NewServer(opts...)
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
			api.Post("/memories/edges", app.handleCreateEdge)
			api.Delete("/memories/edges/{edgeId}", app.handleDeleteEdge)
			api.Get("/memories/{memoryId}", app.handleGetMemory)
			api.Patch("/memories/{memoryId}", app.handleUpdateMemory)
			api.Delete("/memories/{memoryId}", app.handleDeleteMemory)
			api.Get("/memories/{memoryId}/versions", app.handleListVersions)
			api.Post("/memories/{memoryId}/rollback", app.handleRollbackMemory)
			api.Get("/memories/{memoryId}/edges", app.handleGetEdges)
			api.Post("/context/compile", app.handleCompileContext)
			api.Get("/context/pending", app.handlePendingContext)
			api.Get("/audit/log", app.handleAuditLog)
			api.Get("/audit/export", app.handleAuditExport)
			api.Get("/agents", app.handleListAgents)
			api.Get("/agents/{agentId}", app.handleGetAgent)
			api.Patch("/agents/{agentId}", app.handleUpdateAgent)
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
		protected.Route("/internal/v1", func(internal chi.Router) {
			internal.Post("/namespaces", app.handleCreateNamespace)
			internal.Delete("/namespaces/{namespaceId}", app.handleArchiveNamespace)
			internal.Route("/namespaces/{namespaceId}", func(ns chi.Router) {
				ns.Post("/agents", app.handleRegisterAgent)
				ns.Delete("/agents/{agentId}", app.handleDeregisterAgent)
				ns.Post("/triggers/{triggerType}", app.handleTrigger)
				ns.Post("/sync", app.handleSync)
			})
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

	// Admin UI — served without server-side auth so the login screen is
	// accessible. The React SPA itself prompts for a bearer token, stores it
	// in sessionStorage, and includes it in every API call.
	distFS, err := fs.Sub(adminui.FS, "dist")
	if err != nil {
		panic("openbrain: failed to sub admin UI embed: " + err.Error())
	}
	fileServer := http.FileServer(http.FS(distFS))
	// Redirect /admin → /admin/ so relative asset paths resolve correctly.
	r.Get("/admin", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/admin/", http.StatusTemporaryRedirect)
	})
	r.Handle("/admin/*", http.StripPrefix("/admin", fileServer))

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

func (app *application) handleDeleteEdge(w http.ResponseWriter, r *http.Request) {
	actor := actorFromRequest(r)
	namespaceID := namespaceFromPayload(r.URL.Query().Get("namespaceId"), actor.NamespaceID)
	if !maybeNamespaceAuthorized(w, r, namespaceID) {
		return
	}
	if err := app.service.DeleteEdge(r.Context(), actor, namespaceID, chi.URLParam(r, "edgeId")); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			http.Error(w, err.Error(), http.StatusNotFound)
		} else {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "deleted"})
}

func (app *application) handleListAgents(w http.ResponseWriter, r *http.Request) {
	actor := actorFromRequest(r)
	namespaceID := namespaceFromPayload(r.URL.Query().Get("namespaceId"), actor.NamespaceID)
	if !maybeNamespaceAuthorized(w, r, namespaceID) {
		return
	}
	agents, err := app.service.ListAgents(r.Context(), actor, namespaceID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, agents)
}

func (app *application) handleGetAgent(w http.ResponseWriter, r *http.Request) {
	actor := actorFromRequest(r)
	namespaceID := namespaceFromPayload(r.URL.Query().Get("namespaceId"), actor.NamespaceID)
	if !maybeNamespaceAuthorized(w, r, namespaceID) {
		return
	}
	agent, err := app.service.GetRegisteredAgent(r.Context(), namespaceID, chi.URLParam(r, "agentId"))
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			http.Error(w, err.Error(), http.StatusNotFound)
		} else {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
		return
	}
	writeJSON(w, http.StatusOK, agent)
}

func (app *application) handleUpdateAgent(w http.ResponseWriter, r *http.Request) {
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
	agent, err := app.service.UpdateAgent(r.Context(), actor, namespaceID, chi.URLParam(r, "agentId"), patch)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}
	writeJSON(w, http.StatusOK, agent)
}

func (app *application) handleCreateNamespace(w http.ResponseWriter, r *http.Request) {
	actor := actorFromRequest(r)
	var body struct {
		NamespaceID string         `json:"namespaceId"`
		CompanyID   string         `json:"companyId"`
		Settings    map[string]any `json:"settings"`
	}
	if err := decodeJSON(r, &body); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	ns, err := app.service.CreateNamespace(r.Context(), actor, body.NamespaceID, body.CompanyID, body.Settings)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}
	writeJSON(w, http.StatusCreated, ns)
}

func (app *application) handleArchiveNamespace(w http.ResponseWriter, r *http.Request) {
	actor := actorFromRequest(r)
	namespaceID := chi.URLParam(r, "namespaceId")
	if !maybeNamespaceAuthorized(w, r, namespaceID) {
		return
	}
	export, err := app.service.ArchiveNamespace(r.Context(), actor, namespaceID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Disposition", `attachment; filename="namespace-`+namespaceID+`-export.json"`)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(export)
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
	// Redirect to the React admin UI.
	http.Redirect(w, r, "/admin/", http.StatusFound)
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
