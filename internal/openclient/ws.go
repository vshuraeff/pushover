package openclient

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"net/http"
	"time"

	"github.com/coder/websocket"
)

const (
	livenessTimeout    = 60 * time.Second
	stabilityThreshold = 5 * time.Minute
	backoffBase        = time.Second
	backoffCap         = 2 * time.Minute
)

var (
	ErrPermanent = errors.New("websocket credentials permanently rejected")
	ErrReplaced  = errors.New("websocket connection replaced")
)

// clock abstracts time for deterministic tests.
type Clock interface {
	Now() time.Time
	NewTimer(time.Duration) Timer
}

// timer is the resettable clock timer used by the engine.
type Timer interface {
	C() <-chan time.Time
	Reset(time.Duration) bool
	Stop() bool
}

// engine maintains the pushover open client websocket connection.
type Engine struct {
	URL      string
	DeviceID string
	Secret   string
	OnSync   func()
	Clock    Clock
	Logger   *slog.Logger
	Dialer   func(context.Context, string) (*websocket.Conn, *http.Response, error)
}

// run connects and processes websocket frames until cancellation or a terminal frame.
func (e *Engine) Run(ctx context.Context) error {
	if e.URL == "" {
		return fmt.Errorf("websocket url is required")
	}
	if e.DeviceID == "" {
		return fmt.Errorf("websocket device id is required")
	}
	if e.Secret == "" {
		return fmt.Errorf("websocket secret is required")
	}

	clock := e.Clock
	if clock == nil {
		clock = realClock{}
	}
	logger := e.Logger
	if logger == nil {
		logger = slog.Default()
	}
	dial := e.Dialer
	if dial == nil {
		dial = func(ctx context.Context, url string) (*websocket.Conn, *http.Response, error) {
			return websocket.Dial(ctx, url, nil)
		}
	}

	var attempt uint
	for {
		if err := ctx.Err(); err != nil {
			return err
		}

		conn, _, err := dial(ctx, e.URL)
		if err != nil {
			logger.Warn("websocket dial failed", "attempt", attempt, "err", err)
			if conn != nil {
				_ = conn.CloseNow()
			}
			if ctxErr := ctx.Err(); ctxErr != nil {
				return ctxErr
			}
			if err := waitForBackoff(ctx, clock, attempt); err != nil {
				return err
			}
			attempt = nextAttempt(attempt)
			continue
		}

		outcome, stable := func() (connectionOutcome, bool) {
			defer conn.CloseNow()
			return e.serve(ctx, conn, clock, logger, attempt)
		}()

		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		switch outcome {
		case connectionPermanent:
			return ErrPermanent
		case connectionReplaced:
			return ErrReplaced
		case connectionImmediateReconnect:
			continue
		case connectionFailed:
			if stable {
				attempt = 0
			}
			if err := waitForBackoff(ctx, clock, attempt); err != nil {
				return err
			}
			attempt = nextAttempt(attempt)
		}
	}
}

type connectionOutcome uint8

const (
	connectionFailed connectionOutcome = iota
	connectionImmediateReconnect
	connectionPermanent
	connectionReplaced
)

type readResult struct {
	payload []byte
	err     error
}

func (e *Engine) serve(ctx context.Context, conn *websocket.Conn, clock Clock, logger *slog.Logger, attempt uint) (connectionOutcome, bool) {
	login := []byte("login:" + e.DeviceID + ":" + e.Secret + "\n")
	if err := conn.Write(ctx, websocket.MessageBinary, login); err != nil {
		logger.Warn("websocket login write failed", "attempt", attempt, "err", err)
		return connectionFailed, false
	}

	connectedAt := clock.Now()
	liveness := clock.NewTimer(livenessTimeout)
	defer stopTimer(liveness)

	readCtx, cancelRead := context.WithCancel(ctx)
	defer cancelRead()
	reads := make(chan readResult, 1)
	go readFrames(readCtx, conn, reads)

	for {
		select {
		case <-ctx.Done():
			return connectionFailed, clock.Now().Sub(connectedAt) >= stabilityThreshold
		case <-liveness.C():
			return connectionFailed, clock.Now().Sub(connectedAt) >= stabilityThreshold
		case result := <-reads:
			if result.err != nil {
				logger.Debug("websocket read failed", "attempt", attempt, "err", result.err)
				return connectionFailed, clock.Now().Sub(connectedAt) >= stabilityThreshold
			}
			resetTimer(liveness, livenessTimeout)
			if len(result.payload) == 0 {
				logger.Warn("unknown websocket frame", "length", 0)
				continue
			}
			if len(result.payload) != 1 {
				logger.Warn("unexpected websocket frame length", "length", len(result.payload))
			}
			switch result.payload[0] {
			case '#':
			case '!':
				if e.OnSync != nil {
					e.OnSync()
				}
			case 'R':
				return connectionImmediateReconnect, false
			case 'E':
				return connectionPermanent, false
			case 'A':
				return connectionReplaced, false
			default:
				logger.Warn("unknown websocket frame", "frame", result.payload[0])
			}
		}
	}
}

func readFrames(ctx context.Context, conn *websocket.Conn, results chan<- readResult) {
	for {
		_, payload, err := conn.Read(ctx)
		select {
		case results <- readResult{payload: payload, err: err}:
		case <-ctx.Done():
			return
		}
		if err != nil {
			return
		}
	}
}

func waitForBackoff(ctx context.Context, clock Clock, attempt uint) error {
	limit := backoffLimit(attempt)
	delay := time.Duration(rand.Int64N(int64(limit)))
	timer := clock.NewTimer(delay)
	defer stopTimer(timer)

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C():
		return nil
	}
}

func backoffLimit(attempt uint) time.Duration {
	limit := backoffBase
	for range attempt {
		if limit >= backoffCap/2 {
			return backoffCap
		}
		limit *= 2
	}
	return limit
}

func nextAttempt(attempt uint) uint {
	if backoffLimit(attempt) < backoffCap {
		return attempt + 1
	}
	return attempt
}

func resetTimer(timer Timer, duration time.Duration) {
	if !timer.Stop() {
		select {
		case <-timer.C():
		default:
		}
	}
	timer.Reset(duration)
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
