//go:build darwin

package notify

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const fakeHelperEnv = "POVR_HELPER_FAKE"

func TestHelperNotifierProtocolRoundTrip(t *testing.T) {
	notifier := newHelperNotifier(func() *exec.Cmd {
		cmd := exec.Command(os.Args[0], "-test.run=TestHelperSubprocess", "--")
		cmd.Env = append(os.Environ(), fakeHelperEnv+"=1")
		return cmd
	})
	t.Cleanup(func() {
		if err := notifier.Close(); err != nil {
			t.Errorf("Close() error = %v", err)
		}
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err := notifier.Notify(ctx, Notification{
		ID:             "round-trip",
		Title:          "title",
		Body:           "body",
		AppName:        "subtitle",
		IconPath:       "icon.png",
		AttachmentPath: "attachment.png",
		SoundName:      "povr-alarm.caf",
		SoundPath:      "ignored.mp3",
		Priority:       2,
		URL:            "https://example.test/notification",
		ThreadID:       "thread",
	})
	if err != nil {
		t.Fatalf("Notify() error = %v", err)
	}
	select {
	case event := <-notifier.Actions():
		if event.ID != "round-trip" || event.Action != "ack" || event.Thread != "thread" {
			t.Fatalf("Actions() event = %#v", event)
		}
	case <-ctx.Done():
		t.Fatalf("Actions() timeout: %v", ctx.Err())
	}
	select {
	case event := <-notifier.Actions():
		if event.ID != "round-trip" || event.Action != ActionOpen || event.URL != "https://example.test/notification" {
			t.Fatalf("Actions() click event = %#v", event)
		}
	case <-ctx.Done():
		t.Fatalf("Actions() click timeout: %v", ctx.Err())
	}

	err = notifier.Notify(ctx, Notification{ID: "error", Title: "title", Body: "body"})
	if err == nil || !strings.Contains(err.Error(), "fake delivery failed") {
		t.Fatalf("Notify() error = %v, want fake delivery failure", err)
	}

	if err := notifier.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if err := notifier.Close(); err != nil {
		t.Fatalf("second Close() error = %v", err)
	}
}

func TestHelperNotifierCloseBeforeStart(t *testing.T) {
	notifier := newHelperNotifier(func() *exec.Cmd {
		t.Fatal("helper command must not start")
		return nil
	})
	if err := notifier.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if _, ok := <-notifier.Actions(); ok {
		t.Fatal("Actions() channel remained open after Close()")
	}
	if err := notifier.Notify(context.Background(), Notification{}); !errors.Is(err, ErrHelperClosed) {
		t.Fatalf("Notify() error = %v, want %v", err, ErrHelperClosed)
	}
}

func TestHelperNotifierRequestCategory(t *testing.T) {
	notifier := newHelperNotifier(nil)
	if got := notifier.request(Notification{Priority: 2}).Category; got != "povr.emergency" {
		t.Fatalf("emergency category = %q, want %q", got, "povr.emergency")
	}
	if got := notifier.request(Notification{Priority: 1}).Category; got != "" {
		t.Fatalf("high-priority category = %q, want empty", got)
	}
}

func TestNewHelperNotifierErrors(t *testing.T) {
	if _, err := NewHelperNotifier(""); !errors.Is(err, ErrHelperBundlePathEmpty) {
		t.Fatalf("NewHelperNotifier(\"\") error = %v, want %v", err, ErrHelperBundlePathEmpty)
	}

	bundlePath := filepath.Join(t.TempDir(), "Povr.app")
	if _, err := NewHelperNotifier(bundlePath); !errors.Is(err, ErrHelperNotInstalled) {
		t.Fatalf("NewHelperNotifier() error = %v, want %v", err, ErrHelperNotInstalled)
	}
}

func TestHelperSubprocess(t *testing.T) {
	if os.Getenv(fakeHelperEnv) != "1" {
		return
	}

	encoder := json.NewEncoder(os.Stdout)
	if err := encoder.Encode(HelperEvent{Event: "ready"}); err != nil {
		os.Exit(2)
	}

	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		var request HelperRequest
		if err := json.Unmarshal(scanner.Bytes(), &request); err != nil {
			_ = encoder.Encode(HelperEvent{Event: "error", Error: err.Error()})
			continue
		}
		if request.ID == "error" {
			_ = encoder.Encode(HelperEvent{Event: "error", ID: request.ID, Error: "fake delivery failed"})
			continue
		}

		valid := request.ID == "round-trip" &&
			request.Title == "title" &&
			request.Subtitle == "subtitle" &&
			request.Body == "body" &&
			request.Icon == "icon.png" &&
			request.Attachment == "attachment.png" &&
			request.Sound != nil &&
			request.Sound.Name == "povr-alarm.caf" &&
			request.Sound.File == "" &&
			request.Level == "timeSensitive" &&
			request.Thread == "thread" &&
			request.URL == "https://example.test/notification" &&
			request.Category == "povr.emergency"
		if !valid {
			_ = encoder.Encode(HelperEvent{Event: "error", ID: request.ID, Error: "request mismatch"})
			continue
		}
		_ = encoder.Encode(HelperEvent{Event: "delivered", ID: request.ID})
		_ = encoder.Encode(HelperEvent{Event: "action", Action: "ack", ID: request.ID, Thread: request.Thread})
		_ = encoder.Encode(HelperEvent{Event: "clicked", ID: request.ID})
		_ = encoder.Encode(HelperEvent{Event: "clicked", ID: request.ID, URL: request.URL})
	}
	if err := scanner.Err(); err != nil {
		os.Exit(3)
	}
	os.Exit(0)
}
