package config

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/pascualchavez/teleport/internal/bindetect"
)

const (
	globalConfigDir  = ".config/teleport"
	globalConfigFile = "config.toml"
)

type Profile struct {
	Host    string            `toml:"host"`
	Path    string            `toml:"path"`
	Actions map[string]Action `toml:"actions,omitempty"`
}

// Action is a named remote command sequence attached to a profile. Its steps
// run in order over the profile's SSH host; the first non-zero step aborts the
// action. Cwd is a pointer so three states are distinguishable: nil = use the
// profile path, "" = run with no `cd`, any other value = that directory.
type Action struct {
	Run     []string `toml:"run"`
	Cwd     *string  `toml:"cwd,omitempty"`
	Timeout string   `toml:"timeout,omitempty"`
	Confirm bool     `toml:"confirm,omitempty"`
}

const defaultActionTimeout = 10 * time.Minute

// EffectiveCwd returns the directory a step should run in: the explicit Cwd
// when set (including the empty string, meaning "no cd"), otherwise the
// profile path.
func (a Action) EffectiveCwd(profilePath string) string {
	if a.Cwd != nil {
		return *a.Cwd
	}
	return profilePath
}

// EffectiveTimeout parses Timeout, falling back to the 10m default when unset
// or unparseable (validation on load rejects malformed values, so the fallback
// is only reached for a truly empty field).
func (a Action) EffectiveTimeout() time.Duration {
	if a.Timeout == "" {
		return defaultActionTimeout
	}
	d, err := time.ParseDuration(a.Timeout)
	if err != nil || d <= 0 {
		return defaultActionTimeout
	}
	return d
}

// BinProfile describes the destination for `teleport ship` for a given
// target OS — the SSH host, the absolute path of the remote bin dir,
// an optional fixed remote filename, and an optional local binary path.
type BinProfile struct {
	Host       string `toml:"host"`
	BinPath    string `toml:"bin_path"`
	RemoteName string `toml:"remote_name,omitempty"`
	BinFile    string `toml:"bin_file,omitempty"`
}

type GlobalConfig struct {
	Profiles    map[string]Profile    `toml:"profiles"`
	BinProfiles map[string]BinProfile `toml:"bin_profiles,omitempty"`
}

type LocalConfig struct {
	DefaultProfile string    `toml:"default_profile"`
	SyncUntracked  bool      `toml:"sync_untracked,omitempty"`
	LastSync       time.Time `toml:"last_sync,omitempty"`
	BinDir         string    `toml:"bin_dir,omitempty"`

	// BinProfiles holds this project's `teleport ship` destinations, keyed by
	// target OS (linux|macos|windows). Per-project so different repos can ship
	// different binaries to different servers without interfering. Replaces the
	// deprecated global [bin_profiles] section.
	BinProfiles map[string]BinProfile `toml:"bin_profiles,omitempty"`

	// BeamedCommits maps a profile name to the set of commit SHAs already
	// beamed to that destination, with the time each was sent.
	BeamedCommits map[string]map[string]time.Time `toml:"beamed_commits,omitempty"`
}

func GlobalConfigPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("home dir: %w", err)
	}
	return filepath.Join(home, globalConfigDir, globalConfigFile), nil
}

// LocalConfigPath returns ~/.config/teleport/projects/<sha256-of-cwd>.toml.
// Storing it under ~/.config keeps project directories free of teleport files.
func LocalConfigPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("home dir: %w", err)
	}
	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("getwd: %w", err)
	}
	h := sha256.Sum256([]byte(cwd))
	name := fmt.Sprintf("%x.toml", h[:8])
	return filepath.Join(home, globalConfigDir, "projects", name), nil
}

func LoadGlobal() (*GlobalConfig, error) {
	path, err := GlobalConfigPath()
	if err != nil {
		return nil, err
	}

	cfg := &GlobalConfig{Profiles: make(map[string]Profile)}
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return cfg, nil
	}

	if _, err := toml.DecodeFile(path, cfg); err != nil {
		return nil, fmt.Errorf("decode global config: %w", err)
	}
	for k := range cfg.BinProfiles {
		if !bindetect.Valid(k) {
			return nil, fmt.Errorf("unknown bin profile OS %q (expected linux|macos|windows)", k)
		}
	}
	for name, p := range cfg.Profiles {
		for aname, a := range p.Actions {
			if len(a.Run) == 0 {
				return nil, fmt.Errorf("profile %q action %q: at least one `run` step is required", name, aname)
			}
			if a.Timeout != "" {
				if _, err := time.ParseDuration(a.Timeout); err != nil {
					return nil, fmt.Errorf("profile %q action %q: invalid timeout %q: %w", name, aname, a.Timeout, err)
				}
			}
		}
	}
	return cfg, nil
}

func SaveGlobal(cfg *GlobalConfig) error {
	path, err := GlobalConfigPath()
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}

	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create config file: %w", err)
	}
	defer f.Close()

	return toml.NewEncoder(f).Encode(cfg)
}

