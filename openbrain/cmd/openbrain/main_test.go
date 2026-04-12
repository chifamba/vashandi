package main

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"github.com/pgvector/pgvector-go"

	"github.com/chifamba/vashandi/openbrain/db/models"
	pb "github.com/chifamba/vashandi/openbrain/proto/v1"
)

func setupTestDB() *gorm.DB {
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		panic(err)
	}
	db.AutoMigrate(&models.Namespace{}, &models.Memory{}, &models.Edge{}, &models.Proposal{})
	return db
}

func TestMemoryServer(t *testing.T) {
	db := setupTestDB()
	server := &memoryServer{db: db}

	ns := "test_namespace_123"

	// Cleanup before
	db.Where("namespace_id = ?", ns).Delete(&models.Memory{})
	db.Where("id = ?", ns).Delete(&models.Namespace{})

	t.Run("Ingest", func(t *testing.T) {
		req := &pb.IngestRequest{
			NamespaceId: ns,
			Records: []*pb.MemoryRecord{
				{Text: "Test memory alpha", Metadata: map[string]string{"type": "alpha"}},
				{Text: "Test memory beta", Metadata: map[string]string{"type": "beta"}},
			},
		}

		res, err := server.Ingest(context.Background(), req)
		assert.NoError(t, err)
		assert.Equal(t, int32(2), res.RecordsIngested)

		// Fix empty embedding for SQLite scan
		db.Model(&models.Memory{}).Where("namespace_id = ?", ns).Update("embedding", pgvector.NewVector([]float32{0.0}))
	})

	t.Run("Query", func(t *testing.T) {
		req := &pb.QueryRequest{
			NamespaceId: ns,
			Query:       "alpha",
			Limit:       10,
		}

		res, err := server.Query(context.Background(), req)
		assert.NoError(t, err)
		if res != nil {
			assert.Len(t, res.Records, 1)
			assert.Equal(t, "Test memory alpha", res.Records[0].Text)
			assert.Equal(t, "alpha", res.Records[0].Metadata["type"])
		}
	})

	t.Run("Forget", func(t *testing.T) {
		// First query to get the ID
		reqQuery := &pb.QueryRequest{
			NamespaceId: ns,
			Query:       "beta",
			Limit:       10,
		}
		resQuery, _ := server.Query(context.Background(), reqQuery)
		if resQuery != nil && len(resQuery.Records) > 0 {
			assert.Len(t, resQuery.Records, 1)
			idToForget := resQuery.Records[0].Id

			reqForget := &pb.ForgetRequest{
				NamespaceId: ns,
				RecordIds:   []string{idToForget},
			}

			resForget, err := server.Forget(context.Background(), reqForget)
			assert.NoError(t, err)
			assert.Equal(t, int32(1), resForget.RecordsForgotten)

			// Verify
			resQuery2, _ := server.Query(context.Background(), reqQuery)
			assert.Len(t, resQuery2.Records, 0)
		}
	})

	// Cleanup after
	db.Where("namespace_id = ?", ns).Delete(&models.Memory{})
	db.Where("id = ?", ns).Delete(&models.Namespace{})
}
