package cmd

import (
	"fmt"
	"strings"

	lipgloss "charm.land/lipgloss/v2"
)

const (
	iconSync     = '' // cod-sync
	iconGear     = '' // fa-gear
	iconPerson   = '' // cod-person
	iconTag      = '' // cod-tag
	iconUntrack  = '' // cod-diff-added
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

func printHelp() {
	commands := []cmdEntry{
		{iconGear, "init", "configure a sync profile (SSH host + remote directory)"},
		{iconSync, "sync", "sync changed files to the remote server"},
		{iconPerson, "profiles", "list configured sync profiles"},
		{iconGear, "config", "get/set per-directory defaults (e.g. sync-untracked)"},
		{iconTag, "version", "print version information"},
	}

	flags := []flagEntry{
		{"-s", "--sync", iconSync, "sync changed files"},
		{"-u", "--untracked", iconUntrack, "also sync untracked files (use with -s)"},
		{"-i", "--init", iconGear, "configure a sync profile"},
		{"-p", "--profiles", iconPerson, "list configured profiles"},
		{"-v", "--verbose", ' ', "verbose output"},
	}

	examples := [][]string{
		{"teleport -s", "sync only modified files"},
		{"teleport -su", "sync modified + untracked files"},
		{"teleport -i", "run interactive profile setup"},
		{"teleport -p", "list all profiles"},
		{"teleport sync staging", "sync using a specific profile"},
		{"teleport config set sync-untracked true", "remember -u for this wd"},
	}

	var b strings.Builder

	// Title
	fmt.Fprintf(&b, "\n  %s\n\n", titleStyle.Render(" teleport "))
	fmt.Fprintf(&b, "  %s\n\n",
		descStyle.Render("Sync git-tracked files to a remote server via SFTP,"))
	fmt.Fprintf(&b, "  %s\n\n",
		descStyle.Render("keeping your git history clean of post-deploy fix commits."))

	// Commands
	fmt.Fprintf(&b, "  %s\n\n", sectionStyle.Render("Commands"))
	for _, c := range commands {
		icon := iconStyle.Render(string(c.icon))
		name := nameStyle.Render(c.name)
		desc := descStyle.Render(c.desc)
		fmt.Fprintf(&b, "    %s  %s  %s\n", icon, name, desc)
	}

	// Flags / shortcuts
	fmt.Fprintf(&b, "\n  %s\n\n", sectionStyle.Render("Flags & shortcuts"))
	for _, f := range flags {
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

	// Examples
	fmt.Fprintf(&b, "\n  %s\n\n", sectionStyle.Render("Examples"))
	for _, e := range examples {
		cmd := exampleCmdStyle.Render(fmt.Sprintf("%-28s", e[0]))
		desc := descStyle.Render(e[1])
		fmt.Fprintf(&b, "    %s  %s\n", cmd, desc)
	}

	fmt.Println(b.String())
}
