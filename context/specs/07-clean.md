# Unit 07: Clean — descartar cambios "dirty" en el repo remoto

## Goal

Agregar el subcomando `teleport clean [profile]` que, vía SSH, deja
el repo git remoto en su estado HEAD descartando cualquier
modificación o archivo no trackeado — equivalente conceptual a
`git stash` sin guardar el stash, o más explícito:
`git -C <dir> checkout -- .` seguido de `git -C <dir> clean -fd`.

**Importante**: `clean` **no sube ni baja archivos**. Solo ejecuta
comandos git en el servidor. Asume que el `RemoteDir` del profile
es un working tree git válido y aborta con error claro si no lo es.

Además, agregar el flag `-c/--clean` a `beam` para encadenar:
`clean → beam <commits> → sync` (este último si además se pasa `-s`).
El alias mental: `teleport beam -cs` = "limpia primero el server,
manda los commits seleccionados y aplica los cambios del working
tree encima".

## Design

### Subcomando `clean`

Flujo:

1. Resolver profile/host (mismo bloque que `sync`, `beam`, `status`).
2. Conectar SSH (no se necesita SFTP).
3. **Safe guard**: correr `git -C <RemoteDir> rev-parse
   --is-inside-work-tree`. Si exit != 0 o stdout != `true`, abortar
   con:
   ```
   error: clean requires <host>:<RemoteDir> to be a git working tree
   hint: cd <RemoteDir> && git init && git remote add ... && git fetch
   ```
4. Correr `git -C <RemoteDir> status --porcelain=v1 -z` (con
   `--ignored` extra si se pasó `-x`) y parsear en categorías para
   el preview:
   - `M`, ` M`, `MM`, `AM`, `RM` → modificados (M)
   - `??` → untracked (U)
   - ` D`, `D ` → deleted-in-worktree (D)
   - `!!` → ignored (I) — solo aparece si se pasó `-x`
   - `A `, `R ` y demás staged-only: tratarlos como modified (van a
     ser revertidos por `checkout -- .`).
5. Si la lista está vacía:
   ```
   ✓ <host>:<RemoteDir> already clean (HEAD a1b2c3d)
   ```
   Salir con exit 0 sin TUI.
6. Si hay cambios: lanzar TUI de confirmación (ver más abajo). `n`,
   `esc` o `q` → `aborted, no changes made`, exit 0.
7. Si confirma (`y` / `enter`): ejecutar **en el mismo cliente SSH**,
   en dos sesiones separadas (cada `git` corre en su `Session`):
   ```
   git -C <RemoteDir> checkout -- .
   git -C <RemoteDir> clean -fd          # +x si se pasó --ignored
   ```
   Capturar combined output de cada uno; si alguno falla,
   propagar el error con el stderr como contexto.
8. Imprimir resumen:
   ```
   ✓ cleaned <host>:<RemoteDir>
     reverted: N file(s)
     removed:  M file(s)
   ```
   Los contadores salen del parseo del paso 4 (no del output de
   git, que no es estable entre versiones).

Flags:

| Flag               | Default | Efecto                                                                                                     |
| ------------------ | ------- | ---------------------------------------------------------------------------------------------------------- |
| `--yes` / `-y`     | false   | Skipea la TUI de confirmación (para CI).                                                                   |
| `--ignored` / `-x` | false   | También borra archivos ignorados por `.gitignore` (builds, caches, `node_modules`). Pasa `-x` a `git clean`. |

Exit codes: `0` si éxito (con o sin cambios), `1` cualquier error.

### TUI de confirmación

Modelo bubbletea minimalista, sin selección — solo confirmar /
cancelar. Header con `host:RemoteDir` y SHA corto de HEAD remoto
(`git -C <dir> rev-parse --short HEAD`, una llamada extra al
conectar). Lista agrupada por categoría:

