package cmd

import (
	"fmt"
	"strings"
	"time"

	"charm.land/huh/v2"
	"github.com/pascualchavez/teleport/internal/config"
	sshpkg "github.com/pascualchavez/teleport/internal/ssh"
	"github.com/pascualchavez/teleport/internal/tui"
	"github.com/spf13/cobra"
)

var (
	actionNameFlag    string
	actionRunFlag     []string
	actionCwdFlag     string
	actionTimeoutFlag string
	actionConfirmFlag bool
	actionYesFlag     bool
)

var actionsCmd = &cobra.Command{
	Use:   "actions",
	Short: "󰑮 manage a profile's remote actions (deploy, restart, …)",
}

var actionsAddCmd = &cobra.Command{
	Use:   "add [profile]",
	Short: "add an action via an interactive wizard (or flags when headless)",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runActionsAdd,
}

var actionsListCmd = &cobra.Command{
	Use:   "list [profile]",
	Short: "list the profile's actions",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runActionsList,
}

var actionsEditCmd = &cobra.Command{
	Use:   "edit <name> [profile]",
	Short: "edit an existing action",
	Args:  cobra.RangeArgs(1, 2),
	RunE:  runActionsEdit,
}

var actionsRemoveCmd = &cobra.Command{
	Use:     "remove <name> [profile]",
	Aliases: []string{"rm"},
	Short:   "remove an action",
	Args:    cobra.RangeArgs(1, 2),
	RunE:    runActionsRemove,
}

func init() {
	for _, c := range []*cobra.Command{actionsAddCmd, actionsEditCmd} {
		c.Flags().StringArrayVar(&actionRunFlag, "run", nil, "a command step (repeatable); required headless")
		c.Flags().StringVar(&actionCwdFlag, "cwd", "", "working directory for the steps (empty string = no cd)")
		c.Flags().StringVar(&actionTimeoutFlag, "timeout", "", "per-step timeout, e.g. 5m (default 10m)")
		c.Flags().BoolVar(&actionConfirmFlag, "confirm", false, "prompt before running this action")
	}
	actionsAddCmd.Flags().StringVar(&actionNameFlag, "name", "", "the action's name; required headless")
	actionsRemoveCmd.Flags().BoolVarP(&actionYesFlag, "yes", "y", false, "skip the removal confirmation")
	actionsCmd.AddCommand(actionsAddCmd, actionsListCmd, actionsEditCmd, actionsRemoveCmd)
}

// resolveActionProfile resolves the profile from an optional trailing arg (or
// the local default), returning the loaded global config so callers can mutate
// and save it.
func resolveActionProfile(profileArg string) (*config.GlobalConfig, config.Profile, string, error) {
	globalCfg, err := config.LoadGlobal()
	if err != nil {
		return nil, config.Profile{}, "", fmt.Errorf("load global config: %w", err)
	}
	name := profileArg
	if name == "" {
		localCfg, err := config.LoadLocal()
		if err != nil {
			return nil, config.Profile{}, "", fmt.Errorf("load local config: %w", err)
		}
		name = localCfg.DefaultProfile
	}
	if name == "" {
		return nil, config.Profile{}, "", fmt.Errorf("no profile specified; run `teleport init` first or pass a profile name")
	}
	profile, ok := globalCfg.Profiles[name]
	if !ok {
		return nil, config.Profile{}, "", fmt.Errorf("profile %q not found; run `teleport init` to create it", name)
	}
	return globalCfg, profile, name, nil
}

func runActionsList(cmd *cobra.Command, args []string) error {
	var profileArg string
	if len(args) > 0 {
		profileArg = args[0]
	}
	_, profile, name, err := resolveActionProfile(profileArg)
	if err != nil {
		return err
	}
	return listActions(profile, name)
}

