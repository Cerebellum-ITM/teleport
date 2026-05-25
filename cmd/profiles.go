package cmd

import (
	"fmt"
	"sort"

	"github.com/charmbracelet/log"
	"github.com/pascualchavez/teleport/internal/config"
	"github.com/spf13/cobra"
)

var profilesCmd = &cobra.Command{
	Use:   "profiles",
	Short: " list configured sync profiles",
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

var profilesRemoveCmd = &cobra.Command{
	Use:     "remove <name>",
	Aliases: []string{"rm"},
	Short:   "remove a profile from the global config",
	Args:    cobra.ExactArgs(1),
	RunE:    runProfilesRemove,
}

func init() {
	profilesCmd.AddCommand(profilesRemoveCmd)
}

func runProfilesRemove(_ *cobra.Command, args []string) error {
	name := args[0]

	globalCfg, err := config.LoadGlobal()
	if err != nil {
		return fmt.Errorf("load global config: %w", err)
	}

	if _, ok := globalCfg.Profiles[name]; !ok {
		return fmt.Errorf("profile %q not found\nhint: run `teleport profiles` to see all configured profiles", name)
	}

	globalCfg.RemoveProfile(name)

	if err := config.SaveGlobal(globalCfg); err != nil {
		return fmt.Errorf("save global config: %w", err)
	}

	fmt.Printf("Profile %q removed.\n", name)

	localCfg, _ := config.LoadLocal()
	if localCfg != nil && localCfg.DefaultProfile == name {
		log.Warn(fmt.Sprintf("%q was the default profile for this directory", name),
			"hint", "run `teleport config set default-profile <name>` to pick a new one")
	}

	return nil
}
