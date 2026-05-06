package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

const (
	globalConfigDir  = ".config/teleport"
	globalConfigFile = "config.toml"
	localConfigFile  = ".teleport.toml"
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
}

func GlobalConfigPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("home dir: %w", err)
	}
	return filepath.Join(home, globalConfigDir, globalConfigFile), nil
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
	cfg := &LocalConfig{}
	if _, err := os.Stat(localConfigFile); os.IsNotExist(err) {
		return cfg, nil
	}

	if _, err := toml.DecodeFile(localConfigFile, cfg); err != nil {
		return nil, fmt.Errorf("decode local config: %w", err)
	}
	return cfg, nil
}

func SaveLocal(cfg *LocalConfig) error {
	f, err := os.Create(localConfigFile)
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
