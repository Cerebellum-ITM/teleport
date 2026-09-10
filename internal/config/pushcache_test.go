package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPushCacheRoundTrip(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Chdir(t.TempDir())

	cache := LoadPushCache("staging")
	if len(cache.Entries) != 0 {
		t.Fatalf("a fresh cache should be empty, got %d entries", len(cache.Entries))
	}

	cache.Entries["/srv/app/a.deb"] = PushCacheEntry{
		LocalSize: 7, LocalMtime: 100, RemoteSize: 7, RemoteMtime: 200, Hash: "abc",
	}
	if err := SavePushCache("staging", cache); err != nil {
		t.Fatalf("SavePushCache: %v", err)
	}

	if got := LoadPushCache("staging").Entries["/srv/app/a.deb"]; got.Hash != "abc" || got.RemoteMtime != 200 {
		t.Fatalf("round trip lost data: %+v", got)
	}
	if len(LoadPushCache("prod").Entries) != 0 {
		t.Fatal("another profile must not see this project's entries")
	}
}

func TestLoadPushCacheToleratesGarbage(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Chdir(t.TempDir())

	path, err := PushCachePath("staging")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("not json"), 0o644); err != nil {
		t.Fatal(err)
	}

	cache := LoadPushCache("staging")
	if cache == nil || cache.Entries == nil {
		t.Fatal("a corrupt cache must degrade to an empty one, not a nil map")
	}
}
