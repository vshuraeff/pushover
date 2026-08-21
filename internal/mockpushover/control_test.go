package mockpushover

import (
	"encoding/json"
	"net"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

func TestServerLifecycleAndControlPlane(t *testing.T) {
	t.Run("localhost lifecycle", func(t *testing.T) {
		server := startTestServer(t)
		parsed, err := url.Parse(server.URL())
		if err != nil {
			t.Fatalf("url.Parse() error = %v", err)
		}
		host, _, err := net.SplitHostPort(parsed.Host)
		if err != nil {
			t.Fatalf("net.SplitHostPort() error = %v", err)
		}
		if host != "127.0.0.1" {
			t.Fatalf("host = %q, want 127.0.0.1", host)
		}
		if server.Control() == nil {
			t.Fatal("Control() returned nil")
		}
		server.RegisterControlRoute("GET /_control/ping", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusNoContent)
		}))
		response, err := server.client.Get(server.URL() + "/_control/ping")
		if err != nil {
			t.Fatalf("control extension Get() error = %v", err)
		}
		response.Body.Close()
		if response.StatusCode != http.StatusNoContent {
			t.Fatalf("control extension status = %d, want %d", response.StatusCode, http.StatusNoContent)
		}
	})

	t.Run("journal records decoded fields and headers", func(t *testing.T) {
		server := startTestServer(t)
		values := url.Values{"token": {testToken}, "user": {testUser}, "device": {"phone"}}
		request, err := http.NewRequest(http.MethodPost, server.URL()+"/1/users/validate.json", strings.NewReader(values.Encode()))
		if err != nil {
			t.Fatalf("NewRequest() error = %v", err)
		}
		request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		request.Header.Set("X-Test-Request", "journal-value")
		response, err := server.client.Do(request)
		if err != nil {
			t.Fatalf("Do() error = %v", err)
		}
		response.Body.Close()
		requests, err := server.Control().Requests()
		if err != nil {
			t.Fatalf("Requests() error = %v", err)
		}
		if len(requests) != 1 {
			t.Fatalf("len(requests) = %d, want 1", len(requests))
		}
		got := requests[0]
		if got.Method != http.MethodPost || got.Path != "/1/users/validate.json" {
			t.Fatalf("request = %s %s", got.Method, got.Path)
		}
		if got.Fields["user"][0] != testUser || got.Headers.Get("X-Test-Request") != "journal-value" {
			t.Fatalf("journaled request = %#v", got)
		}
	})

	t.Run("fault intercepts only configured count", func(t *testing.T) {
		server := startTestServer(t)
		err := server.Control().Fault(FaultRule{
			Method: http.MethodGet, PathGlob: "/1/sounds.json", Status: http.StatusServiceUnavailable,
			Body: map[string]any{"fault": true}, Times: 1,
		})
		if err != nil {
			t.Fatalf("Fault() error = %v", err)
		}
		response, body := doForm(t, server, http.MethodGet, "/1/sounds.json", url.Values{"token": {testToken}})
		requireStatus(t, response, http.StatusServiceUnavailable, body)
		if decodeObject(t, body)["fault"] != true {
			t.Fatalf("fault body = %s", body)
		}
		response, body = doForm(t, server, http.MethodGet, "/1/sounds.json", url.Values{"token": {testToken}})
		requireStatus(t, response, http.StatusOK, body)
	})

	t.Run("seed replaces and reset restores initial state", func(t *testing.T) {
		server := startTestServer(t)
		replacementToken := "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
		if err := server.Control().Seed(SeedState{Tokens: []AppToken{{Token: replacementToken}}}); err != nil {
			t.Fatalf("Seed() error = %v", err)
		}
		response, body := doForm(t, server, http.MethodGet, "/1/sounds.json", url.Values{"token": {testToken}})
		requireStatus(t, response, http.StatusBadRequest, body)
		response, body = doForm(t, server, http.MethodGet, "/1/sounds.json", url.Values{"token": {replacementToken}})
		requireStatus(t, response, http.StatusOK, body)
		if err := server.Control().Reset(); err != nil {
			t.Fatalf("Reset() error = %v", err)
		}
		response, body = doForm(t, server, http.MethodGet, "/1/sounds.json", url.Values{"token": {testToken}})
		requireStatus(t, response, http.StatusOK, body)
	})
}

func TestErrorShapeToggle(t *testing.T) {
	tests := []struct {
		name  string
		shape ErrorShape
		check func(*testing.T, json.RawMessage)
	}{
		{
			name: "array", shape: ErrorShapeArray,
			check: func(t *testing.T, raw json.RawMessage) {
				t.Helper()
				var errors []string
				if err := json.Unmarshal(raw, &errors); err != nil || len(errors) != 1 {
					t.Fatalf("errors = %s, err = %v", raw, err)
				}
			},
		},
		{
			name: "object", shape: ErrorShapeObject,
			check: func(t *testing.T, raw json.RawMessage) {
				t.Helper()
				var errors map[string][]string
				if err := json.Unmarshal(raw, &errors); err != nil || len(errors["user"]) != 1 {
					t.Fatalf("errors = %s, err = %v", raw, err)
				}
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			seed := testSeed()
			server, err := Start(Config{InitialState: &seed, ErrorShape: test.shape})
			if err != nil {
				t.Fatalf("Start() error = %v", err)
			}
			t.Cleanup(func() { _ = server.Close() })
			response, body := doForm(t, server, http.MethodPost, "/1/users/validate.json", url.Values{
				"token": {testToken}, "user": {"xxxxxxxxxxxxxxxxxxxxxxxxxxxxxx"},
			})
			requireStatus(t, response, http.StatusBadRequest, body)
			var payload struct {
				Errors json.RawMessage `json:"errors"`
			}
			if err := json.Unmarshal(body, &payload); err != nil {
				t.Fatalf("json.Unmarshal() error = %v", err)
			}
			test.check(t, payload.Errors)
		})
	}
}
