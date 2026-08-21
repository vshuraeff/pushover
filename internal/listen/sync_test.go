package listen

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/vshuraeff/pushover/internal/api"
	"github.com/vshuraeff/pushover/internal/config"
	"github.com/vshuraeff/pushover/internal/notify"
	"github.com/vshuraeff/pushover/internal/openclient"
)

func TestSyncerCoalescesTriggersWhileBusy(t *testing.T) {
	started := make(chan int32, 3)
	release := make(chan struct{}, 2)
	var calls atomic.Int32
	syncer := newSyncer(syncerConfig{
		ctx:      context.Background(),
		state:    &config.State{},
		logger:   testLogger(),
		notifier: &channelNotifier{},
		fetch: func(context.Context, *api.Client, string, string) ([]openclient.Message, error) {
			call := calls.Add(1)
			started <- call
			<-release
			return nil, nil
		},
		updateHighest: func(context.Context, string, string, int64) error { return nil },
	})

	syncer.trigger()
	waitForSyncCall(t, started, 1)
	for range 20 {
		syncer.trigger()
	}
	release <- struct{}{}
	waitForSyncCall(t, started, 2)
	release <- struct{}{}
	syncer.stopAndWait()
	if got := calls.Load(); got != 2 {
		t.Fatalf("fetch calls = %d, want 2", got)
	}
}

func TestSyncerSortsDeduplicatesAndPersistsBeforeUpdate(t *testing.T) {
	journal := make([]string, 0, 2)
	store := &memoryStore{state: &config.State{LastProcessedMessageID: 1}, onSave: func() { journal = append(journal, "save") }}
	notifier := &recordingNotifier{}
	var logs bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logs, nil))
	messages := []openclient.Message{
		{ID: 3, IDStr: "3", Title: "third", Message: "third", App: "test app"},
		{ID: 1, IDStr: "1", Title: "duplicate", Message: "first"},
		{ID: 2, Title: "normal", Message: "second", URL: "https://example.invalid", URLTitle: "details"},
	}
	syncer := newSyncer(syncerConfig{
		ctx:      context.Background(),
		store:    store,
		state:    store.state,
		notifier: notifier,
		logger:   logger,
		fetch: func(context.Context, *api.Client, string, string) ([]openclient.Message, error) {
			return append([]openclient.Message(nil), messages...), nil
		},
		updateHighest: func(_ context.Context, _, _ string, id int64) error {
			journal = append(journal, "update")
			if id != 3 {
				t.Fatalf("updated id = %d, want 3", id)
			}
			return nil
		},
	})

	if err := syncer.runOnce(context.Background()); err != nil {
		t.Fatalf("runOnce() error = %v", err)
	}
	if got := notifier.IDs(); !reflect.DeepEqual(got, []string{"2", "3"}) {
		t.Fatalf("notification ids = %#v", got)
	}
	if !reflect.DeepEqual(journal, []string{"save", "update"}) {
		t.Fatalf("journal = %#v", journal)
	}
	if store.state.LastProcessedMessageID != 3 {
		t.Fatalf("saved cursor = %d, want 3", store.state.LastProcessedMessageID)
	}
	if text := logs.String(); !strings.Contains(text, "sync complete") || !strings.Contains(text, "batch_size=3") || !strings.Contains(text, "notified=2") {
		t.Fatalf("sync log = %q", text)
	}
}

