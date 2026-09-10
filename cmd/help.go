package cmd

import (
	"fmt"
	"strings"

	lipgloss "charm.land/lipgloss/v2"
	"github.com/pascualchavez/teleport/internal/theme"
)

const (
	iconSync    = '' // cod-sync
	iconGear    = '' // fa-gear
	iconPerson  = '' // cod-person
	iconTag     = '' // cod-tag
	iconUntrack = '' // cod-diff-added
	iconKey     = '' // cod-key
)

var (
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(theme.TextBright).
			Background(theme.HeaderBg).
			Padding(0, 1)

	sectionStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(theme.Section)

	iconStyle = lipgloss.NewStyle().
			Foreground(theme.Icon)

	nameStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(theme.TextBright).
			Width(12)

	descStyle = lipgloss.NewStyle().
			Foreground(theme.Text)

	shortStyle = lipgloss.NewStyle().
			Foreground(theme.Hint).
			Width(6)

	longFlagStyle = lipgloss.NewStyle().
			Foreground(theme.TextDim).
			Width(14)

	exampleCmdStyle = lipgloss.NewStyle().
			Foreground(theme.Gold)

	keyNameStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(theme.TextBright).
			Width(18)

	keyTypeStyle = lipgloss.NewStyle().
			Foreground(theme.Hint).
			Width(6)

	keyDefStyle = lipgloss.NewStyle().
			Foreground(theme.TextDim).
			Width(10)
)

type cmdEntry struct {
	icon rune
	name string
	desc string
}

type flagEntry struct {
	short string
	long  string
	icon  rune
	desc  string
}

type keyEntry struct {
	icon rune
	name string
	typ  string
	def  string
	desc string
}

type helpDoc struct {
	Title     string
	Tagline   []string
	Commands  []cmdEntry
	Flags     []flagEntry
	Examples  [][]string
	KeysTable []keyEntry
}

func renderHelp(doc helpDoc) string {
	var b strings.Builder

	fmt.Fprintf(&b, "\n  %s\n\n", titleStyle.Render(doc.Title))
	for _, line := range doc.Tagline {
		fmt.Fprintf(&b, "  %s\n\n", descStyle.Render(line))
	}

	if len(doc.Commands) > 0 {
		fmt.Fprintf(&b, "  %s\n\n", sectionStyle.Render("Commands"))
		for _, c := range doc.Commands {
			icon := iconStyle.Render(string(c.icon))
			name := nameStyle.Render(c.name)
			desc := descStyle.Render(c.desc)
			fmt.Fprintf(&b, "    %s  %s  %s\n", icon, name, desc)
		}
	}

	if len(doc.Flags) > 0 {
		fmt.Fprintf(&b, "\n  %s\n\n", sectionStyle.Render("Flags & shortcuts"))
		for _, f := range doc.Flags {
			var icon string
			if f.icon != ' ' {
				icon = iconStyle.Render(string(f.icon))
			} else {
				icon = "  "
			}
			short := shortStyle.Render(f.short)
			long := longFlagStyle.Render(f.long)
			desc := descStyle.Render(f.desc)
			fmt.Fprintf(&b, "    %s  %s  %s  %s\n", icon, short, long, desc)
		}
	}

	if len(doc.Examples) > 0 {
		fmt.Fprintf(&b, "\n  %s\n\n", sectionStyle.Render("Examples"))
		for _, e := range doc.Examples {
			cmd := exampleCmdStyle.Render(fmt.Sprintf("%-28s", e[0]))
			desc := descStyle.Render(e[1])
			fmt.Fprintf(&b, "    %s  %s\n", cmd, desc)
		}
	}

	if len(doc.KeysTable) > 0 {
		fmt.Fprintf(&b, "\n  %s\n\n", sectionStyle.Render("Config keys"))
		for _, k := range doc.KeysTable {
			icon := iconStyle.Render(string(k.icon))
			name := keyNameStyle.Render(k.name)
			typ := keyTypeStyle.Render(k.typ)
			def := keyDefStyle.Render(k.def)
			desc := descStyle.Render(k.desc)
			fmt.Fprintf(&b, "    %s  %s  %s  %s  %s\n", icon, name, typ, def, desc)
		}
	}

	return b.String()
}

