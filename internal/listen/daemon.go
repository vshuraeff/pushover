// package listen runs the long-lived open client notification daemon.
package listen

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/vshuraeff/pushover/internal/api"
	"github.com/vshuraeff/pushover/internal/config"
	"github.com/vshuraeff/pushover/internal/notify"
	"github.com/vshuraeff/pushover/internal/openclient"
)

const defaultEmergencyRealert = 60 * time.Second

// daemon contains the required services and paths for a listen process.
// store, api client, notifier, and websocket url must be provided by the caller.
type Daemon struct {
	Store            config.SessionStore
	APIClient        *api.Client
	Notifier         notify.Notifier
	Logger           *slog.Logger
	WSURL            string
	EmergencyRealert time.Duration
	Clock            Clock
	IconCacheDir     string
	SoundCacheDir    string
	DeadLetterPath   string
}

// run delivers the initial queue and then maintains the websocket engine.
func Run(ctx context.Context, daemon *Daemon) error {
	if daemon == nil {
		return errors.New("listen daemon is required")
	}
	state, err := openclient.LoadSession(daemon.Store, true)
	if err != nil {
		return err
	}
	if daemon.Notifier == nil {
		return errors.New("listen notifier is required")
	}
	if daemon.APIClient == nil {
		return errors.New("listen api client is required")
	}
	if daemon.WSURL == "" {
		return errors.New("listen websocket url is required")
	}

	logger := daemon.Logger
	if logger == nil {
		logger = slog.Default()
	}
	clock := daemon.Clock
	if clock == nil {
		clock = realClock{}
	}
	interval := daemon.EmergencyRealert
	if interval <= 0 {
		interval = defaultEmergencyRealert
	}

	iconDir, soundDir := cacheDirectories(daemon, logger)
	deadLetterPath, err := deadLetterFilePath(daemon.DeadLetterPath)
	if err != nil {
		return err
	}

	runCtx, cancel := context.WithCancel(ctx)
	cache := newAssetCache(daemon.APIClient, iconDir, soundDir, daemon.Notifier, logger)
	emergencies := NewEmergencyScheduler(daemon.Notifier, interval, clock, logger)
	emergencies.cache = cache
	emergencyDone := make(chan struct{})
	go func() {
		emergencies.Run(runCtx)
		close(emergencyDone)
	}()

	syncer := newSyncer(syncerConfig{
		ctx:         runCtx,
		store:       daemon.Store,
		state:       state,
		client:      daemon.APIClient,
		notifier:    daemon.Notifier,
		logger:      logger,
		clock:       clock,
		cache:       cache,
		deadLetters: newDeadLetterWriter(deadLetterPath, clock, logger),
		emergencies: emergencies,
		secret:      state.Secret,
		deviceID:    state.DeviceID,
	})
	var actionDone chan struct{}
	defer func() {
		cancel()
		if actionDone != nil {
			<-actionDone
		}
		syncer.stopAndWait()
		<-emergencyDone
	}()

	if err := syncer.runOnce(runCtx); err != nil {
		if ctx.Err() != nil {
			return nil
		}
		return errors.New(redactError(err, state.Secret, state.DeviceID))
	}

	if source, ok := daemon.Notifier.(notify.ActionSource); ok {
		actionDone = make(chan struct{})
		go func() {
			runActions(runCtx, source.Actions(), daemon.APIClient, state.Secret, state.DeviceID, syncer.trigger, logger)
			close(actionDone)
		}()
	}

	engine := &openclient.Engine{
		URL:      daemon.WSURL,
		DeviceID: state.DeviceID,
		Secret:   state.Secret,
		OnSync:   syncer.trigger,
		Logger:   logger,
	}
	err = engine.Run(runCtx)
	if ctx.Err() != nil {
		return nil
	}
	if errors.Is(err, openclient.ErrPermanent) || errors.Is(err, openclient.ErrReplaced) {
		logger.Error("websocket engine stopped on terminal frame", "error", err)
	}
	return err
}

func cacheDirectories(daemon *Daemon, logger *slog.Logger) (string, string) {
	iconDir := daemon.IconCacheDir
	soundDir := daemon.SoundCacheDir
	if iconDir != "" && soundDir != "" {
		return iconDir, soundDir
	}

	cacheDir, err := os.UserCacheDir()
	if err != nil {
		logger.Warn("locate user cache directory", "error", err)
		return iconDir, soundDir
	}
	if iconDir == "" {
		iconDir = filepath.Join(cacheDir, "povr", "icons")
	}
	if soundDir == "" {
		soundDir = filepath.Join(cacheDir, "povr", "sounds")
	}
	return iconDir, soundDir
}

func deadLetterFilePath(configured string) (string, error) {
	if configured != "" {
		return configured, nil
	}
	dir, err := config.ConfigDir()
	if err != nil {
		return "", fmt.Errorf("locate dead-letter directory: %w", err)
	}
	return filepath.Join(dir, "deadletter.jsonl"), nil
}
