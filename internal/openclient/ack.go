package openclient

import (
	"context"
	"fmt"
	"net/url"

	"github.com/vshuraeff/pushover/internal/api"
)

// acknowledge acknowledges one receipt for the current open client user.
func Acknowledge(ctx context.Context, client *api.Client, secret, receiptID string) error {
	if client == nil {
		return fmt.Errorf("api client is required")
	}
	response, err := client.PostForm(ctx, "/1/receipts/"+url.PathEscape(receiptID)+"/acknowledge.json", url.Values{
		"secret": {secret},
	})
	if err != nil {
		return fmt.Errorf("acknowledge receipt: %w", err)
	}
	if err := decodeSuccess(response, &struct{}{}); err != nil {
		return fmt.Errorf("decode receipt acknowledgement response: %w", err)
	}
	return nil
}