```
Clean staging:/var/www/myproj (HEAD a1b2c3d)

  Will revert (3 modified):
    󰈙 cmd/clean.go
    󰈙 internal/ssh/client.go
    󰈙 README.md

  Will remove (2 untracked):
    󰮈 dist/old-bundle.js
    󰮈 tmp/scratch.txt

  Will restore (1 deleted):
    󰈙 docs/guide.md

  Will remove (4 ignored):                ← solo con -x
    󰮈 node_modules/
    󰮈 dist/bundle.js
    󰮈 .next/cache
    󰮈 coverage/

  [ y / enter ] confirm    [ n / esc / q ] cancel
```

Reusa los estilos de `beamfilepicker.go` (`iconFile`, `iconDelete`)
y el helper `fileTypeIcon` ampliado en Unit 06 para los glyphs por
extensión. Scroll vertical si la lista es larga (mismo patrón que
los pickers existentes).

### Flag `-c/--clean` en `beam`

Encadena clean **antes** del beam, reusando la **misma conexión
SSH**. Orden completo cuando se pasa `beam -cs`:

1. Resolver profile y conectar SSH una vez.
2. **Clean phase**: mismo flujo que el subcomando `clean`. Si hay
   cambios y no se pasó `-y`, mostrar la TUI. Cancelar el clean
   aborta también el beam y el sync (no se ejecuta nada después).
3. **Beam phase**: comportamiento actual de `beam`.
4. **Sync phase** (si `-s`): comportamiento actual.

Si el remoto no es un repo git, el `-c` falla con el mismo error
del safe guard y aborta todo el comando.

Si el remoto está limpio, la fase se reduce a la línea
`✓ already clean` y se pasa a la fase beam sin interrupción.

`-y` aplica al prompt de clean (no afecta el picker de commits del
beam, que es selección activa del usuario).

### Helper SSH nuevo: `RunCommand`

`internal/ssh/client.go`:

```go
// RunCommand executes cmd on the remote host through a fresh ssh
// session and returns its combined stdout. stderr is returned as
// part of the error when the command exits non-zero.
func (c *Client) RunCommand(cmd string) (string, error)
```

Implementación: `c.ssh.NewSession()`, defer `Close`, `CombinedOutput`
(o `Output` + capturar stderr aparte si se prefiere). El comando
se pasa quoted; el caller arma el string con shell-safe paths
(`shellquote` o el helper más simple: como `RemoteDir` viene del
config y lo controla el usuario, no necesitamos blindar contra
inyección agresiva, pero sí escapar espacios — usar
`%q`-style quoting con comilla simple).

Helper auxiliar privado en `cmd/clean.go` o `internal/ssh/`:

```go
func shellQuote(s string) string {
    return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
```

## Implementation

### `internal/ssh/client.go` — `RunCommand`

Agregar al final del archivo. No tocar funciones existentes. Sin
nuevas dependencias (`golang.org/x/crypto/ssh` ya importado vía el
campo `ssh *ssh.Client`).

### `internal/tui/cleanconfirm.go` — nuevo

Modelo bubbletea. API:

```go
type CleanPlan struct {
    Host      string
    RemoteDir string
    HeadSHA   string   // 7 chars
    Modified  []string // se "reverten"
    Untracked []string // se "remueven"
    Deleted   []string // se "restauran"
    Ignored   []string // solo poblado si se pasó --ignored/-x
}

// RunCleanConfirm muestra el plan y devuelve true si el usuario
// confirma, false si cancela.
func RunCleanConfirm(p CleanPlan) (bool, error)
```

### `cmd/clean.go` — nuevo

```go
var (
    cleanYes     bool
    cleanIgnored bool
)

var cleanCmd = &cobra.Command{
    Use:   "clean [profile]",
    Short: "󰃢 discard dirty changes on the remote (git checkout + git clean)",
    Args:  cobra.MaximumNArgs(1),
    RunE:  runClean,
}

func init() {
    cleanCmd.Flags().BoolVarP(&cleanYes, "yes", "y", false,
        "skip the confirmation prompt")
    cleanCmd.Flags().BoolVarP(&cleanIgnored, "ignored", "x", false,
        "also remove gitignored files (builds, caches, node_modules)")
}
```

