package cmd

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	lipgloss "charm.land/lipgloss/v2"
	"github.com/charmbracelet/log"
	"github.com/pascualchavez/teleport/internal/config"
	"github.com/pascualchavez/teleport/internal/git"
	sshpkg "github.com/pascualchavez/teleport/internal/ssh"
	"github.com/spf13/cobra"
)

var statusPending bool

var statusCmd = &cobra.Command{
	Use:   "status [profile]",
	Short: " compare local files against the remote",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runStatus,
}

func init() {
	statusCmd.Flags().BoolVarP(&statusPending, "pending", "p", false,
		"only check files in unpushed commits + dirty working tree")
}

type statusTarget struct {
	Path         string
	ExpectAbsent bool
}

type statusResult struct {
	Path   string
	Marker string // "==", "!=", "??", "--"
}

var (
	statusDiffStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
	statusMissingStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("203"))
	statusOKStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("82"))
)

func runStatus(cmd *cobra.Command, args []string) error {
	localCfg, err := config.LoadLocal()
	if err != nil {
		return fmt.Errorf("load local config: %w", err)
	}

	profileName := localCfg.DefaultProfile
	if len(args) > 0 {
		profileName = args[0]
	}
	if profileName == "" {
		return fmt.Errorf("no profile specified; run `teleport init` first or pass a profile name")
	}

	globalCfg, err := config.LoadGlobal()
	if err != nil {
		return fmt.Errorf("load global config: %w", err)
	}

	profile, ok := globalCfg.Profiles[profileName]
	if !ok {
		return fmt.Errorf("profile %q not found; run `teleport init` to create it", profileName)
	}

	targets, err := collectStatusTargets(localCfg.SyncUntracked)
	if err != nil {
		return err
	}
	if len(targets) == 0 {
		fmt.Println("Nothing to check.")
		return nil
	}

	hosts, err := sshpkg.ParseSSHConfig()
	if err != nil {
		return fmt.Errorf("parse ssh config: %w", err)
	}

	var targetHost *sshpkg.Host
	for _, h := range hosts {
		if h.Name == profile.Host {
			hCopy := h
			targetHost = &hCopy
			break
		}
	}
	if targetHost == nil {
		targetHost = &sshpkg.Host{
			Name:     profile.Host,
			Hostname: profile.Host,
			Port:     "22",
		}
	}

	log.Info("Connecting", "host", targetHost.Name)
	client, err := sshpkg.Connect(*targetHost)
	if err != nil {
		return fmt.Errorf("connect to %s: %w", targetHost.Name, err)
	}
	defer client.Close()

	results := make([]statusResult, 0, len(targets))
	total := len(targets)
	for i, t := range targets {
		fmt.Fprintf(os.Stderr, "\r  Checking %d/%d...", i+1, total)
		marker, err := classifyTarget(client, profile.Path, t)
		if err != nil {
			log.Error("check failed", "path", t.Path, "err", err)
			continue
		}
		results = append(results, statusResult{Path: t.Path, Marker: marker})
	}
	fmt.Fprintf(os.Stderr, "\r\033[K")

	printStatusReport(results, profile.Host, profile.Path)

	for _, r := range results {
		if r.Marker != "==" {
			os.Exit(1)
		}
	}
	return nil
}

func collectStatusTargets(includeUntracked bool) ([]statusTarget, error) {
	if !statusPending {
		files, err := git.TrackedFiles()
		if err != nil {
			return nil, fmt.Errorf("git ls-files: %w", err)
		}
		out := make([]statusTarget, len(files))
		for i, f := range files {
			out[i] = statusTarget{Path: f}
		}
		return out, nil
	}

	byPath := make(map[string]statusTarget)

	commits, err := git.CommitsAhead()
	if err != nil && !errors.Is(err, git.ErrNoUpstream) {
		return nil, err
	}
	if errors.Is(err, git.ErrNoUpstream) {
		log.Warn("no upstream branch; checking only working-tree changes")
	}
	if len(commits) > 0 {
		shas := make([]string, 0, len(commits))
		for i := len(commits) - 1; i >= 0; i-- {
			shas = append(shas, commits[i].SHA)
		}
		changes, err := git.FilesInCommits(shas)
		if err != nil {
			return nil, err
		}
		for _, c := range changes {
			byPath[c.Path] = statusTarget{Path: c.Path, ExpectAbsent: c.Status == 'D'}
		}
	}

	changed, err := git.ChangedFiles()
	if err != nil {
		return nil, fmt.Errorf("git diff: %w", err)
	}
	for _, p := range changed {
		byPath[p] = statusTarget{Path: p}
	}

	if includeUntracked {
		untracked, err := git.UntrackedFiles()
		if err != nil {
			log.Warn("could not list untracked files", "err", err)
		}
		for _, p := range untracked {
			if _, ok := byPath[p]; !ok {
				byPath[p] = statusTarget{Path: p}
			}
		}
	}

	out := make([]statusTarget, 0, len(byPath))
	for _, t := range byPath {
		out = append(out, t)
	}
	return out, nil
}

func classifyTarget(client *sshpkg.Client, basePath string, t statusTarget) (string, error) {
	remote := filepath.Join(basePath, t.Path)

	if t.ExpectAbsent {
		_, err := client.RemoteSHA256(remote)
		if errors.Is(err, os.ErrNotExist) {
			return "==", nil
		}
		if err != nil {
			return "", err
		}
		return "--", nil
	}

	localHash, err := localSHA256(t.Path)
	if err != nil {
		if os.IsNotExist(err) {
			return "--", nil
		}
		return "", err
	}

	remoteHash, err := client.RemoteSHA256(remote)
	if errors.Is(err, os.ErrNotExist) {
		return "??", nil
	}
	if err != nil {
		return "", err
	}

	if localHash == remoteHash {
		return "==", nil
	}
	return "!=", nil
}

func localSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func printStatusReport(results []statusResult, host, path string) {
	total := len(results)
	var differ, missingRemote, missingLocal, extraRemote int
	for _, r := range results {
		switch r.Marker {
		case "!=":
			differ++
		case "??":
			missingRemote++
		case "--":
			missingLocal++
			extraRemote++
		}
	}

	clean := differ == 0 && missingRemote == 0 && missingLocal == 0
	if clean {
		fmt.Println(statusOKStyle.Render(fmt.Sprintf("✓ all %d file(s) in sync with %s:%s", total, host, path)))
		return
	}

	fmt.Printf("Status against %s:%s\n", host, path)
	var parts []string
	if differ > 0 {
		parts = append(parts, fmt.Sprintf("%d differ", differ))
	}
	if missingRemote > 0 {
		parts = append(parts, fmt.Sprintf("%d missing remotely", missingRemote))
	}
	if missingLocal > 0 {
		parts = append(parts, fmt.Sprintf("%d unexpectedly present remotely", missingLocal))
	}
	summary := ""
	for i, p := range parts {
		if i > 0 {
			summary += ", "
		}
		summary += p
	}
	fmt.Printf("  %s (%d total)\n\n", summary, total)

	for _, r := range results {
		switch r.Marker {
		case "!=":
			fmt.Printf("  %s %s\n", statusDiffStyle.Render("!="), r.Path)
		case "??":
			fmt.Printf("  %s %s\n", statusMissingStyle.Render("??"), r.Path)
		case "--":
			fmt.Printf("  %s %s\n", statusMissingStyle.Render("--"), r.Path)
		}
	}
}
