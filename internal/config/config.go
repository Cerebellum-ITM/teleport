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
	Host string `toml:"host"`
	Path string `toml:"path"`
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

func (g *GlobalConfig) SetProfile(name string, p Profile) {
	if g.Profiles == nil {
		g.Profiles = make(map[string]Profile)
	}
	g.Profiles[name] = p
}

func (g *GlobalConfig) RemoveProfile(name string) {
	delete(g.Profiles, name)
}

func (g *GlobalConfig) SetBinProfile(os string, p BinProfile) {
	if g.BinProfiles == nil {
		g.BinProfiles = make(map[string]BinProfile)
	}
	g.BinProfiles[os] = p
}

func (g *GlobalConfig) RemoveBinProfile(os string) {
	delete(g.BinProfiles, os)
}
