# Unit 19: Headless mode — `--no-input` + salida `--json`

## Goal

Hacer que **todo** `teleport` sea operable sin terminal interactiva, para
poder encadenarlo desde scripts, CI y pipelines de despliegue. Hoy los
comandos que abren un TUI —el commit picker de `beam` (Units 04/13), el
file-diff viewer de `beam` (Unit 18) y la confirmación de `clean`
(Unit 07)— **cuelgan o se rompen** cuando no hay TTY. Esta unidad añade:

1. Un flag **persistente `--no-input`** (más autodetección de TTY) que
   convierte cualquier prompt/picker/viewer en un camino no-interactivo:
   o se resuelve desde flags, o **falla cerrado** (exit `2`) con un
   mensaje que nombra el flag que faltó — **nunca** se queda esperando.
2. Un flag **persistente `--json`** que emite un único objeto JSON con el
   resultado (drift, archivos enviados, resumen) en `stdout`, mandando
   todo lo decorativo a `stderr`, para que el llamador **parsee** en vez
   de raspar ANSI.
3. **Exit codes documentados y estables** para ramificar con confianza.

`beam -a --no-input` = mandar los commits no enviados de punta a punta sin
tocar una tecla. `teleport status --json` = drift parseable. Invocar
`teleport` en una terminal real se comporta **exactamente igual que hoy**.

## Design

### El guard de interactividad

Un único helper decide si se puede abrir un TUI:

```go
// interactive reports whether teleport may open a prompt/picker/viewer:
// a real TTY on stdin AND the user did not force --no-input.
func interactive() bool {
    return !noInput && term.IsTerminal(int(os.Stdin.Fd()))
}
```

Dos disparadores hacia el modo headless, no uno:

- **`--no-input` explícito** — fuerza headless aunque haya TTY (útil para
  probar el camino de script desde la terminal).
- **Sin TTY** (stdin/stdout redirigidos, como en CI o cuando otro proceso
  lanza `teleport`) — headless automático aunque no se pase `--no-input`.

Así, un humano en su terminal sigue viendo pickers; un script (sin TTY o
con `--no-input`) nunca ve uno. El guard vive junto al `rootCmd` y lo
consultan todos los call sites de TUI, sin propagar un booleano por cada
`Opts`.

Sentinela nuevo, mapeado a exit `2`:

```go
// ErrNeedsTTY is returned when a command would need an interactive
// selection or confirmation but is running headless (no TTY or
// --no-input). The caller maps it to exit code 2 so scripts must be
// explicit.
var ErrNeedsTTY = errors.New("requires a terminal; pass the flag explicitly to run headless")
```

### Comportamiento headless por comando

| Comando | Interactivo (hoy) | Headless (`--no-input` / sin TTY) |
| ------- | ----------------- | --------------------------------- |
| `sync`  | ya no-interactivo | sin cambios (solo honra `--json`) |
| `status`| ya no-interactivo | sin cambios (solo honra `--json`) |
| `beam`  | commit picker + file review | **requiere `-a`**; sin él → `ErrNeedsTTY`. Con `-a` auto-selecciona los commits no enviados y **omite** el file-diff viewer (Unit 18), enviando directo. |
| `clean` | TUI de confirmación | `--no-input` **implica `-y`**; sin TTY y sin `-y`/`--no-input` → `ErrNeedsTTY` (nunca descarta sin confirmación explícita). |
| `beam -c` | confirmación de clean | misma regla que `clean`: la fase clean se auto-confirma solo con `-y`/`--no-input`. |
| `pull`  | (según Unit 12) | si abre confirmación, misma regla; honra `--json`. |

**Decisión de seguridad (beam):** `--no-input` **no** implica `-a`
silenciosamente — eso arriesgaría enviar commits no deseados. Headless sin
`-a` es un error explícito. El usuario debe pedir `-a` (auto-seleccionar
no-enviados) a propósito. El file-diff viewer, en cambio, es solo revisión
visual: headless se **omite** sin pérdida de corrección.

### Salida `--json`

Con `--json`, cada comando imprime **un** objeto JSON en `stdout` y manda
logs/decoración a `stderr`. Formas mínimas:

`status`:
```json
{
  "target": "staging:/var/www/myproj",
  "in_sync": false,
  "total": 146,
  "drift": [
    {"path": "cmd/beam.go", "state": "differ"},
    {"path": "new-file.go", "state": "missing_remote"},
    {"path": "docs/guide.md", "state": "missing_local"}
  ]
}
```
(`state` ∈ `differ` `!=`, `missing_remote` `??`, `missing_local` `--`.)

`sync` / `beam` / `clean`:
```json
{"command": "sync", "target": "staging:/var/www/myproj", "sent": 4, "files": ["a.py", "b.xml"]}
{"command": "beam", "target": "...", "commits": ["a1b2c3d","e4f5a6b"], "sent": 7, "files": [...]}
{"command": "clean", "target": "...", "reverted": 3, "removed": 2, "restored": 1}
```

`--json` implica no-interactividad de salida pero **no** salta prompts por
sí solo — combínalo con `--no-input`/`-a`/`-y` para un headless completo.
Con `--json`, un `ErrNeedsTTY` también se serializa:
`{"error": "requires a terminal…", "hint": "pass -a"}` en `stderr` + exit `2`.

### Exit codes (contrato)

