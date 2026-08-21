package openclient

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coder/websocket"
)

const (
	testDeviceID = "fake-device-id"
	testSecret   = "fake-secret-do-not-use"
)

func TestWebSocketEngine(t *testing.T) {
	t.Run("login frame is binary and exact", func(t *testing.T) {
		server := newWebSocketTestServer(t)
		engine := testEngine(server.url(), newFakeClock())
		cancel, _ := runTestEngine(t, engine)

		conn := server.nextConnection(t)
		if conn.loginType != websocket.MessageBinary {
			t.Errorf("login message type = %v, want %v", conn.loginType, websocket.MessageBinary)
		}
		want := "login:" + testDeviceID + ":" + testSecret + "\n"
		if string(conn.login) != want {
			t.Errorf("login payload did not match expected fake credentials")
		}

		cancel()
	})

	t.Run("sync frame invokes callback", func(t *testing.T) {
		server := newWebSocketTestServer(t)
		synced := make(chan struct{}, 1)
		engine := testEngine(server.url(), newFakeClock())
		engine.OnSync = func() { synced <- struct{}{} }
		cancel, _ := runTestEngine(t, engine)

		conn := server.nextConnection(t)
		conn.sendFrame(t, '!')
		select {
		case <-synced:
		case <-time.After(2 * time.Second):
			t.Fatal("OnSync was not called")
		}
		cancel()
	})

	t.Run("reconnect frame redials without backoff", func(t *testing.T) {
		server := newWebSocketTestServer(t)
		clock := newFakeClock()
		engine := testEngine(server.url(), clock)
		cancel, _ := runTestEngine(t, engine)

		first := server.nextConnection(t)
		if duration := clock.nextRequest(t); duration != 60*time.Second {
			t.Fatalf("first timer duration = %v, want %v", duration, 60*time.Second)
		}
		first.sendFrame(t, 'R')

		if duration := clock.nextRequest(t); duration != 60*time.Second {
			t.Fatalf("timer after R frame = %v, want immediate redial followed by liveness timer", duration)
		}
		server.nextConnection(t)
		if got := server.connectionCount.Load(); got != 2 {
			t.Errorf("connection count = %d, want 2", got)
		}
		cancel()
	})

	t.Run("reconnect frame preserves accumulated backoff", func(t *testing.T) {
		server := newWebSocketTestServer(t)
		clock := newFakeClock()
		var dialCount atomic.Int32
		dials := make(chan int32, 4)
		var logBuf bytes.Buffer
		engine := testEngine(server.url(), clock)
		engine.Logger = slog.New(slog.NewTextHandler(&logBuf, nil))
		engine.Dialer = func(ctx context.Context, url string) (*websocket.Conn, *http.Response, error) {
			dial := dialCount.Add(1)
			dials <- dial
			switch dial {
			case 1, 2, 4:
				return nil, nil, errors.New("forced dial failure")
			default:
				return websocket.Dial(ctx, url, nil)
			}
		}
		cancel, _ := runTestEngine(t, engine)

		for attempt := uint(0); attempt < 2; attempt++ {
			if dial := waitForDial(t, dials); dial != int32(attempt+1) {
				t.Fatalf("dial = %d, want %d", dial, attempt+1)
			}
			delay := clock.nextRequest(t)
			if delay < 0 || delay >= backoffLimit(attempt) {
				t.Fatalf("attempt %d delay = %v, want [0, %v)", attempt, delay, backoffLimit(attempt))
			}
			clock.Advance(delay)
		}

		conn := server.nextConnection(t)
		if dial := waitForDial(t, dials); dial != 3 {
			t.Fatalf("dial = %d, want 3", dial)
		}
		if duration := clock.nextRequest(t); duration != livenessTimeout {
			t.Fatalf("liveness timer duration = %v, want %v", duration, livenessTimeout)
		}
		conn.sendFrame(t, 'R')

		if duration := clock.nextRequest(t); duration != livenessTimeout {
			t.Fatalf("reset liveness timer duration = %v, want %v", duration, livenessTimeout)
		}
		if dial := waitForDial(t, dials); dial != 4 {
			t.Fatalf("dial after R frame = %d, want immediate dial 4", dial)
		}
		delay := clock.nextRequest(t)
		if delay < 0 || delay >= backoffLimit(2) {
			t.Fatalf("backoff after R frame = %v, want [0, %v)", delay, backoffLimit(2))
		}
		if !bytes.Contains(logBuf.Bytes(), []byte("attempt=2")) {
			t.Fatalf("dial after R frame did not retain attempt 2: %q", logBuf.String())
		}
		cancel()
	})

	t.Run("permanent frame stops reconnecting", func(t *testing.T) {
		server := newWebSocketTestServer(t)
		engine := testEngine(server.url(), newFakeClock())
		_, result := runTestEngine(t, engine)

		server.nextConnection(t).sendFrame(t, 'E')
		if err := waitForRunResult(t, result); !errors.Is(err, ErrPermanent) {
			t.Fatalf("Run() error = %v, want ErrPermanent", err)
		}
		server.expectNoConnection(t)
	})

	t.Run("replaced frame stops reconnecting", func(t *testing.T) {
		server := newWebSocketTestServer(t)
		engine := testEngine(server.url(), newFakeClock())
		_, result := runTestEngine(t, engine)

		server.nextConnection(t).sendFrame(t, 'A')
		if err := waitForRunResult(t, result); !errors.Is(err, ErrReplaced) {
			t.Fatalf("Run() error = %v, want ErrReplaced", err)
		}
		server.expectNoConnection(t)
	})

	t.Run("silent connections retain backoff", func(t *testing.T) {
		server := newWebSocketTestServer(t)
		clock := newFakeClock()
		var dialCount atomic.Int32
		dials := make(chan int32, 4)
		var logBuf bytes.Buffer
		engine := testEngine(server.url(), clock)
		engine.Logger = slog.New(slog.NewTextHandler(&logBuf, nil))
		engine.Dialer = func(ctx context.Context, url string) (*websocket.Conn, *http.Response, error) {
			dial := dialCount.Add(1)
			dials <- dial
			if dial <= 3 {
				return websocket.Dial(ctx, url, nil)
			}
			return nil, nil, errors.New("forced dial failure")
		}
		cancel, _ := runTestEngine(t, engine)

		for attempt := uint(0); attempt < 3; attempt++ {
			if dial := waitForDial(t, dials); dial != int32(attempt+1) {
				t.Fatalf("dial = %d, want %d", dial, attempt+1)
			}
			server.nextConnection(t)
			if duration := clock.nextRequest(t); duration != livenessTimeout {
				t.Fatalf("liveness timer duration = %v, want %v", duration, livenessTimeout)
			}
			clock.Advance(livenessTimeout)
			delay := clock.nextRequest(t)
			if delay < 0 || delay >= backoffLimit(attempt) {
				t.Fatalf("attempt %d delay = %v, want [0, %v)", attempt, delay, backoffLimit(attempt))
			}
			clock.Advance(delay)
		}

		if dial := waitForDial(t, dials); dial != 4 {
			t.Fatalf("dial = %d, want 4", dial)
		}
		if delay := clock.nextRequest(t); delay < 0 || delay >= backoffLimit(3) {
			t.Fatalf("attempt 3 delay = %v, want [0, %v)", delay, backoffLimit(3))
		}
		if !bytes.Contains(logBuf.Bytes(), []byte("attempt=3")) {
			t.Fatalf("silent connections reset backoff: %q", logBuf.String())
		}
		cancel()
	})

	t.Run("keepalive frame resets liveness timer", func(t *testing.T) {
		server := newWebSocketTestServer(t)
		clock := newFakeClock()
		engine := testEngine(server.url(), clock)
		cancel, _ := runTestEngine(t, engine)

		conn := server.nextConnection(t)
		if duration := clock.nextRequest(t); duration != livenessTimeout {
			t.Fatalf("liveness timer duration = %v, want %v", duration, livenessTimeout)
		}
		clock.Advance(59 * time.Second)
		conn.sendFrame(t, '#')
		if duration := clock.nextRequest(t); duration != livenessTimeout {
			t.Fatalf("reset liveness timer duration = %v, want %v", duration, livenessTimeout)
		}
		clock.Advance(59 * time.Second)
		server.expectNoConnection(t)

		clock.Advance(2 * time.Second)
		delay := clock.nextRequest(t)
		if delay < 0 || delay >= backoffBase {
			t.Fatalf("reconnect delay = %v, want [0, %v)", delay, backoffBase)
		}
		clock.Advance(delay)
		server.nextConnection(t)
		if got := server.connectionCount.Load(); got != 2 {
			t.Errorf("connection count = %d, want 2", got)
		}
		cancel()
	})

	t.Run("network failures use capped exponential full jitter", func(t *testing.T) {
		clock := newFakeClock()
		dials := make(chan struct{}, 16)
		engine := testEngine("ws://127.0.0.1.invalid", clock)
		engine.Dialer = func(context.Context, string) (*websocket.Conn, *http.Response, error) {
			dials <- struct{}{}
			return nil, nil, errors.New("forced dial failure")
		}
		cancel, result := runTestEngine(t, engine)

		waitForSignal(t, dials, "initial dial")
		previousBound := time.Duration(0)
		for attempt := uint(0); attempt < 9; attempt++ {
			delay := clock.nextRequest(t)
			bound := expectedBackoffBound(attempt)
			if delay < 0 || delay >= bound {
				t.Errorf("attempt %d delay = %v, want [0, %v)", attempt, delay, bound)
			}
			if bound < previousBound {
				t.Errorf("attempt %d bound = %v, previous bound = %v", attempt, bound, previousBound)
			}
			if bound > 2*time.Minute {
				t.Errorf("attempt %d bound = %v, cap = %v", attempt, bound, 2*time.Minute)
			}
			previousBound = bound
			clock.Advance(delay)
			waitForSignal(t, dials, fmt.Sprintf("dial after attempt %d", attempt))
		}
		cancel()
		if err := waitForRunResult(t, result); !errors.Is(err, context.Canceled) {
			t.Fatalf("Run() error = %v, want context.Canceled", err)
		}
	})

	t.Run("connection shorter than stability threshold retains backoff", func(t *testing.T) {
		server := newWebSocketTestServer(t)
		clock := newFakeClock()
		var dialCount atomic.Int32
		dials := make(chan int32, 4)
		var logBuf bytes.Buffer
		engine := testEngine(server.url(), clock)
		engine.Logger = slog.New(slog.NewTextHandler(&logBuf, nil))
		engine.Dialer = func(ctx context.Context, url string) (*websocket.Conn, *http.Response, error) {
			dial := dialCount.Add(1)
			dials <- dial
			switch dial {
			case 1, 2, 4:
				return nil, nil, errors.New("forced dial failure")
			default:
				return websocket.Dial(ctx, url, nil)
			}
		}
		cancel, _ := runTestEngine(t, engine)

		for attempt := uint(0); attempt < 2; attempt++ {
			if dial := waitForDial(t, dials); dial != int32(attempt+1) {
				t.Fatalf("dial = %d, want %d", dial, attempt+1)
			}
			delay := clock.nextRequest(t)
			if delay < 0 || delay >= backoffLimit(attempt) {
				t.Fatalf("attempt %d delay = %v, want [0, %v)", attempt, delay, backoffLimit(attempt))
			}
			clock.Advance(delay)
		}

		conn := server.nextConnection(t)
		if dial := waitForDial(t, dials); dial != 3 {
			t.Fatalf("dial = %d, want 3", dial)
		}
		if duration := clock.nextRequest(t); duration != livenessTimeout {
			t.Fatalf("liveness timer duration = %v, want %v", duration, livenessTimeout)
		}
		clock.Advance(livenessTimeout - time.Second)
		conn.sendFrame(t, '#')
		if duration := clock.nextRequest(t); duration != livenessTimeout {
			t.Fatalf("reset liveness timer duration = %v, want %v", duration, livenessTimeout)
		}
		clock.Advance(2 * time.Second)
		if err := conn.conn.CloseNow(); err != nil {
			t.Fatalf("close websocket connection: %v", err)
		}

		delay := clock.nextRequest(t)
		if delay < 0 || delay >= backoffLimit(2) {
			t.Fatalf("backoff after short connection = %v, want [0, %v)", delay, backoffLimit(2))
		}
		clock.Advance(delay)
		if dial := waitForDial(t, dials); dial != 4 {
			t.Fatalf("dial = %d, want 4", dial)
		}
		if delay := clock.nextRequest(t); delay < 0 || delay >= backoffLimit(3) {
			t.Fatalf("attempt 3 delay = %v, want [0, %v)", delay, backoffLimit(3))
		}
		if !bytes.Contains(logBuf.Bytes(), []byte("attempt=3")) {
			t.Fatalf("short connection reset backoff: %q", logBuf.String())
		}
		cancel()
	})

	t.Run("stable connection resets backoff to base", func(t *testing.T) {
		server := newWebSocketTestServer(t)
		clock := newFakeClock()
		var dialCount atomic.Int32
		dials := make(chan int32, 4)
		var logBuf bytes.Buffer
		engine := testEngine(server.url(), clock)
		engine.Logger = slog.New(slog.NewTextHandler(&logBuf, nil))
		engine.Dialer = func(ctx context.Context, url string) (*websocket.Conn, *http.Response, error) {
			dial := dialCount.Add(1)
			dials <- dial
			if dial == 1 || dial == 2 || dial == 4 {
				return nil, nil, errors.New("forced dial failure")
			}
			return websocket.Dial(ctx, url, nil)
		}
		cancel, _ := runTestEngine(t, engine)

		for attempt := uint(0); attempt < 2; attempt++ {
			if dial := waitForDial(t, dials); dial != int32(attempt+1) {
				t.Fatalf("dial = %d, want %d", dial, attempt+1)
			}
			delay := clock.nextRequest(t)
			if delay < 0 || delay >= backoffLimit(attempt) {
				t.Fatalf("attempt %d delay = %v, want [0, %v)", attempt, delay, backoffLimit(attempt))
			}
			clock.Advance(delay)
		}

		conn := server.nextConnection(t)
		if dial := waitForDial(t, dials); dial != 3 {
			t.Fatalf("dial = %d, want 3", dial)
		}
		if duration := clock.nextRequest(t); duration != livenessTimeout {
			t.Fatalf("liveness timer duration = %v, want %v", duration, livenessTimeout)
		}
		for range 5 {
			clock.Advance(59 * time.Second)
			conn.sendFrame(t, '#')
			if duration := clock.nextRequest(t); duration != livenessTimeout {
				t.Fatalf("reset liveness timer duration = %v, want %v", duration, livenessTimeout)
			}
		}
		clock.Advance(6 * time.Second)
		if err := conn.conn.CloseNow(); err != nil {
			t.Fatalf("close websocket connection: %v", err)
		}

		delay := clock.nextRequest(t)
		if delay < 0 || delay >= backoffBase {
			t.Fatalf("backoff after stable connection = %v, want [0, %v)", delay, backoffBase)
		}
		clock.Advance(delay)
		if dial := waitForDial(t, dials); dial != 4 {
			t.Fatalf("dial = %d, want 4", dial)
		}
		if delay := clock.nextRequest(t); delay < 0 || delay >= backoffLimit(1) {
			t.Fatalf("attempt 1 delay = %v, want [0, %v)", delay, backoffLimit(1))
		}
		if !bytes.Contains(logBuf.Bytes(), []byte("attempt=1")) {
			t.Fatalf("stable connection did not reset backoff: %q", logBuf.String())
		}
		cancel()
	})

	t.Run("run validates required fields", func(t *testing.T) {
		tests := []struct {
			name string
			set  func(*Engine)
		}{
			{
				name: "url",
				set:  func(engine *Engine) { engine.URL = "" },
			},
			{
				name: "device id",
				set:  func(engine *Engine) { engine.DeviceID = "" },
			},
			{
				name: "secret",
				set:  func(engine *Engine) { engine.Secret = "" },
			},
		}

		for _, test := range tests {
			t.Run(test.name, func(t *testing.T) {
				engine := testEngine("ws://example.invalid", newFakeClock())
				test.set(engine)
				dialed := false
				engine.Dialer = func(context.Context, string) (*websocket.Conn, *http.Response, error) {
					dialed = true
					return nil, nil, errors.New("unexpected dial")
				}

				if err := engine.Run(context.Background()); err == nil {
					t.Fatal("Run() error = nil, want validation error")
				}
				if dialed {
					t.Fatal("Run() attempted a dial after validation failed")
				}
			})
		}
	})

	t.Run("reconnect attempts are logged without leaking the secret", func(t *testing.T) {
		clock := newFakeClock()
		var logBuf bytes.Buffer
		engine := testEngine("ws://example.invalid", clock)
		engine.Logger = slog.New(slog.NewTextHandler(&logBuf, nil))
		engine.Dialer = func(context.Context, string) (*websocket.Conn, *http.Response, error) {
			return nil, nil, errors.New("forced dial failure")
		}
		cancel, result := runTestEngine(t, engine)

		clock.nextRequest(t)
		if logBuf.Len() == 0 {
			t.Fatal("expected dial failure to be logged")
		}
		if !bytes.Contains(logBuf.Bytes(), []byte("forced dial failure")) {
			t.Fatalf("log output did not include the dial failure: %q", logBuf.String())
		}
		if !bytes.Contains(logBuf.Bytes(), []byte("attempt")) {
			t.Fatalf("log output did not include the attempt: %q", logBuf.String())
		}
		if bytes.Contains(logBuf.Bytes(), []byte(testSecret)) {
			t.Fatal("log output leaked the websocket secret")
		}
		cancel()
		if err := waitForRunResult(t, result); !errors.Is(err, context.Canceled) {
			t.Fatalf("Run() error = %v, want context.Canceled", err)
		}
	})

	t.Run("context cancellation interrupts work", func(t *testing.T) {
		t.Run("read", func(t *testing.T) {
			server := newWebSocketTestServer(t)
			engine := testEngine(server.url(), newFakeClock())
			cancel, result := runTestEngine(t, engine)

			server.nextConnection(t)
			cancel()
			if err := waitForRunResult(t, result); !errors.Is(err, context.Canceled) {
				t.Fatalf("Run() error = %v, want context.Canceled", err)
			}
		})

		t.Run("backoff", func(t *testing.T) {
			clock := newFakeClock()
			engine := testEngine("ws://127.0.0.1.invalid", clock)
			engine.Dialer = func(context.Context, string) (*websocket.Conn, *http.Response, error) {
				return nil, nil, errors.New("forced dial failure")
			}
			cancel, result := runTestEngine(t, engine)

			clock.nextRequest(t)
			cancel()
			if err := waitForRunResult(t, result); !errors.Is(err, context.Canceled) {
				t.Fatalf("Run() error = %v, want context.Canceled", err)
			}
		})
	})
}

