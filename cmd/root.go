package cmd

import (
	"os"

	"github.com/charmbracelet/log"
	"github.com/spf13/cobra"
)

var verbose bool
var rootSync bool
var rootInit bool
var rootProfiles bool

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
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func init() {
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "verbose output")
	rootCmd.Flags().BoolVarP(&rootSync, "sync", "s", false, " sync changed files")
	rootCmd.Flags().BoolVarP(&includeUntracked, "untracked", "u", false, " include untracked files (use with -s)")
	rootCmd.Flags().BoolVarP(&rootInit, "init", "i", false, " configure a sync profile")
	rootCmd.Flags().BoolVarP(&rootProfiles, "profiles", "p", false, " list configured profiles")
	rootCmd.AddCommand(initCmd)
	rootCmd.AddCommand(syncCmd)
	rootCmd.AddCommand(profilesCmd)
	rootCmd.AddCommand(versionCmd)
	rootCmd.AddCommand(configCmd)
	rootCmd.AddCommand(beamCmd)
	rootCmd.AddCommand(statusCmd)
}
