package client

import (
	"context"
	"fmt"
	"net/url"
)

type ActivityListOptions struct {
	AgentID    string
	EntityType string
	EntityID   string
}

func (c *Client) ListActivity(ctx context.Context, companyID string, opts ActivityListOptions) ([]ActivityLog, error) {
	endpoint := fmt.Sprintf("/api/v1/companies/%s/activity", companyID)
	params := url.Values{}
	if opts.AgentID != "" {
		params.Set("agentId", opts.AgentID)
	}
	if opts.EntityType != "" {
		params.Set("entityType", opts.EntityType)
	}
	if opts.EntityID != "" {
		params.Set("entityId", opts.EntityID)
	}
	if encoded := params.Encode(); encoded != "" {
		endpoint += "?" + encoded
	}

	var activity []ActivityLog
	if err := c.DoReq(ctx, "GET", endpoint, nil, &activity); err != nil {
		return nil, err
	}
	return activity, nil
}

func (c *Client) GetActivity(ctx context.Context, activityID string) (*ActivityLog, error) {
	endpoint := fmt.Sprintf("/api/v1/activity/%s", activityID)
	var activity ActivityLog
	if err := c.DoReq(ctx, "GET", endpoint, nil, &activity); err != nil {
		return nil, err
	}
	return &activity, nil
}
