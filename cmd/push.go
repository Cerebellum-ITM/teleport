package cmd

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"

	lipgloss "charm.land/lipgloss/v2"
	"github.com/charmbracelet/log"
	"github.com/pascualchavez/teleport/internal/config"
	sshpkg "github.com/pascualchavez/teleport/internal/ssh"
	"github.com/pascualchavez/teleport/internal/theme"
	"github.com/pascualchavez/teleport/internal/tui"
	"github.com/spf13/cobra"
)

var (
	pushTo        string
	pushDryRun    bool
	pushChecksum  bool
	pushExclude   []string
	pushNoExclude bool
	pushForce     bool
	pushYes       bool
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
	pushCmd.Flags().BoolVar(&pushChecksum, "checksum", false, "verify each uploaded file with a SHA256 comparison, and hash instead of trusting the skip cache")
	pushCmd.Flags().BoolVarP(&pushForce, "force", "f", false, "upload every file, even the ones the remote already has")
	pushCmd.Flags().StringArrayVarP(&pushExclude, "exclude", "x", nil, "skip paths matching this glob (repeatable)")
	pushCmd.Flags().BoolVar(&pushNoExclude, "no-exclude", false, "ignore every exclude, this project's list and .git included")
	pushCmd.Flags().BoolVarP(&pushYes, "yes", "y", false, "auto-confirm actions that require confirmation")
	registerThenFlag(pushCmd)
}

// pushDefaultExcludes are skipped by every push. `.git` is here because sending
// it is never the intent when pushing a project directory, and `--no-exclude`
// brings it back for the rare case that wants it (seeding a bare remote repo).
var pushDefaultExcludes = []string{".git"}

// pushItem is one file to upload, with its resolved remote destination.
type pushItem struct {
	Local  string
	Remote string
	Size   int64
	Mtime  int64
	Mode   os.FileMode
}

// pushFile is the per-file --json shape.
type pushFile struct {
	Local   string `json:"local"`
	Remote  string `json:"remote"`
	Bytes   int64  `json:"bytes"`
	Skipped bool   `json:"skipped,omitempty"`
}

// pushResult is the --json shape for push.
type pushResult struct {
	Command  string             `json:"command"`
	Target   string             `json:"target"`
	DryRun   bool               `json:"dry_run,omitempty"`
	Sent     int                `json:"sent"`
	Skipped  int                `json:"skipped"`
	Excluded int                `json:"excluded"`
	Bytes    int64              `json:"bytes"`
	Verified string             `json:"verified"`
	Phases   map[string]float64 `json:"phases,omitempty"`
	Files    []pushFile         `json:"files"`
	Actions  []actionResult     `json:"actions,omitempty"`
}

