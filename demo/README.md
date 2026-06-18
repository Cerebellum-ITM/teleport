# demo — README GIFs (VHS)

The animated GIFs in the project README are **simulations**, recorded with
[charmbracelet/vhs](https://github.com/charmbracelet/vhs). They run no network
and read no real config: every host, path, commit, and file shown is invented
(`deploy@vps-staging:/srv/app`, a toy repo). This keeps the GIFs reproducible
and free of any private/client data while staying byte-faithful to the real
CLI's colors, Nerd Font glyphs, progress bars, and pickers.

## How it works

- `sim/teleport-sim.sh` defines a `teleport()` shell function that renders each
  command's output. VHS `source`s it, so the typed command on screen is the
  real `teleport sync` / `teleport beam` / … while the output is simulated.
- `sim/lib.sh` holds the shared color/glyph/progress-bar helpers (colors mirror
  the `lipgloss.Color()` codes in `internal/tui/*.go` and `cmd/*.go`).
- `sim/glyphs.sh` is **auto-generated** from the Go source so the Nerd Font
  glyphs are byte-for-byte identical to the real CLI. Regenerate it with
  `bash sim/gen-glyphs.sh > sim/glyphs.sh` whenever an icon changes.
- `tapes/_setup.tape` holds the shared VHS settings (font, theme, size) and the
  hidden shell prep; each `tapes/<cmd>.tape` `Source`s it and types one command.

## Requirements

```sh
brew install vhs ttyd ffmpeg     # vhs needs ttyd + ffmpeg
```

A [Nerd Font](https://www.nerdfonts.com/) must be installed (the tapes use
`JetBrainsMono Nerd Font Mono`) so the glyphs render.

## Regenerate

Run from the **repo root** (paths in the tapes are repo-relative):

```sh
# one GIF
vhs demo/tapes/sync.tape

# all of them
for t in main version profiles config status pull clean ship shell init beam help; do
  vhs demo/tapes/$t.tape
done
```

Output lands in `demo/gifs/`. If you add or rename a command, add a matching
`_sim_<cmd>` function in `sim/teleport-sim.sh`, a `tapes/<cmd>.tape`, and embed
the new GIF in the root `README.md`.
