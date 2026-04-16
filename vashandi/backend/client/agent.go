package client

import (
	"context"
	"fmt"
)

func (c *Client) ListAgents(ctx context.Context, companyID string) ([]Agent, error) {
	endpoint := fmt.Sprintf("/api/v1/companies/%s/agents", companyID)
	var agents []Agent
	if err := c.DoReq(ctx, "GET", endpoint, nil, &agents); err != nil {
		return nil, err
	}
	return agents, nil
}

func (c *Client) GetAgent(ctx context.Context, agentID string) (*Agent, error) {
	endpoint := fmt.Sprintf("/api/v1/agents/%s", agentID)
	var agent Agent
	if err := c.DoReq(ctx, "GET", endpoint, nil, &agent); err != nil {
		return nil, err
	}
	return &agent, nil
}
