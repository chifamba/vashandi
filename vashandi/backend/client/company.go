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
	endpoint := fmt.Sprintf("/companies/%s", id)
	var company Company
	if err := c.DoReq(ctx, "GET", endpoint, nil, &company); err != nil {
		return nil, err
	}
	return &company, nil
}

func (c *Client) ListCompanies(ctx context.Context) ([]Company, error) {
	var companies []Company
	if err := c.DoReq(ctx, "GET", "/companies", nil, &companies); err != nil {
		return nil, err
	}
	return companies, nil
}
