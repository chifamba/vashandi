package jobs

import (
	"context"
	"encoding/json"

	"github.com/chifamba/vashandi/openbrain/internal/brain"
)

type CuratorJob struct {
	Service *brain.Service
}

func (c *CuratorJob) HandleCuratorProposal(payload []byte) error {
	var req struct {
		NamespaceID string `json:"namespace_id"`
	}
	if err := json.Unmarshal(payload, &req); err != nil {
		return err
	}
	_, err := c.Service.GenerateCuratorProposals(context.Background(), req.NamespaceID, brain.Actor{Kind: "curator", NamespaceID: req.NamespaceID, TrustTier: 4})
	return err
}
