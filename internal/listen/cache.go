package listen

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/vshuraeff/pushover/internal/api"
	"github.com/vshuraeff/pushover/internal/notify"
)

type soundConverter func(context.Context, string, string) (string, error)

type cachedSound struct {
	path string
	name string
}

type assetCache struct {
	client        *api.Client
	iconDir       string
	soundDir      string
	convertSounds bool
	converter     soundConverter
	logger        *slog.Logger
}

func newAssetCache(client *api.Client, iconDir, soundDir string, notifier notify.Notifier, logger *slog.Logger) *assetCache {
	if logger == nil {
		logger = slog.Default()
	}
	return &assetCache{
		client:        client,
		iconDir:       iconDir,
		soundDir:      soundDir,
		convertSounds: isHelperNotifier(notifier),
		converter:     convertCachedSound,
		logger:        logger,
	}
}

func (c *assetCache) Icon(ctx context.Context, key string) string {
	return c.resolve(ctx, c.iconDir, key, ".png", "/icons/", "icon")
}

func (c *assetCache) Sound(ctx context.Context, key string) cachedSound {
	if key == "" {
		return cachedSound{}
	}
	mp3Path := c.resolve(ctx, c.soundDir, key, ".mp3", "/sounds/", "sound")
	if !c.convertSounds {
		return cachedSound{path: mp3Path, name: key}
	}
	if mp3Path == "" || c.converter == nil {
		return cachedSound{path: mp3Path}
	}
	converted, err := c.converter(ctx, mp3Path, key)
	if err != nil {
		c.logger.Warn("convert cached sound", "sound", key, "error", err)
		return cachedSound{path: mp3Path}
	}
	extension := filepath.Ext(converted)
	name := strings.TrimSuffix(filepath.Base(converted), extension)
	return cachedSound{path: converted, name: name}
}

func (c *assetCache) resolve(ctx context.Context, dir, key, extension, pathPrefix, kind string) string {
	if dir == "" || key == "" {
		return ""
	}
	if filepath.Base(key) != key || key == "." {
		c.logger.Warn("invalid cache key", "kind", kind, "key", key)
		return ""
	}

	path := filepath.Join(dir, key+extension)
	if info, err := os.Stat(path); err == nil {
		if info.IsDir() {
			c.logger.Warn("cached asset is a directory", "kind", kind, "path", path)
			return ""
		}
		return path
	} else if !os.IsNotExist(err) {
		c.logger.Warn("inspect cached asset", "kind", kind, "path", path, "error", err)
		return ""
	}
	if c.client == nil {
		return ""
	}

	response, err := c.client.Get(ctx, pathPrefix+url.PathEscape(key)+extension, nil)
	if err != nil {
		c.logger.Warn("download cached asset", "kind", kind, "key", key, "error", err)
		return ""
	}
	if response == nil || response.StatusCode != http.StatusOK {
		status := 0
		if response != nil {
			status = response.StatusCode
		}
		c.logger.Warn("download cached asset returned unexpected status", "kind", kind, "key", key, "status", status)
		return ""
	}
	if err := writeCacheFile(dir, path, response.Body); err != nil {
		c.logger.Warn("write cached asset", "kind", kind, "key", key, "error", err)
		return ""
	}
	c.logger.Info("cache miss", "kind", kind, "key", key)
	return path
}

func writeCacheFile(dir, target string, body []byte) (err error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create cache directory: %w", err)
	}
	temp, err := os.CreateTemp(dir, ".asset-*.tmp")
	if err != nil {
		return fmt.Errorf("create cache file: %w", err)
	}
	tempPath := temp.Name()
	defer func() {
		_ = temp.Close()
		_ = os.Remove(tempPath)
	}()
	if err := temp.Chmod(0o600); err != nil {
		return fmt.Errorf("set cache file permissions: %w", err)
	}
	if _, err := temp.Write(body); err != nil {
		return fmt.Errorf("write cache file: %w", err)
	}
	if err := temp.Close(); err != nil {
		return fmt.Errorf("close cache file: %w", err)
	}
	if err := os.Rename(tempPath, target); err != nil {
		return fmt.Errorf("replace cache file: %w", err)
	}
	tempPath = ""
	return nil
}