func testEngine(url string, clock Clock) *Engine {
	return &Engine{
		URL:      url,
		DeviceID: testDeviceID,
		Secret:   testSecret,
		Clock:    clock,
	}
}

func runTestEngine(t *testing.T, engine *Engine) (context.CancelFunc, <-chan error) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() {
		result <- engine.Run(ctx)
		close(result)
	}()
	t.Cleanup(func() {
		cancel()
		select {
		case <-result:
		case <-time.After(2 * time.Second):
			t.Error("Run did not stop during cleanup")
		}
	})
	return cancel, result
}

func waitForRunResult(t *testing.T, result <-chan error) error {
	t.Helper()
	select {
	case err := <-result:
		return err
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not return")
		return nil
	}
}

func waitForSignal(t *testing.T, signals <-chan struct{}, description string) {
	t.Helper()
	select {
	case <-signals:
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for %s", description)
	}
}

func waitForDial(t *testing.T, dials <-chan int32) int32 {
	t.Helper()
	select {
	case dial := <-dials:
		return dial
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for dial")
		return 0
	}
}

func expectedBackoffBound(attempt uint) time.Duration {
	bound := time.Second * time.Duration(1<<attempt)
	if bound > 2*time.Minute {
		return 2 * time.Minute
	}
	return bound
}

