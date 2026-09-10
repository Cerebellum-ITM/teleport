package cmd

import (
	"strings"
	"testing"
)

func TestRemoteShellCommand(t *testing.T) {
	cmd := remoteShellCommand("/srv/app one")
	if !strings.HasPrefix(cmd, "cd '/srv/app one' || exit 1; ") {
		t.Fatalf("remoteShellCommand did not cd into the quoted path: %q", cmd)
	}
	for _, sh := range shellCandidates {
		if !strings.Contains(cmd, "command -v "+sh+" >/dev/null 2>&1 && exec "+sh+";") {
			t.Fatalf("remoteShellCommand does not probe %q: %q", sh, cmd)
		}
	}
	if !strings.HasSuffix(cmd, "exit 127") {
		t.Fatalf("remoteShellCommand does not fail when no shell exists: %q", cmd)
	}
}

func TestRemoteShellCommandWithoutPath(t *testing.T) {
	cmd := remoteShellCommand("")
	if strings.Contains(cmd, "cd ") {
		t.Fatalf("remoteShellCommand(\"\") should not cd: %q", cmd)
	}
	if !strings.HasPrefix(cmd, "command -v "+shellCandidates[0]) {
		t.Fatalf("remoteShellCommand(\"\") should start with the first candidate: %q", cmd)
	}
}
