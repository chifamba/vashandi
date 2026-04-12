package jobs

import (
	"encoding/json"
	"log/slog"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/chifamba/vashandi/openbrain/db/models"
)

type CuratorJob struct {
	DB *gorm.DB
}

// HandleCuratorProposal is a job handler that gets triggered to find similar memories
// and generate a deduplication proposal.
func (c *CuratorJob) HandleCuratorProposal(payload []byte) error {
	slog.Info("Running Curator Agent job to find deduplication candidates", "payload", string(payload))

	var req struct {
		NamespaceID string `json:"namespace_id"`
	}
	if err := json.Unmarshal(payload, &req); err != nil {
		slog.Error("Failed to parse Curator job payload", "error", err)
		return err
	}

	// 1. Fetch recent embeddings for the namespace
	var memories []models.Memory
	if err := c.DB.Where("namespace_id = ?", req.NamespaceID).Order("created_at desc").Limit(50).Find(&memories).Error; err != nil {
		return err
	}

	// 2. Compute similarity matrix (mocked for now, as real pgvector similarity requires embeddings)
	if len(memories) >= 2 {
		memIDs := []string{memories[0].ID, memories[1].ID}
		memIDsBytes, _ := json.Marshal(memIDs)

		proposal := models.Proposal{
			ID:            uuid.New().String(),
			NamespaceID:   req.NamespaceID,
			MemoryIDs:     string(memIDsBytes),
			SuggestedText: memories[0].Text + "\n" + memories[1].Text,
			Status:        "pending",
		}

		if err := c.DB.Create(&proposal).Error; err != nil {
			slog.Error("Failed to create proposal", "error", err)
			return err
		}
		slog.Info("Curator proposal generated successfully", "namespace_id", req.NamespaceID, "proposal_id", proposal.ID)
	}

	return nil
}
