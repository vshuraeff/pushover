package listen

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/vshuraeff/pushover/internal/openclient"
)

type deadLetterRecord struct {
	Message  openclient.Message `json:"message"`
	Reason   string             `json:"reason"`
	FailedAt string             `json:"failed_at"`
}

type deadLetterWriter struct {
	path   string
	clock  Clock
	logger *slog.Logger
	mu     sync.Mutex
}

func newDeadLetterWriter(path string, clock Clock, logger *slog.Logger) *deadLetterWriter {
	if clock == nil {
		clock = realClock{}
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &deadLetterWriter{path: path, clock: clock, logger: logger}
}

func (w *deadLetterWriter) Write(message openclient.Message, reason string) error {
	record := deadLetterRecord{
		Message:  message,
		Reason:   reason,
		FailedAt: w.clock.Now().UTC().Format(time.RFC3339Nano),
	}
	data, err := json.Marshal(record)
	if err != nil {
		return fmt.Errorf("encode dead letter: %w", err)
	}
	data = append(data, '\n')

	w.mu.Lock()
	defer w.mu.Unlock()
	dir := filepath.Dir(w.path)
	_, statErr := os.Stat(dir)
	createdDir := os.IsNotExist(statErr)
	if statErr != nil && !createdDir {
		return fmt.Errorf("inspect dead-letter directory: %w", statErr)
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create dead-letter directory: %w", err)
	}
	if createdDir {
		if err := os.Chmod(dir, 0o700); err != nil {
			return fmt.Errorf("set dead-letter directory permissions: %w", err)
		}
	}

	file, err := os.OpenFile(w.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("open dead-letter file: %w", err)
	}
	if err := file.Chmod(0o600); err != nil {
		_ = file.Close()
		return fmt.Errorf("set dead-letter file permissions: %w", err)
	}
	if _, err := file.Write(data); err != nil {
		_ = file.Close()
		return fmt.Errorf("append dead letter: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close dead-letter file: %w", err)
	}
	w.logger.Warn("dead-letter write", "message_id", message.ID, "reason", reason)
	return nil
}
