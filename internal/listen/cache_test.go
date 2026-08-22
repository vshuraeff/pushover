package listen

import (
	"bytes"
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/vshuraeff/pushover/internal/api"
)

func TestAssetCacheDownloadsAtomicallyAndReusesFiles(t *testing.T) {
	var mu sync.Mutex
	hits := make(map[string]int)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		mu.Lock()
		hits[request.URL.Path]++
		mu.Unlock()
		switch request.URL.Path {
		case "/icons/chat.png":
			_, _ = writer.Write([]byte("fake-png-data"))
		case "/sounds/bike.mp3":
			_, _ = writer.Write([]byte("fake-mp3-data"))
		default:
			writer.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	client := api.New(server.URL, "0.0.0-test", api.WithSleep(func(time.Duration) {}))
	iconDir := filepath.Join(t.TempDir(), "icons")
	soundDir := filepath.Join(t.TempDir(), "sounds")
	var logs bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logs, nil))
	cache := newAssetCache(client, iconDir, soundDir, &channelNotifier{}, logger)

	iconPath := cache.Icon(context.Background(), "chat")
	if filepath.Base(iconPath) != "chat.png" {
		t.Fatalf("icon path = %q", iconPath)
	}
	secondIconPath := cache.Icon(context.Background(), "chat")
	if secondIconPath == iconPath {
		t.Fatalf("cached icon path = %q, want a distinct delivery path", secondIconPath)
	}
	soundPath := cache.resolve(context.Background(), soundDir, "bike", ".mp3", "/sounds/", "sound")
	if soundPath != filepath.Join(soundDir, "bike.mp3") {
		t.Fatalf("sound path = %q", soundPath)
	}
	if second := cache.resolve(context.Background(), soundDir, "bike", ".mp3", "/sounds/", "sound"); second != soundPath {
		t.Fatalf("cached sound path = %q, want %q", second, soundPath)
	}

	iconData, err := os.ReadFile(iconPath)
	if err != nil {
		t.Fatalf("ReadFile(icon) error = %v", err)
	}
	secondIconData, err := os.ReadFile(secondIconPath)
	if err != nil {
		t.Fatalf("ReadFile(second icon) error = %v", err)
	}
	soundData, err := os.ReadFile(soundPath)
	if err != nil {
		t.Fatalf("ReadFile(sound) error = %v", err)
	}
	if string(iconData) != "fake-png-data" || string(secondIconData) != "fake-png-data" || string(soundData) != "fake-mp3-data" {
		t.Fatalf("cache contents = %q, %q, %q", iconData, secondIconData, soundData)
	}
	mu.Lock()
	defer mu.Unlock()
	if hits["/icons/chat.png"] != 1 || hits["/sounds/bike.mp3"] != 1 {
		t.Fatalf("request hits = %#v", hits)
	}
	if got := strings.Count(logs.String(), "cache miss"); got != 2 {
		t.Fatalf("cache miss log lines = %d, want 2; logs = %q", got, logs.String())
	}
}

func TestAssetCacheIconResolvesAfterDeliveredFileIsRemoved(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/icons/chat.png" {
			t.Errorf("path = %q", request.URL.Path)
		}
		_, _ = writer.Write([]byte("fake-png-data"))
	}))
	defer server.Close()

	client := api.New(server.URL, "0.0.0-test", api.WithSleep(func(time.Duration) {}))
	cache := newAssetCache(client, t.TempDir(), t.TempDir(), &channelNotifier{}, testLogger())
	first := cache.Icon(context.Background(), "chat")
	if err := os.Remove(first); err != nil {
		t.Fatalf("Remove(first icon) error = %v", err)
	}

	second := cache.Icon(context.Background(), "chat")
	data, err := os.ReadFile(second)
	if err != nil {
		t.Fatalf("ReadFile(second icon) error = %v", err)
	}
	if string(data) != "fake-png-data" {
		t.Fatalf("second icon contents = %q", data)
	}
}

func TestAssetCacheFailureIsBestEffort(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()
	client := api.New(server.URL, "0.0.0-test", api.WithSleep(func(time.Duration) {}))
	cache := newAssetCache(client, t.TempDir(), t.TempDir(), &channelNotifier{}, testLogger())

	if path := cache.Icon(context.Background(), "missing"); path != "" {
		t.Fatalf("missing icon path = %q, want empty", path)
	}
	if path := cache.Icon(context.Background(), "../unsafe"); path != "" {
		t.Fatalf("unsafe icon path = %q, want empty", path)
	}
}

func TestAssetCacheConvertsSoundsOnlyForHelperSignal(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/sounds/bike.mp3" {
			t.Errorf("path = %q", request.URL.Path)
		}
		_, _ = writer.Write([]byte("fake-mp3-data"))
	}))
	defer server.Close()
	client := api.New(server.URL, "0.0.0-test", api.WithSleep(func(time.Duration) {}))
	var conversions int
	converter := func(_ context.Context, mp3Path, key string) (string, error) {
		conversions++
		return filepath.Join(filepath.Dir(mp3Path), "povr-"+key+".caf"), nil
	}

	rawCache := newAssetCache(client, t.TempDir(), t.TempDir(), &channelNotifier{}, testLogger())
	rawCache.converter = converter
	raw := rawCache.Sound(context.Background(), "bike")
	if conversions != 0 || raw.name != "bike" || filepath.Ext(raw.path) != ".mp3" {
		t.Fatalf("raw sound = %#v, conversions = %d", raw, conversions)
	}

	helperCache := newAssetCache(client, t.TempDir(), t.TempDir(), &channelNotifier{}, testLogger())
	helperCache.convertSounds = true
	helperCache.converter = converter
	converted := helperCache.Sound(context.Background(), "bike")
	if conversions != 1 || converted.name != "povr-bike.caf" || filepath.Ext(converted.path) != ".caf" {
		t.Fatalf("converted sound = %#v, conversions = %d", converted, conversions)
	}
}
