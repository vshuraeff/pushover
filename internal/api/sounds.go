package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
)

// sounds service lists sounds available to an application.
type SoundsService struct {
	client *Client
}

// new sounds service constructs a sounds service backed by client.
func NewSoundsService(client *Client) *SoundsService {
	return &SoundsService{client: client}
}

// sounds response contains the sounds returned by pushover.
type SoundsResponse struct {
	Status  int               `json:"status"`
	Sounds  map[string]string `json:"sounds"`
	Request string            `json:"request"`
}

// list returns the built-in and account-specific sounds available to token.
func (s *SoundsService) List(ctx context.Context, token string) (*SoundsResponse, error) {
	response, requestErr := s.client.Get(ctx, "/1/sounds.json", url.Values{"token": {token}})
	if response == nil {
		return nil, requestErr
	}

	var result SoundsResponse
	if err := json.Unmarshal(response.Body, &result); err != nil {
		return nil, fmt.Errorf("decode sounds list response: %w", err)
	}
	return &result, requestErr
}
