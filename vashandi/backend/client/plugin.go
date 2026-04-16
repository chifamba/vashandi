package client

import (
	"context"
	"fmt"
	"net/url"
)

func (c *Client) ListPlugins(ctx context.Context, status string) ([]Plugin, error) {
	endpoint := "/api/v1/plugins"
	if status != "" {
		endpoint += "?status=" + url.QueryEscape(status)
	}
	var plugins []Plugin
	if err := c.DoReq(ctx, "GET", endpoint, nil, &plugins); err != nil {
		return nil, err
	}
	return plugins, nil
}

func (c *Client) GetPlugin(ctx context.Context, pluginID string) (*Plugin, error) {
	endpoint := fmt.Sprintf("/api/v1/plugins/%s", pluginID)
	var plugin Plugin
	if err := c.DoReq(ctx, "GET", endpoint, nil, &plugin); err != nil {
		return nil, err
	}
	return &plugin, nil
}
