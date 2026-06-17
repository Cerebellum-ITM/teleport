package cmd

import (
	"fmt"
	"strings"

	lipgloss "charm.land/lipgloss/v2"
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
			Foreground(lipgloss.Color("255")).
			Background(lipgloss.Color("60")).
			Padding(0, 1)

	sectionStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("104"))

	iconStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("116"))

	nameStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("255")).
			Width(12)

	descStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("252"))

	shortStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("150")).
			Width(6)

	longFlagStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("248")).
			Width(14)

	exampleCmdStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("222"))

	keyNameStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("255")).
			Width(18)

	keyTypeStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("150")).
			Width(6)

	keyDefStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("248")).
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
			{"-v", "--verbose", ' ', "verbose output"},
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
			{"teleport beam -cs", "clean remote → beam commits → sync working tree"},
			{"teleport clean", "discard dirty changes on the remote"},
			{"teleport status -p", "verify pending work matches the remote"},
			{"teleport config set sync-untracked true", "remember -u for this wd"},
			{"teleport ship ./mycli", "deploy a built binary to its OS bin/ dir"},
			{"teleport shell", "ssh into the remote at the profile's path"},
		},
	}

	// Render with the same formatting as before. The output below is
	// byte-identical to the legacy printHelp output.
	fmt.Println(renderHelp(doc))
}
