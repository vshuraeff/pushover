package listen

import (
	"context"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/vshuraeff/pushover/internal/notify"
)

func TestEmergencySchedulerRealertsAndStopsOnDisappearance(t *testing.T) {
	clock := newFakeClock()
	notifier := &channelNotifier{notifications: make(chan notify.Notification, 4)}
	scheduler := NewEmergencyScheduler(notifier, 30*time.Second, clock, testLogger())
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		scheduler.Run(ctx)
		close(done)
	}()
	t.Cleanup(func() {
		cancel()
		waitForDone(t, done, "emergency scheduler")
	})

	scheduler.Update(map[string]notify.Notification{
		"fake-receipt-123": {ID: "10", Body: "first state"},
	})
	if got := clock.nextRequest(t); got != 30*time.Second {
		t.Fatalf("timer duration = %v, want %v", got, 30*time.Second)
	}
	clock.Advance(30 * time.Second)
	first := waitForNotification(t, notifier.notifications)
	if first.ThreadID != "fake-receipt-123" || first.Body != "first state" {
		t.Fatalf("notification = %#v", first)
	}

	clock.nextRequest(t)
	scheduler.Update(map[string]notify.Notification{
		"fake-receipt-123": {ID: "10", Body: "latest state"},
	})
	clock.nextRequest(t)
	clock.Advance(30 * time.Second)
	second := waitForNotification(t, notifier.notifications)
	if second.ThreadID != "fake-receipt-123" || second.Body != "latest state" {
		t.Fatalf("notification = %#v", second)
	}

	scheduler.Update(map[string]notify.Notification{})
	scheduler.mu.Lock()
	tracked := len(scheduler.entries)
	scheduler.mu.Unlock()
	if tracked != 0 {
		t.Fatalf("tracked emergencies = %d, want 0", tracked)
	}
}

func TestEmergencySchedulerCapsContinuousReceiptAtThreeHours(t *testing.T) {
	clock := newFakeClock()
	scheduler := NewEmergencyScheduler(&channelNotifier{}, time.Minute, clock, testLogger())
	open := map[string]notify.Notification{"fake-receipt-cap": {ID: "20"}}
	scheduler.Update(open)

	clock.Advance(emergencyLifetime)
	if due := scheduler.takeDue(clock.Now()); len(due) != 0 {
		t.Fatalf("due notifications at cap = %d, want 0", len(due))
	}
	scheduler.Update(open)
	scheduler.mu.Lock()
	trackedAfterCap := len(scheduler.entries)
	_, capped := scheduler.capped["fake-receipt-cap"]
	scheduler.mu.Unlock()
	if trackedAfterCap != 0 || !capped {
		t.Fatalf("tracked after cap = %d, capped = %t", trackedAfterCap, capped)
	}

	scheduler.Update(map[string]notify.Notification{})
	scheduler.Update(open)
	scheduler.mu.Lock()
	trackedAfterDisappearance := len(scheduler.entries)
	scheduler.mu.Unlock()
	if trackedAfterDisappearance != 1 {
		t.Fatalf("tracked after disappearance and return = %d, want 1", trackedAfterDisappearance)
	}
}

type channelNotifier struct {
	notifications chan notify.Notification
}

func (n *channelNotifier) Notify(_ context.Context, notification notify.Notification) error {
	if n.notifications != nil {
		n.notifications <- notification
	}
	return nil
}

func (*channelNotifier) Caps() notify.Caps {
	return notify.Caps{Images: true, Sound: true}
}

type fakeClock struct {
	mu       sync.Mutex
	now      time.Time
	timers   map[*fakeTimer]struct{}
	requests chan time.Duration
}

type fakeTimer struct {
	clock    *fakeClock
	channel  chan time.Time
	deadline time.Time
	active   bool
}

func newFakeClock() *fakeClock {
	return &fakeClock{
		now:      time.Unix(1_700_000_000, 0),
		timers:   make(map[*fakeTimer]struct{}),
		requests: make(chan time.Duration, 256),
	}
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *fakeClock) NewTimer(duration time.Duration) Timer {
	c.mu.Lock()
	timer := &fakeTimer{
		clock:    c,
		channel:  make(chan time.Time, 1),
		deadline: c.now.Add(duration),
		active:   true,
	}
	c.timers[timer] = struct{}{}
	c.requests <- duration
	c.fireTimer(timer)
	c.mu.Unlock()
	return timer
}

func (c *fakeClock) Advance(duration time.Duration) {
	c.mu.Lock()
	c.now = c.now.Add(duration)
	for timer := range c.timers {
		c.fireTimer(timer)
	}
	c.mu.Unlock()
}

func (c *fakeClock) nextRequest(t *testing.T) time.Duration {
	t.Helper()
	select {
	case duration := <-c.requests:
		return duration
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for clock request")
		return 0
	}
}

func (c *fakeClock) fireTimer(timer *fakeTimer) {
	if !timer.active || timer.deadline.After(c.now) {
		return
	}
	timer.active = false
	timer.channel <- c.now
}

func (t *fakeTimer) C() <-chan time.Time {
	return t.channel
}

func (t *fakeTimer) Reset(duration time.Duration) bool {
	t.clock.mu.Lock()
	wasActive := t.active
	t.deadline = t.clock.now.Add(duration)
	t.active = true
	t.clock.requests <- duration
	t.clock.fireTimer(t)
	t.clock.mu.Unlock()
	return wasActive
}

func (t *fakeTimer) Stop() bool {
	t.clock.mu.Lock()
	wasActive := t.active
	t.active = false
	t.clock.mu.Unlock()
	return wasActive
}

func waitForNotification(t *testing.T, notifications <-chan notify.Notification) notify.Notification {
	t.Helper()
	select {
	case notification := <-notifications:
		return notification
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for notification")
		return notify.Notification{}
	}
}

func waitForDone(t *testing.T, done <-chan struct{}, description string) {
	t.Helper()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for %s", description)
	}
}

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}