func TestSyncerCapsServerHighestUntilEmergencyAcknowledged(t *testing.T) {
	events := make([]string, 0, 3)
	updates := make([]int64, 0, 2)
	store := &memoryStore{state: &config.State{}, onSave: func() { events = append(events, "save") }}
	notifier := &recordingNotifier{}
	emergencies := NewEmergencyScheduler(notifier, time.Minute, newFakeClock(), testLogger())
	messages := []openclient.Message{
		{ID: 8, IDStr: "8", Title: "later normal"},
		{ID: 5, IDStr: "5", Title: "open emergency", Priority: 2, Receipt: "fake-receipt-5"},
	}
	syncer := newSyncer(syncerConfig{
		ctx:         context.Background(),
		store:       store,
		state:       store.state,
		notifier:    notifier,
		logger:      testLogger(),
		emergencies: emergencies,
		fetch: func(context.Context, *api.Client, string, string) ([]openclient.Message, error) {
			return append([]openclient.Message(nil), messages...), nil
		},
		updateHighest: func(_ context.Context, _, _ string, id int64) error {
			events = append(events, "update")
			updates = append(updates, id)
			return nil
		},
	})

	if err := syncer.runOnce(context.Background()); err != nil {
		t.Fatalf("runOnce() error = %v", err)
	}
	if store.state.LastProcessedMessageID != 8 {
		t.Fatalf("local cursor = %d, want 8", store.state.LastProcessedMessageID)
	}
	if !reflect.DeepEqual(events, []string{"save", "update"}) || !reflect.DeepEqual(updates, []int64{4}) {
		t.Fatalf("events = %#v, updates = %#v", events, updates)
	}
	if minimum, ok := emergencies.MinOpenID(); !ok || minimum != 5 {
		t.Fatalf("minimum open emergency = %d, %t", minimum, ok)
	}

	messages = []openclient.Message{{ID: 5, Priority: 2, Receipt: "fake-receipt-5", Acked: 1}}
	if err := syncer.runOnce(context.Background()); err != nil {
		t.Fatalf("second runOnce() error = %v", err)
	}
	if !reflect.DeepEqual(updates, []int64{4, 8}) {
		t.Fatalf("updates after acknowledgement = %#v", updates)
	}
	if _, ok := emergencies.MinOpenID(); ok {
		t.Fatal("acknowledged emergency remained open")
	}
	if got := notifier.IDs(); !reflect.DeepEqual(got, []string{"5", "8"}) {
		t.Fatalf("notification ids = %#v", got)
	}
}