func runPush(_ *cobra.Command, args []string) error {
	profile, profileName, locals, err := resolvePushProfile(args)
	if err != nil {
		return err
	}

	steps := newStepLog("push")

	steps.Start("scan", strings.Join(locals, ", "))
	excludes, err := effectivePushExcludes()
	if err != nil {
		steps.Fail()
		return err
	}

	items, dirs, excluded, err := collectPushItems(locals, profile.Path, pushTo, excludes)
	if err != nil {
		steps.Fail()
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

	scanned := fmt.Sprintf("%d file(s) · %s", len(items), tui.HumanBytes(total))
	if excluded > 0 {
		scanned += fmt.Sprintf(" · %d excluded", excluded)
	}
	steps.Done(scanned)

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
			Command:  "push",
			Target:   target,
			DryRun:   true,
			Excluded: excluded,
			Bytes:    total,
			Phases:   steps.Durations(),
			Files:    files,
		}
		emit(res, func() { printPushPlan(target, dirs, items, total, excluded) })
		return nil
	}

	steps.Start("connect", target)
	client, err := connectToProfile(profile)
	if err != nil {
		steps.Fail()
		return err
	}
	defer client.Close()

	for _, dir := range dirs {
		if err := client.MkdirAll(dir); err != nil {
			steps.Fail()
			return err
		}
	}
	steps.Done(fmt.Sprintf("%d dir(s) ready", len(dirs)))

	if !pushForce {
		steps.Start("compare", fmt.Sprintf("%d file(s)", len(items)))
		unchanged, cache, detail, err := planPushSkips(client, items, profileName, steps)
		if err != nil {
			steps.Fail()
			return err
		}
		steps.Done(detail)
		if err := config.SavePushCache(profileName, cache); err != nil {
			log.Warn("could not save the push cache", "err", err)
		}
		kept := make([]string, 0, len(paths))
		total = 0
		for i, f := range files {
			if unchanged[f.Local] {
				files[i].Skipped = true
				continue
			}
			kept = append(kept, f.Local)
			total += f.Bytes
		}
		paths = kept
		printPushSkipped(files)
	}

	verified := "size"
	if pushChecksum {
		verified = "checksum"
	}

	skipped := len(items) - len(paths)
	if len(paths) == 0 {
		emit(pushResult{
			Command:  "push",
			Target:   target,
			Skipped:  skipped,
			Excluded: excluded,
			Verified: verified,
			Phases:   steps.Durations(),
			Files:    files,
		}, func() {
			fmt.Printf("  %s nothing to upload · %d file(s) already match %s  %s\n",
				okMark, skipped, target, elapsedStyle.Render(steps.Elapsed().Round(time.Millisecond).String()))
		})
		_, actErr := executeActions(client, profile, profileName, thenActions, pushYes || noInput)
		return actErr
	}

	steps.Start("upload", fmt.Sprintf("%d file(s) · %s", len(paths), tui.HumanBytes(total)))
	steps.Detach()

	header := fmt.Sprintf("Pushing %d file(s) · %s → %s", len(paths), tui.HumanBytes(total), target)
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
		steps.Fail()
		return fmt.Errorf("%d file(s) failed to upload", len(failed))
	}
	steps.Done("")

	actions, actErr := executeActions(client, profile, profileName, thenActions, pushYes || noInput)

	res := pushResult{
		Command:  "push",
		Target:   target,
		Sent:     len(paths),
		Skipped:  skipped,
		Excluded: excluded,
		Bytes:    total,
		Verified: verified,
		Phases:   steps.Durations(),
		Files:    files,
		Actions:  actions,
	}
	emit(res, func() {
		line := fmt.Sprintf("  %s pushed %d file(s) · %s · verified (%s)", okMark, len(paths), tui.HumanBytes(total), verified)
		if skipped > 0 {
			line += fmt.Sprintf(" · skipped %d unchanged", skipped)
		}
		if excluded > 0 {
			line += fmt.Sprintf(" · excluded %d", excluded)
		}
		line += elapsedStyle.Render("  " + steps.Elapsed().Round(time.Millisecond).String())
		fmt.Println(line)
	})
	return actErr
}

// effectivePushExcludes unions the built-in defaults, this project's saved list
// and the --exclude flags; --no-exclude drops all three. A malformed glob is an
// error rather than a pattern that silently matches nothing.
func effectivePushExcludes() ([]string, error) {
	if pushNoExclude {
		return nil, nil
	}
	cfg, err := config.LoadLocal()
	if err != nil {
		return nil, fmt.Errorf("load local config: %w", err)
	}

	patterns := make([]string, 0, len(pushDefaultExcludes)+len(cfg.PushExclude)+len(pushExclude))
	patterns = append(patterns, pushDefaultExcludes...)
	patterns = append(patterns, cfg.PushExclude...)
	patterns = append(patterns, pushExclude...)
	for _, p := range patterns {
		if err := validateExcludePattern(p); err != nil {
			return nil, err
		}
	}
	return patterns, nil
}

// validateExcludePattern rejects a glob path.Match cannot parse.
func validateExcludePattern(pattern string) error {
	if strings.TrimSpace(pattern) == "" {
		return fmt.Errorf("empty exclude pattern")
	}
	if _, err := path.Match(pattern, "x"); err != nil {
		return fmt.Errorf("exclude %q: %w", pattern, err)
	}
	return nil
}

// pushExcluded reports whether localPath matches any pattern. A pattern without a
// slash is matched against the base name, so it applies at any depth; one holding
// a slash is matched against the path as it was named on the command line, which
// is also how push prints it.
func pushExcluded(localPath string, patterns []string) bool {
	localPath = filepath.ToSlash(localPath)
	base := path.Base(localPath)
	for _, pattern := range patterns {
		pattern = strings.TrimSuffix(pattern, "/")
		target := base
		if strings.Contains(pattern, "/") {
			target = localPath
		}
		if ok, err := path.Match(pattern, target); err == nil && ok {
			return true
		}
	}
	return false
}

