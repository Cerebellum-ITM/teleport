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
- Custom bubbletea model. Tracked files shown in dim (non-interactive). Untracked files toggleable with `space`.
- Checked files use green check icon; unchecked use orange box icon.
- `enter` confirms; `ctrl+c` cancels.

## Layout Notes

- No full-screen takeover except during TUI programs (bubbletea handles the alternate screen).
- Log output (`charmbracelet/log`) flows before and after TUI sessions as normal stdout.
- All TUI `View()` functions return `tea.NewView(s)` — never plain strings.
