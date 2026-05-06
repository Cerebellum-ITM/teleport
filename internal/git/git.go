package git

import (
	"fmt"
	"os/exec"
	"strings"
)

func TrackedFiles() ([]string, error) {
	return runGit("ls-files")
}

func ChangedFiles() ([]string, error) {
	return runGit("diff", "--name-only", "HEAD")
}

func UntrackedFiles() ([]string, error) {
	return runGit("ls-files", "--others", "--exclude-standard")
}

func runGit(args ...string) ([]string, error) {
	out, err := exec.Command("git", args...).Output()
	if err != nil {
		return nil, fmt.Errorf("git %s: %w", strings.Join(args, " "), err)
	}

	raw := strings.TrimSpace(string(out))
	if raw == "" {
		return nil, nil
	}

	return strings.Split(raw, "\n"), nil
}
