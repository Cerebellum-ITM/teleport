package cmd

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/pascualchavez/teleport/internal/config"
	"github.com/pascualchavez/teleport/internal/tui"
	"github.com/spf13/cobra"
)

var (
	pushTo       string
	pushDryRun   bool
	pushChecksum bool
	pushYes      bool
)

var pushCmd = &cobra.Command{
	Use:   "push <local-path>... [profile]",
	Short: "󰅢 upload paths as-is, even the ones git ignores",
	Args:  cobra.MinimumNArgs(1),
	RunE:  runPush,
}

func init() {
	pushCmd.Flags().StringVar(&pushTo, "to", "", "destination inside the profile path (relative; never absolute)")
	pushCmd.Flags().BoolVar(&pushDryRun, "dry-run", false, "list what would be uploaded without connecting")
	pushCmd.Flags().BoolVar(&pushChecksum, "checksum", false, "verify each uploaded file with a SHA256 comparison")
	pushCmd.Flags().BoolVarP(&pushYes, "yes", "y", false, "auto-confirm actions that require confirmation")
	registerThenFlag(pushCmd)
}

// pushItem is one file to upload, with its resolved remote destination.
type pushItem struct {
	Local  string
	Remote string
	Size   int64
	Mode   os.FileMode
}

// pushFile is the per-file --json shape.
type pushFile struct {
	Local  string `json:"local"`
	Remote string `json:"remote"`
	Bytes  int64  `json:"bytes"`
}

// pushResult is the --json shape for push.
type pushResult struct {
	Command  string         `json:"command"`
	Target   string         `json:"target"`
	DryRun   bool           `json:"dry_run,omitempty"`
	Sent     int            `json:"sent"`
	Bytes    int64          `json:"bytes"`
	Verified string         `json:"verified"`
	Files    []pushFile     `json:"files"`
	Actions  []actionResult `json:"actions,omitempty"`
}

func runPush(_ *cobra.Command, args []string) error {
	profile, profileName, locals, err := resolvePushProfile(args)
	if err != nil {
		return err
	}

	items, dirs, err := collectPushItems(locals, profile.Path, pushTo)
	if err != nil {
		return err
	}
	if len(items) == 0 && len(dirs) == 0 {
		return fmt.Errorf("nothing to push: %s expanded to no files", strings.Join(locals, ", "))
	}

	var total int64
	files := make([]pushFile, 0, len(items))
	byLocal := make(map[string]pushItem, len(items))
	paths := make([]string, 0, len(items))
	for _, it := range items {
		total += it.Size
		files = append(files, pushFile{Local: it.Local, Remote: it.Remote, Bytes: it.Size})
		byLocal[it.Local] = it
		paths = append(paths, it.Local)
	}

	// Validate --then actions before uploading so a typo (or an un-confirmable
	// confirm-action headless) fails fast.
	if err := validateThenActions(profile, profileName, thenActions); err != nil {
		return err
	}
	if err := precheckActionsConfirm(profile, thenActions, pushYes || noInput); err != nil {
		return err
	}

	target := fmt.Sprintf("%s:%s", profile.Host, profile.Path)

	if pushDryRun {
		res := pushResult{
			Command: "push",
			Target:  target,
			DryRun:  true,
			Bytes:   total,
			Files:   files,
		}
		emit(res, func() { printPushPlan(target, dirs, items, total) })
		return nil
	}

	client, err := connectToProfile(profile)
	if err != nil {
		return err
	}
	defer client.Close()

	for _, dir := range dirs {
		if err := client.MkdirAll(dir); err != nil {
			return err
		}
	}

	verified := "size"
	if pushChecksum {
		verified = "checksum"
	}

	header := fmt.Sprintf("Pushing %d file(s) · %s → %s", len(items), tui.HumanBytes(total), target)
	upload := func(local string) error {
		it := byLocal[local]
		if err := client.UploadFileProgress(it.Local, it.Remote, nil); err != nil {
			return err
		}
		if err := client.Chmod(it.Remote, it.Mode); err != nil {
			return err
		}
		if !pushChecksum {
			return nil
		}
		want, err := localSHA256(it.Local)
		if err != nil {
			return err
		}
		got, err := client.RemoteSHA256(it.Remote)
		if err != nil {
			return fmt.Errorf("checksum %s: %w", it.Remote, err)
		}
		if got != want {
			return fmt.Errorf("checksum mismatch for %s (local %s, remote %s)", it.Remote, want, got)
		}
		return nil
	}

	var failed []string
	if useTUI() {
		failed, err = tui.RunSyncProgress(header, paths, upload)
	} else {
		failed, err = tui.RunSyncPlain(header, paths, upload)
	}
	if err != nil {
		return err
	}
	if len(failed) > 0 {
		return fmt.Errorf("%d file(s) failed to upload", len(failed))
	}

	actions, actErr := executeActions(client, profile, profileName, thenActions, pushYes || noInput)

	res := pushResult{
		Command:  "push",
		Target:   target,
		Sent:     len(items),
		Bytes:    total,
		Verified: verified,
		Files:    files,
		Actions:  actions,
	}
	emit(res, func() {
		fmt.Printf("  pushed %d file(s) · %s · verified (%s)\n", len(items), tui.HumanBytes(total), verified)
	})
	return actErr
}