type webSocketTestServer struct {
	server          *httptest.Server
	connections     chan *webSocketTestConnection
	connectionCount atomic.Int32
	mu              sync.Mutex
	active          map[*websocket.Conn]struct{}
}

type webSocketTestConnection struct {
	conn      *websocket.Conn
	loginType websocket.MessageType
	login     []byte
	loginErr  error
}

func newWebSocketTestServer(t *testing.T) *webSocketTestServer {
	t.Helper()
	s := &webSocketTestServer{
		connections: make(chan *webSocketTestConnection, 16),
		active:      make(map[*websocket.Conn]struct{}),
	}
	s.server = httptest.NewServer(http.HandlerFunc(s.handle))
	t.Cleanup(s.close)
	return s
}

func (s *webSocketTestServer) handle(writer http.ResponseWriter, request *http.Request) {
	conn, err := websocket.Accept(writer, request, nil)
	if err != nil {
		return
	}
	s.mu.Lock()
	s.active[conn] = struct{}{}
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		delete(s.active, conn)
		s.mu.Unlock()
		_ = conn.CloseNow()
	}()

	messageType, login, loginErr := conn.Read(context.Background())
	s.connectionCount.Add(1)
	s.connections <- &webSocketTestConnection{
		conn:      conn,
		loginType: messageType,
		login:     login,
		loginErr:  loginErr,
	}
	if loginErr != nil {
		return
	}
	_, _, _ = conn.Read(context.Background())
}

func (s *webSocketTestServer) url() string {
	return "ws" + s.server.URL[len("http"):]
}

func (s *webSocketTestServer) nextConnection(t *testing.T) *webSocketTestConnection {
	t.Helper()
	select {
	case conn := <-s.connections:
		if conn.loginErr != nil {
			t.Fatalf("read login: %v", conn.loginErr)
		}
		return conn
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for websocket connection")
		return nil
	}
}

func (s *webSocketTestServer) expectNoConnection(t *testing.T) {
	t.Helper()
	select {
	case <-s.connections:
		t.Fatal("unexpected second websocket connection")
	case <-time.After(100 * time.Millisecond):
	}
}

func (s *webSocketTestServer) close() {
	s.mu.Lock()
	for conn := range s.active {
		_ = conn.CloseNow()
	}
	s.mu.Unlock()
	s.server.Close()
}

// server frames use text messages; the engine accepts either websocket data message type.
func (c *webSocketTestConnection) sendFrame(t *testing.T, frame byte) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := c.conn.Write(ctx, websocket.MessageText, []byte{frame}); err != nil {
		t.Fatalf("write frame %q: %v", frame, err)
	}
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
