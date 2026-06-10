# UI Context

## Theme

Terminal-only. No web UI. The design language is a dark technical workspace
rendered via lipgloss — muted backgrounds from the terminal's own palette,
vivid accent colors for the selected item and headers, and dim text for
secondary information. The TUI adapts to the terminal's color profile via
charmbracelet/colorprofile.

## Color Tokens (lipgloss)

All components must use these named lipgloss colors — no hardcoded hex values
outside this file.

| Role                  | lipgloss token            | ANSI / Adaptive color |
| --------------------- | ------------------------- | --------------------- |
| Header / title        | `lipgloss.Color("62")`    | Blue-purple           |
| Header foreground     | `lipgloss.Color("230")`   | Near-white            |
| Cursor / selected row | `lipgloss.Color("212")`   | Pink                  |
| Success / selected check | `lipgloss.Color("82")` | Green                 |
| Unselected toggle     | `lipgloss.Color("214")`   | Orange/amber          |
| Dim / secondary text  | `lipgloss.Color("241")`   | Grey                  |
| Tracked file text     | `lipgloss.Color("241")`   | Grey (same as dim)    |
| Delete / warning      | `lipgloss.Color("203")`   | Red-orange            |
| Sent / beamed badge   | `lipgloss.Color("82")`    | Green (same as check) |

### Beam commit palette

The beam file picker groups files by the commit they originate from and
tints each group with a distinct accent so the list reads at a glance. These
are the only colors that may cycle/repeat; they are assigned to commits in
display order and reused (mod length) when there are more commits than colors.
Defined as `beamCommitPalette` in `internal/tui/beamfilepicker.go`. The set
avoids the reserved roles above (green check `82`, red delete `203`, orange
toggle `214`, grey dim `241`, header `62`, cursor pink `212`).

| Index | lipgloss token            | Color         |
| ----- | ------------------------- | ------------- |
| 0     | `lipgloss.Color("39")`    | Blue          |
| 1     | `lipgloss.Color("45")`    | Cyan          |
| 2     | `lipgloss.Color("43")`    | Teal          |
| 3     | `lipgloss.Color("81")`    | Sky           |
| 4     | `lipgloss.Color("220")`   | Gold          |
| 5     | `lipgloss.Color("215")`   | Light orange  |
| 6     | `lipgloss.Color("208")`   | Orange        |
| 7     | `lipgloss.Color("209")`   | Salmon        |
| 8     | `lipgloss.Color("205")`   | Pink          |
| 9     | `lipgloss.Color("213")`   | Light magenta |
| 10    | `lipgloss.Color("199")`   | Deep pink     |
| 11    | `lipgloss.Color("171")`   | Magenta       |
| 12    | `lipgloss.Color("141")`   | Purple        |
| 13    | `lipgloss.Color("99")`    | Violet        |
| 14    | `lipgloss.Color("147")`   | Periwinkle    |
| 15    | `lipgloss.Color("105")`   | Indigo        |

## Typography

Terminal output only — font is controlled by the user's terminal emulator.
`charmbracelet/log` is used for structured log lines; lipgloss Bold() for
section headers.

| Role             | Treatment                         |
| ---------------- | --------------------------------- |
| Section headers  | `lipgloss.NewStyle().Bold(true)`  |
| Selected item    | `lipgloss.NewStyle().Bold(true)`  |
| Dim / secondary  | No bold, dim color token          |
| Log messages     | `charmbracelet/log` default style |

## Nerd Font Icons

The binary assumes the terminal has a Nerd Font installed. All icons are
defined as constants in their respective files — never inline rune literals
elsewhere.

| Constant      | Glyph | Usage                         |
| ------------- | ----- | ----------------------------- |
| `iconServer`  | `󰒋 ` | Host list items               |
| `iconFolder`  | `󰉋 ` | Directory browser entries     |
| `iconFile`    | `󰈙 ` | File list items               |
| `iconChecked` | `󰱒 ` | Selected extra file           |
| `iconSync`    | `󰒃 ` | Sync in progress (reserved)   |
| `iconSyncOK`  | `✓`  | File uploaded successfully    |
| `iconSyncFail`| `✗`  | File upload failed            |
| `iconCommit`  | `󰜘 ` | Commit entry in commit picker |
| `iconDelete`  | `󰮈 ` | File flagged for deletion in beam |
| `iconCube`    | `󰆧 ` | Per-commit color marker in beam file picker |
| `iconSent`    | `󰗠 ` | Commit already beamed to the active profile (commit picker) |

## Keybinding Conventions

App-wide rules every TUI must follow:

- **`tab` toggles selection** (select/deselect an item) in every multi-select
  picker. `space` may be kept as a secondary alias, but `tab` is the canonical
  key and the one shown in footers/help text. Applies to the commit picker,
  beam file picker, sync file picker, and the `huh` multi-select in
  `teleport init` (its `MultiSelect.Toggle` binding).
- **`enter` confirms**, **`ctrl+c` cancels** (and `q` quits in browsers).
- In single-select file/directory browsers there is nothing to toggle, so
  `tab` (and `→/l`) descends into a directory; selection is `enter`. This is
  the one place `tab` does not mean "toggle".

## Component Patterns

### Host Picker
- `bubbles/list` with `NewDefaultDelegate()`, filtering enabled.
- Title bar styled with header token (bg `62`, fg `230`).
- Width 60, height 20.

### Dir Browser
- Custom bubbletea model. Header shows current path in dim style; cursor row in cursor/selected style.
- Navigation: `↑/↓` or `j/k` — move, `enter` — descend, `backspace/h/left` — ascend, `s` — confirm selection, `q` — quit.
- Footer always shows keybindings and current selected path.

### File Picker
- Custom bubbletea model. Tracked files shown in dim (non-interactive). Untracked files toggleable with `tab` (`space` alias).
- Checked files use green check icon; unchecked use orange box icon.
- `enter` confirms; `ctrl+c` cancels.

## Layout Notes

- No full-screen takeover except during TUI programs (bubbletea handles the alternate screen).
- Log output (`charmbracelet/log`) flows before and after TUI sessions as normal stdout.
- All TUI `View()` functions return `tea.NewView(s)` — never plain strings.
