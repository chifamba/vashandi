package jobs
import (
	"encoding/json"
	"log/slog"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"github.com/chifamba/vashandi/openbrain/db/models"
)
type CuratorJob struct { DB *gorm.DB }
func (c *CuratorJob) HandleCuratorProposal(payload []byte) error {
	var req struct { NamespaceID string `json:"namespace_id"` }
	json.Unmarshal(payload, &req)
	var memories []struct { ID string; Text string }
	if err := c.DB.Table("memories").Select("id, text").Where("namespace_id = ?", req.NamespaceID).Limit(50).Find(&memories).Error; err != nil { return err }
	if len(memories) >= 2 {
		memIDsBytes, _ := json.Marshal([]string{memories[0].ID, memories[1].ID})
		proposal := models.Proposal{
			ID: uuid.New().String(), NamespaceID: req.NamespaceID, MemoryIDs: string(memIDsBytes),
			SuggestedText: memories[0].Text + "\n" + memories[1].Text, Status: "pending",
		}
		if err := c.DB.Create(&proposal).Error; err != nil { return err }
		slog.Info("Curator proposal generated successfully")
	}
	return nil
}
