package config

import (
	"testing"
	"time"
)

func TestActionEffectiveCwd(t *testing.T) {
	empty := ""
	dir := "/opt/app"

	t.Run("nil cwd uses profile path", func(t *testing.T) {
		a := Action{}
		if got := a.EffectiveCwd("/var/www"); got != "/var/www" {
			t.Fatalf("EffectiveCwd = %q, want /var/www", got)
		}
	})
	t.Run("empty cwd means no cd", func(t *testing.T) {
		a := Action{Cwd: &empty}
		if got := a.EffectiveCwd("/var/www"); got != "" {
			t.Fatalf("EffectiveCwd = %q, want empty", got)
		}
	})
	t.Run("explicit cwd wins over profile path", func(t *testing.T) {
		a := Action{Cwd: &dir}
		if got := a.EffectiveCwd("/var/www"); got != dir {
			t.Fatalf("EffectiveCwd = %q, want %q", got, dir)
		}
	})
}

func TestActionEffectiveTimeout(t *testing.T) {
	cases := []struct {
		timeout string
		want    time.Duration
	}{
		{"", defaultActionTimeout},
		{"5m", 5 * time.Minute},
		{"30s", 30 * time.Second},
		{"bogus", defaultActionTimeout}, // load-time validation rejects this; fallback for safety
		{"0s", defaultActionTimeout},    // non-positive falls back
	}
	for _, c := range cases {
		a := Action{Timeout: c.timeout}
		if got := a.EffectiveTimeout(); got != c.want {
			t.Fatalf("EffectiveTimeout(%q) = %v, want %v", c.timeout, got, c.want)
		}
	}
}

func TestSetRemoveAction(t *testing.T) {
	g := &GlobalConfig{Profiles: map[string]Profile{
		"prod": {Host: "h", Path: "/p"},
	}}

	g.SetAction("prod", "deploy", Action{Run: []string{"echo hi"}})
	if _, ok := g.Profiles["prod"].Actions["deploy"]; !ok {
		t.Fatalf("SetAction did not store the action: %+v", g.Profiles["prod"])
	}

	// Setting on an unknown profile is a no-op (no panic, no creation).
	g.SetAction("ghost", "x", Action{Run: []string{"true"}})
	if _, ok := g.Profiles["ghost"]; ok {
		t.Fatalf("SetAction created a ghost profile")
	}

	g.RemoveAction("prod", "deploy")
	if _, ok := g.Profiles["prod"].Actions["deploy"]; ok {
		t.Fatalf("RemoveAction did not delete the action")
	}
}
