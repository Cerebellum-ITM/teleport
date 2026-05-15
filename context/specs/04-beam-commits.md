# Unit 04: Beam — sync por commits locales

## Goal

Agregar el subcomando `teleport beam [profile]` que permite seleccionar uno
o varios commits locales no pusheados y subir al remoto el contenido de los
archivos exactamente como quedó en esos commits (estilo cherry-pick, pero
hacia un servidor SSH). Cubre el caso en el que existen commits locales que
nunca llegarán a la rama remota pero cuyos cambios sí deben reflejarse en
el servidor.

## Design

Flujo en dos pasos TUI seguido del upload:

1. **Commit picker** (nuevo) — lista multi-select de commits en
   `@{u}..HEAD`. Sin upstream configurado, el comando aborta con mensaje
   claro: `beam requires an upstream branch (git push -u origin <branch>)`.
   Muestra por línea: hash corto (7) + subject + fecha relativa, en
   tokens existentes (header, cursor pink, dim).

2. **File picker (reuso adaptado)** — abre el filepicker con los archivos
   tocados por los commits seleccionados, **todos pre-marcados**. El
   usuario puede desmarcar para excluir. Los archivos con estado *delete*
   se muestran con un glifo distinto y un color de aviso; al confirmar,
   los marcados como delete se eliminan en remoto vía `SFTP.Remove`, el
   resto se sube.

3. **Upload** — por cada archivo a subir, obtener el blob con
   `git show <sha>:<path>`, donde `<sha>` es el **commit más reciente
   seleccionado** que toca ese archivo. Subida vía nuevo
   `Client.UploadBytes(remotePath, content)`. Para deletes, `Client.Remove`.

El comando reusa la resolución de profile/host del `sync` actual y el
progreso TUI (`tui.RunSyncProgress`) existente.

## Implementation

### `internal/git/git.go` — extender

Agregar tipos y funciones:

```go
type Commit struct {
    SHA      string // full sha
    Short    string // 7 chars
    Subject  string
    RelDate  string // "2 hours ago"
}

type FileChange struct {
    Path   string
    Status rune // 'A' add, 'M' modify, 'D' delete, 'R' rename (treated as delete+add)
    SHA    string // commit where this status was observed (most recent in selection)
}

// CommitsAhead returns commits in @{u}..HEAD, newest first.
// Fails with a structured error when no upstream is configured.
func CommitsAhead() ([]Commit, error)

// FilesInCommits returns the per-file effective change across the given
// commits (in chronological order: oldest → newest). For each path, the
// final Status and SHA reflect the most recent commit in the slice that
// touched it. Renames are split: old path becomes 'D', new path 'A'.
func FilesInCommits(shas []string) ([]FileChange, error)

// FileAtCommit returns the blob content of path at commit sha
// (`git show sha:path`). Used for non-deleted files.
func FileAtCommit(sha, path string) ([]byte, error)
```

Implementación interna:

- `CommitsAhead`: corre `git log @{u}..HEAD --format=%H%x09%h%x09%s%x09%cr`.
  Si el exit code señala `no upstream`, devolver un error sentinel
  `ErrNoUpstream` para que `cmd/beam.go` lo detecte y muestre el mensaje
  amigable.
- `FilesInCommits`: para cada sha (oldest first) correr
  `git show --name-status --format= <sha>` y acumular en un `map[string]FileChange`,
  sobrescribiendo siempre con el sha actual. Las líneas con `R<score>` producen
  dos entradas (D para old, A para new). Devolver slice ordenado por path.
- `FileAtCommit`: `git show <sha>:<path>` capturando stdout como bytes
  (no usar `runGit` actual porque parte en líneas).

`runGit` se mantiene pero se añade un helper `runGitBytes(args ...string) ([]byte, error)`
para el caso binario.

### `internal/ssh/client.go` — extender

```go
// UploadBytes writes content to remotePath, creating parent dirs.
func (c *Client) UploadBytes(remotePath string, content []byte) error

// Remove deletes remotePath. Returns nil if the file does not exist.
func (c *Client) Remove(remotePath string) error
```

