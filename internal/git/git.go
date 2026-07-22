package git

import (
	"bytes"
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

var (
	ErrNoUpstream = errors.New("no upstream branch configured")
	ErrNoCommits  = errors.New("no commits ahead of remote")
)

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

func LocalBranches() (current string, all []string, err error) {
	lines, err := runGit("branch", "--format=%(refname:short)")
	if err != nil {
		return "", nil, err
	}

	currLines, err := runGit("rev-parse", "--abbrev-ref", "HEAD")
	if err != nil || len(currLines) == 0 {
		return "", nil, fmt.Errorf("get current branch: %w", err)
	}
	current = strings.TrimSpace(currLines[0])

	all = make([]string, 0, len(lines))
	all = append(all, current)
	for _, b := range lines {
		if b != current {
			all = append(all, b)
		}
	}
	return current, all, nil
}

func CommitsAheadOf(branch string) ([]Commit, error) {
	// Try to get the upstream for this branch.
	upstreamCmd := exec.Command("git", "rev-parse", "--abbrev-ref", branch+"@{u}")
	upstreamOut, err := upstreamCmd.Output()
	upstream := strings.TrimSpace(string(upstreamOut))
	hasUpstream := err == nil && upstream != ""

	var logArgs []string
	if hasUpstream {
		logArgs = []string{"log", upstream + ".." + branch, "--format=%H%x09%h%x09%s%x09%cr"}
	} else {
		logArgs = []string{"log", branch, "--not", "--remotes", "--format=%H%x09%h%x09%s%x09%cr"}
	}

	cmd := exec.Command("git", logArgs...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("git %s: %w (%s)", strings.Join(logArgs, " "), err, strings.TrimSpace(stderr.String()))
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

func CommitsAhead() ([]Commit, error) {
	_, branches, err := LocalBranches()
	if err != nil || len(branches) == 0 {
		return nil, err
	}
	return CommitsAheadOf(branches[0])
}

// FilesInCommits accepts shas in chronological order (oldest first) and
// returns the effective per-file change across them: the latest commit
// that touched each path wins. Renames split into delete (old) + add (new).
//
// It also returns commitPaths: for each sha, the list of paths that commit
// touched (renames split the same way). Unlike the deduped first return,
// commitPaths keeps every commit's own paths even when a later commit
// supersedes the same file — callers attributing work back to commits must
// use this, not the winning SHA on each FileChange.
func FilesInCommits(shas []string) ([]FileChange, map[string][]string, error) {
	byPath := make(map[string]FileChange)
	commitPaths := make(map[string][]string)

	for _, sha := range shas {
		cmd := exec.Command("git", "show", "--name-status", "--format=", sha)
		out, err := cmd.Output()
		if err != nil {
			return nil, nil, fmt.Errorf("git show %s: %w", sha, err)
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
				commitPaths[sha] = append(commitPaths[sha], old, new)
			case code == "A", code == "M", code == "D":
				byPath[fields[1]] = FileChange{Path: fields[1], Status: rune(code[0]), SHA: sha}
				commitPaths[sha] = append(commitPaths[sha], fields[1])
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
	return out, commitPaths, nil
}

// FileAtCommit returns the blob contents of path as of commit sha.
func FileAtCommit(sha, path string) ([]byte, error) {
	out, err := exec.Command("git", "show", sha+":"+path).Output()
	if err != nil {
		return nil, fmt.Errorf("git show %s:%s: %w", sha, path, err)
	}
	return out, nil
}

// FileBeforeCommit returns the blob contents of path as it was in the first
// parent of commit sha. Used to view a file that this commit deleted.
func FileBeforeCommit(sha, path string) ([]byte, error) {
	out, err := exec.Command("git", "show", sha+"^:"+path).Output()
	if err != nil {
		return nil, fmt.Errorf("git show %s^:%s: %w", sha, path, err)
	}
	return out, nil
}

// CommitDiff returns the full multi-file diff a commit introduced relative to
// its first parent (git show, no commit message/metadata).
func CommitDiff(sha string) ([]byte, error) {
	out, err := exec.Command("git", "show", "--format=", sha).Output()
	if err != nil {
		return nil, fmt.Errorf("git show %s: %w", sha, err)
	}
	return out, nil
}

// FileDiffAtCommit returns the unified diff that commit sha introduced for a
// single path (the change relative to its first parent).
func FileDiffAtCommit(sha, path string) ([]byte, error) {
	out, err := exec.Command("git", "show", "--format=", sha, "--", path).Output()
	if err != nil {
		return nil, fmt.Errorf("git show %s -- %s: %w", sha, path, err)
	}
	return out, nil
}

// HasUncommittedChanges reports whether the working tree has any
// staged or unstaged changes according to git status --porcelain.
func HasUncommittedChanges() (bool, error) {
	lines, err := runGit("status", "--porcelain")
	if err != nil {
		return false, err
	}
	return len(lines) > 0, nil
}

// LocalHEAD returns the full SHA of HEAD.
func LocalHEAD() (string, error) {
	lines, err := runGit("rev-parse", "HEAD")
	if err != nil || len(lines) == 0 {
		return "", fmt.Errorf("git rev-parse HEAD: %w", err)
	}
	return strings.TrimSpace(lines[0]), nil
}

// RevParse returns the full SHA that ref resolves to (branch name, tag, SHA…).
func RevParse(ref string) (string, error) {
	out, err := exec.Command("git", "rev-parse", ref).Output()
	if err != nil {
		return "", fmt.Errorf("git rev-parse %s: %w", ref, err)
	}
	return strings.TrimSpace(string(out)), nil
}

// IsAncestor reports whether maybeAncestor is an ancestor of (or equal to)
// descendant, via `git merge-base --is-ancestor` (exit 0 = yes, 1 = no).
func IsAncestor(maybeAncestor, descendant string) (bool, error) {
	err := exec.Command("git", "merge-base", "--is-ancestor", maybeAncestor, descendant).Run()
	if err == nil {
		return true, nil
	}
	var ee *exec.ExitError
	if errors.As(err, &ee) && ee.ExitCode() == 1 {
		return false, nil
	}
	return false, fmt.Errorf("git merge-base --is-ancestor: %w", err)
}

// MergeBase returns the best common ancestor of a and b, or "" when the
// histories are unrelated (git exits 1 with no output).
func MergeBase(a, b string) (string, error) {
	out, err := exec.Command("git", "merge-base", a, b).Output()
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) && ee.ExitCode() == 1 {
			return "", nil
		}
		return "", fmt.Errorf("git merge-base %s %s: %w", a, b, err)
	}
	return strings.TrimSpace(string(out)), nil
}

// CommitsBetween returns the commits in base..tip (reachable from tip but not
// from base), newest first. When base is "", tip's full history is returned.
func CommitsBetween(base, tip string) ([]Commit, error) {
	rangeArg := tip
	if base != "" {
		rangeArg = base + ".." + tip
	}
	cmd := exec.Command("git", "log", rangeArg, "--format=%H%x09%h%x09%s%x09%cr")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("git log %s: %w (%s)", rangeArg, err, strings.TrimSpace(stderr.String()))
	}
	return parseCommitLog(stdout.String()), nil
}

// parseCommitLog parses tab-separated `%H\t%h\t%s\t%cr` log lines.
func parseCommitLog(raw string) []Commit {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
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
	return commits
}

// BundleTo writes a git bundle at dst containing the commits in base..tip
// (tip inclusive). When base is "", tip's full history is bundled (for a
// diverged/unrelated or unborn remote under --force). tip is recorded under
// the temporary ref refs/teleport/mirror so the remote can fetch it by name;
// the ref is created and removed locally around the bundle write.
func BundleTo(base, tip, dst string) error {
	const ref = "refs/teleport/mirror"
	if err := exec.Command("git", "update-ref", ref, tip).Run(); err != nil {
		return fmt.Errorf("git update-ref %s %s: %w", ref, tip, err)
	}
	defer exec.Command("git", "update-ref", "-d", ref).Run()

	args := []string{"bundle", "create", dst}
	if base == "" {
		args = append(args, ref)
	} else {
		args = append(args, base+".."+ref)
	}
	cmd := exec.Command("git", args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("git bundle create: %w (%s)", err, strings.TrimSpace(stderr.String()))
	}
	return nil
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
