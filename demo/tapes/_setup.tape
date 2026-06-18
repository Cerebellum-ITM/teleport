# _setup.tape — shared VHS settings + hidden shell prep. Included via `Source`.
Set Shell bash
Set FontFamily "JetBrainsMono Nerd Font Mono"
Set FontSize 17
Set Width 1040
Set Height 520
Set Padding 24
Set Margin 18
Set BorderRadius 10
# Custom theme: TokyoNight-style dark background but a BRIGHT foreground so the
# default-colored "main" text (file names, body) reads crisp instead of the dim
# muted lavender the stock theme uses. Styled colors (✓/icons/bar) are ANSI 256
# and unaffected by the theme palette.
Set Theme { "name": "teleport-demo", "background": "#1a1b26", "foreground": "#e8eaf4", "cursor": "#e8eaf4", "selection": "#283457", "black": "#15161e", "red": "#f7768e", "green": "#9ece6a", "yellow": "#e0af68", "blue": "#7aa2f7", "magenta": "#bb9af7", "cyan": "#7dcfff", "white": "#d5d9ee", "brightBlack": "#414868", "brightRed": "#f7768e", "brightGreen": "#9ece6a", "brightYellow": "#e0af68", "brightBlue": "#7aa2f7", "brightMagenta": "#bb9af7", "brightCyan": "#7dcfff", "brightWhite": "#ffffff" }
Set TypingSpeed 55ms
Set PlaybackSpeed 1.0

Hide
# Force 24-bit color so nothing is downsampled/remapped away from teleport's palette.
Type "export COLORTERM=truecolor TERM=xterm-256color" Enter
Type "source demo/sim/teleport-sim.sh" Enter
Type `export PS1='\[\e[38;5;141m\]teleport\[\e[0m\] \[\e[38;5;245m\]~/app\[\e[0m\] \[\e[38;5;78m\]❯\[\e[0m\] '` Enter
Type "clear" Enter
Show
Sleep 700ms
