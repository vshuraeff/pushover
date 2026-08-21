package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"unicode/utf8"
)

// glances service sends glance updates.
type GlancesService struct {
	client *Client
}

// new glances service constructs a glances service backed by client.
func NewGlancesService(client *Client) *GlancesService {
	return &GlancesService{client: client}
}

// glance request contains the update fields to send to pushover.
type GlanceRequest struct {
	Token  string
	User   string
	Device string

	Title   *string
	Text    *string
	Subtext *string
	Count   *int
	Percent *int
}

// glance response contains the result returned by pushover.
type GlanceResponse struct {
	Status  int    `json:"status"`
	Request string `json:"request"`
}

// send validates and sends a glance update.
func (s *GlancesService) Send(ctx context.Context, request GlanceRequest) (*GlanceResponse, error) {
	if err := validateGlanceString("title", request.Title); err != nil {
		return nil, err
	}
	if err := validateGlanceString("text", request.Text); err != nil {
		return nil, err
	}
	if err := validateGlanceString("subtext", request.Subtext); err != nil {
		return nil, err
	}
	if request.Percent != nil && (*request.Percent < 0 || *request.Percent > 100) {
		return nil, fmt.Errorf("percent must be between 0 and 100")
	}

	values := url.Values{
		"token": {request.Token},
		"user":  {request.User},
	}
	if request.Device != "" {
		values.Set("device", request.Device)
	}
	if request.Title != nil {
		values.Set("title", *request.Title)
	}
	if request.Text != nil {
		values.Set("text", *request.Text)
	}
	if request.Subtext != nil {
		values.Set("subtext", *request.Subtext)
	}
	if request.Count != nil {
		values.Set("count", strconv.Itoa(*request.Count))
	}
	if request.Percent != nil {
		values.Set("percent", strconv.Itoa(*request.Percent))
	}

	response, requestErr := s.client.PostForm(ctx, "/1/glances.json", values)
	if response == nil {
		return nil, requestErr
	}

	var result GlanceResponse
	if err := json.Unmarshal(response.Body, &result); err != nil {
		return nil, fmt.Errorf("decode glances send response: %w", err)
	}
	return &result, requestErr
}

func validateGlanceString(name string, value *string) error {
	if value != nil && utf8.RuneCountInString(*value) > 100 {
		return fmt.Errorf("%s must be at most 100 characters", name)
	}
	return nil
}
