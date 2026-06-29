package config

import (
	"testing"
	"time"
)

func TestBeamedHelpers(t *testing.T) {
	cfg := &LocalConfig{}
	now := time.Unix(1_700_000_000, 0)

	// Empty config: SentSet returns an empty (non-nil) map.
	if got := cfg.SentSet("prod"); len(got) != 0 {
		t.Fatalf("SentSet on empty cfg = %v, want empty", got)
	}

	// MarkBeamed creates the nested maps and records SHAs per profile.
	cfg.MarkBeamed("prod", []string{"a", "b"}, now)
	cfg.MarkBeamed("staging", []string{"b"}, now)

	if got := cfg.SentSet("prod"); !got["a"] || !got["b"] {
		t.Fatalf("prod SentSet = %v, want a and b", got)
	}
	// State is per profile: "a" is not sent for staging.
	if got := cfg.SentSet("staging"); got["a"] || !got["b"] {
		t.Fatalf("staging SentSet = %v, want only b", got)
	}

	// PruneBeamed drops SHAs not in keep and removes empty profiles.
	cfg.PruneBeamed("prod", map[string]bool{"b": true})
	if got := cfg.SentSet("prod"); got["a"] || !got["b"] {
		t.Fatalf("after prune prod = %v, want only b", got)
	}
	cfg.PruneBeamed("staging", map[string]bool{}) // nothing kept
	if _, ok := cfg.BeamedCommits["staging"]; ok {
		t.Fatalf("empty staging profile should have been removed: %v", cfg.BeamedCommits)
	}

	// MarkBeamed with no SHAs is a no-op.
	cfg.MarkBeamed("prod", nil, now)
	if got := cfg.SentSet("prod"); len(got) != 1 {
		t.Fatalf("MarkBeamed(nil) changed state: %v", got)
	}
}

func TestApplyBeamedDelta(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)

	t.Run("add only", func(t *testing.T) {
		cfg := &LocalConfig{}
		cfg.ApplyBeamedDelta("prod", []string{"a", "b"}, nil, now)
		if got := cfg.SentSet("prod"); !got["a"] || !got["b"] || len(got) != 2 {
			t.Fatalf("SentSet = %v, want {a,b}", got)
		}
	})

	t.Run("mixed add and remove in one pass", func(t *testing.T) {
		cfg := &LocalConfig{}
		cfg.MarkBeamed("prod", []string{"a", "b"}, now)
		cfg.ApplyBeamedDelta("prod", []string{"c"}, []string{"a"}, now)
		got := cfg.SentSet("prod")
		if got["a"] || !got["b"] || !got["c"] || len(got) != 2 {
			t.Fatalf("SentSet = %v, want {b,c}", got)
		}
	})

	t.Run("removing an absent SHA is a no-op", func(t *testing.T) {
		cfg := &LocalConfig{}
		cfg.MarkBeamed("prod", []string{"a"}, now)
		cfg.ApplyBeamedDelta("prod", nil, []string{"zzz"}, now)
		if got := cfg.SentSet("prod"); !got["a"] || len(got) != 1 {
			t.Fatalf("SentSet = %v, want {a}", got)
		}
	})

	t.Run("emptying a profile drops the profile key", func(t *testing.T) {
		cfg := &LocalConfig{}
		cfg.MarkBeamed("prod", []string{"a"}, now)
		cfg.ApplyBeamedDelta("prod", nil, []string{"a"}, now)
		if _, ok := cfg.BeamedCommits["prod"]; ok {
			t.Fatalf("empty prod profile should have been removed: %v", cfg.BeamedCommits)
		}
	})

	t.Run("scoped per profile", func(t *testing.T) {
		cfg := &LocalConfig{}
		cfg.MarkBeamed("staging", []string{"a"}, now)
		cfg.ApplyBeamedDelta("prod", []string{"a"}, nil, now)
		if got := cfg.SentSet("staging"); !got["a"] || len(got) != 1 {
			t.Fatalf("staging untouched? = %v", got)
		}
	})

	t.Run("empty delta is a no-op", func(t *testing.T) {
		cfg := &LocalConfig{}
		cfg.ApplyBeamedDelta("prod", nil, nil, now)
		if len(cfg.BeamedCommits) != 0 {
			t.Fatalf("empty delta created state: %v", cfg.BeamedCommits)
		}
	})
}
