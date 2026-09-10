package cmd

import (
	"fmt"
	"sort"

	"charm.land/huh/v2"
	"github.com/charmbracelet/log"
	"github.com/pascualchavez/teleport/internal/config"
)

// maybeMigrateBinProfiles offers to adopt a deprecated global [bin_profiles]
// section into the current project's local config. It is a no-op when the
// project already has bin profiles or the global section is empty. When
// running non-interactively it never prompts — it warns once and leaves the
// global section unused. On acceptance it copies every global entry into
// localCfg (persisting it) and then optionally clears the global section.
//
// localCfg is mutated and saved in place; callers pass the same instance they
// go on to use for resolution.
func maybeMigrateBinProfiles(localCfg *config.LocalConfig) error {
	if len(localCfg.BinProfiles) > 0 {
		return nil
	}

	globalCfg, err := config.LoadGlobal()
	if err != nil {
		return fmt.Errorf("load global config: %w", err)
	}
	if len(globalCfg.BinProfiles) == 0 {
		return nil
	}

	if !interactive() {
		log.Warn(`global [bin_profiles] is deprecated — run "teleport ship" interactively to migrate it into this project`)
		return nil
	}

	fmt.Println("\n  Found a deprecated global [bin_profiles] section:")
	for _, osName := range sortedBinOSes(globalCfg.BinProfiles) {
		p := globalCfg.BinProfiles[osName]
		line := fmt.Sprintf("    %-8s %s:%s", osName, p.Host, p.BinPath)
		if p.RemoteName != "" {
			line += fmt.Sprintf("  (as %s)", p.RemoteName)
		}
		fmt.Println(line)
	}
	fmt.Println()

	adopt := false
	form := huh.NewForm(huh.NewGroup(
		huh.NewConfirm().
			Title("Adopt these bin profile(s) into this project?").
			Description("Bin profiles are now per-project; the global section is deprecated.").
			Value(&adopt),
	))
	if err := form.Run(); err != nil {
		return fmt.Errorf("form: %w", err)
	}
	if !adopt {
		return nil
	}

	for osName, p := range globalCfg.BinProfiles {
		localCfg.SetBinProfile(osName, p)
	}
	if err := config.SaveLocal(localCfg); err != nil {
		return fmt.Errorf("save local config: %w", err)
	}
	fmt.Println("  Adopted into this project's config.")

	removeGlobal := false
	form2 := huh.NewForm(huh.NewGroup(
		huh.NewConfirm().
			Title("Remove [bin_profiles] from the global config?").
			Description("Keep it if another project still needs to adopt these.").
			Value(&removeGlobal),
	))
	if err := form2.Run(); err != nil {
		return fmt.Errorf("form: %w", err)
	}
	if removeGlobal {
		globalCfg.BinProfiles = nil
		if err := config.SaveGlobal(globalCfg); err != nil {
			return fmt.Errorf("save global config: %w", err)
		}
		fmt.Println("  Removed the global [bin_profiles] section.")
	}
	return nil
}

// sortedBinOSes returns the OS keys of m in a stable order.
func sortedBinOSes(m map[string]config.BinProfile) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
