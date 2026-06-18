#!/usr/bin/env bash
# teleport-sim.sh — defines a teleport() shell function that renders faithful,
# fully-invented simulations of each teleport command for the VHS demo GIFs.
# No network, no real config: every host/path/file below is fictional.
#
# Usage (inside a VHS tape):
#   Hide
#   Type "source demo/sim/teleport-sim.sh" Enter
#   Type "clear" Enter
#   Show
#   Type "teleport sync" Enter
_SIMDIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=lib.sh
source "$_SIMDIR/lib.sh"

# --- Fictional fixtures ---------------------------------------------------
HOST="deploy@vps-staging"
PATH_="/srv/app"

# Files the demo "changed since HEAD" (path|glyph)
_CHANGED=(
  "src/server.go|$G_GO"
  "internal/api/handler.go|$G_GO"
  "config/app.yml|$G_YML"
  "web/assets/styles.css|$G_CSS"
  "Dockerfile|$G_DOCKER"
  "scripts/deploy.sh|$G_SH"
  "README.md|$G_MD"
)

# ==========================================================================
_sim_version() {
  printf 'teleport 0.4.0 (commit 4cc4d16, built 2026-06-18)\n'
}

_sim_help() {
  if [ -f "$_SIMDIR/_help.out" ]; then
    command cat "$_SIMDIR/_help.out"
  else
    command teleport --help 2>/dev/null
  fi
}

_sim_profiles() {
  printf 'Sync profiles:\n'
  printf '%s%-20s  %s\n' "$(c "$CL_OK" '* ')" 'staging'    "$HOST:$PATH_"
  printf '  %-20s  %s\n' 'production' 'web@prod-1:/var/www/app'
  printf '  %-20s  %s\n' 'homelab'    'pi@homelab:/opt/api'
  printf '\nBin profiles:\n'
  printf '  %-20s  %s\n' 'linux' 'web@prod-1:/usr/local/bin'
}

_sim_config() {
  printf '\n  %s\n\n' "$(title ' teleport config ')"
  printf '  default-profile  = %s\n' 'staging'
  printf '  sync-untracked   = %s\n' 'false'
  printf '  bin-dir          = %s\n' './bin'
  printf '\n  %s %s\n' "$(c "$CL_ICON" "$G_PERSON")" "$(cbold "$CL_SECTION" 'profile staging')"
  printf '    host  =  %s\n' "$HOST"
  printf '    path  =  %s\n' "$PATH_"
  printf '\n  %s %s\n' "$(c "$CL_ICON" "$G_SYNC")" "$(cbold "$CL_SECTION" 'last sync')"
  printf '    %s  %s\n\n' '2026-06-18 09:14:22 -06' "$(c "$CL_DESC" '(hace 2 horas)')"
}

_sim_status() {
  # brief checking spinner, then an out-of-sync report
  local i
  for i in 1 2 3 4 5; do
    printf '\r\033[K  %s' "$(c "$CL_SEP" "Checking $i/5...")"; sleep 0.18
  done
  printf '\r\033[K'
  printf 'Status against %s:%s\n' "$HOST" "$PATH_"
  printf '  %s (%d total)\n\n' '2 differ, 1 missing remotely' 3
  printf '  %s %s\n' "$(c "$CL_DIFF" '!=')" 'src/server.go'
  printf '  %s %s\n' "$(c "$CL_DIFF" '!=')" 'config/app.yml'
  printf '  %s %s\n' "$(c "$CL_ERR2" '??')" 'internal/api/handler.go'
}

_sim_pull() {
  sleep 0.3
  printf '  %s %s\n'  "$(c "$CL_OK" '✓')"    'src/server.go';        sleep 0.25
  printf '  %s %s\n'  "$(c "$CL_OK" '✓')"    'config/app.yml';       sleep 0.25
  printf '  %s %s\n'  "$(c "$CL_DIFF" '-')"  'legacy/old_client.go'; sleep 0.25
  printf '  %s %s\n'  "$(c "$CL_OK" '✓')"    'web/assets/styles.css'; sleep 0.3
  printf '\nPulled %d file(s) from %s:%s\n' 3 "$HOST" "$PATH_"
}

