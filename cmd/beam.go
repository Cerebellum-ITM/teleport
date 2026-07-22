package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/colorprofile"
	"github.com/charmbracelet/log"
	"github.com/pascualchavez/teleport/internal/config"
	"github.com/pascualchavez/teleport/internal/git"
	"github.com/pascualchavez/teleport/internal/highlight"
	sshpkg "github.com/pascualchavez/teleport/internal/ssh"
	"github.com/pascualchavez/teleport/internal/tui"
	"github.com/spf13/cobra"
)

var (
	beamBranch   string
	beamThenSync bool
	beamClean    bool
	beamYes      bool
	beamAuto     bool
	beamCommits  []string
)

var beamCmd = &cobra.Command{
	Use:   "beam [profile]",
	Short: "󰜘 send selected local commits to the remote server",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runBeam,
}

func init() {
	beamCmd.Flags().StringVarP(&beamBranch, "branch", "b", "", "source branch for commits (default: current branch)")
	beamCmd.Flags().BoolVarP(&beamThenSync, "then-sync", "s", false, "run sync after beam (working-tree changes over the just-beamed snapshot)")
	beamCmd.Flags().BoolVarP(&beamClean, "clean", "c", false, "run clean before beam (discard dirty changes on the remote)")
	beamCmd.Flags().BoolVarP(&beamYes, "yes", "y", false, "skip the clean confirmation prompt")
	beamCmd.Flags().BoolVarP(&beamAuto, "auto", "a", false, "skip the commit picker; auto-select commits not yet sent and go straight to file review")
	beamCmd.Flags().StringArrayVarP(&beamCommits, "commit", "C", nil, "send exactly this commit (repeatable); skips the picker")
	registerThenFlag(beamCmd)
}