`runClean` orquesta:

1. Resolver profile, conectar SSH.
2. Llamar a `cleanRemote(c, profile, cleanYes, cleanIgnored)`
   (helper privado en el mismo paquete `cmd`) que encapsula los
   pasos 3-8 del Design.
3. Devolver su error.

Helper `cleanRemote` retorna `(reverted, removed, restored,
removedIgnored int, err error)` para que `beam -c` pueda imprimir
o ignorar.

`parsePorcelain(out string) (modified, untracked, deleted []string)`
en el mismo archivo: parsea formato `-z` (registros separados por
NUL, dos chars de status + espacio + path; rename produce dos paths
separados por NUL — el "to" es el path actual, ese va a la lista).

### `cmd/beam.go` — flag `-c`

```go
beamCmd.Flags().BoolVarP(&beamClean, "clean", "c", false,
    "run clean before beam (discard dirty changes on the remote)")
```

En `runBeam`, después de conectar y antes del beam:

```go
if beamClean {
    if _, _, _, _, err := cleanRemote(c, profile, beamYes, false); err != nil {
        return err
    }
}
```

`beam -c` no expone `-x`; si el usuario quiere limpiar también
ignorados antes de un beam, debe correr `teleport clean -x` por
separado primero. Mantiene la combinatoria de flags de beam acotada.

Si `beam` no tiene aún `-y`, agregarlo solo para skipear el prompt
de clean dentro de beam (no afecta el picker de commits).

### `cmd/root.go`

`rootCmd.AddCommand(cleanCmd)`.

### `cmd/help.go`

Agregar a la lista de comandos:

```go
{iconSync, "clean", "discard dirty changes on the remote (git checkout + git clean)"},
```

Y a la lista de flags de `beam`:

```go
{"-c", "--clean", iconSync, "clean before beam"},
```

### Documentación

- `CHANGELOG.md` bajo *Unreleased*:
  - `[ADD] clean: discard dirty changes on the remote git working tree`
  - `[ADD] beam: chain a clean before the beam with --clean/-c`
- `context/progress-tracker.md`: agregar Unit 07 a Completed.

## Dependencies

Ninguna nueva. `golang.org/x/crypto/ssh` ya disponible vía el
struct `Client`. Bubbletea/lipgloss ya en uso para los otros TUIs.

## Verify when done

- [ ] `teleport clean` falla con mensaje claro si `RemoteDir` no es
      un working tree git.
- [ ] Si el repo remoto está limpio, imprime
      `✓ ... already clean (HEAD ...)` y exit 0, sin TUI.
- [ ] Si hay cambios, muestra TUI agrupada en revert / remove /
      restore con el glyph por extensión correcto.
- [ ] `y`/`enter` ejecuta `git checkout -- .` y `git clean -fd` en
      el remoto y reporta el resumen.
- [ ] `n`/`esc`/`q` aborta sin tocar el remoto.
- [ ] `teleport clean -y` salta la TUI y aplica.
- [ ] `teleport clean -x` lista y borra también archivos
      gitignored (sección extra "Will remove (N ignored)" en la TUI).
- [ ] Sin `-x`, los archivos gitignored del remoto no se tocan.
- [ ] `clean` **no** sube ni baja archivos (verificable con
      `tcpdump` o por ausencia de SFTP en la sesión).
- [ ] `teleport beam -c` ejecuta clean antes; cancelar clean aborta
      el beam.
- [ ] `teleport beam -cs` ejecuta clean → beam → sync en ese orden,
      en una sola sesión SSH.
- [ ] Cuando `RemoteDir` contiene espacios o caracteres especiales,
      el shellQuote evita que se rompa el comando.
- [ ] `teleport help` lista `clean` y el flag `-c` de beam.
- [ ] `go build ./...` y `go vet ./...` pasan.
