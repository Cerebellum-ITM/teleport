//go:build windows

package cmd

import (
	"os"
	"os/exec"
)

// execSSH runs ssh as a child with inherited stdio and exits with its code.
// syscall.Exec is unavailable on Windows; this keeps the package compiling.
func execSSH(bin string, argv []string) error {
	c := exec.Command(bin, argv[1:]...)
	c.Stdin, c.Stdout, c.Stderr = os.Stdin, os.Stdout, os.Stderr
	if err := c.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			os.Exit(exitErr.ExitCode())
		}
		return err
	}
	os.Exit(0)
	return nil
}
