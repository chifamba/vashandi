package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"
	"google.golang.org/grpc"
	"gorm.io/gorm"

	"github.com/chifamba/vashandi/openbrain/db/models"
	pb "github.com/chifamba/vashandi/openbrain/proto/v1"
)

type memoryServer struct {
	pb.UnimplementedMemoryServiceServer
	db *gorm.DB
}

func (s *memoryServer) Ingest(ctx context.Context, req *pb.IngestRequest) (*pb.IngestResponse, error) {
	slog.Info("gRPC Ingest called", "namespace", req.NamespaceId, "records", len(req.Records))

	// Ensure namespace exists
	var ns models.Namespace
	if err := s.db.FirstOrCreate(&ns, models.Namespace{ID: req.NamespaceId, CompanyID: req.NamespaceId}).Error; err != nil {
		return nil, err
	}

	var count int32
	for _, rec := range req.Records {
		metaBytes, _ := json.Marshal(rec.Metadata)
		mem := models.Memory{
			ID:          uuid.New().String(),
			NamespaceID: req.NamespaceId,
			Text:        rec.Text,
			Metadata:    string(metaBytes),
		}
		if err := s.db.Create(&mem).Error; err == nil {
			count++
		}
	}

	return &pb.IngestResponse{RecordsIngested: count}, nil
}

func (s *memoryServer) Query(ctx context.Context, req *pb.QueryRequest) (*pb.QueryResponse, error) {
	slog.Info("gRPC Query called", "namespace", req.NamespaceId, "query", req.Query)

	var memories []models.Memory
	queryStr := "%" + req.Query + "%"

	if err := s.db.Where("namespace_id = ? AND text LIKE ?", req.NamespaceId, queryStr).Limit(int(req.Limit)).Find(&memories).Error; err != nil {
		return nil, err
	}

	var pbRecords []*pb.MemoryRecord
	for _, mem := range memories {
		var meta map[string]string
		json.Unmarshal([]byte(mem.Metadata), &meta)
		pbRecords = append(pbRecords, &pb.MemoryRecord{
			Id:       mem.ID,
			Text:     mem.Text,
			Metadata: meta,
		})
	}

	return &pb.QueryResponse{Records: pbRecords}, nil
}

func (s *memoryServer) Forget(ctx context.Context, req *pb.ForgetRequest) (*pb.ForgetResponse, error) {
	slog.Info("gRPC Forget called", "namespace", req.NamespaceId, "records", len(req.RecordIds))

	res := s.db.Where("namespace_id = ? AND id IN ?", req.NamespaceId, req.RecordIds).Delete(&models.Memory{})
	if res.Error != nil {
		return nil, res.Error
	}

	return &pb.ForgetResponse{RecordsForgotten: int32(res.RowsAffected)}, nil
}