// printPushPlan renders the --dry-run plan. It goes through emit's human branch,
// so under --json nothing of this reaches stdout.
func printPushPlan(target string, dirs []string, items []pushItem, total int64) {
	fmt.Printf("Would push %d file(s) · %s → %s (dry run)\n", len(items), tui.HumanBytes(total), target)
	for _, d := range dirs {
		fmt.Printf("  mkdir  %s\n", d)
	}
	for _, it := range items {
		fmt.Printf("  %s → %s  (%s)\n", it.Local, it.Remote, tui.HumanBytes(it.Size))
	}
}

// resolvePushProfile splits args into local paths and an optional trailing
// profile name, then resolves the profile. The last arg is treated as a profile
// only when it names an existing one AND does not exist locally — a directory
// named like a profile must stay a source path.
func resolvePushProfile(args []string) (config.Profile, string, []string, error) {
	localCfg, err := config.LoadLocal()
	if err != nil {
		return config.Profile{}, "", nil, fmt.Errorf("load local config: %w", err)
	}
	globalCfg, err := config.LoadGlobal()
	if err != nil {
		return config.Profile{}, "", nil, fmt.Errorf("load global config: %w", err)
	}

	name := localCfg.DefaultProfile
	locals := args
	if len(args) >= 2 {
		last := args[len(args)-1]
		if _, ok := globalCfg.Profiles[last]; ok {
			if _, statErr := os.Lstat(last); statErr != nil {
				name = last
				locals = args[:len(args)-1]
			}
		}
	}
	if name == "" {
		return config.Profile{}, "", nil, fmt.Errorf("no profile specified; run `teleport init` first or pass a profile name")
	}
	profile, ok := globalCfg.Profiles[name]
	if !ok {
		return config.Profile{}, "", nil, fmt.Errorf("profile %q not found; run `teleport init` to create it", name)
	}
	return profile, name, locals, nil
}

// resolvePushDest maps one local path to its absolute remote destination.
// base is the profile path; to is the raw --to value ("" mirrors the local path
// relative to the current directory); single reports whether exactly one local
// path was given, which makes --to the exact destination rather than a parent
// directory. The result is always inside base — anything that escapes is an
// error, so push can never write outside the profile.
func resolvePushDest(base, localPath, to string, single bool) (string, error) {
	cleanBase := path.Clean(base)

	var rel string
	switch {
	case to == "":
		r, err := localRelPath(localPath)
		if err != nil {
			return "", err
		}
		rel = r
	case filepath.IsAbs(to) || strings.HasPrefix(to, "/"):
		return "", fmt.Errorf("--to %q must be relative to the profile path %s, not absolute", to, cleanBase)
	case single:
		rel = to
	default:
		rel = path.Join(to, filepath.Base(localPath))
	}

	dest := path.Clean(path.Join(cleanBase, filepath.ToSlash(rel)))
	if dest != cleanBase && !strings.HasPrefix(dest, cleanBase+"/") {
		return "", fmt.Errorf("destination %s escapes the profile path %s", dest, cleanBase)
	}
	return dest, nil
}

