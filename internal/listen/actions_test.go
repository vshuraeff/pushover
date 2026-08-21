package listen

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/vshuraeff/pushover/internal/api"
	"github.com/vshuraeff/pushover/internal/config"
	"github.com/vshuraeff/pushover/internal/notify"
)

func TestRunAcknowledgesNotificationAction(t *testing.T) {
	const (
		secret  = "fake-secret-do-not-use"
		receipt = "fake-receipt-123"
	)
	type acknowledgeRequest struct {
		path   string
		secret string
	}
	acknowledged := make(chan acknowledgeRequest, 1)
	resynced := make(chan struct{}, 1)
	var messageRequests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/1/messages.json":
			if messageRequests.Add(1) > 1 {
				resynced <- struct{}{}
			}
			_, _ = writer.Write([]byte(`{"status":1,"messages":[]}`))
		case "/1/receipts/" + receipt + "/acknowledge.json":
			if err := request.ParseForm(); err != nil {
				t.Errorf("ParseForm() error = %v", err)
				writer.WriteHeader(http.StatusBadRequest)
				return
			}
			acknowledged <- acknowledgeRequest{path: request.URL.Path, secret: request.FormValue("secret")}
			_, _ = writer.Write([]byte(`{"status":1}`))
		default:
			writer.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	notifier := &actionNotifier{actions: make(chan notify.ActionEvent, 1)}
	tempDir := t.TempDir()
	done := make(chan error, 1)
	go func() {
		done <- Run(ctx, &Daemon{
			Store:            &memoryStore{state: &config.State{DeviceID: "fake-device-123", Secret: secret}},
			APIClient:        api.New(server.URL, "0.0.0-test", api.WithSleep(func(time.Duration) {})),
			Notifier:         notifier,
			Logger:           testLogger(),
			WSURL:            "ws://127.0.0.1.invalid",
			EmergencyRealert: time.Minute,
			IconCacheDir:     filepath.Join(tempDir, "icons"),
			SoundCacheDir:    filepath.Join(tempDir, "sounds"),
			DeadLetterPath:   filepath.Join(tempDir, "deadletter.jsonl"),
		})
	}()

	notifier.actions <- notify.ActionEvent{ID: "9", Action: "ack", Thread: receipt}
	select {
	case got := <-acknowledged:
		if got.path != "/1/receipts/"+receipt+"/acknowledge.json" || got.secret != secret {
			t.Fatalf("acknowledge request = %#v", got)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for acknowledge request")
	}
	select {
	case <-resynced:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for post-acknowledgement sync")
	}
	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run() error = %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for Run()")
	}
}

type actionNotifier struct {
	actions chan notify.ActionEvent
}

func (*actionNotifier) Notify(context.Context, notify.Notification) error {
	return nil
}

func (*actionNotifier) Caps() notify.Caps {
	return notify.Caps{Actions: true}
}

func (n *actionNotifier) Actions() <-chan notify.ActionEvent {
	return n.actions
}
