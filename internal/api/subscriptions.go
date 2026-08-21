package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
)

// migrate subscription migrates a subscription and returns its user key.
func (c *Client) MigrateSubscription(ctx context.Context, token, subscription, user, deviceName, sound string) (string, error) {
	values := url.Values{
		"token":        {token},
		"subscription": {subscription},
		"user":         {user},
	}
	if deviceName != "" {
		values.Set("device_name", deviceName)
	}
	if sound != "" {
		values.Set("sound", sound)
	}

	response, err := c.PostForm(ctx, "/1/subscriptions/migrate.json", values)
	if err != nil {
		return "", fmt.Errorf("migrate subscription: %w", err)
	}

	var result subscriptionMigrationResponse
	if err := json.Unmarshal(response.Body, &result); err != nil {
		return "", fmt.Errorf("decode subscription migration response: %w", err)
	}
	return result.SubscribedUserKey, nil
}

type subscriptionMigrationResponse struct {
	SubscribedUserKey string `json:"subscribed_user_key"`
}
