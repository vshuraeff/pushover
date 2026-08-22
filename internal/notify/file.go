package notify

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sync"
)

const consumeFilesEnv = "PUSHOVER_FILE_NOTIFIER_CONSUME_FILES"

// FileNotifier records intended notifications as line-delimited json.
type FileNotifier struct {
	mu           sync.Mutex
	file         *os.File
	path         string
	consumeFiles bool
}

type fileRecord struct {
	ID              string `json:"id"`
	Title           string `json:"title"`
	Body            string `json:"body"`
	AppName         string `json:"appName"`
	IconPath        string `json:"iconPath"`
	AttachmentPath  string `json:"attachmentPath"`
	SoundName       string `json:"soundName"`
	SoundPath       string `json:"soundPath"`
	Priority        int    `json:"priority"`
	URL             string `json:"url"`
	URLTitle        string `json:"urlTitle"`
	ThreadID        string `json:"threadID"`
	Level           Level  `json:"level"`
	SoundSuppressed bool   `json:"soundSuppressed"`
}

// NewFileNotifier creates a notifier that appends records to path.
func NewFileNotifier(path string) (*FileNotifier, error) {
	return newFileNotifier(path, false)
}

func newFileNotifier(path string, consumeFiles bool) (*FileNotifier, error) {
	file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, err
	}

	return &FileNotifier{file: file, path: path, consumeFiles: consumeFiles}, nil
}

// Notify appends one record for n.
func (f *FileNotifier) Notify(_ context.Context, n Notification) error {
	level := PriorityToLevel(n.Priority)
	record := fileRecord{
		ID:              n.ID,
		Title:           n.Title,
		Body:            n.Body,
		AppName:         n.AppName,
		IconPath:        n.IconPath,
		AttachmentPath:  n.AttachmentPath,
		SoundName:       n.SoundName,
		SoundPath:       n.SoundPath,
		Priority:        n.Priority,
		URL:             n.URL,
		URLTitle:        n.URLTitle,
		ThreadID:        n.ThreadID,
		Level:           level,
		SoundSuppressed: SuppressSoundForLevel(level),
	}
	data, err := json.Marshal(record)
	if err != nil {
		return err
	}
	data = append(data, '\n')

	f.mu.Lock()
	defer f.mu.Unlock()
	if f.file == nil {
		return errors.New("notify: file notifier is closed")
	}
	paths := []string{n.IconPath, n.AttachmentPath}
	if f.consumeFiles {
		for _, path := range paths {
			if path == "" {
				continue
			}
			if _, err := os.Stat(path); err != nil {
				return fmt.Errorf("notify: consume file %q: %w", path, err)
			}
		}
	}
	if _, err = f.file.Write(data); err != nil {
		return err
	}
	if f.consumeFiles {
		for _, path := range paths {
			if path == "" {
				continue
			}
			if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
				return fmt.Errorf("notify: remove consumed file %q: %w", path, err)
			}
		}
	}
	return nil
}

// Caps returns the fields that the file backend records.
func (*FileNotifier) Caps() Caps {
	return Caps{Images: true, Sound: true, Levels: true}
}

// Close closes the notification log file.
func (f *FileNotifier) Close() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.file == nil {
		return nil
	}
	err := f.file.Close()
	f.file = nil
	return err
}
