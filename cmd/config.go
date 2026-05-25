package cmd

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/pascualchavez/teleport/internal/config"
	"github.com/spf13/cobra"
)

var configCmd = &cobra.Command{
	Use:   "config",
	Short: " manage per-directory teleport defaults",
	Run: func(c *cobra.Command, _ []string) {
		_ = c.Help()
	},
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

	configCmd.SetHelpFunc(func(_ *cobra.Command, _ []string) {
		fmt.Println(renderHelp(configHelpDoc()))
	})
	configGetCmd.SetHelpFunc(func(_ *cobra.Command, _ []string) {
		fmt.Println(renderHelp(configGetHelpDoc()))
	})
	configSetCmd.SetHelpFunc(func(_ *cobra.Command, _ []string) {
		fmt.Println(renderHelp(configSetHelpDoc()))
	})
	configUnsetCmd.SetHelpFunc(func(_ *cobra.Command, _ []string) {
		fmt.Println(renderHelp(configUnsetHelpDoc()))
	})
}

func configKeysTable() []keyEntry {
	return []keyEntry{
		{iconKey, "default-profile", "string", "<unset>", "profile used when no name is passed to sync/beam/clean"},
		{iconKey, "sync-untracked", "bool", "false", "include untracked files on every sync (same as -u)"},
	}
}

func configHelpDoc() helpDoc {
	return helpDoc{
		Title: " teleport config ",
		Tagline: []string{
			"Per-directory defaults for teleport.",
			"Saved in ~/.config/teleport/projects/ — never inside the project tree.",
		},
		Commands: []cmdEntry{
			{iconGear, "get", "print all values + active profile + last sync (or one key)"},
			{iconGear, "set", "set a local config value for this wd"},
			{iconGear, "unset", "reset a local config value to its default"},
		},
		Examples: [][]string{
			{"teleport config get", "show all values + active profile + last sync"},
			{"teleport config get default-profile", "print one value (scriptable)"},
			{"teleport config set sync-untracked true", "remember -u for this wd"},
			{"teleport config unset default-profile", "fall back to global default"},
		},
		KeysTable: configKeysTable(),
	}
}

func configGetHelpDoc() helpDoc {
	return helpDoc{
		Title: " teleport config get ",
		Tagline: []string{
			"Without a key: print all values, the resolved profile (host:path),",
			"and the last time this wd synced with the remote.",
			"With a key: print only the raw value (suitable for $() in scripts).",
		},
		Examples: [][]string{
			{"teleport config get", "show all values + active profile + last sync"},
			{"teleport config get default-profile", "print one value"},
			{"teleport config get sync-untracked", "print true/false"},
		},
		KeysTable: configKeysTable(),
	}
}

func configSetHelpDoc() helpDoc {
	return helpDoc{
		Title: " teleport config set ",
		Tagline: []string{
			"Set a per-directory default for teleport.",
			"Stored under ~/.config/teleport/projects/<hash>.toml.",
		},
		Examples: [][]string{
			{"teleport config set sync-untracked true", "remember -u for this wd"},
			{"teleport config set default-profile staging", "pin this wd to a profile"},
		},
		KeysTable: configKeysTable(),
	}
}

func configUnsetHelpDoc() helpDoc {
	return helpDoc{
		Title: " teleport config unset ",
		Tagline: []string{
			"Reset a per-directory default to its built-in fallback.",
		},
		Examples: [][]string{
			{"teleport config unset default-profile", "fall back to global default"},
			{"teleport config unset sync-untracked", "stop including untracked files"},
		},
		KeysTable: configKeysTable(),
	}
}

func runConfigGet(_ *cobra.Command, args []string) error {
	cfg, err := config.LoadLocal()
	if err != nil {
		return fmt.Errorf("load local config: %w", err)
	}

	if len(args) > 0 {
		val, err := readConfigKey(cfg, args[0])
		if err != nil {
			return err
		}
		fmt.Println(val)
		return nil
	}

	return printConfigOverview(cfg)
}

func printConfigOverview(local *config.LocalConfig) error {
	globalCfg, err := config.LoadGlobal()
	if err != nil {
		return fmt.Errorf("load global config: %w", err)
	}

	var b strings.Builder

	fmt.Fprintf(&b, "\n  %s\n\n", titleStyle.Render(" teleport config "))

	defaultProfileDisplay := local.DefaultProfile
	fromGlobal := false
	if defaultProfileDisplay == "" {
		// fall back to global if there's exactly one profile
		if len(globalCfg.Profiles) == 1 {
			for name := range globalCfg.Profiles {
				defaultProfileDisplay = name
				fromGlobal = true
			}
		}
	}

	if defaultProfileDisplay == "" {
		fmt.Fprintf(&b, "  default-profile  = %s\n", descStyle.Render("<unset>"))
	} else if fromGlobal {
		fmt.Fprintf(&b, "  default-profile  = %s %s\n",
			defaultProfileDisplay,
			descStyle.Render("(from global)"))
	} else {
		fmt.Fprintf(&b, "  default-profile  = %s\n", defaultProfileDisplay)
	}
	fmt.Fprintf(&b, "  sync-untracked   = %t\n", local.SyncUntracked)

	if defaultProfileDisplay != "" {
		profile, ok := globalCfg.Profiles[defaultProfileDisplay]
		fmt.Fprintf(&b, "\n  %s %s\n",
			iconStyle.Render(string(iconPerson)),
			sectionStyle.Render(fmt.Sprintf("profile %s", defaultProfileDisplay)))
		if !ok {
			fmt.Fprintf(&b, "    %s\n", descStyle.Render("(not found in global config)"))
		} else {
			fmt.Fprintf(&b, "    host  =  %s\n", profile.Host)
			fmt.Fprintf(&b, "    path  =  %s\n", profile.Path)
		}
	}

	fmt.Fprintf(&b, "\n  %s %s\n",
		iconStyle.Render(string(iconSync)),
		sectionStyle.Render("last sync"))
	if local.LastSync.IsZero() {
		fmt.Fprintf(&b, "    %s\n", descStyle.Render("never"))
	} else {
		ts := local.LastSync.Local().Format("2006-01-02 15:04:05 -07")
		fmt.Fprintf(&b, "    %s  %s\n",
			ts,
			descStyle.Render("("+humanizeSince(local.LastSync)+")"))
	}

	fmt.Println(b.String())
	return nil
}

// humanizeSince renders a Spanish, human-friendly "ago" string.
func humanizeSince(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "hace unos segundos"
	case d < time.Hour:
		n := int(d / time.Minute)
		if n == 1 {
			return "hace 1 minuto"
		}
		return fmt.Sprintf("hace %d minutos", n)
	case d < 24*time.Hour:
		n := int(d / time.Hour)
		if n == 1 {
			return "hace 1 hora"
		}
		return fmt.Sprintf("hace %d horas", n)
	case d < 7*24*time.Hour:
		n := int(d / (24 * time.Hour))
		if n == 1 {
			return "hace 1 día"
		}
		return fmt.Sprintf("hace %d días", n)
	default:
		months := []string{"ene", "feb", "mar", "abr", "may", "jun", "jul", "ago", "sep", "oct", "nov", "dic"}
		return fmt.Sprintf("hace %d %s", t.Day(), months[int(t.Month())-1])
	}
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