func runBeam(cmd *cobra.Command, args []string) error {
	localCfg, err := config.LoadLocal()
	if err != nil {
		return fmt.Errorf("load local config: %w", err)
	}

	profile, profileName, err := resolveProfile(args)
	if err != nil {
		return err
	}

	// --commit (explicit set) and -a (all unsent) are contradictory: one asks
	// for exactly these, the other for everything not yet sent. Reject before
	// touching anything.
	if len(beamCommits) > 0 && beamAuto {
		return fmt.Errorf("--commit and -a are mutually exclusive")
	}

	// Headless beam must be explicit about what to send: --no-input does not
	// silently imply -a (that would risk beaming unintended commits). Fail
	// closed before touching the remote so nothing is sent. --commit is an
	// equally explicit headless path.
	if !interactive() && !beamAuto && len(beamCommits) == 0 {
		return errNeedsTTY("pass -a (unsent commits) or --commit <sha> (explicit commits)")
	}

	// Validate --then actions up front so a typo (or an un-confirmable
	// confirm-action headless) fails before anything is sent.
	if err := validateThenActions(profile, profileName, thenActions); err != nil {
		return err
	}
	if err := precheckActionsConfirm(profile, thenActions, beamYes || noInput); err != nil {
		return err
	}

	// If --clean is set, connect now and run the clean phase before
	// anything else. The connection is reused by the beam phase.
	var client *sshpkg.Client
	if beamClean {
		client, err = connectToProfile(profile)
		if err != nil {
			return err
		}
		defer func() {
			if client != nil {
				client.Close()
			}
		}()
		// --no-input implies -y for the clean phase (same rule as `clean`).
		counts, err := cleanRemote(client, profile, beamYes || noInput, false)
		if err != nil {
			return err
		}
		if counts.Skipped {
			fmt.Println("aborted, no changes made")
			return nil
		}
	}

	branch, err := resolveBranch(beamBranch)
	if err != nil {
		return err
	}
	if branch == "" {
		return nil
	}

	commits, err := git.CommitsAheadOf(branch)
	if err != nil {
		return err
	}
	if len(commits) == 0 {
		if !beamClean {
			fmt.Printf("Nothing to beam — no local commits on %s ahead of remote.\n", branch)
		}
		return nil
	}

	// Prune the recorded beamed-set for this profile to the commits still ahead
	// (rebased/pushed SHAs fall off), then pre-select the unsent ones.
	ahead := make(map[string]bool, len(commits))
	for _, c := range commits {
		ahead[c.SHA] = true
	}
	localCfg.PruneBeamed(profileName, ahead)

	var selectedCommits []git.Commit
	if len(beamCommits) > 0 {
		// Explicit selection: send exactly the requested commits, skipping the
		// picker. Resolve each ref to a full SHA and require it to be among the
		// commits ahead — an out-of-range commit fails here, before connecting.
		selectedCommits, err = resolveExplicitCommits(beamCommits, commits)
		if err != nil {
			return err
		}
	} else if beamAuto {
		// Skip the picker: auto-select exactly the commits not yet beamed to
		// this profile (the picker's default pre-selection).
		sent := localCfg.SentSet(profileName)
		for _, c := range commits {
			if !sent[c.SHA] {
				selectedCommits = append(selectedCommits, c)
			}
		}
		if len(selectedCommits) == 0 {
			fmt.Printf("Nothing to beam — all local commits on %s already sent.\n", branch)
			return nil
		}
	} else {
		var delta tui.SentMarkDelta
		selectedCommits, delta, err = tui.RunCommitPicker(commits, localCfg.SentSet(profileName))
		if err != nil {
			return err
		}
		// Persist manual sent-marks as soon as the picker confirms, before the
		// file picker / connection / upload — so they survive even if the user
		// cancels a later step or the upload fails. Done before the
		// no-commits-selected early return on purpose: marking everything sent
		// and selecting nothing to beam must still stick.
		if len(delta.Added) > 0 || len(delta.Removed) > 0 {
			localCfg.ApplyBeamedDelta(profileName, delta.Added, delta.Removed, time.Now())
			if err := config.SaveLocal(localCfg); err != nil {
				log.Warn("could not record manual sent marks", "err", err)
			}
		}
		if len(selectedCommits) == 0 {
			fmt.Println("No commits selected.")
			return nil
		}
	}

	// CommitsAhead returns newest first; FilesInCommits needs oldest first
	// so the most recent commit wins for shared paths.
	shas := make([]string, 0, len(selectedCommits))
	for i := len(selectedCommits) - 1; i >= 0; i-- {
		shas = append(shas, selectedCommits[i].SHA)
	}

	allChanges, commitPaths, err := git.FilesInCommits(shas)
	if err != nil {
		return err
	}
	if len(allChanges) == 0 {
		fmt.Println("Selected commits touched no files.")
		return nil
	}

	// The file-diff viewer (Unit 18) is visual review only: headless skips it
	// and sends every changed path directly, without opening the tea.Program.
	var changes []git.FileChange
	if interactive() {
		changes, err = tui.RunBeamFilePicker(allChanges, selectedCommits, beamViewerLoader)
		if err != nil {
			return err
		}
		if len(changes) == 0 {
			fmt.Println("No files selected.")
			return nil
		}
	} else {
		changes = allChanges
	}

	if client == nil {
		client, err = connectToProfile(profile)
		if err != nil {
			return err
		}
		defer client.Close()
	}

	var toUpload []git.FileChange
	var toDelete []git.FileChange
	for _, c := range changes {
		if c.Status == 'D' {
			toDelete = append(toDelete, c)
		} else {
			toUpload = append(toUpload, c)
		}
	}

	failedPaths := make(map[string]bool)

	// Per-commit colors, computed from the full change set the picker used so
	// the send view matches the file picker exactly.
	styles := tui.BeamFileStyles(allChanges, selectedCommits)

	if len(toUpload) > 0 {
		byPath := make(map[string]git.FileChange, len(toUpload))
		for _, c := range toUpload {
			byPath[c.Path] = c
		}

		// Group upload paths under their commit, preserving the selected-commit
		// order so the send view reads top-to-bottom like the file picker.
		pathsBySHA := make(map[string][]string, len(selectedCommits))
		for _, c := range toUpload {
			pathsBySHA[c.SHA] = append(pathsBySHA[c.SHA], c.Path)
		}
		var groups []tui.BeamGroup
		for _, c := range selectedCommits {
			if ps := pathsBySHA[c.SHA]; len(ps) > 0 {
				groups = append(groups, tui.BeamGroup{
					Style:   styles[c.SHA],
					Short:   c.Short,
					Subject: c.Subject,
					Paths:   ps,
				})
			}
		}

		header := fmt.Sprintf("Beaming %d file(s) to %s:%s", len(toUpload), profile.Host, profile.Path)
		send := func(path string) error {
			fc := byPath[path]
			content, err := git.FileAtCommit(fc.SHA, fc.Path)
			if err != nil {
				return err
			}
			return client.UploadBytes(filepath.Join(profile.Path, fc.Path), content)
		}
		var failed []string
		if useTUI() {
			failed, err = tui.RunBeamSendProgress(header, groups, send)
		} else {
			failed, err = tui.RunBeamSendPlain(header, groups, send)
		}
		if err != nil {
			return err
		}
		for _, p := range failed {
			failedPaths[p] = true
		}
	}

	for _, c := range toDelete {
		remote := filepath.Join(profile.Path, c.Path)
		if err := client.Remove(remote); err != nil {
			log.Error("remove failed", "path", remote, "err", err)
			failedPaths[c.Path] = true
		} else {
			log.Info("removed", "path", remote)
		}
	}

	// Record commits as sent: a commit counts only if every path it touched was
	// uploaded/deleted without error. Persist even on partial failure so
	// fully-successful commits are not re-sent next time.
	rememberBeamedCommits(localCfg, profileName, commitPaths, changes, failedPaths)

	if len(failedPaths) > 0 {
		return fmt.Errorf("%d operation(s) failed", len(failedPaths))
	}

	if beamThenSync {
		if err := runChainedSync(client, profile, localCfg.SyncUntracked); err != nil {
			return err
		}
	}
	if err := config.TouchLastSync(); err != nil {
		log.Warn("could not update last sync timestamp", "err", err)
	}

	actions, actErr := executeActions(client, profile, profileName, thenActions, beamYes || noInput)

	sentFiles := make([]string, 0, len(changes))
	for _, c := range changes {
		if !failedPaths[c.Path] {
			sentFiles = append(sentFiles, c.Path)
		}
	}
	shortSHAs := make([]string, 0, len(selectedCommits))
	for _, c := range selectedCommits {
		shortSHAs = append(shortSHAs, c.Short)
	}
	res := beamResult{
		Command: "beam",
		Target:  fmt.Sprintf("%s:%s", profile.Host, profile.Path),
		Commits: shortSHAs,
		Sent:    len(sentFiles),
		Files:   sentFiles,
		Actions: actions,
	}
	emit(res, func() {}) // human path already printed progress + summary inline
	return actErr
}