func main() {
	fmt.Println("OpenBrain service starting...")
	slog.Info("Initializing database...")
	db := InitDB()
	slog.Info("Database initialized successfully", "db", db)

	// Start HTTP Server for REST API
	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(AuthMiddleware)

	// REST Ingest
	r.Post("/v1/namespaces/{namespaceId}/memories", func(w http.ResponseWriter, r *http.Request) {
		namespaceID := chi.URLParam(r, "namespaceId")

		var req struct {
			Records []struct {
				Text     string            `json:"text"`
				Metadata map[string]string `json:"metadata"`
			} `json:"records"`
		}

		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		var ns models.Namespace
		if err := db.FirstOrCreate(&ns, models.Namespace{ID: namespaceID, CompanyID: namespaceID}).Error; err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		for _, rec := range req.Records {
			metaBytes, _ := json.Marshal(rec.Metadata)
			mem := models.Memory{
				ID:          uuid.New().String(),
				NamespaceID: namespaceID,
				Text:        rec.Text,
				Metadata:    string(metaBytes),
			}
			db.Create(&mem)
		}

		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]string{"status": "ingested"})
	})

	// REST Query
	r.Post("/v1/namespaces/{namespaceId}/memories/query", func(w http.ResponseWriter, r *http.Request) {
		namespaceID := chi.URLParam(r, "namespaceId")

		var req struct {
			Query string `json:"query"`
			Limit int    `json:"limit"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		var memories []models.Memory
		queryStr := "%" + req.Query + "%"
		if req.Limit <= 0 {
			req.Limit = 10
		}

		if err := db.Where("namespace_id = ? AND text LIKE ?", namespaceID, queryStr).Limit(req.Limit).Find(&memories).Error; err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		type MemoryResult struct {
			ID       string            `json:"id"`
			Text     string            `json:"text"`
			Metadata map[string]string `json:"metadata"`
		}
		var results []MemoryResult
		for _, mem := range memories {
			var meta map[string]string
			json.Unmarshal([]byte(mem.Metadata), &meta)
			results = append(results, MemoryResult{
				ID:       mem.ID,
				Text:     mem.Text,
				Metadata: meta,
			})
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{"records": results})
	})

	// REST Forget
	// Context Engine
	r.Post("/v1/namespaces/{namespaceId}/context/compile", func(w http.ResponseWriter, r *http.Request) {
		namespaceID := chi.URLParam(r, "namespaceId")
		var req struct {
			AgentID     string `json:"agentId"`
			TaskQuery   string `json:"taskQuery"`
			Intent      string `json:"intent"`
			TokenBudget int    `json:"tokenBudget"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		var memories []models.Memory
		queryStr := "%" + req.TaskQuery + "%"
		limit := 50
		if err := db.Where("namespace_id = ? AND text LIKE ?", namespaceID, queryStr).Limit(limit).Find(&memories).Error; err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		type Snippet struct {
			ID   string `json:"id"`
			Text string `json:"text"`
		}
		var snippets []Snippet
		tokenCount := 0
		for _, mem := range memories {
			cost := len(mem.Text) / 4
			if cost == 0 {
				cost = 1
			}
			if req.TokenBudget > 0 && tokenCount+cost > req.TokenBudget {
				break
			}
			snippets = append(snippets, Snippet{ID: mem.ID, Text: mem.Text})
			tokenCount += cost
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{"snippets": snippets, "tokenCount": tokenCount, "latencyMs": 10})
	})

	r.Post("/v1/namespaces/{namespaceId}/triggers/run_start", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"status": "run_start_triggered"})
	})

	r.Post("/v1/namespaces/{namespaceId}/triggers/run_complete", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"status": "run_complete_triggered"})
	})

	r.Post("/v1/namespaces/{namespaceId}/triggers/checkout", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"status": "checkout_triggered"})
	})

	r.Delete("/v1/namespaces/{namespaceId}/memories", func(w http.ResponseWriter, r *http.Request) {
		namespaceID := chi.URLParam(r, "namespaceId")

		var req struct {
			RecordIDs []string `json:"record_ids"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		res := db.Where("namespace_id = ? AND id IN ?", namespaceID, req.RecordIDs).Delete(&models.Memory{})
		if res.Error != nil {
			http.Error(w, res.Error.Error(), http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{"records_forgotten": res.RowsAffected})
	})

	// Curator proposals endpoints
	r.Get("/v1/namespaces/{namespaceId}/proposals", func(w http.ResponseWriter, r *http.Request) {
		namespaceID := chi.URLParam(r, "namespaceId")
		var proposals []models.Proposal
		if err := db.Where("namespace_id = ? AND status = ?", namespaceID, "pending").Find(&proposals).Error; err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		type ProposalResult struct {
			ID            string   `json:"id"`
			NamespaceID   string   `json:"namespace_id"`
			MemoryIDs     []string `json:"memory_ids"`
			SuggestedText string   `json:"suggested_text"`
			Status        string   `json:"status"`
		}

		var results []ProposalResult
		for _, p := range proposals {
			var mIDs []string
			json.Unmarshal([]byte(p.MemoryIDs), &mIDs)
			results = append(results, ProposalResult{
				ID:            p.ID,
				NamespaceID:   p.NamespaceID,
				MemoryIDs:     mIDs,
				SuggestedText: p.SuggestedText,
				Status:        p.Status,
			})
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(results)
	})

	r.Post("/v1/namespaces/{namespaceId}/proposals/{proposalId}/resolve", func(w http.ResponseWriter, r *http.Request) {
		namespaceID := chi.URLParam(r, "namespaceId")
		proposalID := chi.URLParam(r, "proposalId")

		var req struct {
			Action string `json:"action"` // approve or reject
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		var proposal models.Proposal
		if err := db.Where("id = ? AND namespace_id = ?", proposalID, namespaceID).First(&proposal).Error; err != nil {
			http.Error(w, "Proposal not found", http.StatusNotFound)
			return
		}

		if req.Action == "approve" {
			var mIDs []string
			json.Unmarshal([]byte(proposal.MemoryIDs), &mIDs)

			// Forget old memories
			db.Where("namespace_id = ? AND id IN ?", namespaceID, mIDs).Delete(&models.Memory{})

			// Ingest new memory
			metaBytes, _ := json.Marshal(map[string]string{"type": "deduplicated", "source_proposal": proposalID})
			mem := models.Memory{
				ID:          uuid.New().String(),
				NamespaceID: namespaceID,
				Text:        proposal.SuggestedText,
				Metadata:    string(metaBytes),
			}
			db.Create(&mem)
		}

		proposal.Status = req.Action
		db.Save(&proposal)

		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"status": req.Action})
	})

	// Vashandi Sync events (Task 2.3)
	r.Post("/internal/v1/namespaces/{namespaceId}/agents", func(w http.ResponseWriter, r *http.Request) {
		namespaceID := chi.URLParam(r, "namespaceId")
		slog.Info("Internal Agent Created Webhook", "namespaceId", namespaceID)
		w.WriteHeader(http.StatusOK)
	})
	r.Delete("/internal/v1/namespaces/{namespaceId}/agents/{agentId}", func(w http.ResponseWriter, r *http.Request) {
		namespaceID := chi.URLParam(r, "namespaceId")
		agentID := chi.URLParam(r, "agentId")
		slog.Info("Internal Agent Deleted Webhook", "namespaceId", namespaceID, "agentId", agentID)
		w.WriteHeader(http.StatusOK)
	})

	httpPort := os.Getenv("PORT")
	if httpPort == "" {
		httpPort = "3101"
	}

	go func() {
		slog.Info("Starting REST server", "port", httpPort)
		if err := http.ListenAndServe(":"+httpPort, r); err != nil {
			slog.Error("REST server failed", "error", err)
		}
	}()

	// Start gRPC Server
	grpcPort := os.Getenv("GRPC_PORT")
	if grpcPort == "" {
		grpcPort = "50051"
	}

	lis, err := net.Listen("tcp", ":"+grpcPort)
	if err != nil {
		slog.Error("Failed to listen for gRPC", "error", err)
		os.Exit(1)
	}

	grpcServer := grpc.NewServer()
	pb.RegisterMemoryServiceServer(grpcServer, &memoryServer{db: db})

	slog.Info("Starting gRPC server", "port", grpcPort)
	if err := grpcServer.Serve(lis); err != nil {
		slog.Error("gRPC server failed", "error", err)
	}
}