`UploadBytes` reusa la misma lógica que `UploadFile` pero recibe `[]byte`
en lugar de leer del disco. Refactor opcional: `UploadFile` puede pasar
a leer y delegar en `UploadBytes`, sin cambiar la API pública.

`Remove` envuelve `c.SFTP.Remove(remotePath)`; tolera "not exist" devolviendo
`nil` (el commit pudo haber añadido y luego borrado el archivo sin que el
remoto lo conociera).

### `internal/tui/commitpicker.go` — nuevo

Modelo bubbletea v2 con la misma forma que `filepicker`:

- Campos: `commits []git.Commit`, `selected map[int]bool`, `cursor int`,
  `done`, `quitting`.
- Render por línea: `▶ ` cuando es cursor, `iconChecked` verde si
  seleccionado, `󰄱 ` naranja si no. Después el hash corto en bold, el
  subject normal, y la fecha relativa en dim al final.
- Keys: `↑/k`, `↓/j`, `space` toggle, `a` toggle-all, `enter` confirma
  (rechaza si no hay ninguno seleccionado), `ctrl+c` cancela.
- Header: `  Local commits ahead of upstream` con estilo header existente.
- Footer: `space=toggle  a=all  enter=confirm  ctrl+c=quit`.
- API: `RunCommitPicker(commits []git.Commit) ([]git.Commit, error)`.

Reusa `headerStyle`, `dimStyle`, `checkStyle`, `uncheckedStyle` ya definidos.
No define colores propios. Agrega `iconCommit = "󰜘 "` como constante (o reusa
`iconFile`; preferir un glifo nuevo para diferenciar).

### `internal/tui/filepicker.go` — extender

Para soportar el flujo beam sin romper el actual:

- Nuevo constructor `NewBeamFilePicker(changes []git.FileChange) FilePicker`
  que internamente pone todos los paths en `untracked`, todos
  pre-seleccionados (`selected[i] = true`), y guarda el `Status` por
  índice en un nuevo mapa opcional `status map[int]rune`.
- `View()` se ajusta: cuando `status` es no-nil, oculta la sección
  "Tracked (always included)", renombra el header a "Files from selected
  commits", y para entradas con `Status == 'D'` muestra el icono nuevo
  `iconDelete = "󰮈 "` en color de aviso (token `lipgloss.Color("203")` —
  agregar a `ui-context.md` si no existe; si ya hay un color de error,
  reusar).
- `SelectedFiles()` se reemplaza para este flujo por
  `SelectedChanges() []git.FileChange` que devuelve solo las entradas
  marcadas. Mantener la API original intacta.
- Nuevo runner: `RunBeamFilePicker(changes []git.FileChange) ([]git.FileChange, error)`.

Alternativa más limpia si la rama anterior crece: separar en
`internal/tui/beamfilepicker.go`. Decidir durante implementación; si
agregar branches al `View()` actual lo ensucia, partirlo. Mantener el
contrato (multi-select, space=toggle, enter=confirm).

### `cmd/beam.go` — nuevo

Estructura paralela a `cmd/sync.go`:

```go
var beamCmd = &cobra.Command{
    Use:   "beam [profile]",
    Short: "󰜘 send selected local commits to the remote server",
    Args:  cobra.MaximumNArgs(1),
    RunE:  runBeam,
}
```

`runBeam`:

1. Resolver `localCfg`, `profileName`, `profile` igual que `runSync`.
2. `commits, err := git.CommitsAhead()`. Si `errors.Is(err, git.ErrNoUpstream)`,
   devolver mensaje amigable. Si `len(commits) == 0`, imprimir
   `Nothing to beam — no local commits ahead of upstream.` y salir 0.
3. `selectedCommits, err := tui.RunCommitPicker(commits)`. Si cancelado,
   salir sin error.
4. Construir slice de shas en orden cronológico (más antiguo primero),
   llamar `git.FilesInCommits(shas)`.
