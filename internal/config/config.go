package config

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

const (
	globalConfigDir  = ".config/teleport"
	globalConfigFile = "config.toml"
)

type Profile struct {
	Host string `toml:"host"`
	Path string `toml:"path"`
}

type GlobalConfig struct {
	Profiles map[string]Profile `toml:"profiles"`
}

type LocalConfig struct {
	DefaultProfile string `toml:"default_profile"`
	SyncUntracked  bool   `toml:"sync_untracked,omitempty"`
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

func (g *GlobalConfig) SetProfile(name string, p Profile) {
	if g.Profiles == nil {
		g.Profiles = make(map[string]Profile)
	}
	g.Profiles[name] = p
}
