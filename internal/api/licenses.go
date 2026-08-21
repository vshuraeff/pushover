package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
)

// assign license spends a license credit to assign it to exactly one recipient.
func (c *Client) AssignLicense(ctx context.Context, token, user, email, operatingSystem string) (int, error) {
	if (user == "") == (email == "") {
		return 0, fmt.Errorf("exactly one of user or email is required")
	}
	if operatingSystem != "" && operatingSystem != "Android" && operatingSystem != "iOS" && operatingSystem != "Desktop" {
		return 0, fmt.Errorf("invalid license operating system %q; must be Android, iOS, or Desktop", operatingSystem)
	}

	values := url.Values{"token": {token}}
	if user != "" {
		values.Set("user", user)
	} else {
		values.Set("email", email)
	}
	if operatingSystem != "" {
		values.Set("os", operatingSystem)
	}

	response, err := c.PostForm(ctx, "/1/licenses/assign.json", values, NoRetry())
	if err != nil {
		return 0, fmt.Errorf("assign license: %w", err)
	}

	credits, err := decodeLicenseCredits(response.Body)
	if err != nil {
		return 0, fmt.Errorf("decode license assignment response: %w", err)
	}
	return credits, nil
}

// license credits returns the application's remaining license credits.
func (c *Client) LicenseCredits(ctx context.Context, token string) (int, error) {
	response, err := c.Get(ctx, "/1/licenses.json", url.Values{"token": {token}})
	if err != nil {
		return 0, fmt.Errorf("get license credits: %w", err)
	}

	credits, err := decodeLicenseCredits(response.Body)
	if err != nil {
		return 0, fmt.Errorf("decode license credits response: %w", err)
	}
	return credits, nil
}

type licenseCreditsResponse struct {
	Credits int `json:"credits"`
}

func decodeLicenseCredits(body []byte) (int, error) {
	var result licenseCreditsResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return 0, err
	}
	return result.Credits, nil
}
