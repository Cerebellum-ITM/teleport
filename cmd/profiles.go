package cmd

import (
	"fmt"
	"sort"

	"github.com/pascualchavez/teleport/internal/config"
	"github.com/spf13/cobra"
)

var profilesCmd = &cobra.Command{
	Use:   "profiles",
	Short: "List configured sync profiles",
	RunE:  runProfiles,
}

func runProfiles(_ *cobra.Command, _ []string) error {
	cfg, err := config.LoadGlobal()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	if len(cfg.Profiles) == 0 {
		fmt.Println("No profiles configured. Run `teleport init` to create one.")
		return nil
	}

	localCfg, _ := config.LoadLocal()
	defaultProfile := ""
	if localCfg != nil {
		defaultProfile = localCfg.DefaultProfile
	}

	names := make([]string, 0, len(cfg.Profiles))
	for n := range cfg.Profiles {
		names = append(names, n)
	}
	sort.Strings(names)

	fmt.Println("Configured profiles:")
	for _, name := range names {
		p := cfg.Profiles[name]
		marker := "  "
		if name == defaultProfile {
			marker = "* "
		}
		fmt.Printf("%s%-20s  %s:%s\n", marker, name, p.Host, p.Path)
	}

	return nil
}
