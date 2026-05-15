package git

import (
	"bytes"
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

var ErrNoUpstream = errors.New("no upstream branch configured")

type Commit struct {
	SHA     string
	Short   string
	Subject string
	RelDate string
}

type FileChange struct {
	Path   string
	Status rune
	SHA    string
}

func TrackedFiles() ([]string, error) {
	return runGit("ls-files")
}

func ChangedFiles() ([]string, error) {
	return runGit("diff", "--name-only", "HEAD")
}

func UntrackedFiles() ([]string, error) {
	return runGit("ls-files", "--others", "--exclude-standard")
}

func CommitsAhead() ([]Commit, error) {
	cmd := exec.Command("git", "log", "@{u}..HEAD", "--format=%H%x09%h%x09%s%x09%cr")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := stderr.String()
		if strings.Contains(msg, "no upstream") || strings.Contains(msg, "unknown revision") {
			return nil, ErrNoUpstream
		}
		return nil, fmt.Errorf("git log @{u}..HEAD: %w (%s)", err, strings.TrimSpace(msg))
	}

	raw := strings.TrimSpace(stdout.String())
	if raw == "" {
		return nil, nil
	}

	var commits []Commit
	for _, line := range strings.Split(raw, "\n") {
		parts := strings.SplitN(line, "\t", 4)
		if len(parts) != 4 {
			continue
		}
		commits = append(commits, Commit{
			SHA:     parts[0],
			Short:   parts[1],
			Subject: parts[2],
			RelDate: parts[3],
		})
	}
	return commits, nil
}

// FilesInCommits accepts shas in chronological order (oldest first) and
// returns the effective per-file change across them: the latest commit
// that touched each path wins. Renames split into delete (old) + add (new).
func FilesInCommits(shas []string) ([]FileChange, error) {
	byPath := make(map[string]FileChange)

	for _, sha := range shas {
		cmd := exec.Command("git", "show", "--name-status", "--format=", sha)
		out, err := cmd.Output()
		if err != nil {
			return nil, fmt.Errorf("git show %s: %w", sha, err)
		}
		for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
			if line == "" {
				continue
			}
			fields := strings.Split(line, "\t")
			if len(fields) < 2 {
				continue
			}
			code := fields[0]
			switch {
			case strings.HasPrefix(code, "R"):
				if len(fields) < 3 {
					continue
				}
				old, new := fields[1], fields[2]
				byPath[old] = FileChange{Path: old, Status: 'D', SHA: sha}
				byPath[new] = FileChange{Path: new, Status: 'A', SHA: sha}
			case code == "A", code == "M", code == "D":
				byPath[fields[1]] = FileChange{Path: fields[1], Status: rune(code[0]), SHA: sha}
			}
		}
	}

	out := make([]FileChange, 0, len(byPath))
	for _, fc := range byPath {
		out = append(out, fc)
	}
	// stable ordering by path
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j-1].Path > out[j].Path; j-- {
			out[j-1], out[j] = out[j], out[j-1]
		}
	}
	return out, nil
}

// FileAtCommit returns the blob contents of path as of commit sha.
func FileAtCommit(sha, path string) ([]byte, error) {
	out, err := exec.Command("git", "show", sha+":"+path).Output()
	if err != nil {
		return nil, fmt.Errorf("git show %s:%s: %w", sha, path, err)
	}
	return out, nil
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
