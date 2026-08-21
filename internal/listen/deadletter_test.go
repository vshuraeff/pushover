package listen

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/vshuraeff/pushover/internal/openclient"
)

func TestDeadLetterWriterAppendsFullMessagesWithRestrictedPermissions(t *testing.T) {
	clock := newFakeClock()
	dir := filepath.Join(t.TempDir(), "config")
	path := filepath.Join(dir, "deadletter.jsonl")
	writer := newDeadLetterWriter(path, clock, testLogger())
	messages := []openclient.Message{
		{ID: 10, IDStr: "10", Title: "first", Message: "body one", Receipt: "fake-receipt-one"},
		{ID: 11, IDStr: "11", Title: "second", Message: "body two", Receipt: "fake-receipt-two"},
	}
	for _, message := range messages {
		if err := writer.Write(message, "forced notifier failure"); err != nil {
			t.Fatalf("Write() error = %v", err)
		}
	}

	file, err := os.Open(path)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	var records []deadLetterRecord
	for scanner.Scan() {
		var record deadLetterRecord
		if err := json.Unmarshal(scanner.Bytes(), &record); err != nil {
			t.Fatalf("Unmarshal() error = %v", err)
		}
		records = append(records, record)
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("Scan() error = %v", err)
	}
	if len(records) != 2 || records[0].Message != messages[0] || records[1].Message != messages[1] {
		t.Fatalf("dead-letter records = %#v", records)
	}
	if records[0].Reason != "forced notifier failure" || records[0].FailedAt == "" {
		t.Fatalf("dead-letter context = %#v", records[0])
	}
	dirInfo, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("Stat(dir) error = %v", err)
	}
	fileInfo, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat(file) error = %v", err)
	}
	if dirInfo.Mode().Perm() != 0o700 || fileInfo.Mode().Perm() != 0o600 {
		t.Fatalf("permissions = dir %o file %o", dirInfo.Mode().Perm(), fileInfo.Mode().Perm())
	}
}
