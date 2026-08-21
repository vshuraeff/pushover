package listen

import (
	"context"
	"log/slog"
	"strconv"
	"sync"
	"time"

	"github.com/vshuraeff/pushover/internal/notify"
)

const emergencyLifetime = 3 * time.Hour

type emergencyState struct {
	messageID    int64
	notification notify.Notification
}

// clock and timer match the daemon's deterministic scheduling needs.
type Clock interface {
	Now() time.Time
	NewTimer(time.Duration) Timer
}

type Timer interface {
	C() <-chan time.Time
	Reset(time.Duration) bool
	Stop() bool
}

type scheduledEmergency struct {
	messageID    int64
	notification notify.Notification
	firstSeen    time.Time
	nextAlert    time.Time
	generation   uint64
}

type dueEmergency struct {
	receipt      string
	generation   uint64
	notification notify.Notification
}

// emergencyscheduler locally re-alerts receipts until the server view closes them.
// callers enforce the user-facing thirty-second minimum interval.
type EmergencyScheduler struct {
	notifier notify.Notifier
	interval time.Duration
	clock    Clock
	logger   *slog.Logger

	mu      sync.Mutex
	entries map[string]*scheduledEmergency
	capped  map[string]struct{}
	wake    chan struct{}
}

func NewEmergencyScheduler(notifier notify.Notifier, interval time.Duration, clock Clock, logger *slog.Logger) *EmergencyScheduler {
	if clock == nil {
		clock = realClock{}
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &EmergencyScheduler{
		notifier: notifier,
		interval: interval,
		clock:    clock,
		logger:   logger,
		entries:  make(map[string]*scheduledEmergency),
		capped:   make(map[string]struct{}),
		wake:     make(chan struct{}, 1),
	}
}

// update replaces the complete set of receipts that remain open in the latest fetch.
func (s *EmergencyScheduler) Update(open map[string]notify.Notification) {
	states := make(map[string]emergencyState, len(open))
	for receipt, notification := range open {
		messageID, _ := strconv.ParseInt(notification.ID, 10, 64)
		states[receipt] = emergencyState{messageID: messageID, notification: notification}
	}
	s.update(states)
}

func (s *EmergencyScheduler) update(open map[string]emergencyState) {
	now := s.clock.Now()
	s.mu.Lock()
	for receipt := range s.entries {
		if _, ok := open[receipt]; !ok {
			delete(s.entries, receipt)
		}
	}
	for receipt := range s.capped {
		if _, ok := open[receipt]; !ok {
			delete(s.capped, receipt)
		}
	}
	for receipt, emergency := range open {
		if _, capped := s.capped[receipt]; capped {
			continue
		}
		notification := emergency.notification
		notification.ThreadID = receipt
		if entry, ok := s.entries[receipt]; ok {
			entry.messageID = emergency.messageID
			entry.notification = notification
			entry.generation++
			continue
		}
		s.entries[receipt] = &scheduledEmergency{
			messageID:    emergency.messageID,
			notification: notification,
			firstSeen:    now,
			nextAlert:    now.Add(s.interval),
			generation:   1,
		}
	}
	s.mu.Unlock()
	s.signalWake()
}

// minopenid returns the lowest server message id still held open for acknowledgement.
func (s *EmergencyScheduler) MinOpenID() (int64, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var minimum int64
	found := false
	for _, entry := range s.entries {
		if !found || entry.messageID < minimum {
			minimum = entry.messageID
			found = true
		}
	}
	return minimum, found
}

// run processes scheduler updates and timer expirations until cancellation.
func (s *EmergencyScheduler) Run(ctx context.Context) {
	for {
		delay, ok := s.nextDelay()
		if !ok {
			select {
			case <-ctx.Done():
				return
			case <-s.wake:
				continue
			}
		}

		if delay < 0 {
			delay = 0
		}
		timer := s.clock.NewTimer(delay)
		select {
		case <-ctx.Done():
			stopTimer(timer)
			return
		case <-s.wake:
			stopTimer(timer)
			continue
		case <-timer.C():
		}

		for _, due := range s.takeDue(s.clock.Now()) {
			if !s.isCurrent(due) {
				continue
			}
			if err := s.notifier.Notify(ctx, due.notification); err != nil && ctx.Err() == nil {
				s.logger.Warn("emergency re-alert failed", "receipt", due.receipt, "error", err)
			}
		}
	}
}

func (s *EmergencyScheduler) nextDelay() (time.Duration, bool) {
	now := s.clock.Now()
	s.mu.Lock()
	defer s.mu.Unlock()
	var next time.Time
	for _, entry := range s.entries {
		candidate := entry.nextAlert
		capAt := entry.firstSeen.Add(emergencyLifetime)
		if capAt.Before(candidate) {
			candidate = capAt
		}
		if next.IsZero() || candidate.Before(next) {
			next = candidate
		}
	}
	if next.IsZero() {
		return 0, false
	}
	return next.Sub(now), true
}

func (s *EmergencyScheduler) takeDue(now time.Time) []dueEmergency {
	s.mu.Lock()
	defer s.mu.Unlock()
	due := make([]dueEmergency, 0)
	for receipt, entry := range s.entries {
		if !now.Before(entry.firstSeen.Add(emergencyLifetime)) {
			delete(s.entries, receipt)
			s.capped[receipt] = struct{}{}
			continue
		}
		if now.Before(entry.nextAlert) {
			continue
		}
		due = append(due, dueEmergency{
			receipt:      receipt,
			generation:   entry.generation,
			notification: entry.notification,
		})
		entry.nextAlert = now.Add(s.interval)
	}
	return due
}

func (s *EmergencyScheduler) isCurrent(due dueEmergency) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	entry, ok := s.entries[due.receipt]
	return ok && entry.generation == due.generation
}

func (s *EmergencyScheduler) signalWake() {
	select {
	case s.wake <- struct{}{}:
	default:
	}
}

func waitForClock(ctx context.Context, clock Clock, duration time.Duration) error {
	timer := clock.NewTimer(duration)
	defer stopTimer(timer)
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C():
		return nil
	}
}

func stopTimer(timer Timer) {
	if !timer.Stop() {
		select {
		case <-timer.C():
		default:
		}
	}
}

type realClock struct{}

func (realClock) Now() time.Time {
	return time.Now()
}

func (realClock) NewTimer(duration time.Duration) Timer {
	return &realTimer{timer: time.NewTimer(duration)}
}

type realTimer struct {
	timer *time.Timer
}

func (t *realTimer) C() <-chan time.Time {
	return t.timer.C
}

func (t *realTimer) Reset(duration time.Duration) bool {
	return t.timer.Reset(duration)
}

func (t *realTimer) Stop() bool {
	return t.timer.Stop()
}
