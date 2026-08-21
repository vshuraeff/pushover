package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
)

// users service validates user and group keys.
type UsersService struct {
	client *Client
}

// new users service constructs a users service backed by client.
func NewUsersService(client *Client) *UsersService {
	return &UsersService{client: client}
}

// validate request contains the credentials and optional device to validate.
type ValidateRequest struct {
	Token  string
	User   string
	Device string
}

// validate response contains the validation result returned by pushover.
type ValidateResponse struct {
	Status   int           `json:"status"`
	Request  string        `json:"request"`
	Devices  []string      `json:"devices,omitempty"`
	Licenses []string      `json:"licenses,omitempty"`
	Errors   ErrorMessages `json:"errors,omitempty"`
}

// validate returns a decoded response together with any API validation error.
func (s *UsersService) Validate(ctx context.Context, request ValidateRequest) (*ValidateResponse, error) {
	values := url.Values{
		"token": {request.Token},
		"user":  {request.User},
	}
	if request.Device != "" {
		values.Set("device", request.Device)
	}

	response, requestErr := s.client.PostForm(ctx, "/1/users/validate.json", values)
	if response == nil {
		return nil, requestErr
	}

	var result ValidateResponse
	if err := json.Unmarshal(response.Body, &result); err != nil {
		return nil, fmt.Errorf("decode users validate response: %w", err)
	}
	return &result, requestErr
}