func runActionsRemove(cmd *cobra.Command, args []string) error {
	actionName := args[0]
	profileArg := ""
	if len(args) > 1 {
		profileArg = args[1]
	}
	globalCfg, profile, name, err := resolveActionProfile(profileArg)
	if err != nil {
		return err
	}
	if _, ok := profile.Actions[actionName]; !ok {
		return fmt.Errorf("action %q not found on profile %q; available: %s", actionName, name, availableActions(profile))
	}

	autoConfirm := actionYesFlag || noInput
	if !autoConfirm {
		if !interactive() {
			return errNeedsTTY("pass -y to remove the action")
		}
		ok, err := tui.RunActionConfirm(fmt.Sprintf("Remove action %q from %s?", actionName, name), profile.Actions[actionName].Run)
		if err != nil {
			return err
		}
		if !ok {
			fmt.Println("aborted, nothing removed")
			return nil
		}
	}

	globalCfg.RemoveAction(name, actionName)
	if err := config.SaveGlobal(globalCfg); err != nil {
		return fmt.Errorf("save global config: %w", err)
	}
	emit(map[string]string{"command": "actions.remove", "profile": name, "action": actionName}, func() {
		fmt.Printf("Removed action %q from %s.\n", actionName, name)
	})
	return nil
}

func runActionsAdd(cmd *cobra.Command, args []string) error {
	var profileArg string
	if len(args) > 0 {
		profileArg = args[0]
	}
	globalCfg, profile, name, err := resolveActionProfile(profileArg)
	if err != nil {
		return err
	}
	return addOrEditAction(cmd, globalCfg, profile, name, "", config.Action{})
}

func runActionsEdit(cmd *cobra.Command, args []string) error {
	actionName := args[0]
	profileArg := ""
	if len(args) > 1 {
		profileArg = args[1]
	}
	globalCfg, profile, name, err := resolveActionProfile(profileArg)
	if err != nil {
		return err
	}
	existing, ok := profile.Actions[actionName]
	if !ok {
		return fmt.Errorf("action %q not found on profile %q; available: %s", actionName, name, availableActions(profile))
	}
	return addOrEditAction(cmd, globalCfg, profile, name, actionName, existing)
}

// addOrEditAction drives the wizard (or the headless flag path) to build an
// action and persist it. When editName is non-empty the wizard is pre-filled
// for an edit; the name field is locked to editName.
func addOrEditAction(cmd *cobra.Command, globalCfg *config.GlobalConfig, profile config.Profile, profileName, editName string, existing config.Action) error {
	if !interactive() {
		return addActionHeadless(cmd, globalCfg, profile, profileName, editName, existing)
	}

	name := editName
	if name == "" {
		if err := runNameForm(&name, profile, editName); err != nil {
			return err
		}
	}

	steps := strings.Join(existing.Run, "\n")
	if err := runStepsForm(&steps); err != nil {
		return err
	}
	stepList := splitSteps(steps)
	if len(stepList) == 0 {
		return fmt.Errorf("an action needs at least one step")
	}

	cwd, client, err := runCwdForm(profile, existing)
	if err != nil {
		return err
	}
	if client != nil {
		defer client.Close()
	}

	confirm := existing.Confirm
	timeout := existing.Timeout
	if err := runOptionsForm(&confirm, &timeout); err != nil {
		return err
	}
	if timeout != "" {
		if _, err := time.ParseDuration(timeout); err != nil {
			return fmt.Errorf("invalid timeout %q: %w", timeout, err)
		}
	}

	action := config.Action{Run: stepList, Cwd: cwd, Timeout: timeout, Confirm: confirm}

	choice, err := runSummaryForm(name, profileName, action)
	if err != nil {
		return err
	}
	if choice == summaryCancel {
		fmt.Println("aborted, nothing saved")
		return nil
	}

	if choice == summaryTest {
		if client == nil {
			client, err = connectToProfile(profile)
			if err != nil {
				return err
			}
			defer client.Close()
		}
		// Temporarily attach the action so executeActions can run it.
		probe := profile
		probe.Actions = map[string]config.Action{name: action}
		if _, err := executeActions(client, probe, profileName, []string{name}, true); err != nil {
			fmt.Printf("\nAction failed: %v\nNot saved. Re-run `teleport actions add` to adjust.\n", err)
			return nil
		}
	}

	globalCfg.SetAction(profileName, name, action)
	if err := config.SaveGlobal(globalCfg); err != nil {
		return fmt.Errorf("save global config: %w", err)
	}
	fmt.Printf("\nAction %q saved on %s.\n", name, profileName)
	return nil
}

