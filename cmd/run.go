package cmd

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/pascualchavez/teleport/internal/config"
	sshpkg "github.com/pascualchavez/teleport/internal/ssh"
	"github.com/pascualchavez/teleport/internal/tui"
	"github.com/spf13/cobra"
)

var (
	runList bool
	runYes  bool
)

var runCmd = &cobra.Command{
	Use:   "run <action>... [profile]",
	Short: "󰑮 run a profile's remote action(s), streaming their logs",
	Args:  cobra.ArbitraryArgs,
	RunE:  runRun,
}

func init() {
	runCmd.Flags().BoolVar(&runList, "list", false, "list the actions defined on the profile")
	runCmd.Flags().BoolVarP(&runYes, "yes", "y", false, "auto-confirm actions that require confirmation")
}

func runRun(cmd *cobra.Command, args []string) error {
	globalCfg, err := config.LoadGlobal()
	if err != nil {
		return fmt.Errorf("load global config: %w", err)
	}
	localCfg, err := config.LoadLocal()
	if err != nil {
		return fmt.Errorf("load local config: %w", err)
	}

	// Split args into action names and an optional trailing profile: the last
	// arg is treated as the profile only when it names an existing one.
	profileName := localCfg.DefaultProfile
	names := args
	if len(args) > 0 {
		if _, ok := globalCfg.Profiles[args[len(args)-1]]; ok {
			profileName = args[len(args)-1]
			names = args[:len(args)-1]
		}
	}
	if profileName == "" {
		return fmt.Errorf("no profile specified; run `teleport init` first or pass a profile name")
	}
	profile, ok := globalCfg.Profiles[profileName]
	if !ok {
		return fmt.Errorf("profile %q not found; run `teleport init` to create it", profileName)
	}

	if runList {
		return listActions(profile, profileName)
	}

	if len(names) == 0 {
		return fmt.Errorf("no action specified; run `teleport run --list` to see the profile's actions")
	}

	// Validate every requested action exists before connecting, listing the
	// available ones on a miss.
	for _, n := range names {
		if _, ok := profile.Actions[n]; !ok {
			return fmt.Errorf("action %q not found on profile %q; available: %s",
				n, profileName, availableActions(profile))
		}
	}

	// Fail closed before connecting when a confirm-required action can't be
	// confirmed headless.
	if err := precheckActionsConfirm(profile, names, runYes); err != nil {
		return err
	}

	client, err := connectToProfile(profile)
	if err != nil {
		return err
	}
	defer client.Close()

	results, execErr := executeActions(client, profile, profileName, names, runYes)

	res := runResult{Command: "run", Profile: profileName, Actions: results}
	emit(res, func() {})
	return execErr
}

// actionResult is the per-action --json shape, nested under `run` and under the
// transfer commands' results when actions run via --then.
type actionResult struct {
	Name       string `json:"name"`
	Steps      int    `json:"steps"`
	Completed  int    `json:"completed"`
	ExitCode   int    `json:"exit_code"`
	DurationMs int64  `json:"duration_ms"`
}

// runResult is the --json shape for `teleport run`.
type runResult struct {
	Command string         `json:"command"`
	Profile string         `json:"profile"`
	Actions []actionResult `json:"actions"`
}

// executeActions runs the named actions in order over an already-connected
// client, streaming each step's output per the useTUI() contract. An action
// with Confirm=true is gated: interactively it prompts; headless it requires
// autoConfirm (-y / --no-input) or fails with ErrNeedsTTY. Execution stops at
// the first action (and step) that fails or exits non-zero; the returned error
// is non-nil in that case. Names are assumed pre-validated by the caller.
func executeActions(client *sshpkg.Client, profile config.Profile, profileName string, names []string, autoConfirm bool) ([]actionResult, error) {
	var results []actionResult

	for _, name := range names {
		act := profile.Actions[name]

		if act.Confirm {
			switch {
			case interactive():
				ok, err := tui.RunActionConfirm(fmt.Sprintf("Run action %q on %s?", name, profileName), act.Run)
				if err != nil {
					return results, err
				}
				if !ok {
					fmt.Println("aborted, no action run")
					return results, nil
				}
			case !autoConfirm:
				return results, errNeedsTTY(fmt.Sprintf("pass -y to run %q (it requires confirmation)", name))
			}
		}

		result, err := runOneAction(client, profile, profileName, name, act)
		results = append(results, result)
		if err != nil {
			return results, err
		}
	}
	return results, nil
}