func TestSyncerUsesConvertedSoundBaseName(t *testing.T) {
	soundDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(soundDir, "bike.mp3"), []byte("fake-mp3-data"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	cache := newAssetCache(nil, t.TempDir(), soundDir, &channelNotifier{}, testLogger())
	cache.convertSounds = true
	cache.converter = func(context.Context, string, string) (string, error) {
		return filepath.Join(soundDir, "povr-bike.caf"), nil
	}
	syncer := newSyncer(syncerConfig{cache: cache, notifier: &recordingNotifier{}})

	notification := syncer.notification(context.Background(), openclient.Message{ID: 12, Sound: "bike"})
	if notification.SoundName != "povr-bike" || notification.SoundPath != filepath.Join(soundDir, "povr-bike.caf") {
		t.Fatalf("notification sound = name %q path %q", notification.SoundName, notification.SoundPath)
	}
}

func TestSyncerRetriesThenDeadLettersAndContinues(t *testing.T) {
	clock := newFakeClock()
	deadLetterPath := filepath.Join(t.TempDir(), "deadletter.jsonl")
	store := &memoryStore{state: &config.State{}}
	notifier := &failingNotifier{err: errors.New("forced notification failure")}
	syncer := newSyncer(syncerConfig{
		ctx:         context.Background(),
		store:       store,
		state:       store.state,
		notifier:    notifier,
		logger:      testLogger(),
		clock:       clock,
		deadLetters: newDeadLetterWriter(deadLetterPath, clock, testLogger()),
		fetch: func(context.Context, *api.Client, string, string) ([]openclient.Message, error) {
			return []openclient.Message{{ID: 7, IDStr: "7", Title: "failed", Message: "full body"}}, nil
		},
		updateHighest: func(context.Context, string, string, int64) error { return nil },
	})

	result := make(chan error, 1)
	go func() { result <- syncer.runOnce(context.Background()) }()
	if got := clock.nextRequest(t); got != notifyBackoff {
		t.Fatalf("first retry delay = %v, want %v", got, notifyBackoff)
	}
	clock.Advance(notifyBackoff)
	if got := clock.nextRequest(t); got != 2*notifyBackoff {
		t.Fatalf("second retry delay = %v, want %v", got, 2*notifyBackoff)
	}
	clock.Advance(2 * notifyBackoff)
	select {
	case err := <-result:
		if err != nil {
			t.Fatalf("runOnce() error = %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for sync")
	}
	if notifier.calls.Load() != notifyAttempts {
		t.Fatalf("notify calls = %d, want %d", notifier.calls.Load(), notifyAttempts)
	}
	data, err := os.ReadFile(deadLetterPath)
	if err != nil {
		t.Fatalf("ReadFile(dead letter) error = %v", err)
	}
	if len(data) == 0 || store.state.LastProcessedMessageID != 7 {
		t.Fatalf("dead-letter bytes = %d, saved cursor = %d", len(data), store.state.LastProcessedMessageID)
	}
}

func TestSyncerRetriesHighestUpdateOnDuplicateOnlyFetch(t *testing.T) {
	store := &memoryStore{state: &config.State{}}
	notifier := &recordingNotifier{}
	var updates atomic.Int32
	syncer := newSyncer(syncerConfig{
		ctx:      context.Background(),
		store:    store,
		state:    store.state,
		notifier: notifier,
		logger:   testLogger(),
		fetch: func(context.Context, *api.Client, string, string) ([]openclient.Message, error) {
			return []openclient.Message{{ID: 5, IDStr: "5"}}, nil
		},
		updateHighest: func(context.Context, string, string, int64) error {
			if updates.Add(1) == 1 {
				return errors.New("forced update failure")
			}
			return nil
		},
	})
	if err := syncer.runOnce(context.Background()); err != nil {
		t.Fatalf("first runOnce() error = %v", err)
	}
	if err := syncer.runOnce(context.Background()); err != nil {
		t.Fatalf("second runOnce() error = %v", err)
	}
	if got := updates.Load(); got != 2 {
		t.Fatalf("update calls = %d, want 2", got)
	}
	if got := notifier.IDs(); !reflect.DeepEqual(got, []string{"5"}) {
		t.Fatalf("notification ids = %#v", got)
	}
}

type memoryStore struct {
	mu     sync.Mutex
	state  *config.State
	onSave func()
}

func (s *memoryStore) Load() (*config.State, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.state == nil {
		return nil, os.ErrNotExist
	}
	copy := *s.state
	return &copy, nil
}

func (s *memoryStore) Save(state *config.State) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	copy := *state
	s.state = &copy
	if s.onSave != nil {
		s.onSave()
	}
	return nil
}

type recordingNotifier struct {
	mu            sync.Mutex
	notifications []notify.Notification
}

func (n *recordingNotifier) Notify(_ context.Context, notification notify.Notification) error {
	n.mu.Lock()
	n.notifications = append(n.notifications, notification)
	n.mu.Unlock()
	return nil
}

func (*recordingNotifier) Caps() notify.Caps {
	return notify.Caps{}
}

func (n *recordingNotifier) IDs() []string {
	n.mu.Lock()
	defer n.mu.Unlock()
	ids := make([]string, 0, len(n.notifications))
	for _, notification := range n.notifications {
		ids = append(ids, notification.ID)
	}
	return ids
}

func (n *recordingNotifier) Last() notify.Notification {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.notifications[len(n.notifications)-1]
}

type failingNotifier struct {
	err   error
	calls atomic.Int32
}

func (n *failingNotifier) Notify(context.Context, notify.Notification) error {
	n.calls.Add(1)
	return n.err
}

func (*failingNotifier) Caps() notify.Caps {
	return notify.Caps{}
}

func waitForSyncCall(t *testing.T, started <-chan int32, want int32) {
	t.Helper()
	select {
	case got := <-started:
		if got != want {
			t.Fatalf("sync call = %d, want %d", got, want)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for sync call %d", want)
	}
}