func LoadLocal() (*LocalConfig, error) {
	path, err := LocalConfigPath()
	if err != nil {
		return nil, err
	}

	cfg := &LocalConfig{}
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return cfg, nil
	}

	if _, err := toml.DecodeFile(path, cfg); err != nil {
		return nil, fmt.Errorf("decode local config: %w", err)
	}
	for k := range cfg.BinProfiles {
		if !bindetect.Valid(k) {
			return nil, fmt.Errorf("unknown bin profile OS %q (expected linux|macos|windows)", k)
		}
	}
	return cfg, nil
}

func SaveLocal(cfg *LocalConfig) error {
	path, err := LocalConfigPath()
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return fmt.Errorf("create local config dir: %w", err)
	}

	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create local config: %w", err)
	}
	defer f.Close()

	return toml.NewEncoder(f).Encode(cfg)
}

// TouchLastSync sets LastSync = time.Now() on the local config and
// persists it. Safe to call on a fresh wd (creates the file).
func TouchLastSync() error {
	cfg, err := LoadLocal()
	if err != nil {
		return err
	}
	cfg.LastSync = time.Now()
	return SaveLocal(cfg)
}

// SentSet returns a membership view of the commits already beamed to profile.
// The returned map is safe to read even when no commits have been recorded.
func (c *LocalConfig) SentSet(profile string) map[string]bool {
	out := make(map[string]bool, len(c.BeamedCommits[profile]))
	for sha := range c.BeamedCommits[profile] {
		out[sha] = true
	}
	return out
}

// PruneBeamed drops every recorded SHA for profile that is not present in keep.
// Caller is responsible for persisting via SaveLocal.
func (c *LocalConfig) PruneBeamed(profile string, keep map[string]bool) {
	sent := c.BeamedCommits[profile]
	if sent == nil {
		return
	}
	for sha := range sent {
		if !keep[sha] {
			delete(sent, sha)
		}
	}
	if len(sent) == 0 {
		delete(c.BeamedCommits, profile)
	}
}

// MarkBeamed records each SHA as beamed to profile at time t, creating the
// nested maps as needed. Caller is responsible for persisting via SaveLocal.
func (c *LocalConfig) MarkBeamed(profile string, shas []string, t time.Time) {
	if len(shas) == 0 {
		return
	}
	if c.BeamedCommits == nil {
		c.BeamedCommits = make(map[string]map[string]time.Time)
	}
	if c.BeamedCommits[profile] == nil {
		c.BeamedCommits[profile] = make(map[string]time.Time)
	}
	for _, sha := range shas {
		c.BeamedCommits[profile][sha] = t
	}
}

// ApplyBeamedDelta applies a manual sent-mark delta to profile in one pass:
// every SHA in add is recorded as beamed at time t, and every SHA in remove is
// dropped. Removals of absent SHAs are no-ops. If the profile's set ends up
// empty it is deleted (consistent with PruneBeamed). No-op when both slices are
// empty. Mutator; caller is responsible for persisting via SaveLocal.
func (c *LocalConfig) ApplyBeamedDelta(profile string, add, remove []string, t time.Time) {
	if len(add) == 0 && len(remove) == 0 {
		return
	}
	if len(add) > 0 {
		c.MarkBeamed(profile, add, t)
	}
	if sent := c.BeamedCommits[profile]; sent != nil {
		for _, sha := range remove {
			delete(sent, sha)
		}
		if len(sent) == 0 {
			delete(c.BeamedCommits, profile)
		}
	}
}

func (g *GlobalConfig) SetProfile(name string, p Profile) {
	if g.Profiles == nil {
		g.Profiles = make(map[string]Profile)
	}
	g.Profiles[name] = p
}

func (g *GlobalConfig) RemoveProfile(name string) {
	delete(g.Profiles, name)
}

// SetAction stores action a under name on the named profile. It is a no-op
// when the profile does not exist (callers validate the profile first).
func (g *GlobalConfig) SetAction(profile, name string, a Action) {
	p, ok := g.Profiles[profile]
	if !ok {
		return
	}
	if p.Actions == nil {
		p.Actions = make(map[string]Action)
	}
	p.Actions[name] = a
	g.Profiles[profile] = p
}

// RemoveAction deletes the named action from the profile, if present.
func (g *GlobalConfig) RemoveAction(profile, name string) {
	p, ok := g.Profiles[profile]
	if !ok {
		return
	}
	delete(p.Actions, name)
	g.Profiles[profile] = p
}

// SetBinProfile on GlobalConfig is retained only so the deprecated global
// [bin_profiles] section can be read and cleared during migration. New bin
// profiles are written per-project via LocalConfig.SetBinProfile.
func (g *GlobalConfig) SetBinProfile(os string, p BinProfile) {
	if g.BinProfiles == nil {
		g.BinProfiles = make(map[string]BinProfile)
	}
	g.BinProfiles[os] = p
}

func (g *GlobalConfig) RemoveBinProfile(os string) {
	delete(g.BinProfiles, os)
}

// SetBinProfile stores this project's ship destination p under target OS os.
func (c *LocalConfig) SetBinProfile(os string, p BinProfile) {
	if c.BinProfiles == nil {
		c.BinProfiles = make(map[string]BinProfile)
	}
	c.BinProfiles[os] = p
}

// RemoveBinProfile deletes the bin profile for target OS os, if present.
func (c *LocalConfig) RemoveBinProfile(os string) {
	delete(c.BinProfiles, os)
}
