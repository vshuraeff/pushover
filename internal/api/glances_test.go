package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestGlancesSend(t *testing.T) {
	title := ""
	text := "hello"
	subtext := "detail"
	count := 3
	percent := 75
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			t.Errorf("method = %s, want %s", request.Method, http.MethodPost)
		}
		if request.URL.Path != "/1/glances.json" {
			t.Errorf("path = %s, want /1/glances.json", request.URL.Path)
		}
		if err := request.ParseForm(); err != nil {
			t.Errorf("parse form: %v", err)
		}
		if values, ok := request.PostForm["title"]; !ok || len(values) != 1 || values[0] != "" {
			t.Errorf("title = %#v, want explicit empty value", values)
		}
		for name, want := range map[string]string{
			"token":   "test-app-token-not-real",
			"user":    "test-user-key-not-real",
			"device":  "phone",
			"text":    "hello",
			"subtext": "detail",
			"count":   "3",
			"percent": "75",
		} {
			if got := request.PostForm.Get(name); got != want {
				t.Errorf("%s = %q, want %q", name, got, want)
			}
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"status":1,"request":"test-request"}`))
	}))
	defer server.Close()

	service := NewGlancesService(New(server.URL, "test-version"))
	response, err := service.Send(context.Background(), GlanceRequest{
		Token:   "test-app-token-not-real",
		User:    "test-user-key-not-real",
		Device:  "phone",
		Title:   &title,
		Text:    &text,
		Subtext: &subtext,
		Count:   &count,
		Percent: &percent,
	})
	if err != nil {
		t.Fatalf("send: %v", err)
	}
	if response.Status != 1 {
		t.Fatalf("status = %d, want 1", response.Status)
	}
}

func TestGlancesSendValidatesBeforeNetwork(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		calls.Add(1)
		writer.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	service := NewGlancesService(New(server.URL, "test-version"))
	tooLong := strings.Repeat("a", 101)
	if _, err := service.Send(context.Background(), GlanceRequest{Title: &tooLong}); err == nil {
		t.Fatal("send succeeded with an overlong title")
	}
	percent := 101
	if _, err := service.Send(context.Background(), GlanceRequest{Percent: &percent}); err == nil {
		t.Fatal("send succeeded with an out-of-range percent")
	}
	if got := calls.Load(); got != 0 {
		t.Fatalf("network calls = %d, want 0", got)
	}
}

func TestGlancesSendCountsRunes(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		calls.Add(1)
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"status":1}`))
	}))
	defer server.Close()

	service := NewGlancesService(New(server.URL, "test-version"))
	accepted := strings.Repeat("Ж", 100)
	if _, err := service.Send(context.Background(), GlanceRequest{Title: &accepted}); err != nil {
		t.Fatalf("send 100-rune title: %v", err)
	}
	rejected := strings.Repeat("Ж", 101)
	if _, err := service.Send(context.Background(), GlanceRequest{Title: &rejected}); err == nil {
		t.Fatal("send succeeded with a 101-rune title")
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("network calls = %d, want 1", got)
	}
}