func printHelp() {
	doc := helpDoc{
		Title: " teleport ",
		Tagline: []string{
			"Sync git-tracked files to a remote server via SFTP,",
			"keeping your git history clean of post-deploy fix commits.",
		},
		Commands: []cmdEntry{
			{iconGear, "init", "configure a sync profile (SSH host + remote directory)"},
			{iconSync, "sync", "sync changed files to the remote server"},
			{iconSync, "beam", "send selected local commits (cherry-pick style)"},
			{iconGear, "status", "compare local files against the remote (SHA256)"},
			{iconSync, "clean", "discard dirty changes on the remote (git checkout + git clean)"},
			{iconSync, "mirror", "advance the remote branch to your local commits (same hash)"},
			{iconSync, "run", "run a profile's remote action(s), streaming their logs"},
			{iconGear, "actions", "manage a profile's remote actions (add/list/edit/remove)"},
			{iconSync, "push", "upload paths as-is, even the ones git ignores"},
			{iconGear, "exclude", "manage the paths push skips in this project"},
			{iconSync, "pull", "download remote changes to local working tree"},
			{iconSync, "ship", "deploy a local binary to its OS-matching bin profile"},
			{iconGear, "shell", "open an interactive shell on the remote at the profile's path"},
			{iconPerson, "profiles", "list configured sync profiles"},
			{iconGear, "config", "get/set per-directory defaults (e.g. sync-untracked)"},
			{iconTag, "version", "print version information"},
		},
		Flags: []flagEntry{
			{"-s", "--sync", iconSync, "sync changed files"},
			{"-u", "--untracked", iconUntrack, "also sync untracked files (use with -s)"},
			{"-i", "--init", iconGear, "configure a sync profile"},
			{"-p", "--profiles", iconPerson, "list configured profiles"},
			{"-b", "--beam", iconSync, "send selected local commits to the remote"},
			{"", "--sh", iconGear, "open an interactive shell on the remote"},
			{"-v", "--verbose", ' ', "verbose output"},
			{"", "--no-input", iconSync, "never prompt; fail closed instead (exit 2)"},
			{"", "--json", iconSync, "machine-readable result on stdout"},
			{"-q", "--quiet", ' ', "only the final result and errors"},
		},
		Examples: [][]string{
			{"teleport -s", "sync only modified files"},
			{"teleport -su", "sync modified + untracked files"},
			{"teleport -i", "run interactive profile setup"},
			{"teleport -p", "list all profiles"},
			{"teleport -b", "pick local commits to send"},
			{"teleport sync staging", "sync using a specific profile"},
			{"teleport beam", "pick local commits to send"},
			{"teleport beam -a", "auto-select unsent commits, skip the commit picker"},
			{"teleport beam -C a1b2c3d", "send exactly this commit, headless-friendly"},
			{"teleport beam -cs", "clean remote → beam commits → sync working tree"},
			{"teleport mirror -a", "advance the remote branch to HEAD (same hashes)"},
			{"teleport run deploy", "run the 'deploy' action, streaming its logs"},
			{"teleport mirror -a --then deploy", "mirror, then run 'deploy' on success"},
			{"teleport actions add", "define a remote action via the wizard"},
			{"teleport actions add --name doctor", "define one headless (with --run)"},
			{"teleport push web/dist", "upload a gitignored build directory"},
			{"teleport push dist --to dist.new --then swap", "upload, then swap it in"},
			{"teleport push . -x node_modules", "skip a directory for this run"},
			{"teleport exclude", "pick what push skips in this project"},
			{"teleport clean", "discard dirty changes on the remote"},
			{"teleport status -p", "verify pending work matches the remote"},
			{"teleport config set sync-untracked true", "remember -u for this wd"},
			{"teleport ship ./mycli", "deploy a built binary to its OS bin/ dir"},
			{"teleport shell", "ssh into the remote at the profile's path"},
			{"teleport --sh", "shortcut for `teleport shell`"},
		},
	}

	// Render with the same formatting as before. The output below is
	// byte-identical to the legacy printHelp output.
	fmt.Println(renderHelp(doc))
}
