package client

import (
	"context"
	"fmt"
	"net/url"
)

func (c *Client) ListApprovals(ctx context.Context, companyID, status string) ([]Approval, error) {
	endpoint := fmt.Sprintf("/api/v1/companies/%s/approvals", companyID)
	if status != "" {
		endpoint += "?status=" + url.QueryEscape(status)
	}
	var approvals []Approval
	if err := c.DoReq(ctx, "GET", endpoint, nil, &approvals); err != nil {
		return nil, err
	}
	return approvals, nil
}

func (c *Client) GetApproval(ctx context.Context, approvalID string) (*Approval, error) {
	endpoint := fmt.Sprintf("/api/v1/approvals/%s", approvalID)
	var approval Approval
	if err := c.DoReq(ctx, "GET", endpoint, nil, &approval); err != nil {
		return nil, err
	}
	return &approval, nil
}
