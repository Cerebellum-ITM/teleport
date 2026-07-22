# Unit 23: Actions — comandos remotos con streaming de logs

## Goal

Permitir automatizar procesos post-despliegue: definir **actions** con nombre
(comandos remotos, ej. `deploy`, `restart`, `logs`) por perfil, ejecutarlas
standalone (`teleport run deploy`) o encadenadas a un transfer
(`sync`/`beam`/`mirror` + `--then deploy`), con la salida del proceso remoto
**streameada línea a línea** en vivo. Las actions no están amarradas al
directorio del perfil: cada una declara su propio `cwd` (o ninguno).

## Design

### Modelo: qué es una action

Una action es un comando (o lista ordenada de pasos) que corre en el host del
perfil vía SSH. Vive en el config global, anidada bajo su perfil — hereda host
y, por default, el path del perfil:

```toml
[profiles.staging]
host = "vps-staging"
path = "/var/www/app"

[profiles.staging.actions.deploy]
run = [
  "composer install --no-dev",
  "php artisan migrate --force",
  "sudo systemctl restart app.service",
]
cwd = "/var/www/app"   # opcional; default = path del perfil; "" = sin cd
timeout = "5m"         # opcional; default "10m"
confirm = true         # opcional; pide confirmación antes de ejecutar

[profiles.staging.actions.logs]
run = ["journalctl -u app.service -n 100 --no-pager"]
```

Reglas de ejecución:

- Los pasos de `run` se ejecutan **en orden**; el primer paso que sale
  non-zero **aborta** la action y propaga su exit code.
- Cada paso corre en una sesión SSH nueva sobre la **misma conexión** ya
  abierta (patrón existente de `RunCommand`); con `cwd` no vacío el paso se
  envuelve en `cd <cwd-quoted> && <paso>`.
- `timeout` aplica **por paso** (context deadline); al vencer se cierra la
  sesión y la action falla.
- `confirm = true` pide confirmación interactiva antes de correr; headless se
  auto-confirma solo con `-y`/`--no-input` (misma regla que `clean`, Unit 19).

**Decisión de diseño:** las actions se definen **por perfil en el config
global**, no en un archivo del proyecto — preserva el modelo de storage
(los proyectos no cargan archivos de teleport) y una action siempre sabe su
host sin resolución extra.

### Invocación standalone: `teleport run`

```
teleport run <action> [profile]   # ejecuta; perfil default si se omite
teleport run --list [profile]     # lista actions del perfil (nombre, pasos, cwd)
```

Encadenable: `teleport run deploy logs` ejecuta varias actions en orden,
abortando en la primera que falle.

### Invocación encadenada: flag `--then`

`sync`, `beam` y `mirror` ganan un flag repetible `--then <action>`
(`-t` corto). Las actions corren **solo si el transfer terminó sin fallos**
(cero uploads fallidos / mirror aplicado), en el orden dado, reusando la
conexión SSH del transfer. Si una action falla, el comando sale `1` aunque el
transfer haya sido exitoso — el JSON distingue ambas fases (ver abajo).

Sin hooks automáticos en config en esta unidad (`after_sync = [...]` queda
explícitamente fuera de alcance): `--then` explícito es predecible y no
sorprende en un transfer casual.

### Streaming de logs

Nuevo método en `internal/ssh`, base de toda la unidad:

```go
// RunCommandStream executes cmd on a fresh SSH session and delivers each
// stdout/stderr line via onLine as it arrives. Returns the remote exit
// code (0 on success). ctx cancels/times out the session.
func (c *Client) RunCommandStream(ctx context.Context, cmd string,
    onLine func(source Source, line string)) (int, error)
```

Implementación: `sess.StdoutPipe()` + `sess.StderrPipe()`, una goroutine con
`bufio.Scanner` por pipe, líneas al callback (serializadas con un mutex o
canal — el callback nunca se llama concurrente). **Sin PTY**: la salida queda
limpia y sin ANSI de progreso; los comandos remotos detectan no-TTY y logean
plano (trade-off aceptado: no se ven barras de progreso interactivas).
Exit code non-zero se extrae de `*ssh.ExitError` y **no** es error de
transporte: se devuelve `(code, nil)`; error real solo por sesión/red/timeout.

### Presentación (contrato `useTUI()` de Unit 19)

**Decisión de implementación:** en vez de un modelo bubbletea con viewport, la
presentación es un **streaming-printer** (`internal/tui/ActionSink`) que imprime
las líneas conforme llegan, sin pantalla alterna. Razón: los logs de un proceso
remoto son de tamaño desconocido y el objetivo es *verlos en vivo*; un viewport
en alt-screen pelearía con el scrollback natural de la terminal y con salidas
largas. Un printer da streaming real, más simple y robusto, y el mismo `exec`
sirve a ambos caminos (rico/plano) cambiando solo estilo y destino.

