package listen

import (
	"context"
	"errors"
	"log/slog"
	"net/url"
	"os/exec"
	"runtime"

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
	openURL func(context.Context, string) error,
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
			switch {
			case event.Action == notify.ActionOpen:
				if !isWebURL(event.URL) {
					logger.Warn("refusing to open notification url with unsupported scheme", "id", event.ID)
					continue
				}
				if err := openURL(ctx, event.URL); err != nil {
					logger.Warn("open notification url", "id", event.ID, "error", err)
				}
			case event.Action == "ack" && event.Thread != "":
				if err := openclient.Acknowledge(ctx, client, secret, event.Thread); err != nil {
					logger.Warn("acknowledge emergency notification", "error", redactError(err, secret, deviceID))
					continue
				}
				trigger()
			}
		}
	}
}

// isWebURL reports whether target is an absolute http or https url with a host; message urls come
// from the network, so other schemes (file, app handlers) are never opened.
func isWebURL(target string) bool {
	parsed, err := url.Parse(target)
	if err != nil {
		return false
	}
	return (parsed.Scheme == "http" || parsed.Scheme == "https") && parsed.Host != ""
}

func openWithSystem(ctx context.Context, target string) error {
	if runtime.GOOS != "darwin" {
		return errors.New("opening urls is only supported on macOS")
	}
	return exec.CommandContext(ctx, "/usr/bin/open", target).Run()
}
