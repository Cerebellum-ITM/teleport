package cmd

import (
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/pascualchavez/teleport/internal/config"
	"github.com/pascualchavez/teleport/internal/tui"
	"github.com/spf13/cobra"
)

var excludeCmd = &cobra.Command{
	Use:   "exclude",
	Short: "󰈉 manage the paths `teleport push` skips in this project",
	Args:  cobra.NoArgs,
	RunE:  runExcludePick,
}

var excludeListCmd = &cobra.Command{
	Use:   "list",
	Short: "show the patterns push skips here and where each comes from",
	Args:  cobra.NoArgs,
	RunE:  runExcludeList,
}

var excludeAddCmd = &cobra.Command{
	Use:   "add <pattern>...",
	Short: "add patterns to this project's exclude list",
	Args:  cobra.MinimumNArgs(1),
	RunE:  runExcludeAdd,
}

var excludeRemoveCmd = &cobra.Command{
	Use:     "remove <pattern>...",
	Aliases: []string{"rm"},
	Short:   "drop patterns from this project's exclude list",
	Args:    cobra.MinimumNArgs(1),
	RunE:    runExcludeRemove,
}

func init() {
	excludeCmd.AddCommand(excludeListCmd)
	excludeCmd.AddCommand(excludeAddCmd)
	excludeCmd.AddCommand(excludeRemoveCmd)
	rootCmd.AddCommand(excludeCmd)
}

// excludeList is the --json shape for `teleport exclude list`.
type excludeList struct {
	Command  string   `json:"command"`
	Defaults []string `json:"defaults"`
	Project  []string `json:"project"`
}

func runExcludeList(_ *cobra.Command, _ []string) error {
	cfg, err := config.LoadLocal()
	if err != nil {
		return fmt.Errorf("load local config: %w", err)
	}

	res := excludeList{Command: "exclude", Defaults: pushDefaultExcludes, Project: cfg.PushExclude}
	emit(res, func() {
		fmt.Println("Excluded by push:")
		for _, p := range res.Defaults {
			fmt.Printf("  %-24s default\n", p)
		}
		for _, p := range res.Project {
			fmt.Printf("  %-24s project\n", p)
		}
		if len(res.Project) == 0 {
			fmt.Println("  (nothing project-specific; add one with `teleport exclude add <pattern>`)")
		}
	})
	return nil
}

func runExcludeAdd(_ *cobra.Command, args []string) error {
	for _, p := range args {
		if err := validateExcludePattern(p); err != nil {
			return err
		}
	}
	return saveProjectExcludes(func(current []string) []string {
		return append(current, args...)
	})
}

func runExcludeRemove(_ *cobra.Command, args []string) error {
	drop := make(map[string]bool, len(args))
	for _, p := range args {
		drop[strings.TrimSpace(p)] = true
	}
	return saveProjectExcludes(func(current []string) []string {
		kept := make([]string, 0, len(current))
		for _, p := range current {
			if !drop[p] {
				kept = append(kept, p)
			}
		}
		return kept
	})
}

// runExcludePick is the bare command: a browser over the project tree where any
// entry, at any depth, can be toggled. A pick is stored as its path relative to
// the project root, so a nested directory becomes an anchored pattern
// (`web/node_modules`) while a top-level one stays a bare name that matches
// anywhere. Patterns the browser never showed — globs like `*.log`, or entries
// inside folders that were not opened — are left untouched.
func runExcludePick(_ *cobra.Command, _ []string) error {
	if !interactive() {
		return errNeedsTTY("pass `teleport exclude add <pattern>`")
	}

	root, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getwd: %w", err)
	}
	cfg, err := config.LoadLocal()
	if err != nil {
		return fmt.Errorf("load local config: %w", err)
	}

	picks, shown, err := tui.RunExcludePicker(root, "  Paths push should skip in this project", cfg.PushExclude)
	if errors.Is(err, tui.ErrPickerCancelled) {
		fmt.Println("  nothing changed")
		return nil
	}
	if err != nil {
		return err
	}

	visited := make(map[string]bool, len(shown))
	for _, p := range shown {
		visited[p] = true
	}
	return saveProjectExcludes(func(existing []string) []string {
		kept := make([]string, 0, len(existing)+len(picks))
		for _, p := range existing {
			if !visited[p] {
				kept = append(kept, p)
			}
		}
		return append(kept, picks...)
	})
}

// saveProjectExcludes applies change to the project's exclude list and persists
// it deduplicated and sorted, then reports the result through the same renderer
// as `exclude list`.
func saveProjectExcludes(change func([]string) []string) error {
	cfg, err := config.LoadLocal()
	if err != nil {
		return fmt.Errorf("load local config: %w", err)
	}

	seen := make(map[string]bool)
	var patterns []string
	for _, p := range change(cfg.PushExclude) {
		p = strings.TrimSpace(p)
		if p == "" || seen[p] {
			continue
		}
		seen[p] = true
		patterns = append(patterns, p)
	}
	sort.Strings(patterns)

	cfg.PushExclude = patterns
	if err := config.SaveLocal(cfg); err != nil {
		return err
	}
	return runExcludeList(nil, nil)
}
