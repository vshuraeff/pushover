package notify

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFileNotifierWritesNotificationRecord(t *testing.T) {
	path := filepath.Join(t.TempDir(), "notifications.jsonl")
	notifier, err := NewFileNotifier(path)
	if err != nil {
		t.Fatalf("NewFileNotifier() error = %v", err)
	}
	t.Cleanup(func() {
		if err := notifier.Close(); err != nil {
			t.Errorf("Close() error = %v", err)
		}
	})

	notification := Notification{
		ID:             "id-1",
		Title:          "title",
		Body:           "body",
		AppName:        "povr",
		IconPath:       "icon.png",
		AttachmentPath: "attachment.png",
		SoundName:      "Ping",
		SoundPath:      "sound.aiff",
		Priority:       1,
		URL:            "https://example.invalid",
		URLTitle:       "open",
		ThreadID:       "thread-1",
	}
	if err := notifier.Notify(context.Background(), notification); err != nil {
		t.Fatalf("Notify() error = %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if !strings.HasSuffix(string(data), "\n") {
		t.Fatal("notification record does not end with a newline")
	}
	lines := strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")
	if len(lines) != 1 {
		t.Fatalf("record count = %d, want 1", len(lines))
	}

	var record fileRecord
	if err := json.Unmarshal([]byte(lines[0]), &record); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if record.ID != notification.ID || record.Title != notification.Title || record.Body != notification.Body {
		t.Fatalf("record basic fields = %#v, want notification fields %#v", record, notification)
	}
	if record.SoundPath != notification.SoundPath || record.URLTitle != notification.URLTitle || record.ThreadID != notification.ThreadID {
		t.Fatalf("record extended fields = %#v, want notification fields %#v", record, notification)
	}
	if record.Level != LevelTimeSensitive || record.SoundSuppressed {
		t.Fatalf("record level = %q, soundSuppressed = %t", record.Level, record.SoundSuppressed)
	}
}
