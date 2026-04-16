package client

import (
	"context"
	"fmt"
)

func (c *Client) WakeupHeartbeat(ctx context.Context, req HeartbeatWakeupRequest) (*HeartbeatRun, error) {
	var run HeartbeatRun
	if err := c.DoReq(ctx, "POST", "/api/v1/heartbeat/wakeup", req, &run); err != nil {
		return nil, err
	}
	return &run, nil
}

func (c *Client) GetHeartbeatRun(ctx context.Context, runID string) (*HeartbeatRun, error) {
	endpoint := fmt.Sprintf("/api/v1/heartbeat-runs/%s", runID)
	var run HeartbeatRun
	if err := c.DoReq(ctx, "GET", endpoint, nil, &run); err != nil {
		return nil, err
	}
	return &run, nil
}

func (c *Client) ListHeartbeatRunEvents(ctx context.Context, runID string, afterSeq, limit int) ([]HeartbeatRunEvent, error) {
	endpoint := fmt.Sprintf("/api/v1/heartbeat-runs/%s/events?afterSeq=%d&limit=%d", runID, afterSeq, limit)
	var events []HeartbeatRunEvent
	if err := c.DoReq(ctx, "GET", endpoint, nil, &events); err != nil {
		return nil, err
	}
	return events, nil
}