func addActionHeadless(cmd *cobra.Command, globalCfg *config.GlobalConfig, profile config.Profile, profileName, editName string, existing config.Action) error {
	name := editName
	if name == "" {
		name = strings.TrimSpace(actionNameFlag)
	}
	if name == "" {
		return errNeedsTTY("pass --name <action> to add an action headless")
	}
	if err := validateActionName(name); err != nil {
		return err
	}
	steps := actionRunFlag
	if len(steps) == 0 {
		steps = existing.Run
	}
	if len(steps) == 0 {
		return errNeedsTTY("pass --run <cmd> (repeatable) to define the action's steps")
	}

	action := config.Action{Run: steps, Cwd: existing.Cwd, Timeout: existing.Timeout, Confirm: existing.Confirm}
	if cmd.Flags().Changed("cwd") {
		cwd := actionCwdFlag
		action.Cwd = &cwd
	}
	if cmd.Flags().Changed("timeout") {
		if actionTimeoutFlag != "" {
			if _, err := time.ParseDuration(actionTimeoutFlag); err != nil {
				return fmt.Errorf("invalid timeout %q: %w", actionTimeoutFlag, err)
			}
		}
		action.Timeout = actionTimeoutFlag
	}
	if cmd.Flags().Changed("confirm") {
		action.Confirm = actionConfirmFlag
	}

	globalCfg.SetAction(profileName, name, action)
	if err := config.SaveGlobal(globalCfg); err != nil {
		return fmt.Errorf("save global config: %w", err)
	}
	emit(map[string]any{"command": "actions.add", "profile": profileName, "action": name, "steps": len(steps)}, func() {
		fmt.Printf("Action %q saved on %s.\n", name, profileName)
	})
	return nil
}

// validateActionName holds the naming rules shared by the wizard and the
// headless --name path: non-empty, no whitespace.
func validateActionName(s string) error {
	s = strings.TrimSpace(s)
	if s == "" {
		return fmt.Errorf("name cannot be empty")
	}
	if strings.ContainsAny(s, " \t") {
		return fmt.Errorf("name cannot contain spaces")
	}
	return nil
}

func runNameForm(name *string, profile config.Profile, editName string) error {
	form := huh.NewForm(huh.NewGroup(
		huh.NewInput().
			Title("Action name").
			Description("e.g. deploy, restart, logs").
			Value(name).
			Validate(func(s string) error {
				if err := validateActionName(s); err != nil {
					return err
				}
				s = strings.TrimSpace(s)
				if _, exists := profile.Actions[s]; exists && s != editName {
					return fmt.Errorf("action %q already exists", s)
				}
				return nil
			}),
	))
	if err := form.Run(); err != nil {
		return fmt.Errorf("form: %w", err)
	}
	*name = strings.TrimSpace(*name)
	return nil
}

func runStepsForm(steps *string) error {
	form := huh.NewForm(huh.NewGroup(
		huh.NewText().
			Title("Steps — one command per line").
			Description("Run in order; the first non-zero step aborts the action").
			Value(steps).
			Validate(func(s string) error {
				if len(splitSteps(s)) == 0 {
					return fmt.Errorf("at least one command is required")
				}
				return nil
			}),
	))
	if err := form.Run(); err != nil {
		return fmt.Errorf("form: %w", err)
	}
	return nil
}