// localRelPath expresses localPath relative to the current directory, which is
// the reference point for the default destination. A path outside it has no
// mirror position on the remote, so it requires --to.
func localRelPath(localPath string) (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("get working directory: %w", err)
	}
	abs, err := filepath.Abs(localPath)
	if err != nil {
		return "", fmt.Errorf("resolve %s: %w", localPath, err)
	}
	rel, err := filepath.Rel(cwd, abs)
	if err != nil {
		return "", fmt.Errorf("resolve %s: %w", localPath, err)
	}
	rel = filepath.ToSlash(rel)
	if rel == ".." || strings.HasPrefix(rel, "../") {
		return "", fmt.Errorf("%s is outside the current directory; pass --to <remote-subpath> to place it", localPath)
	}
	return rel, nil
}

// collectPushItems walks every local path and resolves each file's remote
// destination. Symlinks are followed; a missing path, broken symlink, symlink
// cycle or unsupported file type is a loud error. Nothing is uploaded until this
// returns cleanly, so those failures abort with an untouched remote.
func collectPushItems(locals []string, base, to string) ([]pushItem, []string, error) {
	single := len(locals) == 1
	var items []pushItem
	dirSet := make(map[string]struct{})

	for _, local := range locals {
		dest, err := resolvePushDest(base, local, to, single)
		if err != nil {
			return nil, nil, err
		}

		info, err := os.Stat(local)
		if err != nil {
			return nil, nil, fmt.Errorf("push %s: %w", local, err)
		}

		switch {
		case info.Mode().IsRegular():
			items = append(items, pushItem{
				Local:  filepath.ToSlash(local),
				Remote: dest,
				Size:   info.Size(),
				Mode:   pushMode(info.Mode()),
			})
		case info.IsDir():
			dirSet[dest] = struct{}{}
			seen := make(map[string]struct{})
			sub, err := walkPushDir(local, dest, seen, dirSet)
			if err != nil {
				return nil, nil, err
			}
			items = append(items, sub...)
		default:
			return nil, nil, fmt.Errorf("push %s: unsupported file type %s", local, info.Mode().Type())
		}
	}

	dirs := make([]string, 0, len(dirSet))
	for d := range dirSet {
		dirs = append(dirs, d)
	}
	// Shortest first so parents are created before their children.
	sort.Slice(dirs, func(i, j int) bool { return len(dirs[i]) < len(dirs[j]) })
	return items, dirs, nil
}

// walkPushDir recurses into localDir, following symlinks. seen holds the
// canonical paths of directories already visited on this branch so a symlink
// cycle fails loudly instead of looping forever.
func walkPushDir(localDir, remoteDir string, seen map[string]struct{}, dirSet map[string]struct{}) ([]pushItem, error) {
	real, err := filepath.EvalSymlinks(localDir)
	if err != nil {
		return nil, fmt.Errorf("push %s: %w", localDir, err)
	}
	if _, dup := seen[real]; dup {
		return nil, fmt.Errorf("push %s: symlink cycle through %s", localDir, real)
	}
	seen[real] = struct{}{}
	// Popped on unwind so the guard is per-branch: two sibling symlinks to the
	// same directory are a duplicate, not a cycle, and must both upload.
	defer delete(seen, real)

	entries, err := os.ReadDir(localDir)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", localDir, err)
	}

	var items []pushItem
	for _, e := range entries {
		localPath := filepath.Join(localDir, e.Name())
		remotePath := path.Join(remoteDir, e.Name())

		info, err := os.Stat(localPath)
		if err != nil {
			return nil, fmt.Errorf("push %s: %w", localPath, err)
		}

		switch {
		case info.Mode().IsRegular():
			items = append(items, pushItem{
				Local:  filepath.ToSlash(localPath),
				Remote: remotePath,
				Size:   info.Size(),
				Mode:   pushMode(info.Mode()),
			})
		case info.IsDir():
			dirSet[remotePath] = struct{}{}
			sub, err := walkPushDir(localPath, remotePath, seen, dirSet)
			if err != nil {
				return nil, err
			}
			items = append(items, sub...)
		default:
			return nil, fmt.Errorf("push %s: unsupported file type %s", localPath, info.Mode().Type())
		}
	}
	return items, nil
}

// pushMode preserves the execution bit and nothing else: local umask artifacts
// (and meaningless Windows modes) must not leak onto the remote, and making
// everything executable is ship's job, not push's.
func pushMode(m os.FileMode) os.FileMode {
	if m.Perm()&0o111 != 0 {
		return 0o755
	}
	return 0o644
}
