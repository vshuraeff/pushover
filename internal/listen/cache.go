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
	cachedPath := c.resolve(ctx, c.iconDir, key, ".png", "/icons/", "icon")
	if cachedPath == "" {
		return ""
	}
	deliveryPath, err := stageCachedAsset(c.iconDir, cachedPath)
	if err != nil {
		c.logger.Warn("stage cached icon", "key", key, "error", err)
		return ""
	}
	return deliveryPath
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
	name := filepath.Base(converted)
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
		file, openErr := os.Open(path)
		if openErr == nil {
			_ = file.Close()
			return path
		}
		c.logger.Warn("read cached asset", "kind", kind, "path", path, "error", openErr)
	} else if !os.IsNotExist(err) {
		c.logger.Warn("inspect cached asset", "kind", kind, "path", path, "error", err)
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

func stageCachedAsset(dir, cachedPath string) (path string, err error) {
	deliveryDir, err := os.MkdirTemp(dir, ".delivery-")
	if err != nil {
		return "", fmt.Errorf("create delivery directory: %w", err)
	}
	target := filepath.Join(deliveryDir, filepath.Base(cachedPath))
	defer func() {
		if err == nil {
			return
		}
		_ = os.Remove(target)
		_ = os.Remove(deliveryDir)
	}()

	if linkErr := os.Link(cachedPath, target); linkErr == nil {
		return target, nil
	}
	data, err := os.ReadFile(cachedPath)
	if err != nil {
		return "", fmt.Errorf("read cached asset: %w", err)
	}
	if err := os.WriteFile(target, data, 0o600); err != nil {
		return "", fmt.Errorf("copy cached asset: %w", err)
	}
	return target, nil
}

func (c *assetCache) cleanupIcon(path string) {
	if path == "" || c.iconDir == "" {
		return
	}
	deliveryDir := filepath.Dir(path)
	if filepath.Dir(deliveryDir) != filepath.Clean(c.iconDir) || !strings.HasPrefix(filepath.Base(deliveryDir), ".delivery-") {
		return
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		c.logger.Debug("remove delivered icon", "path", path, "error", err)
	}
	if err := os.Remove(deliveryDir); err != nil && !os.IsNotExist(err) {
		c.logger.Debug("remove icon delivery directory", "path", deliveryDir, "error", err)
	}
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
