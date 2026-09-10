package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// PushCacheEntry records a destination push has already proven identical, with
// the stat of both ends at that moment. A later run may reuse Hash only while
// both stats still match: a local edit changes one, an edit made on the server
// changes the other.
type PushCacheEntry struct {
	LocalSize   int64  `json:"local_size"`
	LocalMtime  int64  `json:"local_mtime"`
	RemoteSize  int64  `json:"remote_size"`
	RemoteMtime int64  `json:"remote_mtime"`
	Hash        string `json:"hash"`
}

// PushCache maps a remote path to what was last verified about it. It is
// disposable: deleting the file costs one round of hashing, never correctness.
type PushCache struct {
	Entries map[string]PushCacheEntry `json:"entries"`
}

// PushCachePath returns ~/.config/teleport/cache/<project>-<profile>.json. The
// cache lives apart from the project config because every command reads that
// config, and a large tree would put thousands of entries in front of all of
// them.
func PushCachePath(profile string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("home dir: %w", err)
	}
	key, err := projectKey()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, globalConfigDir, "cache", fmt.Sprintf("%s-%s.json", key, profile)), nil
}

// LoadPushCache reads the cache for a profile. A missing or corrupt file yields
// an empty cache rather than an error — it exists to save work, so a failure to
// read it may only cost work.
func LoadPushCache(profile string) *PushCache {
	cache := &PushCache{Entries: make(map[string]PushCacheEntry)}

	path, err := PushCachePath(profile)
	if err != nil {
		return cache
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return cache
	}
	if err := json.Unmarshal(data, cache); err != nil || cache.Entries == nil {
		return &PushCache{Entries: make(map[string]PushCacheEntry)}
	}
	return cache
}

func SavePushCache(profile string, cache *PushCache) error {
	path, err := PushCachePath(profile)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create cache dir: %w", err)
	}
	data, err := json.Marshal(cache)
	if err != nil {
		return fmt.Errorf("encode push cache: %w", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write push cache: %w", err)
	}
	return nil
}
