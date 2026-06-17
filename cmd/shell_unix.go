//go:build !windows

package cmd

import (
	"os"
	"syscall"
)

// execSSH replaces the current process with ssh. On success it never returns,
// so no teleport process lingers while the session is open.
func execSSH(bin string, argv []string) error {
	return syscall.Exec(bin, argv, os.Environ())
}