_sim_sync() {
  local total=${#_CHANGED[@]} bw=58 sl; sl=$(sep_line 72)
  local n i p ic secs
  for ((n = 0; n <= total; n++)); do
    clear_screen
    printf '\n  %s\n\n' "$(c "$CL_HEADER" "Syncing $total file(s) to $HOST:$PATH_")"
    for ((i = 0; i < n; i++)); do
      IFS='|' read -r p ic <<< "${_CHANGED[i]}"
      printf '  %s %s %s\n' "$(c "$CL_OK" '✓')" "$(c "$CL_ICON" "$ic")" "$p"
    done
    for ((i = n; i < total; i++)); do printf '\n'; done
    secs=$(( n ))
    printf '  %s\n' "$(c "$CL_SEP" "$sl")"
    render_bar "$n" "$total" "$bw" "$secs"; printf '\n'
    printf '  %s\n' "$(c "$CL_SEP" "$sl")"
    sleep 0.34
  done
  sleep 0.8
}

_sim_ship() {
  local bin='mycli' host='web@prod-1' dest='/usr/local/bin' os='linux'
  local steps=(
    "$G_UPLOAD  uploading   $bin"
    "$G_RENAME  renaming    $bin → mycli-linux-amd64"
    "$G_CHMOD  chmod +x"
    "$G_MOVE  moving   → $host:$dest"
  )
  local n=${#steps[@]} spin=(⠋ ⠙ ⠹ ⠸ ⠼ ⠴ ⠦ ⠧ ⠇ ⠏)
  local active=-1 j done_to=-1 f=0
  local dtimes=('' '' '' '')   # per-step elapsed, filled as each completes
  draw() { # $1 = inline extra for the active step
    clear_screen
    printf '\n  %s\n' "$(c "$CL_HEADER" "$G_SHIP  Shipping $bin → $host:$dest")"
    for ((j = 0; j < n; j++)); do
      if   (( j <= done_to )); then
        printf '  %s  %s  %s\n' "$(c "$CL_OK" '✓')" "$(c "$CL_OK" "${steps[j]}")" "$(c "$CL_SEP" "${dtimes[j]}")"
      elif (( j == active )); then
        printf '  %s  %s%s\n' "$(c "$CL_GOLD" "${spin[f % 10]}")" "$(c "$CL_GOLD" "${steps[j]}")" "$1"
      else
        printf '  %s  %s\n' "$(c "$CL_SEP" '·')" "$(c "$CL_SEP" "${steps[j]}")"
      fi
    done
  }
  # step 0: upload with progress bar
  active=0
  local w
  for w in 18 42 71 98 125; do
    local pct=$(( w*100/125 ))
    local extra="  $(c "$CL_BAR" "$(printf '[%-32s]' "$(printf '=%.0s' $(seq 1 $((w*32/125))))")")  ${w}.0 MB / 125.0 MB  ${pct}%"
    draw "$extra"; f=$((f+1)); sleep 0.22
  done
  done_to=0; dtimes[0]='248ms'; draw ''; sleep 0.25
  # step 1: rename
  active=1; draw ''; sleep 0.3; done_to=1; dtimes[1]='8ms'; draw ''; sleep 0.2
  # step 2: chmod
  active=2; draw ''; sleep 0.3; done_to=2; dtimes[2]='6ms'; draw ''; sleep 0.2
  # step 3: move
  active=3; draw ''; sleep 0.4; done_to=3; dtimes[3]='17ms'; draw ''; sleep 0.3
  printf '\n  %s  %s → %s\n' "$(c "$CL_OK" "$G_SENT  shipped")" \
    "$(cbold 255 'mycli-linux-amd64')" "$(cbold 255 "$host:$dest")"
  printf '  %s  %s\n' "$(c "$CL_SEP" "$os")" "$(c "$CL_SEP" '279ms')"
  sleep 0.8
}

_sim_clean() {
  printf '\n  %s\n\n' "$(c "$CL_HEADER" "Clean $HOST:$PATH_ (HEAD 4cc4d16)")"
  printf '  %s\n' "$(c "$CL_DIFF" 'Will revert (2 modified):')"
  printf '    %s %s\n' "$(c "$CL_ICON" "$G_FILE")" 'src/server.go'
  printf '    %s %s\n' "$(c "$CL_ICON" "$G_FILE")" 'config/app.yml'
  printf '  %s\n' "$(c "$CL_ERR2" 'Will remove (1 untracked):')"
  printf '    %s %s\n' "$(c "$CL_ERR2" "$G_DELETE")" 'tmp/scratch.log'
  printf '\n  %s    %s\n' "$(c "$CL_OK" '[ y / enter ] confirm')" "$(c "$CL_SEP" '[ n / esc / q ] cancel')"
  sleep 1.4
  printf '%s\n' 'y'
  sleep 0.4
  printf '\n%s\n' "$(c "$CL_OK" "✓ cleaned $HOST:$PATH_")"
  printf '  %s\n' 'reverted: 2 file(s)'
  printf '  %s\n' 'removed:  1 file(s)'
  sleep 0.6
}

_sim_status_ok() { :; }

_sim_init() {
  # 1) host picker
  clear_screen
  printf '\n  %s\n\n' "$(cbold 230 ' Select SSH Host ')" | sed 's/^/ /'
  printf '  %s\n' "$(c 212 '▶') $(c "$CL_ICON" "$G_SERVER") vps-staging   (deploy@198.51.100.20:22)"
  printf '    %s\n' "$(c "$CL_ICON" "$G_SERVER") prod-1        (web@203.0.113.8:22)"
  printf '    %s\n' "$(c "$CL_ICON" "$G_SERVER") homelab       (pi@192.168.1.40:22)"
  printf '\n  %s\n' "$(c "$CL_SEP" '↑/↓ navigate   enter select   / filter   q quit')"
  sleep 1.6
  # 2) remote dir browser
  clear_screen
  printf '\n  %s\n' "$(cbold "$CL_TITLEBLUE" '  Remote Directory Browser')"
  printf '  %s\n' "$(c 86 '/srv/app')"
  printf '  %s\n\n' "$(c "$CL_SEP" 'filter...')"
  printf '  %s %s %s\n' "$(cbold 212 '▶')" "$(c "$CL_ICON" "$G_FOLDER")" 'src'
  printf '    %s %s\n' "$(c "$CL_SEP" "$G_FOLDER")" "$(c "$CL_SEP" 'config')"
  printf '    %s %s\n' "$(c "$CL_SEP" "$G_FOLDER")" "$(c "$CL_SEP" 'web')"
  printf '\n  %s\n' "$(c "$CL_SEP" '↑/↓ navigate  tab/→=descend  ←=up  enter=select  q=quit')"
  printf '  %s\n' "$(c 86 'Selected: /srv/app')"
  sleep 1.6
  # 3) name + success
  clear_screen
  printf '\n%s\n' "$(c "$CL_OK" 'Sync profile "staging" configured:')"
  printf '  host: %s\n' 'vps-staging'
  printf '  path: %s\n' "$PATH_"
  sleep 0.8
}

_sim_shell() {
  printf '%s\n' "$(c "$CL_SEP" 'Connecting to vps-staging...')"
  sleep 0.6
  local p; p="$(c 82 'deploy@vps-staging')$(c 252 ':')$(c 39 "$PATH_")$(c 252 '$') "
  printf '%b' "$p"; sleep 0.5; printf 'ls\n'; sleep 0.3
  printf '%s\n' 'Dockerfile  config  internal  scripts  src  web'
  printf '%b' "$p"; sleep 0.5; printf 'tail -n2 logs/app.log\n'; sleep 0.4
  printf '%s\n' '12:04:51 INFO  server listening on :8080'
  printf '%s\n' '12:04:53 INFO  connected to postgres'
  printf '%b' "$p"; sleep 0.6; printf 'exit\n'; sleep 0.3
  printf '%s\n' "$(c "$CL_SEP" 'Connection to vps-staging closed.')"
}

_sim_beam() {
  # 1) commit picker — two already sent, two unsent (pre-selected)
  clear_screen
  printf '\n  %s\n\n' "$(cbold "$CL_TITLEBLUE" 'Local commits ahead of upstream')"
  printf '  %s %s %s  %s  %s\n' "$(cbold 212 '▶')" "$(c "$CL_OK" '☑')" "$(cbold 212 'a1b2c3d')" 'Add rate limiter to API'      "$(c "$CL_SEP" '2 hours ago')"
  printf '    %s %s  %s  %s\n'  "$(c "$CL_OK" '☑')" "$(cbold 212 'e4f5a6b')" 'Fix nil deref in handler'     "$(c "$CL_SEP" '5 hours ago')"
  printf '    %s %s %s  %s  %s\n' "$(c "$CL_OK" '☐')" "$(c "$CL_OK" "$G_SENT")" "$(c "$CL_SEP" '9c8d7e6')" "$(c "$CL_SEP" 'Bump deps')" "$(c "$CL_SEP" '1 day ago')"
  printf '\n  %s\n' "$(c "$CL_SEP" 'tab=toggle  a=all  u=unsent  enter=confirm  ctrl+c=quit')"
  sleep 1.8
  # 2) send view, grouped by commit
  local sl; sl=$(sep_line 72)
  local g1=("src/api/limiter.go|$G_GO" "config/app.yml|$G_YML")
  local g2=("internal/api/handler.go|$G_GO")
  local files=("${g1[@]}" "${g2[@]}") total=3
  draw_beam() { # done_count
    clear_screen
    printf '\n  %s\n\n' "$(c "$CL_HEADER" "Beaming $total file(s) to $HOST:$PATH_")"
    local idx=0 p ic done=$1
    printf '  %s %s %s\n' "$(c 39 "$G_CUBE[a1b2c3d]")" "$(c "$CL_SEP" '─')" "$(c "$CL_HEADER" 'Add rate limiter to API')"
    for f in "${g1[@]}"; do
      IFS='|' read -r p ic <<< "$f"
      if (( idx < done )); then printf '      %s %s %s\n' "$(c "$CL_OK" '✓')" "$(c "$CL_ICON" "$ic")" "$p"
      else printf '      %s %s\n' "$(c "$CL_SEP" '·')" "$(c "$CL_SEP" "$p")"; fi
      idx=$((idx+1))
    done
    printf '  %s %s %s\n' "$(c 45 "$G_CUBE[e4f5a6b]")" "$(c "$CL_SEP" '─')" "$(c "$CL_HEADER" 'Fix nil deref in handler')"
    for f in "${g2[@]}"; do
      IFS='|' read -r p ic <<< "$f"
      if (( idx < done )); then printf '      %s %s %s\n' "$(c "$CL_OK" '✓')" "$(c "$CL_ICON" "$ic")" "$p"
      else printf '      %s %s\n' "$(c "$CL_SEP" '·')" "$(c "$CL_SEP" "$p")"; fi
      idx=$((idx+1))
    done
    printf '  %s\n' "$(c "$CL_SEP" "$sl")"
    render_bar "$done" "$total" 58 "$done"; printf '\n'
    printf '  %s\n' "$(c "$CL_SEP" "$sl")"
  }
  local d
  for d in 0 1 2 3; do draw_beam "$d"; sleep 0.45; done
  sleep 0.8
}

# --- dispatcher -----------------------------------------------------------
teleport() {
  local cmd="${1:-}"; shift || true
  case "$cmd" in
    ""|-h|--help|help) _sim_help ;;
    version|-V)        _sim_version ;;
    profiles|-p)       _sim_profiles ;;
    config)            _sim_config ;;
    status)            _sim_status ;;
    sync|-s|-su)       _sim_sync ;;
    pull)              _sim_pull ;;
    ship)              _sim_ship ;;
    clean)             _sim_clean ;;
    init|-i)           _sim_init ;;
    shell)             _sim_shell ;;
    beam|-b)           _sim_beam ;;
    *)                 _sim_help ;;
  esac
}
