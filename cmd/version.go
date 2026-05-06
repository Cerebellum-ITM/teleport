package cmd

import (
	"fmt"

	"github.com/pascualchavez/teleport/internal/version"
	"github.com/spf13/cobra"
)

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: " print version information",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("teleport %s (commit %s, built %s)\n", version.Version, version.Commit, version.Date)
	},
}
