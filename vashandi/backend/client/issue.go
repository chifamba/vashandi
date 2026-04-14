package client

import (
	"context"
	"fmt"
)

type Issue struct {
	ID     string `json:"id"`
	Status string `json:"status"`
	Title  string `json:"title"`
}

type UpdateIssueBody struct {
	Status string `json:"status,omitempty"`
	AssigneeId string `json:"assigneeId,omitempty"`
}

func (c *Client) UpdateIssueStatus(ctx context.Context, issueID, newStatus string) (*Issue, error) {
	endpoint := fmt.Sprintf("/api/v1/issues/%s", issueID)
	body := UpdateIssueBody{
		Status: newStatus,
	}

	var issue Issue
	if err := c.DoReq(ctx, "PATCH", endpoint, body, &issue); err != nil {
		return nil, err
	}
	return &issue, nil
}
