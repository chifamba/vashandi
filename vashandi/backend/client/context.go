package client

import (
	"context"
	"fmt"
)

func (c *Client) ListContextOperations(ctx context.Context, companyID string) ([]ContextOperation, error) {
	endpoint := fmt.Sprintf("/api/v1/companies/%s/context", companyID)
	var operations []ContextOperation
	if err := c.DoReq(ctx, "GET", endpoint, nil, &operations); err != nil {
		return nil, err
	}
	return operations, nil
}

func (c *Client) GetContextOperation(ctx context.Context, companyID, operation string) (*ContextOperation, error) {
	endpoint := fmt.Sprintf("/api/v1/companies/%s/context/%s", companyID, operation)
	var op ContextOperation
	if err := c.DoReq(ctx, "GET", endpoint, nil, &op); err != nil {
		return nil, err
	}
	return &op, nil
}
