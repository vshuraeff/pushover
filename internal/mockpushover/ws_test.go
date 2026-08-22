package mockpushover

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
)

func TestWebSocketLogin(t *testing.T) {
	server := startTestServer(t)
	conn := dialWebSocket(t, server)
	writeWebSocket(t, conn, "login:"+testDeviceID+":"+testBinding+"\n")

	status := waitWebSocketStatus(t, server, func(status WSStatus) bool {
		return status.Connected && status.ConnectCount == 1
	})
	if status.LoginDeviceID != testDeviceID || !status.LoginOK {
		t.Fatalf("status = %+v", status)
	}
}

func TestWebSocketRejectsBadLogin(t *testing.T) {
	tests := []struct {
		name  string
		login string
	}{
		{name: "wrong secret", login: "login:" + testDeviceID + ":wrong-secret\n"},
		{name: "unknown device", login: "login:unknown-device:" + testBinding + "\n"},
		{name: "empty secret", login: "login:" + testDeviceID + ":\n"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := startTestServer(t)
			conn := dialWebSocket(t, server)
			writeWebSocket(t, conn, test.login)
			requireWebSocketFrame(t, conn, 'E')
			requireWebSocketClosed(t, conn)

			status := waitWebSocketStatus(t, server, func(status WSStatus) bool {
				return !status.LoginOK
			})
			if status.Connected || status.ConnectCount != 0 {
				t.Fatalf("status = %+v", status)
			}
		})
	}
}

func TestWebSocketRejectsDisabledDevice(t *testing.T) {
	server := startTestServer(t)
	if err := server.Control().Seed(SeedState{Devices: []Device{{
		ID: testDeviceID, Secret: testBinding, Enabled: false,
	}}}); err != nil {
		t.Fatalf("Seed() error = %v", err)
	}
	conn := dialWebSocket(t, server)
	writeWebSocket(t, conn, "login:"+testDeviceID+":"+testBinding+"\n")
	requireWebSocketFrame(t, conn, 'E')
	requireWebSocketClosed(t, conn)
}

// TestWebSocketBinaryLoginIsIgnored mirrors the real pushover open client
// websocket server: a login payload sent as a binary frame is silently
// dropped (no login, no 'E' error, no session) and the connection idles
// until the server-side login timeout closes it.
func TestWebSocketBinaryLoginIsIgnored(t *testing.T) {
	const loginTimeout = 200 * time.Millisecond
	server := startTestServerWithConfig(t, func(cfg *Config) {
		cfg.LoginTimeout = loginTimeout
	})
	conn := dialWebSocket(t, server)
	writeWebSocketBinary(t, conn, "login:"+testDeviceID+":"+testBinding+"\n")

	// No status flip while the login is being ignored.
	time.Sleep(loginTimeout / 2)
	status, err := server.Control().WSStatus()
	if err != nil {
		t.Fatalf("WSStatus() error = %v", err)
	}
	if status.Connected || status.LoginOK || status.ConnectCount != 0 {
		t.Fatalf("status = %+v, want an ignored binary login", status)
	}

	// The server closes the idle connection once the login timeout elapses,
	// without ever sending a frame.
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if _, payload, readErr := conn.Read(ctx); readErr == nil {
		t.Fatalf("Read() payload = %q, want server close", payload)
	} else if errors.Is(readErr, context.DeadlineExceeded) {
		t.Fatalf("Read() error = %v, want server close before deadline", readErr)
	}

	status, err = server.Control().WSStatus()
	if err != nil {
		t.Fatalf("WSStatus() error = %v", err)
	}
	if status.Connected || status.LoginOK || status.ConnectCount != 0 {
		t.Fatalf("status = %+v, want an ignored binary login", status)
	}
}

// TestWebSocketSendsProactiveKeepalive verifies that a logged-in session
// receives '#' keepalive frames on its own, without a control-plane nudge.
func TestWebSocketSendsProactiveKeepalive(t *testing.T) {
	server := startTestServerWithConfig(t, func(cfg *Config) {
		cfg.KeepaliveInterval = 100 * time.Millisecond
	})
	conn := dialWebSocket(t, server)
	writeWebSocket(t, conn, "login:"+testDeviceID+":"+testBinding+"\n")
	waitWebSocketStatus(t, server, func(status WSStatus) bool { return status.Connected })

	requireWebSocketFrame(t, conn, '#')
	requireWebSocketFrame(t, conn, '#')
}

func TestControlWebSocketSend(t *testing.T) {
	server := startTestServer(t)
	conn := dialWebSocket(t, server)
	writeWebSocket(t, conn, "login:"+testDeviceID+":"+testBinding+"\n")
	waitWebSocketStatus(t, server, func(status WSStatus) bool { return status.Connected })

	for _, frame := range []byte{'#', '!', 'R', 'E', 'A'} {
		if err := server.Control().WSSend(string(frame)); err != nil {
			t.Fatalf("WSSend(%q) error = %v", frame, err)
		}
		requireWebSocketFrame(t, conn, frame)
	}
}

