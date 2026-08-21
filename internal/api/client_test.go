package api

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

const testVersion = "0.0.0-test"

func newTestClient(baseURL string) *Client {
	return New(baseURL, testVersion, WithSleep(func(time.Duration) {}))
}

func TestGetSendsUserAgent(t *testing.T) {
	var userAgent string
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		userAgent = request.Header.Get("User-Agent")
		if request.URL.Query().Get("token") != "test-token-not-real" {
			t.Errorf("unexpected query: %s", request.URL.RawQuery)
		}
		_, _ = writer.Write([]byte(`{"status":1}`))
	}))
	defer server.Close()

	response, err := newTestClient(server.URL).Get(context.Background(), "/users/validate.json", url.Values{"token": {"test-token-not-real"}})
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.StatusCode, http.StatusOK)
	}
	want := "povr/0.0.0-test (unofficial Pushover Open Client; +https://github.com/vshuraeff/pushover)"
	if userAgent != want {
		t.Errorf("User-Agent = %q, want %q", userAgent, want)
	}
}

func TestGetRetriesServerErrors(t *testing.T) {
	tests := []struct {
		name      string
		succeedOn int
		wantHits  int32
		wantErr   bool
	}{
		{name: "succeeds after retry", succeedOn: 2, wantHits: 2},
		{name: "exhausts retries", succeedOn: 0, wantHits: 3, wantErr: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var hits atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				hit := hits.Add(1)
				if test.succeedOn > 0 && hit == int32(test.succeedOn) {
					_, _ = writer.Write([]byte(`{"status":1}`))
					return
				}
				writer.WriteHeader(http.StatusServiceUnavailable)
				_, _ = writer.Write([]byte(`{"status":0,"errors":["temporarily unavailable"]}`))
			}))
			defer server.Close()

			_, err := newTestClient(server.URL).Get(context.Background(), "/endpoint", nil)
			if (err != nil) != test.wantErr {
				t.Fatalf("Get() error = %v, want error %t", err, test.wantErr)
			}
			if got := hits.Load(); got != test.wantHits {
				t.Errorf("handler hits = %d, want %d", got, test.wantHits)
			}
		})
	}
}

func TestPostFormDoesNotRetryClientErrors(t *testing.T) {
	var hits atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		hits.Add(1)
		if got := request.Header.Get("Content-Type"); got != "application/x-www-form-urlencoded" {
			t.Errorf("Content-Type = %q", got)
		}
		writer.WriteHeader(http.StatusBadRequest)
		_, _ = writer.Write([]byte(`{"status":0,"errors":["invalid request"]}`))
	}))
	defer server.Close()

	_, err := newTestClient(server.URL).PostForm(context.Background(), "/messages.json", url.Values{"token": {"test-token-not-real"}})
	if err == nil {
		t.Fatal("PostForm() error = nil, want error")
	}
	if got := hits.Load(); got != 1 {
		t.Errorf("handler hits = %d, want 1", got)
	}
}

func TestNoRetry(t *testing.T) {
	var hits atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		hits.Add(1)
		writer.WriteHeader(http.StatusInternalServerError)
		_, _ = writer.Write([]byte(`{"status":0,"errors":["server error"]}`))
	}))
	defer server.Close()

	_, err := newTestClient(server.URL).Get(context.Background(), "/endpoint", nil, NoRetry())
	if err == nil {
		t.Fatal("Get() error = nil, want error")
	}
	if got := hits.Load(); got != 1 {
		t.Errorf("handler hits = %d, want 1", got)
	}
}

func TestNetworkErrorRedactsQueryCredentials(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen() error = %v", err)
	}
	address := listener.Addr().String()
	if err := listener.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	const (
		appToken = "test-app-token-must-not-appear-in-errors"
		userKey  = "test-user-key-must-not-appear-in-errors"
	)
	_, err = newTestClient("http://"+address).Get(
		context.Background(),
		"/1/limits.json",
		url.Values{"token": {appToken}, "user": {userKey}},
		NoRetry(),
	)
	if err == nil {
		t.Fatal("Get() error = nil, want network error")
	}
	for _, credential := range []string{appToken, userKey} {
		if strings.Contains(err.Error(), credential) {
			t.Error("Get() error contains a query credential")
		}
	}
	var urlError *url.Error
	if !errors.As(err, &urlError) {
		t.Fatalf("Get() error type = %T, want wrapped *url.Error", err)
	}
	for _, credential := range []string{appToken, userKey} {
		if strings.Contains(urlError.URL, credential) {
			t.Error("url error URL contains a query credential")
		}
	}
}

func TestPostMultipartStreamsFieldsAndFile(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if err := request.ParseMultipartForm(1 << 20); err != nil {
			t.Fatalf("ParseMultipartForm() error = %v", err)
		}
		if got := request.FormValue("token"); got != "test-token-not-real" {
			t.Errorf("token = %q", got)
		}
		if got := request.FormValue("message"); got != "hello from test" {
			t.Errorf("message = %q", got)
		}
		file, header, err := request.FormFile("attachment")
		if err != nil {
			t.Fatalf("FormFile() error = %v", err)
		}
		defer file.Close()
		if header.Filename != "example.txt" {
			t.Errorf("filename = %q", header.Filename)
		}
		contents, err := io.ReadAll(file)
		if err != nil {
			t.Fatalf("ReadAll() error = %v", err)
		}
		if got := string(contents); got != "file content" {
			t.Errorf("file contents = %q", got)
		}
		_, _ = writer.Write([]byte(`{"status":1}`))
	}))
	defer server.Close()

	_, err := newTestClient(server.URL).PostMultipart(
		context.Background(),
		"/messages.json",
		url.Values{"token": {"test-token-not-real"}, "message": {"hello from test"}},
		FilePart{FieldName: "attachment", FileName: "example.txt", Reader: strings.NewReader("file content")},
	)
	if err != nil {
		t.Fatalf("PostMultipart() error = %v", err)
	}
}

func TestQuotaErrorIncludesResetHeader(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("X-Limit-App-Reset", "1720000000")
		writer.WriteHeader(http.StatusTooManyRequests)
		_, _ = writer.Write([]byte(`{"status":0,"request":"request-test-id","errors":["quota exceeded"]}`))
	}))
	defer server.Close()

	response, err := newTestClient(server.URL).Get(context.Background(), "/endpoint", nil)
	if err == nil {
		t.Fatal("Get() error = nil, want quota error")
	}
	var quotaError *QuotaError
	if !errors.As(err, &quotaError) {
		t.Fatalf("error type = %T, want *QuotaError", err)
	}
	if quotaError.Reset != "1720000000" {
		t.Errorf("reset = %q", quotaError.Reset)
	}
	if response.RateLimits.Reset != "1720000000" {
		t.Errorf("response reset = %q", response.RateLimits.Reset)
	}
}

func TestResponseIncludesRateLimitHeaders(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("X-Limit-App-Limit", "10000")
		writer.Header().Set("X-Limit-App-Remaining", "9999")
		writer.Header().Set("X-Limit-App-Reset", "1720000000")
		_, _ = writer.Write([]byte(`{"status":1}`))
	}))
	defer server.Close()

	response, err := newTestClient(server.URL).Get(context.Background(), "/endpoint", nil)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if response.RateLimits != (RateLimits{Limit: "10000", Remaining: "9999", Reset: "1720000000"}) {
		t.Errorf("rate limits = %#v", response.RateLimits)
	}
}
