package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"time"
)

type Receipt struct {
	Status          int    `json:"status"`
	Acknowledged    int    `json:"acknowledged"`
	AcknowledgedAt  int64  `json:"acknowledged_at"`
	AcknowledgedBy  string `json:"acknowledged_by"`
	LastDeliveredAt int64  `json:"last_delivered_at"`
	Expired         int    `json:"expired"`
	ExpiresAt       int64  `json:"expires_at"`
	CalledBack      int    `json:"called_back"`
	CalledBackAt    int64  `json:"called_back_at"`
	Cancelled       bool   `json:"cancelled"`
	CancelledAt     int64  `json:"cancelled_at"`
	CancelledByTag  bool   `json:"cancelled_by_tag"`
}

func (c *Client) ReceiptStatus(ctx context.Context, token, receipt string, options ...RequestOption) (*Receipt, error) {
	response, err := c.Get(ctx, "1/receipts/"+url.PathEscape(receipt)+".json", url.Values{"token": {token}}, options...)
	if err != nil {
		return nil, err
	}

	var result Receipt
	if err := json.Unmarshal(response.Body, &result); err != nil {
		return nil, fmt.Errorf("decode receipt status response: %w", err)
	}
	return &result, nil
}

func (c *Client) CancelReceipt(ctx context.Context, token, receipt string, options ...RequestOption) error {
	_, err := c.PostForm(ctx, "1/receipts/"+url.PathEscape(receipt)+"/cancel.json", url.Values{"token": {token}}, options...)
	return err
}

func (c *Client) CancelReceiptsByTag(ctx context.Context, token, tag string, options ...RequestOption) (int, error) {
	response, err := c.PostForm(ctx, "1/receipts/cancel_by_tag/"+url.PathEscape(tag)+".json", url.Values{"token": {token}}, options...)
	if err != nil {
		return 0, err
	}

	var result struct {
		Cancelled int `json:"cancelled"`
	}
	if err := json.Unmarshal(response.Body, &result); err != nil {
		return 0, fmt.Errorf("decode cancel receipts response: %w", err)
	}
	return result.Cancelled, nil
}

// poll receipt observes a receipt with a five-second minimum interval.
func (c *Client) PollReceipt(ctx context.Context, token, receipt string, stop func(*Receipt) bool, sleep func(time.Duration), options ...RequestOption) (*Receipt, error) {
	if stop == nil {
		return nil, fmt.Errorf("receipt polling stop function is required")
	}
	pollOptions := append([]RequestOption{NoRetry()}, options...)
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		status, err := c.ReceiptStatus(ctx, token, receipt, pollOptions...)
		if err != nil {
			return nil, err
		}
		if stop(status) {
			return status, nil
		}
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if sleep == nil {
			timer := time.NewTimer(5 * time.Second)
			select {
			case <-ctx.Done():
				if !timer.Stop() {
					select {
					case <-timer.C:
					default:
					}
				}
				return nil, ctx.Err()
			case <-timer.C:
			}
			continue
		}
		sleep(5 * time.Second)
		if err := ctx.Err(); err != nil {
			return nil, err
		}
	}
}
