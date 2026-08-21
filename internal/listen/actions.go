package listen

import (
	"context"
	"log/slog"

	"github.com/vshuraeff/pushover/internal/api"
	"github.com/vshuraeff/pushover/internal/notify"
	"github.com/vshuraeff/pushover/internal/openclient"
)

func runActions(
	ctx context.Context,
	actions <-chan notify.ActionEvent,
	client *api.Client,
	secret string,
	deviceID string,
	trigger func(),
	logger *slog.Logger,
) {
	for {
		select {
		case <-ctx.Done():
			return
		case event, ok := <-actions:
			if !ok {
				return
			}
			if event.Action != "ack" || event.Thread == "" {
				continue
			}
			if err := openclient.Acknowledge(ctx, client, secret, event.Thread); err != nil {
				logger.Warn("acknowledge emergency notification", "error", redactError(err, secret, deviceID))
				continue
			}
			trigger()
		}
	}
}