// beamResult is the --json shape for beam.
type beamResult struct {
	Command string         `json:"command"`
	Target  string         `json:"target"`
	Commits []string       `json:"commits"`
	Sent    int            `json:"sent"`
	Files   []string       `json:"files"`
	Actions []actionResult `json:"actions,omitempty"`
}

func runChainedSync(client *sshpkg.Client, profile config.Profile, includeUntracked bool) error {
	changed, err := git.ChangedFiles()
	if err != nil {
		return fmt.Errorf("git diff: %w", err)
	}

	if includeUntracked {
		untracked, err := git.UntrackedFiles()
		if err != nil {
			log.Warn("Could not list untracked files", "err", err)
		} else {
			changed = append(changed, untracked...)
		}
	}

	changed = dedupe(changed)
	if len(changed) == 0 {
		fmt.Println("Sync stage: nothing to sync — working tree matches HEAD.")
		return nil
	}

	header := fmt.Sprintf("Syncing %d working-tree file(s) to %s:%s", len(changed), profile.Host, profile.Path)
	upload := func(localPath string) error {
		return client.UploadFile(localPath, filepath.Join(profile.Path, localPath))
	}
	var failed []string
	if useTUI() {
		failed, err = tui.RunSyncProgress(header, changed, upload)
	} else {
		failed, err = tui.RunSyncPlain(header, changed, upload)
	}
	if err != nil {
		return err
	}
	if len(failed) > 0 {
		return fmt.Errorf("%d file(s) failed to upload in sync stage", len(failed))
	}
	return nil
}

// rememberBeamedCommits records which commits were fully sent and persists the
// local config. A commit counts as sent only when every path it touched made it
// into this beam (i.e. survived the file picker) and uploaded without error —
// attribution is by path, not by the winning SHA, so a commit whose file was
// superseded by a newer commit is still credited. Failures are warn-logged.
func rememberBeamedCommits(cfg *config.LocalConfig, profileName string, commitPaths map[string][]string, changes []git.FileChange, failedPaths map[string]bool) {
	// covered = paths that were part of this beam and did not fail.
	covered := make(map[string]bool, len(changes))
	for _, c := range changes {
		if !failedPaths[c.Path] {
			covered[c.Path] = true
		}
	}

	sent := commitsFullySent(commitPaths, covered)
	if len(sent) == 0 {
		return
	}

	cfg.MarkBeamed(profileName, sent, time.Now())
	if err := config.SaveLocal(cfg); err != nil {
		log.Warn("could not record beamed commits", "err", err)
	}
}