func TestWebSocketSecondLoginReplacesFirst(t *testing.T) {
	server := startTestServer(t)
	first := dialWebSocket(t, server)
	writeWebSocket(t, first, "login:"+testDeviceID+":"+testBinding+"\n")
	waitWebSocketStatus(t, server, func(status WSStatus) bool {
		return status.Connected && status.ConnectCount == 1
	})

	second := dialWebSocket(t, server)
	writeWebSocket(t, second, "login:"+testDeviceID2+":"+testBinding2+"\n")
	requireWebSocketFrame(t, first, 'A')
	requireWebSocketClosed(t, first)

	status := waitWebSocketStatus(t, server, func(status WSStatus) bool {
		return status.Connected && status.ConnectCount == 2
	})
	if status.LoginDeviceID != testDeviceID2 || !status.LoginOK {
		t.Fatalf("status = %+v", status)
	}
}

func TestControlWebSocketClose(t *testing.T) {
	server := startTestServer(t)
	conn := dialWebSocket(t, server)
	writeWebSocket(t, conn, "login:"+testDeviceID+":"+testBinding+"\n")
	waitWebSocketStatus(t, server, func(status WSStatus) bool { return status.Connected })

	if err := server.Control().WSClose(); err != nil {
		t.Fatalf("WSClose() error = %v", err)
	}
	requireWebSocketClosed(t, conn)
	status := waitWebSocketStatus(t, server, func(status WSStatus) bool { return !status.Connected })
	if status.ConnectCount != 1 {
		t.Fatalf("status = %+v", status)
	}
}

func TestWebSocketConnectCountReset(t *testing.T) {
	server := startTestServer(t)
	first := dialWebSocket(t, server)
	writeWebSocket(t, first, "login:"+testDeviceID+":"+testBinding+"\n")
	waitWebSocketStatus(t, server, func(status WSStatus) bool { return status.ConnectCount == 1 })
	if err := server.Control().WSClose(); err != nil {
		t.Fatalf("WSClose() error = %v", err)
	}
	requireWebSocketClosed(t, first)

	second := dialWebSocket(t, server)
	writeWebSocket(t, second, "login:"+testDeviceID+":"+testBinding+"\n")
	waitWebSocketStatus(t, server, func(status WSStatus) bool {
		return status.Connected && status.ConnectCount == 2
	})
	if err := server.Control().Reset(); err != nil {
		t.Fatalf("Reset() error = %v", err)
	}
	requireWebSocketClosed(t, second)

	status := waitWebSocketStatus(t, server, func(status WSStatus) bool {
		return !status.Connected && status.ConnectCount == 0
	})
	if status.LoginDeviceID != "" || status.LoginOK {
		t.Fatalf("status = %+v", status)
	}
}

func TestControlWebSocketSendWithoutConnection(t *testing.T) {
	server := startTestServer(t)
	body := bytes.NewBufferString(`{"frame":"#"}`)
	request, err := http.NewRequest(http.MethodPost, server.URL()+"/_control/ws/send", body)
	if err != nil {
		t.Fatalf("NewRequest() error = %v", err)
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := server.client.Do(request)
	if err != nil {
		t.Fatalf("Do() error = %v", err)
	}
	defer response.Body.Close()
	responseBody, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("ReadAll() error = %v", err)
	}
	if response.StatusCode < 400 || response.StatusCode >= 500 {
		t.Fatalf("status = %d, body = %s", response.StatusCode, responseBody)
	}
	var payload map[string]any
	if err := json.Unmarshal(responseBody, &payload); err != nil {
		t.Fatalf("json.Unmarshal(%q) error = %v", responseBody, err)
	}
	if payload["status"] != float64(0) {
		t.Fatalf("payload = %#v", payload)
	}
}

func dialWebSocket(t *testing.T, server *Server) *websocket.Conn {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	wsURL := strings.Replace(server.URL(), "http://", "ws://", 1) + "/push"
	conn, response, err := websocket.Dial(ctx, wsURL, nil)
	if err != nil {
		if response != nil {
			t.Fatalf("Dial() status = %d, error = %v", response.StatusCode, err)
		}
		t.Fatalf("Dial() error = %v", err)
	}
	t.Cleanup(func() { _ = conn.CloseNow() })
	return conn
}

func writeWebSocket(t *testing.T, conn *websocket.Conn, message string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := conn.Write(ctx, websocket.MessageText, []byte(message)); err != nil {
		t.Fatalf("Write() error = %v", err)
	}
}

func writeWebSocketBinary(t *testing.T, conn *websocket.Conn, message string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := conn.Write(ctx, websocket.MessageBinary, []byte(message)); err != nil {
		t.Fatalf("Write() error = %v", err)
	}
}

func requireWebSocketFrame(t *testing.T, conn *websocket.Conn, want byte) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, payload, err := conn.Read(ctx)
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}
	if len(payload) != 1 || payload[0] != want {
		t.Fatalf("frame = %q, want %q", payload, want)
	}
}

func requireWebSocketClosed(t *testing.T, conn *websocket.Conn) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, payload, err := conn.Read(ctx); err == nil {
		t.Fatalf("Read() payload = %q, want close error", payload)
	} else if errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Read() error = %v, want server close", err)
	}
}

func waitWebSocketStatus(t *testing.T, server *Server, ready func(WSStatus) bool) WSStatus {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	var last WSStatus
	for time.Now().Before(deadline) {
		status, err := server.Control().WSStatus()
		if err != nil {
			t.Fatalf("WSStatus() error = %v", err)
		}
		last = status
		if ready(status) {
			return status
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("websocket status did not become ready: %+v", last)
	return WSStatus{}
}
