package listen

import (
	"context"
	"fmt"
	"log/slog"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/vshuraeff/pushover/internal/api"
	"github.com/vshuraeff/pushover/internal/config"
	"github.com/vshuraeff/pushover/internal/notify"
	"github.com/vshuraeff/pushover/internal/openclient"
)

const (
	notifyAttempts = 3
	notifyBackoff  = 100 * time.Millisecond
)

type fetchMessagesFunc func(context.Context, *api.Client, string, string) ([]openclient.Message, error)
type updateHighestFunc func(context.Context, string, string, int64) error

type syncerConfig struct {
	ctx           context.Context
	store         config.SessionStore
	state         *config.State
	client        *api.Client
	notifier      notify.Notifier
	logger        *slog.Logger
	clock         Clock
	cache         *assetCache
	deadLetters   *deadLetterWriter
	emergencies   *EmergencyScheduler
	secret        string
	deviceID      string
	fetch         fetchMessagesFunc
	updateHighest updateHighestFunc
}

type syncer struct {
	ctx           context.Context
	store         config.SessionStore
	state         *config.State
	client        *api.Client
	notifier      notify.Notifier
	logger        *slog.Logger
	clock         Clock
	cache         *assetCache
	deadLetters   *deadLetterWriter
	emergencies   *EmergencyScheduler
	secret        string
	deviceID      string
	fetch         fetchMessagesFunc
	updateHighest updateHighestFunc

	mu      sync.Mutex
	running bool
	pending bool
	stopped bool
	workers sync.WaitGroup
}

func newSyncer(configuration syncerConfig) *syncer {
	if configuration.ctx == nil {
		configuration.ctx = context.Background()
	}
	if configuration.logger == nil {
		configuration.logger = slog.Default()
	}
	if configuration.clock == nil {
		configuration.clock = realClock{}
	}
	if configuration.fetch == nil {
		configuration.fetch = openclient.Fetch
	}
	if configuration.updateHighest == nil {
		configuration.updateHighest = func(ctx context.Context, secret, deviceID string, id int64) error {
			return openclient.UpdateHighest(ctx, configuration.client, secret, deviceID, id)
		}
	}
	return &syncer{
		ctx:           configuration.ctx,
		store:         configuration.store,
		state:         configuration.state,
		client:        configuration.client,
		notifier:      configuration.notifier,
		logger:        configuration.logger,
		clock:         configuration.clock,
		cache:         configuration.cache,
		deadLetters:   configuration.deadLetters,
		emergencies:   configuration.emergencies,
		secret:        configuration.secret,
		deviceID:      configuration.deviceID,
		fetch:         configuration.fetch,
		updateHighest: configuration.updateHighest,
	}
}

func (s *syncer) trigger() {
	s.mu.Lock()
	if s.stopped || s.ctx.Err() != nil {
		s.mu.Unlock()
		return
	}
	if s.running {
		s.pending = true
		s.mu.Unlock()
		return
	}
	s.running = true
	s.workers.Add(1)
	s.mu.Unlock()

	go s.work()
}

func (s *syncer) work() {
	defer s.workers.Done()
	for {
		if err := s.runOnce(s.ctx); err != nil && s.ctx.Err() == nil {
			s.logger.Error("sync messages", "error", redactError(err, s.secret, s.deviceID))
		}

		s.mu.Lock()
		if s.pending && !s.stopped && s.ctx.Err() == nil {
			s.pending = false
			s.mu.Unlock()
			continue
		}
		s.running = false
		s.pending = false
		s.mu.Unlock()
		return
	}
}

func redactError(err error, values ...string) string {
	message := err.Error()
	for _, value := range values {
		if value != "" {
			for _, variant := range []string{value, url.QueryEscape(value), url.PathEscape(value)} {
				message = strings.ReplaceAll(message, variant, "<redacted>")
			}
		}
	}
	return message
}

func (s *syncer) stopAndWait() {
	s.mu.Lock()
	s.stopped = true
	s.pending = false
	s.mu.Unlock()
	s.workers.Wait()
}

