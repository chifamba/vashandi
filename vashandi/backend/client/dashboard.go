package client

import (
	"context"
	"fmt"
)

func (c *Client) GetDashboard(ctx context.Context, companyID string) (*DashboardSummary, error) {
	endpoint := fmt.Sprintf("/api/v1/companies/%s/dashboard", companyID)
	var summary DashboardSummary
	if err := c.DoReq(ctx, "GET", endpoint, nil, &summary); err != nil {
		return nil, err
	}
	return &summary, nil
}

func (c *Client) GetPlatformMetrics(ctx context.Context) (*PlatformMetrics, error) {
	var metrics PlatformMetrics
	if err := c.DoReq(ctx, "GET", "/api/v1/platform/metrics", nil, &metrics); err != nil {
		return nil, err
	}
	return &metrics, nil
}