// planPushSkips decides which items the remote already holds, asking the cheapest
// question that can settle each one. A remote stat comes first: a missing file or a
// different size needs no hash at all. For the rest, a cache entry whose recorded
// stats still match both ends proves the file is unchanged without reading a byte.
// Only what is still ambiguous is hashed — on the server for the remote side, so
// the contents never cross the network. The returned cache carries the new
// findings and is for the caller to persist.
func planPushSkips(client *sshpkg.Client, items []pushItem, profileName string, steps *tui.StepLog) (map[string]bool, *config.PushCache, string, error) {
	remotes := make([]string, 0, len(items))
	for _, it := range items {
		remotes = append(remotes, it.Remote)
	}
	stats, err := client.RemoteStatMany(remotes, func(done, total int) {
		steps.Update(fmt.Sprintf("stat %d/%d", done, total))
	})
	if err != nil {
		return nil, nil, "", err
	}

	cache := config.LoadPushCache(profileName)
	unchanged := make(map[string]bool, len(items))
	hits := 0
	var ambiguous []pushItem

	for _, it := range items {
		st, ok := stats[it.Remote]
		if !ok {
			log.Debug("sending: not on the remote", "file", it.Local, "remote", it.Remote)
			continue
		}
		if st.Size != it.Size {
			log.Debug("sending: size differs", "file", it.Local, "local", it.Size, "remote", st.Size)
			continue
		}

		entry, cached := cache.Entries[it.Remote]
		fresh := cached &&
			entry.LocalSize == it.Size && entry.LocalMtime == it.Mtime &&
			entry.RemoteSize == st.Size && entry.RemoteMtime == st.ModTime
		if fresh && !pushChecksum {
			log.Debug("skipping: cache hit", "file", it.Local)
			unchanged[it.Local] = true
			hits++
			continue
		}
		ambiguous = append(ambiguous, it)
	}

	detail := fmt.Sprintf("stat %d · cache %d · hash %d", len(stats), hits, len(ambiguous))
	if len(ambiguous) == 0 {
		return unchanged, cache, detail, nil
	}

	toHash := make([]string, 0, len(ambiguous))
	for _, it := range ambiguous {
		toHash = append(toHash, it.Remote)
	}
	remoteHashes, err := client.RemoteSHA256Many(toHash, func(done, total int) {
		steps.Update(fmt.Sprintf("hashing %d/%d", done, total))
	})
	if err != nil {
		return nil, nil, "", err
	}

	for _, it := range ambiguous {
		remoteHash, ok := remoteHashes[it.Remote]
		if !ok {
			log.Debug("sending: not on the remote", "file", it.Local, "remote", it.Remote)
			continue
		}
		localHash, err := localSHA256(it.Local)
		if err != nil {
			log.Debug("sending: local hash failed", "file", it.Local, "err", err)
			continue
		}
		if localHash != remoteHash {
			log.Debug("sending: content differs", "file", it.Local, "local", localHash, "remote", remoteHash)
			continue
		}

		unchanged[it.Local] = true
		st := stats[it.Remote]
		cache.Entries[it.Remote] = config.PushCacheEntry{
			LocalSize:   it.Size,
			LocalMtime:  it.Mtime,
			RemoteSize:  st.Size,
			RemoteMtime: st.ModTime,
			Hash:        localHash,
		}
	}
	return unchanged, cache, detail, nil
}

// printPushSkipped lists the files the remote already has, so a run does not
// look like it ignored them. It writes to stderr, like the plain progress view,
// keeping stdout a single object under --json.
func printPushSkipped(files []pushFile) {
	mark := lipgloss.NewStyle().Foreground(theme.TextDim).Render("↷")
	name := lipgloss.NewStyle().Foreground(theme.TextDim)
	for _, f := range files {
		if f.Skipped {
			fmt.Fprintf(os.Stderr, "  %s  %s\n", mark, name.Render(f.Local))
		}
	}
}