func (s *syncer) runOnce(ctx context.Context) error {
	messages, err := s.fetch(ctx, s.client, s.secret, s.deviceID)
	if err != nil {
		return err
	}
	sort.SliceStable(messages, func(i, j int) bool {
		return messages[i].ID < messages[j].ID
	})

	initialLast := s.state.LastProcessedMessageID
	notifications := make(map[int]notify.Notification)
	openEmergencies := make(map[string]emergencyState)
	for index, message := range messages {
		isOpenEmergency := message.Priority == 2 && message.Acked == 0 && message.Receipt != ""
		if message.ID > initialLast || isOpenEmergency {
			notification := s.notification(ctx, message)
			notifications[index] = notification
			if isOpenEmergency {
				openEmergencies[message.Receipt] = emergencyState{
					messageID:    message.ID,
					notification: notification,
				}
			}
		}
		if message.Receipt != "" && message.Acked == 1 {
			delete(openEmergencies, message.Receipt)
		}
	}
	if s.emergencies != nil {
		// fetch is treated as the server's complete current open-emergency view.
		s.emergencies.update(openEmergencies)
	}

	runningLast := initialLast
	processed := 0
	notified := 0
	deadLettered := 0
	for index, message := range messages {
		if message.ID <= runningLast {
			continue
		}

		notification, ok := notifications[index]
		if !ok {
			notification = s.notification(ctx, message)
		}
		processed++
		if err := s.notifyWithRetry(ctx, notification); err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			reason := fmt.Sprintf("notification failed after %d attempts: %v", notifyAttempts, err)
			if s.deadLetters == nil {
				s.logger.Error("dead-letter writer unavailable", "message_id", message.ID, "reason", reason)
			} else if writeErr := s.deadLetters.Write(message, reason); writeErr != nil {
				s.logger.Error("write dead letter", "message_id", message.ID, "error", writeErr)
			} else {
				deadLettered++
			}
		} else {
			notified++
		}
		runningLast = message.ID
	}

	if runningLast > initialLast {
		nextState := *s.state
		nextState.LastProcessedMessageID = runningLast
		if err := s.store.Save(&nextState); err != nil {
			return fmt.Errorf("save last processed message id: %w", err)
		}
		s.state = &nextState
		serverHighest := s.serverHighest(runningLast)
		if err := s.updateHighest(ctx, s.secret, s.deviceID, serverHighest); err != nil {
			s.logger.Error("update highest message id", "message_id", serverHighest, "error", err)
		}
	} else if len(messages) > 0 && initialLast > 0 && messages[len(messages)-1].ID <= initialLast {
		// duplicate-only fetches reassert the durable cursor after a prior update failure.
		serverHighest := s.serverHighest(initialLast)
		if err := s.updateHighest(ctx, s.secret, s.deviceID, serverHighest); err != nil {
			s.logger.Error("update highest message id", "message_id", serverHighest, "error", err)
		}
	}
	batchHighest := int64(0)
	if len(messages) > 0 {
		batchHighest = messages[len(messages)-1].ID
	}
	s.logger.Info("sync complete",
		"batch_size", len(messages),
		"batch_highest", batchHighest,
		"processed", processed,
		"notified", notified,
		"dead_lettered", deadLettered,
	)
	return nil
}

func (s *syncer) serverHighest(natural int64) int64 {
	if s.emergencies == nil {
		return natural
	}
	minimum, ok := s.emergencies.MinOpenID()
	if !ok || minimum > natural {
		return natural
	}
	if minimum <= 0 {
		return 0
	}
	return minimum - 1
}

func (s *syncer) notification(ctx context.Context, message openclient.Message) notify.Notification {
	id := message.IDStr
	if id == "" {
		id = strconv.FormatInt(message.ID, 10)
	}
	notification := notify.Notification{
		ID:       id,
		Title:    message.Title,
		Body:     message.Message,
		AppName:  message.App,
		Priority: message.Priority,
		URL:      message.URL,
		URLTitle: message.URLTitle,
	}
	if message.Priority == 2 && message.Receipt != "" {
		notification.ThreadID = message.Receipt
	}
	if s.cache != nil {
		notification.IconPath = s.cache.Icon(ctx, message.Icon)
		sound := s.cache.Sound(ctx, message.Sound)
		notification.SoundPath = sound.path
		notification.SoundName = sound.name
	} else {
		notification.SoundName = message.Sound
	}
	return notification
}

func (s *syncer) notifyWithRetry(ctx context.Context, notification notify.Notification) error {
	var lastErr error
	for attempt := 0; attempt < notifyAttempts; attempt++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		lastErr = s.notifier.Notify(ctx, notification)
		if lastErr == nil {
			return nil
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		s.logger.Warn("notification delivery failed", "message_id", notification.ID, "attempt", attempt+1, "error", lastErr)
		if attempt+1 < notifyAttempts {
			if err := waitForClock(ctx, s.clock, notifyBackoff<<attempt); err != nil {
				return err
			}
		}
	}
	return lastErr
}
