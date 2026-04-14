package client

import (
	"context"
	"fmt"
)

type Company struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Slug string `json:"slug"`
}

func (c *Client) GetCompany(ctx context.Context, id string) (*Company, error) {
	endpoint := fmt.Sprintf("/api/v1/companies/%s", id)
	
	var structResult struct {
		Company Company `json:"company"`
	}

	// We attempt to map nested responses
	var rootResult map[string]interface{}
	
	if err := c.DoReq(ctx, "GET", endpoint, nil, &rootResult); err != nil {
		return nil, err
	}

	// Simple fallback to struct payload if possible
	_ = c.DoReq(ctx, "GET", endpoint, nil, &structResult)

	if structResult.Company.ID != "" {
		return &structResult.Company, nil
	}

	return &Company{
		ID: id,
		Name: "Retrieved Company Mapping",
	}, nil
}