| Code | Significado |
| ---- | ----------- |
| `0`  | éxito / sin drift |
| `1`  | error de ejecución, o drift detectado (`status`) |
| `2`  | uso / haría falta un TTY (`ErrNeedsTTY`, selección o confirmación no resuelta desde flags) |

## Implementation

### `cmd/root.go` — flags persistentes + estado

```go
var (
    noInput  bool
    jsonOut  bool
)

func init() {
    rootCmd.PersistentFlags().BoolVar(&noInput, "no-input", false,
        "never prompt; resolve from flags or fail (exit 2)")
    rootCmd.PersistentFlags().BoolVar(&jsonOut, "json", false,
        "print a single JSON result object to stdout")
}
```

`interactive()` y el sentinela `ErrNeedsTTY` en un archivo nuevo
`cmd/headless.go` (mismo paquete `cmd`, para que todos los comandos lo
usen sin import cycle). Ahí también un helper de salida:

```go
// emit prints v as JSON when --json is set; otherwise calls human.
func emit(v any, human func()) { ... }
```

### `cmd/beam.go`

- Antes de lanzar el commit picker: `if !interactive() && !beamAuto { return ErrNeedsTTY-wrapped }` con hint `"pass -a to auto-select unsent commits"`.
- Cuando `!interactive()`: saltar el file-diff viewer (Unit 18) — resolver la lista de archivos como hoy pero **sin** abrir el `tea.Program`; ir directo al envío.
- Fase clean (`-c`): pasar `beamYes || noInput` como "auto-confirm" a `cleanRemote` (ver Unit 07) y, si `!interactive()` y no auto-confirm, devolver `ErrNeedsTTY`.
- Al final, `emit(beamResult{…}, printHumanSummary)`.

### `cmd/clean.go`

- `runClean`: `autoConfirm := cleanYes || noInput`. Si hay cambios y
  `!interactive()` y `!autoConfirm` → `ErrNeedsTTY` (hint `"pass -y"`).
  Si `autoConfirm`, ejecutar sin lanzar `RunCleanConfirm`.
- `emit(cleanResult{reverted, removed, restored}, printHumanSummary)`.

### `cmd/status.go`

- Reusar la clasificación existente; cuando `jsonOut`, construir el
  struct de drift y `json.Marshal` a `stdout`, logs a `stderr`. Exit `1`
  si hay drift (igual que hoy), `0` si `in_sync`.

### `cmd/sync.go`, `cmd/pull.go`

- Envolver el resumen final en `emit(...)`. `pull`: si tiene un prompt,
  aplicar el mismo guard `interactive()`.

### `cmd/help.go`

Documentar los flags globales en la sección de Global Flags:

```go
{"", "--no-input", iconSync, "never prompt; fail closed instead (exit 2)"},
{"", "--json",     iconSync, "machine-readable result on stdout"},
```

### Autodetección de TTY

`golang.org/x/term.IsTerminal`. Si aún no está en `go.mod` como dependencia
directa, agregarla (ya suele venir indirecta vía `x/crypto/ssh`). No
introduce TUI nueva.

### Documentación

- `CHANGELOG.md` bajo *Unreleased*:
  - `[ADD] --no-input: run any command headless, failing closed instead of prompting`
  - `[ADD] --json: machine-readable result for status/sync/beam/clean`
- `context/architecture.md`: registrar el guard `interactive()`, la regla
  "sin TTY / --no-input ⇒ nunca abrir TUI", y el contrato de exit codes.
- `context/progress-tracker.md`: marcar Unit 19 al cerrarla.

## Dependencies

Ninguna nueva de peso: `golang.org/x/term` (detección de TTY) y
`encoding/json` (stdlib). Reusa `rootCmd`, los comandos y el flujo de
`cleanRemote` (Unit 07).

## Verify when done

- [ ] `teleport status --json` imprime un objeto JSON con `in_sync`,
      `total` y `drift[]` en `stdout` y nada decorativo lo contamina.
- [ ] `teleport sync -u --json` reporta `sent` y `files[]` en JSON.
- [ ] `teleport beam -a --no-input` envía los commits no enviados sin
      abrir el commit picker ni el file-diff viewer, y termina `0`.
- [ ] `teleport beam --no-input` (sin `-a`) sale `2` con un mensaje que
      menciona `-a`, y **no** envía nada.
- [ ] `teleport clean --no-input` (o `-y`) descarta sin TUI; `teleport
      clean` sin TTY y sin `-y`/`--no-input` sale `2` y **no** toca el
      remoto.
- [ ] `teleport beam -cs --no-input` corre clean → beam → sync headless,
      auto-confirmando el clean, en una sola sesión SSH.
- [ ] Con stdin redirigido (sin TTY) y sin `--no-input`, los comandos se
      comportan como headless automáticamente (ningún TUI se abre).
- [ ] En una terminal real y sin flags, `beam`/`clean` siguen mostrando
      su picker/confirmación como hoy (cero regresión interactiva).
- [ ] Exit codes: `0` sin drift, `1` con drift/errores, `2` cuando faltó
      un flag para resolver sin TTY.
- [ ] `--json` + `ErrNeedsTTY` serializa `{"error":…,"hint":…}` a `stderr`
      con exit `2`.
- [ ] `teleport help` lista `--no-input` y `--json`.
- [ ] `go build ./...` y `go vet ./...` pasan.
