package listen

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/vshuraeff/pushover/internal/api"
	"github.com/vshuraeff/pushover/internal/config"
	"github.com/vshuraeff/pushover/internal/notify"
)

func TestRunRequiresNotifier(t *testing.T) {
	err := Run(context.Background(), &Daemon{
		Store: &memoryStore{state: &config.State{DeviceID: "fake-device-123", Secret: "fake-secret-do-not-use"}},
	})
	if err == nil || !strings.Contains(err.Error(), "notifier is required") {
		t.Fatalf("Run() error = %v", err)
	}
}

func TestRunPropagatesSessionSetupGuidance(t *testing.T) {
	store := &memoryStore{}
	err := Run(context.Background(), &Daemon{
		Store:     store,
		APIClient: api.New("https://example.invalid", "0.0.0-test"),
		Notifier:  &channelNotifier{},
		WSURL:     "wss://example.invalid/push",
	})
	if err == nil || !strings.Contains(err.Error(), "povr client login") || !strings.Contains(err.Error(), "povr client register") {
		t.Fatalf("Run() error = %v", err)
	}
}

func TestRunPerformsInitialSyncAndTreatsCancellationAsClean(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/1/messages.json":
			_, _ = writer.Write([]byte(`{"status":1,"messages":[{"id":9,"id_str":"9","title":"queued","message":"initial"}]}`))
		case "/1/devices/fake-device-123/update_highest_message.json":
			_, _ = writer.Write([]byte(`{"status":1}`))
		default:
			writer.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()
	ctx, cancel := context.WithCancel(context.Background())
	notifier := &cancelNotifier{cancel: cancel}
	store := &memoryStore{state: &config.State{DeviceID: "fake-device-123", Secret: "fake-secret-do-not-use"}}
	tempDir := t.TempDir()
	err := Run(ctx, &Daemon{
		Store:            store,
		APIClient:        api.New(server.URL, "0.0.0-test", api.WithSleep(func(time.Duration) {})),
		Notifier:         notifier,
		Logger:           testLogger(),
		WSURL:            "ws://127.0.0.1.invalid",
		EmergencyRealert: time.Minute,
		IconCacheDir:     filepath.Join(tempDir, "icons"),
		SoundCacheDir:    filepath.Join(tempDir, "sounds"),
		DeadLetterPath:   filepath.Join(tempDir, "deadletter.jsonl"),
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if notifier.calls != 1 || store.state.LastProcessedMessageID != 9 {
		t.Fatalf("notify calls = %d, saved cursor = %d", notifier.calls, store.state.LastProcessedMessageID)
	}
}

func TestRunRedactsSecretFromInitialSyncError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		hijacker, ok := writer.(http.Hijacker)
		if !ok {
			t.Error("response writer does not support hijacking")
			return
		}
		connection, _, err := hijacker.Hijack()
		if err != nil {
			t.Errorf("Hijack() error = %v", err)
			return
		}
		_ = connection.Close()
	}))
	defer server.Close()
	const secret = "fake-secret-must-not-leak"
	tempDir := t.TempDir()
	err := Run(context.Background(), &Daemon{
		Store:            &memoryStore{state: &config.State{DeviceID: "fake-device-redacted", Secret: secret}},
		APIClient:        api.New(server.URL, "0.0.0-test", api.WithSleep(func(time.Duration) {})),
		Notifier:         &channelNotifier{},
		Logger:           testLogger(),
		WSURL:            "ws://127.0.0.1.invalid",
		EmergencyRealert: time.Minute,
		IconCacheDir:     filepath.Join(tempDir, "icons"),
		SoundCacheDir:    filepath.Join(tempDir, "sounds"),
		DeadLetterPath:   filepath.Join(tempDir, "deadletter.jsonl"),
	})
	if err == nil {
		t.Fatal("Run() error = nil")
	}
	if strings.Contains(err.Error(), secret) || !strings.Contains(err.Error(), "<redacted>") {
		t.Fatalf("Run() error was not safely redacted: %v", err)
	}
}

type cancelNotifier struct {
	cancel context.CancelFunc
	calls  int
}

func (n *cancelNotifier) Notify(context.Context, notify.Notification) error {
	n.calls++
	n.cancel()
	return nil
}

func (*cancelNotifier) Caps() notify.Caps {
	return notify.Caps{}
}