// commitsFullySent returns the SHAs whose every touched path is covered. A
// commit with no recorded paths is skipped (nothing of it was sent).
func commitsFullySent(commitPaths map[string][]string, covered map[string]bool) []string {
	var sent []string
	for sha, paths := range commitPaths {
		if len(paths) == 0 {
			continue
		}
		all := true
		for _, p := range paths {
			if !covered[p] {
				all = false
				break
			}
		}
		if all {
			sent = append(sent, sha)
		}
	}
	return sent
}

// resolveExplicitCommits maps the --commit refs to entries of the ahead list.
// Each ref is rev-parsed to a full SHA and must belong to ahead; any ref that
// resolves outside it fails, naming the rejected commit and listing the valid
// shorts. Duplicate refs are deduped. The result preserves ahead's order (the
// branch's topological order), not the flag order, so uploads read the same as
// the picker's.
func resolveExplicitCommits(refs []string, ahead []git.Commit) ([]git.Commit, error) {
	bySHA := make(map[string]git.Commit, len(ahead))
	for _, c := range ahead {
		bySHA[c.SHA] = c
	}

	want := make(map[string]bool, len(refs))
	for _, ref := range refs {
		sha, err := git.RevParse(ref)
		if err != nil {
			return nil, fmt.Errorf("resolve --commit %q: %w", ref, err)
		}
		if _, ok := bySHA[sha]; !ok {
			return nil, fmt.Errorf("commit %s is not among the commits ahead of the remote; available: %s",
				ref, availableShorts(ahead))
		}
		want[sha] = true
	}

	var selected []git.Commit
	for _, c := range ahead {
		if want[c.SHA] {
			selected = append(selected, c)
		}
	}
	return selected, nil
}

// availableShorts renders the short SHAs of ahead as a comma-separated list
// for the --commit rejection message.
func availableShorts(ahead []git.Commit) string {
	shorts := make([]string, 0, len(ahead))
	for _, c := range ahead {
		shorts = append(shorts, c.Short)
	}
	return strings.Join(shorts, ", ")
}

func resolveBranch(explicit string) (string, error) {
	current, all, err := git.LocalBranches()
	if err != nil {
		return "", fmt.Errorf("list branches: %w", err)
	}
	if explicit != "" {
		for _, b := range all {
			if b == explicit {
				return b, nil
			}
		}
		return "", fmt.Errorf("branch %q not found locally", explicit)
	}
	if len(all) == 1 {
		return current, nil
	}
	if !interactive() {
		return "", errNeedsTTY("pass --branch to choose the source branch")
	}
	return tui.RunBranchPicker(all, current)
}

// beamViewerLoader fetches and renders a file's contents or diff for the beam
// file picker's viewer. All git + highlight I/O lives here so the TUI model
// stays I/O-free (architecture invariant #3).
func beamViewerLoader(fc git.FileChange, mode tui.ViewerMode, width int) (tui.ViewerContent, error) {
	profile := colorprofile.Detect(os.Stdout, os.Environ())

	if mode == tui.ViewDiff {
		raw, err := git.FileDiffAtCommit(fc.SHA, fc.Path)
		if err != nil {
			return tui.ViewerContent{}, err
		}
		body, adds, dels := highlight.Diff(raw, fc.Path, width, profile)
		return tui.ViewerContent{Body: body, Adds: adds, Dels: dels}, nil
	}

	// File mode. A deleted path has no blob at its own commit, so show the
	// contents from just before the deletion.
	var (
		src  []byte
		err  error
		note string
	)
	if fc.Status == 'D' {
		src, err = git.FileBeforeCommit(fc.SHA, fc.Path)
		note = "(before delete)"
	} else {
		src, err = git.FileAtCommit(fc.SHA, fc.Path)
	}
	if err != nil {
		return tui.ViewerContent{}, err
	}
	if highlight.IsBinary(src) {
		return tui.ViewerContent{Binary: true, Bytes: len(src), Note: note}, nil
	}
	body, lang, lines, err := highlight.Code(src, fc.Path, profile)
	if err != nil {
		return tui.ViewerContent{}, err
	}
	return tui.ViewerContent{Body: body, Lang: lang, Lines: lines, Note: note}, nil
}
