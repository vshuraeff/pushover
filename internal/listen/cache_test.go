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
	"github.com/vshuraeff/pushover/internal/config"
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

func TestAssetCacheResolveRedownloadsUnreadableFile(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("chmod-based permission test is meaningless when running as root")
	}
	var mu sync.Mutex
	hits := 0
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/icons/chat.png" {
			t.Errorf("path = %q", request.URL.Path)
			writer.WriteHeader(http.StatusNotFound)
			return
		}
		mu.Lock()
		hits++
		hit := hits
		mu.Unlock()
		if hit == 1 {
			_, _ = writer.Write([]byte("first-download"))
			return
		}
		_, _ = writer.Write([]byte("second-download"))
	}))
	defer server.Close()

	iconDir := t.TempDir()
	cache := newAssetCache(api.New(server.URL, "0.0.0-test", api.WithSleep(func(time.Duration) {})), iconDir, t.TempDir(), &channelNotifier{}, testLogger())
	path := cache.resolve(context.Background(), iconDir, "chat", ".png", "/icons/", "icon")
	if err := os.Chmod(path, 0o000); err != nil {
		t.Fatalf("Chmod(cached icon) error = %v", err)
	}
	defer func() {
		if err := os.Chmod(path, 0o600); err != nil && !os.IsNotExist(err) {
			t.Errorf("restore cached icon permissions: %v", err)
		}
	}()
	file, err := os.Open(path)
	if err == nil {
		_ = file.Close()
		t.Fatal("Open(unreadable cached icon) error = nil")
	}

	resolved := cache.resolve(context.Background(), iconDir, "chat", ".png", "/icons/", "icon")
	if resolved != path {
		t.Fatalf("resolved path = %q, want %q", resolved, path)
	}
	mu.Lock()
	gotHits := hits
	mu.Unlock()
	if gotHits != 2 {
		t.Fatalf("download hits = %d, want 2", gotHits)
	}
	data, err := os.ReadFile(resolved)
	if err != nil {
		t.Fatalf("ReadFile(re-downloaded icon) error = %v", err)
	}
	if string(data) != "second-download" {
		t.Fatalf("re-downloaded icon contents = %q", data)
	}
}

func TestStageCachedAssetFallsBackToCopyWhenHardlinkFails(t *testing.T) {
	dir := t.TempDir()
	cachedPath := filepath.Join(dir, "chat.png")
	if err := os.WriteFile(cachedPath, []byte("fake-png-data"), 0o644); err != nil {
		t.Fatalf("WriteFile(cached icon) error = %v", err)
	}
	originalLink := osLink
	osLink = func(string, string) error { return os.ErrPermission }
	t.Cleanup(func() {
		osLink = originalLink
	})

	stagedPath, err := stageCachedAsset(dir, cachedPath)
	if err != nil {
		t.Fatalf("stageCachedAsset() error = %v", err)
	}
	data, err := os.ReadFile(stagedPath)
	if err != nil {
		t.Fatalf("ReadFile(staged icon) error = %v", err)
	}
	if string(data) != "fake-png-data" {
		t.Fatalf("staged icon contents = %q", data)
	}
	stagedInfo, err := os.Stat(stagedPath)
	if err != nil {
		t.Fatalf("Stat(staged icon) error = %v", err)
	}
	if stagedInfo.Mode().Perm() != 0o600 {
		t.Fatalf("staged icon permissions = %#o, want 0600", stagedInfo.Mode().Perm())
	}
	cachedInfo, err := os.Stat(cachedPath)
	if err != nil {
		t.Fatalf("Stat(cached icon) error = %v", err)
	}
	if os.SameFile(cachedInfo, stagedInfo) {
		t.Fatal("staged icon is a hardlink to the cached icon")
	}
}

func TestRunSweepsStaleIconDeliveryStaging(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/1/messages.json":
			_, _ = writer.Write([]byte(`{"status":1,"messages":[{"id":9,"id_str":"9","title":"queued","message":"initial"}]}`))
		case "/1/devices/fake-device-123/update_highest_message.json":
			_, _ = writer.Write([]byte(`{"status":1}`))
		default:
			writer.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	tempDir := t.TempDir()
	iconDir := filepath.Join(tempDir, "icons")
	stagingDir := filepath.Join(iconDir, ".delivery-stale")
	if err := os.MkdirAll(filepath.Join(stagingDir, "nested"), 0o700); err != nil {
		t.Fatalf("MkdirAll(stale staging) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(stagingDir, "nested", "chat.png"), []byte("stale"), 0o600); err != nil {
		t.Fatalf("WriteFile(stale staged icon) error = %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	notifier := &cancelNotifier{cancel: cancel}
	err := Run(ctx, &Daemon{
		Store:            &memoryStore{state: &config.State{DeviceID: "fake-device-123", Secret: "fake-secret-do-not-use"}},
		APIClient:        api.New(server.URL, "0.0.0-test", api.WithSleep(func(time.Duration) {})),
		Notifier:         notifier,
		Logger:           testLogger(),
		WSURL:            "ws://127.0.0.1.invalid",
		EmergencyRealert: time.Minute,
		IconCacheDir:     iconDir,
		SoundCacheDir:    filepath.Join(tempDir, "sounds"),
		DeadLetterPath:   filepath.Join(tempDir, "deadletter.jsonl"),
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if _, err := os.Stat(stagingDir); !os.IsNotExist(err) {
		t.Fatalf("stale staging directory still exists: %v", err)
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