// printPushPlan renders the --dry-run plan. It goes through emit's human branch,
// so under --json nothing of this reaches stdout.
func printPushPlan(target string, dirs []string, items []pushItem, total int64, excluded int) {
	fmt.Printf("Would push %d file(s) · %s → %s (dry run)\n", len(items), tui.HumanBytes(total), target)
	if excluded > 0 {
		fmt.Printf("  excluded %d path(s)\n", excluded)
	}
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
func collectPushItems(locals []string, base, to string, excludes []string) ([]pushItem, []string, int, error) {
	single := len(locals) == 1
	var items []pushItem
	excluded := 0
	dirSet := make(map[string]struct{})

	for _, local := range locals {
		dest, err := resolvePushDest(base, local, to, single)
		if err != nil {
			return nil, nil, 0, err
		}

		info, err := os.Stat(local)
		if err != nil {
			return nil, nil, 0, fmt.Errorf("push %s: %w", local, err)
		}

		// A path named on the command line that an exclude matches is an error:
		// naming something and having nothing happen is the silent-skip failure
		// push exists to avoid.
		if pushExcluded(local, excludes) {
			return nil, nil, 0, fmt.Errorf("push %s: matches an exclude; drop it with `teleport exclude remove` or pass --no-exclude", local)
		}

		switch {
		case info.Mode().IsRegular():
			items = append(items, pushItem{
				Local:  filepath.ToSlash(local),
				Remote: dest,
				Size:   info.Size(),
				Mtime:  info.ModTime().Unix(),
				Mode:   pushMode(info.Mode()),
			})
		case info.IsDir():
			dirSet[dest] = struct{}{}
			seen := make(map[string]struct{})
			sub, dropped, err := walkPushDir(local, dest, seen, dirSet, excludes)
			if err != nil {
				return nil, nil, 0, err
			}
			items = append(items, sub...)
			excluded += dropped
		default:
			return nil, nil, 0, fmt.Errorf("push %s: unsupported file type %s", local, info.Mode().Type())
		}
	}

	dirs := make([]string, 0, len(dirSet))
	for d := range dirSet {
		dirs = append(dirs, d)
	}
	// Shortest first so parents are created before their children.
	sort.Slice(dirs, func(i, j int) bool { return len(dirs[i]) < len(dirs[j]) })
	return items, dirs, excluded, nil
}

// walkPushDir recurses into localDir, following symlinks. seen holds the
// canonical paths of directories already visited on this branch so a symlink
// cycle fails loudly instead of looping forever.
func walkPushDir(localDir, remoteDir string, seen map[string]struct{}, dirSet map[string]struct{}, excludes []string) ([]pushItem, int, error) {
	real, err := filepath.EvalSymlinks(localDir)
	if err != nil {
		return nil, 0, fmt.Errorf("push %s: %w", localDir, err)
	}
	if _, dup := seen[real]; dup {
		return nil, 0, fmt.Errorf("push %s: symlink cycle through %s", localDir, real)
	}
	seen[real] = struct{}{}
	// Popped on unwind so the guard is per-branch: two sibling symlinks to the
	// same directory are a duplicate, not a cycle, and must both upload.
	defer delete(seen, real)

	entries, err := os.ReadDir(localDir)
	if err != nil {
		return nil, 0, fmt.Errorf("read %s: %w", localDir, err)
	}

	var items []pushItem
	excluded := 0
	for _, e := range entries {
		localPath := filepath.Join(localDir, e.Name())
		remotePath := path.Join(remoteDir, e.Name())

		// A matching directory is counted once and never walked, so excluding
		// `.git` costs one check instead of enumerating thousands of objects.
		if pushExcluded(localPath, excludes) {
			log.Debug("excluded", "path", localPath)
			excluded++
			continue
		}

		info, err := os.Stat(localPath)
		if err != nil {
			return nil, 0, fmt.Errorf("push %s: %w", localPath, err)
		}

		switch {
		case info.Mode().IsRegular():
			items = append(items, pushItem{
				Local:  filepath.ToSlash(localPath),
				Remote: remotePath,
				Size:   info.Size(),
				Mtime:  info.ModTime().Unix(),
				Mode:   pushMode(info.Mode()),
			})
		case info.IsDir():
			dirSet[remotePath] = struct{}{}
			sub, dropped, err := walkPushDir(localPath, remotePath, seen, dirSet, excludes)
			if err != nil {
				return nil, 0, err
			}
			items = append(items, sub...)
			excluded += dropped
		default:
			return nil, 0, fmt.Errorf("push %s: unsupported file type %s", localPath, info.Mode().Type())
		}
	}
	return items, excluded, nil
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
