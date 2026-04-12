package main

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/chifamba/vashandi/openbrain/internal/brain"
	pb "github.com/chifamba/vashandi/openbrain/proto/v1"
)

func setupService(t *testing.T) *brain.Service {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	service := brain.NewService(db)
	require.NoError(t, service.AutoMigrate())
	return service
}

func TestMemoryServer(t *testing.T) {
	service := setupService(t)
	server := &memoryServer{service: service}
	ns := "test-namespace"

	res, err := server.Ingest(context.Background(), &pb.IngestRequest{NamespaceId: ns, Records: []*pb.MemoryRecord{{Text: "Test memory alpha", Metadata: map[string]string{"type": "fact"}}, {Text: "Test memory beta", Metadata: map[string]string{"type": "note"}}}})
	require.NoError(t, err)
	assert.Equal(t, int32(2), res.RecordsIngested)

	queryRes, err := server.Query(context.Background(), &pb.QueryRequest{NamespaceId: ns, Query: "alpha", Limit: 10})
	require.NoError(t, err)
	require.NotEmpty(t, queryRes.Records)
	assert.Equal(t, "Test memory alpha", queryRes.Records[0].Text)

	forgetRes, err := server.Forget(context.Background(), &pb.ForgetRequest{NamespaceId: ns, RecordIds: []string{queryRes.Records[0].Id}})
	require.NoError(t, err)
	assert.Equal(t, int32(1), forgetRes.RecordsForgotten)
}
