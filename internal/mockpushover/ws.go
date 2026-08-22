package mockpushover

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/coder/websocket"
)

var errNoWebSocketClient = errors.New("websocket client is not connected")

// defaultLoginTimeout and defaultKeepaliveInterval mirror the real pushover
// open client websocket behavior (a text-frame-only login gate and a
// proactive keepalive), scaled down so tests do not have to wait out the
// production 60s login window.
const (
	defaultLoginTimeout      = 3 * time.Second
	defaultKeepaliveInterval = 2 * time.Second
)

type webSocketState struct {
	mu            sync.Mutex
	conn          *websocket.Conn
	loginDeviceID string
	loginOK       bool
	connectCount  int
	keepaliveStop chan struct{}
}

func (s *Server) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(s, w, r, http.MethodGet) {
		return
	}
	conn, err := websocket.Accept(w, r, nil)
	if err != nil {
		return
	}
	conn.SetReadLimit(4 << 10)

	msgType, payload, readErr := conn.Read(r.Context())
	if readErr != nil {
		_ = conn.CloseNow()
		return
	}

	if msgType != websocket.MessageText {
		// The real pushover server silently ignores a login frame sent over a
		// binary websocket message: no error frame, no session, the
		// connection just idles until the server-side timeout closes it.
		s.waitForLoginTimeout(r.Context())
		_ = conn.CloseNow()
		return
	}

	deviceID, secret, parsed := parseWebSocketLogin(payload)
	if !parsed || !s.validWebSocketLogin(deviceID, secret) {
		s.ws.mu.Lock()
		s.ws.loginOK = false
		s.ws.mu.Unlock()
		_ = writeWebSocketFrame(conn, 'E')
		_ = conn.CloseNow()
		return
	}

	s.registerWebSocket(conn, deviceID)
	_, _, _ = conn.Read(context.Background())
	s.disconnectWebSocket(conn)
}

func (s *Server) waitForLoginTimeout(ctx context.Context) {
	timer := time.NewTimer(s.loginTimeout)
	defer timer.Stop()
	select {
	case <-timer.C:
	case <-ctx.Done():
	}
}

func parseWebSocketLogin(payload []byte) (string, string, bool) {
	login := string(payload)
	if !strings.HasSuffix(login, "\n") {
		return "", "", false
	}
	parts := strings.Split(strings.TrimSuffix(login, "\n"), ":")
	if len(parts) != 3 || parts[0] != "login" {
		return "", "", false
	}
	return parts[1], parts[2], true
}

func (s *Server) validWebSocketLogin(deviceID, secret string) bool {
	s.store.mu.RLock()
	device, ok := s.store.devices[deviceID]
	s.store.mu.RUnlock()
	return ok && device.Enabled && device.Secret == secret && secret != ""
}

func (s *Server) registerWebSocket(conn *websocket.Conn, deviceID string) {
	s.ws.mu.Lock()
	if s.ws.conn != nil {
		_ = writeWebSocketFrame(s.ws.conn, 'A')
		_ = s.ws.conn.CloseNow()
	}
	s.stopKeepaliveLocked()
	stop := make(chan struct{})
	s.ws.conn = conn
	s.ws.loginDeviceID = deviceID
	s.ws.loginOK = true
	s.ws.connectCount++
	s.ws.keepaliveStop = stop
	s.ws.mu.Unlock()
	go s.runKeepalive(stop)
}

// runKeepalive proactively sends '#' frames to the logged-in session at
// s.keepaliveInterval, mirroring the real server so listeners never rely
// solely on client-triggered control frames.
func (s *Server) runKeepalive(stop chan struct{}) {
	if s.keepaliveInterval <= 0 {
		return
	}
	ticker := time.NewTicker(s.keepaliveInterval)
	defer ticker.Stop()
	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
			if err := s.sendWebSocketFrame('#'); err != nil {
				return
			}
		}
	}
}

// stopKeepaliveLocked stops the active keepalive goroutine, if any. Callers
// must hold s.ws.mu.
func (s *Server) stopKeepaliveLocked() {
	if s.ws.keepaliveStop != nil {
		close(s.ws.keepaliveStop)
		s.ws.keepaliveStop = nil
	}
}

func (s *Server) disconnectWebSocket(conn *websocket.Conn) {
	s.ws.mu.Lock()
	if s.ws.conn == conn {
		s.ws.conn = nil
		s.stopKeepaliveLocked()
	}
	s.ws.mu.Unlock()
	_ = conn.CloseNow()
}

func (s *Server) sendWebSocketFrame(frame byte) error {
	s.ws.mu.Lock()
	defer s.ws.mu.Unlock()
	if s.ws.conn == nil {
		return errNoWebSocketClient
	}
	if err := writeWebSocketFrame(s.ws.conn, frame); err != nil {
		_ = s.ws.conn.CloseNow()
		s.ws.conn = nil
		s.stopKeepaliveLocked()
		return errNoWebSocketClient
	}
	return nil
}

func writeWebSocketFrame(conn *websocket.Conn, frame byte) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return conn.Write(ctx, websocket.MessageText, []byte{frame})
}

func (s *Server) closeWebSocket() {
	s.ws.mu.Lock()
	conn := s.ws.conn
	s.ws.conn = nil
	s.stopKeepaliveLocked()
	s.ws.mu.Unlock()
	if conn != nil {
		_ = conn.CloseNow()
	}
}

func (s *Server) resetWebSocket() {
	s.ws.mu.Lock()
	conn := s.ws.conn
	s.ws.conn = nil
	s.ws.loginDeviceID = ""
	s.ws.loginOK = false
	s.ws.connectCount = 0
	s.stopKeepaliveLocked()
	s.ws.mu.Unlock()
	if conn != nil {
		_ = conn.CloseNow()
	}
}

func (s *Server) webSocketStatus() WSStatus {
	s.ws.mu.Lock()
	defer s.ws.mu.Unlock()
	return WSStatus{
		Connected:     s.ws.conn != nil,
		LoginDeviceID: s.ws.loginDeviceID,
		LoginOK:       s.ws.loginOK,
		ConnectCount:  s.ws.connectCount,
	}
}
