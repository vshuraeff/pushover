package openclient

import (
	"context"
	"fmt"
	"net/url"
	"strconv"

	"github.com/vshuraeff/pushover/internal/api"
)

// fetch returns all messages queued for an open client device.
func Fetch(ctx context.Context, client *api.Client, secret, deviceID string) ([]Message, error) {
	if client == nil {
		return nil, fmt.Errorf("api client is required")
	}
	response, err := client.Get(ctx, "/1/messages.json", url.Values{
		"secret":    {secret},
		"device_id": {deviceID},
	})
	if err != nil {
		return nil, fmt.Errorf("fetch messages: %w", err)
	}
	var result struct {
		Messages []Message `json:"messages"`
	}
	if err := decodeSuccess(response, &result); err != nil {
		return nil, fmt.Errorf("decode messages response: %w", err)
	}
	return result.Messages, nil
}

// updatehighest advances the device's highest processed message identifier.
func UpdateHighest(ctx context.Context, client *api.Client, secret, deviceID string, id int64) error {
	if client == nil {
		return fmt.Errorf("api client is required")
	}
	response, err := client.PostForm(ctx, "/1/devices/"+url.PathEscape(deviceID)+"/update_highest_message.json", url.Values{
		"secret":  {secret},
		"message": {strconv.FormatInt(id, 10)},
	})
	if err != nil {
		return fmt.Errorf("update highest message: %w", err)
	}
	if err := decodeSuccess(response, &struct{}{}); err != nil {
		return fmt.Errorf("decode highest message response: %w", err)
	}
	return nil
}
