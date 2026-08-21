package listen

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
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
	cache := newAssetCache(client, iconDir, soundDir, testLogger())

	iconPath := cache.Icon(context.Background(), "chat")
	if iconPath != filepath.Join(iconDir, "chat.png") {
		t.Fatalf("icon path = %q", iconPath)
	}
	if second := cache.Icon(context.Background(), "chat"); second != iconPath {
		t.Fatalf("cached icon path = %q, want %q", second, iconPath)
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
	soundData, err := os.ReadFile(soundPath)
	if err != nil {
		t.Fatalf("ReadFile(sound) error = %v", err)
	}
	if string(iconData) != "fake-png-data" || string(soundData) != "fake-mp3-data" {
		t.Fatalf("cache contents = %q, %q", iconData, soundData)
	}
	mu.Lock()
	defer mu.Unlock()
	if hits["/icons/chat.png"] != 1 || hits["/sounds/bike.mp3"] != 1 {
		t.Fatalf("request hits = %#v", hits)
	}
}

func TestAssetCacheFailureIsBestEffort(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()
	client := api.New(server.URL, "0.0.0-test", api.WithSleep(func(time.Duration) {}))
	cache := newAssetCache(client, t.TempDir(), t.TempDir(), testLogger())

	if path := cache.Icon(context.Background(), "missing"); path != "" {
		t.Fatalf("missing icon path = %q, want empty", path)
	}
	if path := cache.Icon(context.Background(), "../unsafe"); path != "" {
		t.Fatalf("unsafe icon path = %q, want empty", path)
	}
}