// runOneAction streams a single action's steps and returns its result plus the
// first failing step's error (nil when every step exited 0).
func runOneAction(client *sshpkg.Client, profile config.Profile, profileName, name string, act config.Action) (actionResult, error) {
	sink := tui.NewActionSink(name, !useTUI())
	sink.Header(fmt.Sprintf("action %s · %s:%s", name, profile.Host, profile.Path))

	cwd := act.EffectiveCwd(profile.Path)
	timeout := act.EffectiveTimeout()
	start := time.Now()
	result := actionResult{Name: name, Steps: len(act.Run)}

	for i, step := range act.Run {
		full := step
		if cwd != "" {
			full = "cd " + sshpkg.ShellQuote(cwd) + " && " + step
		}
		sink.StepStart(i+1, len(act.Run), step)

		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		stepStart := time.Now()
		code, err := client.RunCommandStream(ctx, full, func(src sshpkg.Source, line string) {
			sink.Line(src == sshpkg.SourceStderr, line)
		})
		cancel()
		sink.StepDone(err, code, time.Since(stepStart))

		if err != nil {
			result.ExitCode = -1
			result.DurationMs = time.Since(start).Milliseconds()
			return result, fmt.Errorf("action %q step %d (%s): %w", name, i+1, step, err)
		}
		if code != 0 {
			result.ExitCode = code
			result.DurationMs = time.Since(start).Milliseconds()
			return result, fmt.Errorf("action %q step %d (%s) exited with code %d", name, i+1, step, code)
		}
		result.Completed++
	}

	result.DurationMs = time.Since(start).Milliseconds()
	return result, nil
}

// listActions prints the profile's actions, honoring --json.
func listActions(profile config.Profile, profileName string) error {
	type actionInfo struct {
		Name    string `json:"name"`
		Steps   int    `json:"steps"`
		Cwd     string `json:"cwd"`
		Confirm bool   `json:"confirm"`
	}
	names := sortedActionNames(profile)
	infos := make([]actionInfo, 0, len(names))
	for _, n := range names {
		a := profile.Actions[n]
		infos = append(infos, actionInfo{
			Name:    n,
			Steps:   len(a.Run),
			Cwd:     a.EffectiveCwd(profile.Path),
			Confirm: a.Confirm,
		})
	}

	emit(infos, func() {
		if len(infos) == 0 {
			fmt.Printf("No actions on profile %q. Add one with `teleport actions add`.\n", profileName)
			return
		}
		fmt.Printf("Actions on %s:\n", profileName)
		for _, in := range infos {
			extra := ""
			if in.Confirm {
				extra = " (confirm)"
			}
			fmt.Printf("  %-16s %d step(s)  cwd=%s%s\n", in.Name, in.Steps, in.Cwd, extra)
		}
	})
	return nil
}

// thenActions holds the --then action names for sync/beam/mirror. Only one of
// those commands runs per invocation, so a shared var is safe.
var thenActions []string

// registerThenFlag adds the repeatable --then/-t flag to a transfer command.
func registerThenFlag(c *cobra.Command) {
	c.Flags().StringArrayVarP(&thenActions, "then", "t", nil, "run this action after a successful transfer (repeatable)")
}

// validateThenActions ensures every --then name exists on the profile, so a
// typo fails before the transfer starts rather than after uploading.
func validateThenActions(profile config.Profile, profileName string, names []string) error {
	for _, n := range names {
		if _, ok := profile.Actions[n]; !ok {
			return fmt.Errorf("action %q (--then) not found on profile %q; available: %s",
				n, profileName, availableActions(profile))
		}
	}
	return nil
}

// precheckActionsConfirm fails closed (ErrNeedsTTY) when running headless and
// any of the named actions requires confirmation that can't be auto-confirmed.
// Called before connecting so a headless run never opens a session it can't use.
func precheckActionsConfirm(profile config.Profile, names []string, autoConfirm bool) error {
	if interactive() || autoConfirm {
		return nil
	}
	for _, n := range names {
		if profile.Actions[n].Confirm {
			return errNeedsTTY(fmt.Sprintf("pass -y to run %q (it requires confirmation)", n))
		}
	}
	return nil
}

func availableActions(profile config.Profile) string {
	names := sortedActionNames(profile)
	if len(names) == 0 {
		return "(none)"
	}
	return strings.Join(names, ", ")
}

func sortedActionNames(profile config.Profile) []string {
	names := make([]string, 0, len(profile.Actions))
	for n := range profile.Actions {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}