5. `changes, err := tui.RunBeamFilePicker(allChanges)`.
6. Resolver host + `sshpkg.Connect`, `defer client.Close()`.
7. Separar `changes` en `toUpload` (A/M) y `toDelete` (D).
8. Para uploads, construir slice de paths y pasar a `tui.RunSyncProgress`
   con un closure que llame `git.FileAtCommit(change.SHA, change.Path)` y
   luego `client.UploadBytes(filepath.Join(profile.Path, change.Path), content)`.
   Necesita un mapa `path → change` para que el closure recupere el sha.
9. Para deletes, ejecutar secuencialmente y loggear con `log.Info`
   `removed <path>` o `log.Error` si falla. (Pocos archivos esperados;
   no requiere TUI de progreso.)
10. Si `failedUploads > 0` o algún delete falló, devolver error final con
    el conteo combinado.

Registrar el comando con `rootCmd.AddCommand(beamCmd)` en `cmd/root.go`,
siguiendo el patrón actual.

### `cmd/beam.go` — flag de encadenamiento

Bandera local `--then-sync` / `-s` (booleana, default `false`). Cuando
está activa y la fase beam termina sin failures, ejecuta una fase sync
reusando el mismo `*sshpkg.Client` (ahorra reconexión). La fase sync
replica el flujo de `cmd/sync.go`:

- `git.ChangedFiles()` para obtener archivos del working tree vs HEAD
- Suma `git.UntrackedFiles()` sólo si `localCfg.SyncUntracked` está en
  `true` (no se agrega `-u` propio a `beam` para no inflar la API; el
  usuario persiste el default vía `teleport config set sync-untracked
  true` si lo necesita)
- Reusa `tui.RunSyncProgress` con `client.UploadFile`

Orden garantizado: beam primero (snapshot histórico de commits), sync
después (working tree gana). Si un archivo está en ambas fases, el
contenido en disco sobrescribe al blob del commit, que es la semántica
natural.

### `cmd/help.go` — actualizar

Añadir `beam` a la lista de comandos en el help personalizado.

### `context/ui-context.md` — actualizar

Si se agrega `iconCommit` y `iconDelete` y/o un token de color de aviso
nuevo, registrarlos en la tabla de iconos y, si aplica, en la tabla de
colores. Hacer en el commit junto al código.

### `CHANGELOG.md`

Bajo *Unreleased*:

- `[ADD] beam: new subcommand to send selected local commits via SFTP`

## Dependencies

- ninguna nueva (cobra, bubbletea v2, lipgloss, pkg/sftp, BurntSushi/toml,
  charmbracelet/log ya están en `go.mod`).

## Verify when done

- [ ] En una rama con upstream y commits locales no pusheados,
      `teleport beam` lista esos commits con hash, subject y fecha
      relativa.
- [ ] En una rama sin upstream, `teleport beam` falla con mensaje claro
      que menciona configurar upstream; sin panic, sin stack trace.
- [ ] En una rama sin commits ahead, `teleport beam` imprime
      "Nothing to beam — no local commits ahead of upstream." y sale 0.
- [ ] Seleccionar 1 commit y enter lleva al file picker con todos los
      archivos del commit pre-marcados.
- [ ] Seleccionar varios commits: si dos tocan el mismo archivo, sólo
      aparece una vez en el file picker y al subir se envía el contenido
      del commit más reciente entre los seleccionados.
- [ ] Un archivo eliminado en alguno de los commits seleccionados se
      muestra con icono y color de delete, y al confirmar se borra del
      remoto vía SFTP.
- [ ] Un archivo eliminado que no existe en remoto no produce error
      (operación idempotente).
- [ ] Desmarcar un archivo en el file picker lo excluye del upload (y
      del delete si era delete).
- [ ] El contenido subido coincide byte a byte con
      `git show <sha>:<path>` para el sha elegido por la regla del más
      reciente.
- [ ] Cancelar con `ctrl+c` en cualquiera de los dos TUIs no hace
      conexión SSH ni escribe en remoto.
- [ ] `teleport help` muestra `beam` en la lista.
- [ ] `go build ./...` pasa sin errores.
- [ ] Ninguna ruta importa `cmd/` desde `internal/` (invariante 1
      preservada).
