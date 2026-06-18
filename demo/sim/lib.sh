#!/usr/bin/env bash
# lib.sh — shared rendering helpers for the teleport demo simulations.
# Colors and glyphs mirror the real CLI (see internal/tui/*.go, cmd/*.go).
_LIBDIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=glyphs.sh
source "$_LIBDIR/glyphs.sh"

# ANSI 256-color helpers --------------------------------------------------
c()     { printf '\033[38;5;%sm%s\033[0m' "$1" "$2"; }   # foreground
cbold() { printf '\033[1;38;5;%sm%s\033[0m' "$1" "$2"; } # bold foreground
title() { printf '\033[1;48;5;60;38;5;255m%s\033[0m' "$1"; } # bg=60 fg=255 bold

# Palette (matches lipgloss Color() codes in source) ----------------------
CL_OK=82; CL_ERR=196; CL_ERR2=203; CL_SEP=241; CL_BAR=116
CL_STATS=252; CL_HEADER=252; CL_ICON=116; CL_SECTION=104
CL_DESC=252; CL_DIFF=214; CL_GOLD=220; CL_TITLEBLUE=62

sep_line() { local w="${1:-72}"; printf '─%.0s' $(seq 1 "$w"); }

# render_bar DONE TOTAL WIDTH ELAPSED_SECONDS
# Reproduces internal/tui/syncprogress.go renderBar(): "  [===>   ]  d/t  pct%  MM:SS  "
render_bar() {
  local done=$1 total=$2 width=$3 secs=${4:-0} i filled inner=""
  local pct=0; (( total > 0 )) && pct=$(( done * 100 / total ))
  (( total > 0 )) && filled=$(( done * width / total )) || filled=0
  for ((i = 0; i < width; i++)); do
    if   (( i <  filled - 1 )); then inner+="="
    elif (( i == filled - 1 && done <  total )); then inner+=">"
    elif (( i == filled - 1 && done == total )); then inner+="="
    else inner+=" "; fi
  done
  local mm ss; mm=$(printf '%02d' $(( secs / 60 ))); ss=$(printf '%02d' $(( secs % 60 )))
  printf '  [%s]%s' "$(c "$CL_BAR" "$inner")" \
    "$(c "$CL_STATS" "$(printf '  %d/%d  %3d%%  %s:%s  ' "$done" "$total" "$pct" "$mm" "$ss")")"
}

clear_screen() { printf '\033[2J\033[H'; }