- **TUI interactivo** (`useTUI()` true) — `ActionSink` colorea a `stdout`:
  header con `HeaderBg`/`HeaderFg` (`action deploy · h:/path`), marcador de paso
  en `Gold` (`▸ step 2/3` + comando dim), líneas en vivo (`stdout` en `Text`,
  `stderr` en `Warn`), y cierre por paso con `✓`/`✗` (`Success`/`Danger`) +
  duración. La cancelación por timeout/`ctrl+c` cierra la sesión remota vía
  `context`. El I/O SSH vive en el caller (invariante #3 intacto).
- **Plano / headless** — mismas llamadas, `stdout`→`stderr`, sin color, líneas
  con prefijo `[<action>] `, estilo `RunSyncPlain`.
- **Confirmación** — `tui.RunActionConfirm` (modelo bubbletea simple y/n) para
  actions con `confirm = true` en modo interactivo.
- **`--json`** — logs en vivo a `stderr` (no se pierden); al final un objeto
  en `stdout`:

```json
{"command":"run","profile":"staging","actions":[
  {"name":"deploy","steps":3,"completed":3,"exit_code":0,"duration_ms":42180}
]}
```

En `sync`/`beam`/`mirror` con `--then`, el resultado JSON existente gana un
campo `actions` con la misma forma anidada, para que un script distinga
"subió pero no deployó".

### Gestión interactiva: `teleport actions`

```
teleport actions add [profile]     # wizard interactivo
teleport actions list [profile]    # tabla: nombre, pasos, cwd, confirm
teleport actions edit <name>       # wizard pre-llenado con valores actuales
teleport actions remove <name>     # con confirmación
```

Wizard de `add`/`edit` (formulario `huh`, mismo estilo que `init`):

1. **Nombre** — valida no-vacío, sin espacios, único en el perfil (en `edit`
   se permite conservar el propio).
2. **Pasos** — `huh.NewText` multilínea, un comando por línea; líneas vacías
   se descartan; mínimo 1 paso.
3. **Directorio de trabajo** — select de 3 opciones: usar el path del perfil
   (default) / navegar el remoto con el **dir browser SFTP existente** de
   `init` / sin directorio (`cwd = ""`).
4. **Opciones** — toggle `confirm`, input de `timeout` (placeholder `10m`,
   valida `time.ParseDuration`).
5. **Resumen** — muestra la action completa y ofrece: guardar / **probar
   ahora y guardar** / cancelar. "Probar ahora" reusa la conexión SSH ya
   abierta (la del dir browser, o abre una) y corre la action con el
   `actionrunner`; si falla, ofrece volver a editar pasos en vez de guardar
   a ciegas.

Config se escribe **solo al confirmar el resumen** — `Esc` a media captura no
toca nada (invariante #4). `actions add` headless se resuelve por flags
(`--run` repetible, `--cwd`, `--timeout`, `--confirm`) o falla con
`ErrNeedsTTY` (exit `2`); `remove` sin TTY exige `-y`.

### Exit codes

Sin cambios al contrato: `0` éxito; `1` action falló (incluye "transfer OK,
action falló") o error de ejecución; `2` `ErrNeedsTTY` (action con
`confirm = true` sin `-y` headless, wizard sin flags, o action inexistente
resoluble solo por picker).

## Implementation

### `internal/config/config.go` — tipo `Action`

```go
// Action is a named remote command sequence attached to a profile.
type Action struct {
    Run     []string `toml:"run"`
    Cwd     *string  `toml:"cwd,omitempty"`     // nil = profile path, "" = no cd
    Timeout string   `toml:"timeout,omitempty"` // Go duration, default "10m"
    Confirm bool     `toml:"confirm,omitempty"`
}
```

- `Profile` gana `Actions map[string]Action \`toml:"actions,omitempty"\``.
- Validación en `LoadGlobal`: cada action con `len(Run) > 0`, `Timeout`
  parseable si no-vacío; error con nombre de perfil y action.
- Helpers `(*GlobalConfig).SetAction/RemoveAction(profile, name, ...)`
  (patrón `SetProfile`/`SetBinProfile`).
- Método `(*Action).EffectiveCwd(profilePath string) string` y
  `(*Action).EffectiveTimeout() time.Duration`.

### `internal/ssh/client.go` — `RunCommandStream`

- Firma de arriba; `type Source int` con `SourceStdout`/`SourceStderr`.
- Cancela vía `ctx`: goroutine que en `<-ctx.Done()` hace `sess.Close()`.
- `*ssh.ExitError` → `(exitStatus, nil)`; otros errores → `(‑1, err)`.

### `cmd/run.go` — comando standalone + motor compartido

- `runCmd` cobra: `Use: "run <action>... [profile]"` — el último arg que
  coincida con un perfil existente se toma como perfil; el resto son actions.
  Flag `--list`. Resolución de perfil idéntica a `sync`.
- Motor compartido exportado dentro del paquete:

```go
// executeActions runs the named actions in order over client, streaming
// output per the useTUI()/plain/json contract. Returns per-action results
// and the first failure.
func executeActions(client *ssh.Client, profile config.Profile,
    profileName string, names []string) ([]actionResult, error)
```

  - Valida que toda action exista **antes** de correr la primera (fail fast,
    lista las disponibles en el error).
  - `confirm = true`: `interactive()` → confirmación TUI simple (patrón
    `cleanconfirm`); headless → requiere `yes || noInput`, si no `ErrNeedsTTY`
    con hint `"pass -y"`.
  - Por paso: construye `cd ... && paso` según `EffectiveCwd`, context con
    `EffectiveTimeout`, llama `RunCommandStream`, alimenta el actionrunner o
    el path plano.
- `emit(runResult{...}, humanSummary)` al final.

### `internal/tui/actionrunner.go`

- Modelo bubbletea nuevo según Design. API estilo `RunShipProgress`:
  `RunActionProgress(header string, steps []string, exec func(emit func(...)))`,
  más `RunActionPlain` para el camino headless (prefijo `[name] ` a stderr).
- Íconos: reusar `iconSyncOK`/`iconSyncFail`; paso activo con spinner
  (`bubbles/v2/spinner`) en `Gold`. Colores solo vía `internal/theme`.
- `View()` devuelve `tea.NewView(s)`.

### `cmd/actions.go` — gestión

- `actionsCmd` con subcomandos `add`/`list`/`edit`/`remove` según Design.
- Wizard con `huh` (patrón `init.go`); paso de cwd reusa
  `tui.RunDirPicker` con la conexión del perfil.
- Escritura vía `config.SaveGlobal` únicamente al confirmar el resumen.
- Headless: `add` desde flags o `ErrNeedsTTY`; `list` siempre funciona
  (`--json` → array de actions); `remove` exige `-y` sin TTY.

### `cmd/sync.go`, `cmd/beam.go`, `cmd/mirror.go` — flag `--then`

- `--then/-t` `StringArrayVar` en los tres comandos.
- Al final del flujo exitoso (cero `failed` / mirror aplicado), si hay
  actions: llamar `executeActions` **reusando el client ya conectado**, antes
  del `emit` final; anidar `actions` en el result JSON del comando.
- Validación temprana: los nombres de `--then` se verifican contra el perfil
  **antes** de iniciar el transfer (no descubrir el typo después de subir).

### `cmd/help.go` y docs

- Registrar `run` y `actions` en el help estilizado; documentar `--then` en
  los comandos de transfer.
- `README.md`: sección "Actions" (definición TOML, wizard, `run`, `--then`,
  streaming, contrato JSON).
- `CHANGELOG.md` bajo *Unreleased*:
  - `[ADD] actions: named remote command sequences per profile with live log streaming`
  - `[ADD] run: execute profile actions standalone`
  - `[ADD] sync/beam/mirror: --then flag to chain actions after a successful transfer`
- `context/architecture.md`: nueva sección Actions (modelo, invariante de
  ejecución, streaming sin PTY); `context/ui-context.md`: patrón del
  actionrunner; `context/progress-tracker.md`: marcar Unit 23 al cerrar.

## Dependencies

Ninguna nueva: `context` (stdlib) para timeout/cancel, `bufio` para el
scanner, y reuso de `huh`, `bubbles/v2/{viewport,spinner}`, `internal/theme`,
el dir browser de `init`, `interactive()`/`emit` (Unit 19) y el patrón de
conexión de `connectToProfile`.

## Verify when done

- [ ] Una action definida a mano en `config.toml` corre con
      `teleport run deploy` y sus líneas aparecen **en vivo** (no en bloque
      al final) en el actionrunner; al terminar muestra ✓/✗ y duración por
      paso.
- [ ] Un paso que falla (exit non-zero) aborta los pasos restantes, la
      action reporta ✗ y el comando sale `1`.
- [ ] `run` de un paso que excede `timeout` cancela la sesión remota y
      falla con mensaje de timeout.
- [ ] `cwd` funciona en sus tres modos: omitido (usa path del perfil),
      absoluto distinto, y `""` (sin `cd`).
- [ ] `teleport sync --then deploy` corre la action solo tras un sync sin
      fallos, sobre la misma conexión; con typo en `--then` falla **antes**
      de subir nada.
- [ ] `teleport mirror --then deploy --then logs` ejecuta ambas en orden;
      si `deploy` falla, `logs` no corre y el exit es `1`.
- [ ] `teleport actions add` completa el wizard (nombre → pasos → cwd con
      dir browser → opciones → resumen), y "probar ahora" ejecuta la action
      con streaming antes de guardar; `Esc` a medio wizard no escribe config.
- [ ] `teleport actions list` muestra las actions del perfil;
      `edit` pre-llena valores; `remove` pide confirmación.
- [ ] Headless: `run` con `confirm = true` sin `-y` sale `2` nombrando el
      flag; con `--no-input -y` corre; las líneas van a `stderr` con prefijo
      `[deploy] `.
- [ ] `teleport run deploy --json` imprime el objeto `runResult` limpio en
      `stdout` (logs solo en `stderr`); `sync --then deploy --json` anida
      `actions` en su result.
- [ ] `teleport help` lista `run`, `actions` y `--then`.
- [ ] Ningún color inline fuera de `internal/theme`;
      `go build ./...` y `go vet ./...` pasan.
