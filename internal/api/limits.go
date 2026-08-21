package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
)

type AppLimits struct {
	Status    int        `json:"status"`
	Request   string     `json:"request"`
	Limit     int        `json:"limit"`
	Remaining int        `json:"remaining"`
	Reset     int64      `json:"reset"`
	Header    RateLimits `json:"-"`
}

func (c *Client) AppLimits(ctx context.Context, token string, options ...RequestOption) (*AppLimits, error) {
	response, err := c.Get(ctx, "/1/apps/limits.json", url.Values{"token": {token}}, options...)
	if err != nil {
		return nil, err
	}

	var result AppLimits
	if err := json.Unmarshal(response.Body, &result); err != nil {
		return nil, fmt.Errorf("decode app limits response: %w", err)
	}
	result.Header = response.RateLimits
	return &result, nil
}
