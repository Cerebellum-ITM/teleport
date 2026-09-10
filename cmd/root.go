package cmd

import (
	"errors"
	"fmt"
	"os"

	"github.com/charmbracelet/log"
	"github.com/spf13/cobra"
)

var verbose bool
var rootSync bool
var rootInit bool
var rootProfiles bool
var rootBeam bool
var rootShell bool
var noInput bool
var jsonOut bool
var quietOut bool

var rootCmd = &cobra.Command{
	Use:   "teleport",
	Short: " Sync git-tracked files to a remote server via SFTP",
	Long: `teleport syncs files tracked by git (and optional extras) to a
remote server over SSH/SFTP before committing — keeping your git
history clean of post-deploy fix commits.`,
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		if verbose {
			log.SetLevel(log.DebugLevel)
		}
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		switch {
		case rootSync:
			return runSync(cmd, args)
		case rootInit:
			return runInit(cmd, args)
		case rootProfiles:
			return runProfiles(cmd, args)
		case rootBeam:
			return runBeam(cmd, args)
		case rootShell:
			return runShell(cmd, args)
		default:
			printHelp()
			return nil
		}
	},
}

func Execute() {
	rootCmd.SetHelpFunc(func(cmd *cobra.Command, args []string) {
		if cmd == rootCmd {
			printHelp()
			return
		}
		cmd.Usage()
	})
	// We report errors ourselves so --json can serialize ErrNeedsTTY and so
	// exit codes follow the documented contract (2 = would need a TTY).
	rootCmd.SilenceErrors = true
	rootCmd.SilenceUsage = true
	if err := rootCmd.Execute(); err != nil {
		os.Exit(exitCode(err))
	}
}

// exitCode maps an error to the documented exit contract and prints it:
// 2 when a command would need a TTY it does not have, 1 otherwise.
func exitCode(err error) int {
	if errors.Is(err, ErrNeedsTTY) {
		reportError(err)
		if !jsonOut {
			fmt.Fprintln(os.Stderr, "Error:", err)
		}
		return 2
	}
	fmt.Fprintln(os.Stderr, "Error:", err)
	return 1
}

func init() {
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "verbose output")
	rootCmd.PersistentFlags().BoolVar(&noInput, "no-input", false, "never prompt; resolve from flags or fail (exit 2)")
	rootCmd.PersistentFlags().BoolVar(&jsonOut, "json", false, "print a single JSON result object to stdout")
	rootCmd.PersistentFlags().BoolVarP(&quietOut, "quiet", "q", false, "print only the final result and errors")
	rootCmd.Flags().BoolVarP(&rootSync, "sync", "s", false, " sync changed files")
	rootCmd.Flags().BoolVarP(&includeUntracked, "untracked", "u", false, " include untracked files (use with -s)")
	rootCmd.Flags().BoolVarP(&rootInit, "init", "i", false, " configure a sync profile")
	rootCmd.Flags().BoolVarP(&rootProfiles, "profiles", "p", false, " list configured profiles")
	rootCmd.Flags().BoolVarP(&rootBeam, "beam", "b", false, "󰜘 send selected local commits to the remote server")
	rootCmd.Flags().BoolVarP(&beamAuto, "auto", "a", false, "with -b: auto-select commits not yet sent, skip the commit picker")
	rootCmd.Flags().BoolVar(&rootShell, "sh", false, " open an interactive shell on the remote")
	rootCmd.AddCommand(initCmd)
	rootCmd.AddCommand(syncCmd)
	rootCmd.AddCommand(profilesCmd)
	rootCmd.AddCommand(versionCmd)
	rootCmd.AddCommand(configCmd)
	rootCmd.AddCommand(beamCmd)
	rootCmd.AddCommand(statusCmd)
	rootCmd.AddCommand(cleanCmd)
	rootCmd.AddCommand(mirrorCmd)
	rootCmd.AddCommand(pushCmd)
	rootCmd.AddCommand(runCmd)
	rootCmd.AddCommand(actionsCmd)
}
