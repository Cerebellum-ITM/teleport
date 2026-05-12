package cmd

import (
	"fmt"
	"strconv"

	"github.com/pascualchavez/teleport/internal/config"
	"github.com/spf13/cobra"
)

var configCmd = &cobra.Command{
	Use:   "config",
	Short: " manage per-directory teleport defaults",
}

var configGetCmd = &cobra.Command{
	Use:   "get [key]",
	Short: "print one or all local config values for the current directory",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runConfigGet,
}

var configSetCmd = &cobra.Command{
	Use:   "set <key> <value>",
	Short: "set a local config value for the current directory",
	Args:  cobra.ExactArgs(2),
	RunE:  runConfigSet,
}

var configUnsetCmd = &cobra.Command{
	Use:   "unset <key>",
	Short: "reset a local config value to its default",
	Args:  cobra.ExactArgs(1),
	RunE:  runConfigUnset,
}

func init() {
	configCmd.AddCommand(configGetCmd)
	configCmd.AddCommand(configSetCmd)
	configCmd.AddCommand(configUnsetCmd)
}

func runConfigGet(_ *cobra.Command, args []string) error {
	cfg, err := config.LoadLocal()
	if err != nil {
		return fmt.Errorf("load local config: %w", err)
	}

	if len(args) == 0 {
		fmt.Printf("default-profile = %s\n", cfg.DefaultProfile)
		fmt.Printf("sync-untracked = %t\n", cfg.SyncUntracked)
		return nil
	}

	val, err := readConfigKey(cfg, args[0])
	if err != nil {
		return err
	}
	fmt.Println(val)
	return nil
}

func runConfigSet(_ *cobra.Command, args []string) error {
	cfg, err := config.LoadLocal()
	if err != nil {
		return fmt.Errorf("load local config: %w", err)
	}

	if err := applyConfigKey(cfg, args[0], args[1]); err != nil {
		return err
	}

	if err := config.SaveLocal(cfg); err != nil {
		return fmt.Errorf("save local config: %w", err)
	}

	fmt.Printf("%s = %s (saved for this wd)\n", args[0], args[1])
	return nil
}

func runConfigUnset(_ *cobra.Command, args []string) error {
	cfg, err := config.LoadLocal()
	if err != nil {
		return fmt.Errorf("load local config: %w", err)
	}

	switch args[0] {
	case "sync-untracked":
		cfg.SyncUntracked = false
	case "default-profile":
		cfg.DefaultProfile = ""
	default:
		return unknownConfigKey(args[0])
	}

	if err := config.SaveLocal(cfg); err != nil {
		return fmt.Errorf("save local config: %w", err)
	}

	fmt.Printf("%s reset to default\n", args[0])
	return nil
}

func readConfigKey(cfg *config.LocalConfig, key string) (string, error) {
	switch key {
	case "sync-untracked":
		return strconv.FormatBool(cfg.SyncUntracked), nil
	case "default-profile":
		return cfg.DefaultProfile, nil
	default:
		return "", unknownConfigKey(key)
	}
}

func applyConfigKey(cfg *config.LocalConfig, key, value string) error {
	switch key {
	case "sync-untracked":
		b, err := strconv.ParseBool(value)
		if err != nil {
			return fmt.Errorf("invalid value %q for sync-untracked: expected true/false", value)
		}
		cfg.SyncUntracked = b
	case "default-profile":
		if value == "" {
			return fmt.Errorf("default-profile cannot be empty; use `teleport config unset default-profile` to clear")
		}
		cfg.DefaultProfile = value
	default:
		return unknownConfigKey(key)
	}
	return nil
}

func unknownConfigKey(key string) error {
	return fmt.Errorf("unknown config key %q (valid: sync-untracked, default-profile)", key)
}