const (
	cwdProfile = "profile"
	cwdBrowse  = "browse"
	cwdNone    = "none"
)

// runCwdForm resolves the working directory. It returns a pointer with the
// three-state semantics of Action.Cwd (nil = profile path, "" = no cd, value =
// dir) and, when the user browses, the still-open SSH client so a subsequent
// "test now" can reuse it.
func runCwdForm(profile config.Profile, existing config.Action) (*string, *sshpkg.Client, error) {
	choice := cwdProfile
	switch {
	case existing.Cwd == nil:
		choice = cwdProfile
	case *existing.Cwd == "":
		choice = cwdNone
	default:
		choice = cwdBrowse
	}

	form := huh.NewForm(huh.NewGroup(
		huh.NewSelect[string]().
			Title("Working directory").
			Options(
				huh.NewOption("Use the profile path ("+profile.Path+")", cwdProfile),
				huh.NewOption("Browse the remote for another directory", cwdBrowse),
				huh.NewOption("No directory (run commands as-is, e.g. systemctl)", cwdNone),
			).
			Value(&choice),
	))
	if err := form.Run(); err != nil {
		return nil, nil, fmt.Errorf("form: %w", err)
	}

	switch choice {
	case cwdProfile:
		return nil, nil, nil
	case cwdNone:
		empty := ""
		return &empty, nil, nil
	default:
		client, err := connectToProfile(profile)
		if err != nil {
			return nil, nil, err
		}
		start := profile.Path
		if start == "" {
			start = "/"
		}
		dir, err := tui.RunDirPickerWith(client, start, "  Select the action's working directory")
		if err != nil {
			client.Close()
			return nil, nil, err
		}
		return &dir, client, nil
	}
}

func runOptionsForm(confirm *bool, timeout *string) error {
	form := huh.NewForm(huh.NewGroup(
		huh.NewConfirm().
			Title("Confirm before running?").
			Description("Ask for confirmation each time this action runs").
			Value(confirm),
		huh.NewInput().
			Title("Per-step timeout (optional)").
			Description("Go duration like 5m or 30s; empty = 10m default").
			Placeholder("10m").
			Value(timeout).
			Validate(func(s string) error {
				s = strings.TrimSpace(s)
				if s == "" {
					return nil
				}
				if _, err := time.ParseDuration(s); err != nil {
					return fmt.Errorf("not a duration (e.g. 5m, 30s)")
				}
				return nil
			}),
	))
	if err := form.Run(); err != nil {
		return fmt.Errorf("form: %w", err)
	}
	*timeout = strings.TrimSpace(*timeout)
	return nil
}

const (
	summarySave   = "save"
	summaryTest   = "test"
	summaryCancel = "cancel"
)

func runSummaryForm(name, profileName string, action config.Action) (string, error) {
	cwdDesc := "profile path"
	if action.Cwd != nil {
		if *action.Cwd == "" {
			cwdDesc = "none"
		} else {
			cwdDesc = *action.Cwd
		}
	}
	fmt.Printf("\n  Action %q on %s\n", name, profileName)
	for i, s := range action.Run {
		fmt.Printf("    %d. %s\n", i+1, s)
	}
	fmt.Printf("  cwd: %s · timeout: %s · confirm: %v\n", cwdDesc, action.EffectiveTimeout(), action.Confirm)

	choice := summarySave
	form := huh.NewForm(huh.NewGroup(
		huh.NewSelect[string]().
			Title("Save this action?").
			Options(
				huh.NewOption("Save", summarySave),
				huh.NewOption("Test now, then save", summaryTest),
				huh.NewOption("Cancel", summaryCancel),
			).
			Value(&choice),
	))
	if err := form.Run(); err != nil {
		return "", fmt.Errorf("form: %w", err)
	}
	return choice, nil
}

func splitSteps(s string) []string {
	var out []string
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			out = append(out, line)
		}
	}
	return out
}
