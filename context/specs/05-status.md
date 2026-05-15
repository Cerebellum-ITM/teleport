# Unit 05: Status — verificar local vs remoto

## Goal

Agregar el subcomando `teleport status [profile]` que compara archivos
locales contra el remoto vía SFTP por hash (SHA256) y reporta drift.
Dos modos:

- **Default (full)**: todos los `git ls-files` del proyecto. Útil para
  auditar que el remoto refleja el estado git de la rama actual.
- **`--pending` / `-p`**: solo archivos en commits ahead-of-upstream +
  changed-vs-HEAD + untracked (si está habilitado en config). Útil
  durante una sesión de trabajo para verificar que beam + sync dejaron
  todo aplicado.

No persiste estado del remoto ni rastrea qué commits fueron beam-eados;
se basa en comparar el disco actual contra lo que SFTP devuelve hoy.

## Design

Flujo lineal sin TUI:

1. Resolver profile/host (mismo patrón que `sync` y `beam`).
2. Construir la lista de archivos a comparar según modo.
3. Conectar SSH/SFTP.
4. Para cada archivo:
   - Calcular SHA256 del local (si existe; si no, marcar como deletion).
   - Calcular SHA256 del remoto (si existe; si no, marcar como missing-remote).
   - Comparar y clasificar.
5. Imprimir resumen + lista de drift.

Clasificación:

| Marcador | Significado                                  |
| -------- | -------------------------------------------- |
| `==`     | hashes iguales (no se lista; solo cuenta)    |
| `!=`     | hashes difieren                              |
| `??`     | existe local, no existe remoto               |
| `--`     | existe remoto, no existe local (solo modo pending con deletes de commits) |

Si todo coincide: una sola línea verde `✓ all N file(s) in sync with <host>:<path>`.
Si hay drift: línea de resumen + lista solo de archivos problemáticos.

Ejemplo de output con drift:

```
Status against staging:/var/www/myproj
  3 differ, 1 missing remotely (146 total)

  != cmd/beam.go
  != internal/git/git.go
  != README.md
  ?? new-file.go
```

Sin drift:

```
✓ all 146 file(s) in sync with staging:/var/www/myproj
```

Exit code: `0` si todo coincide, `1` si hay drift.

## Implementation

### `internal/ssh/client.go` — `RemoteSHA256`

```go
// RemoteSHA256 streams remotePath and returns its lowercase hex SHA256.
// Returns ("", os.ErrNotExist) when the remote file does not exist.
func (c *Client) RemoteSHA256(remotePath string) (string, error)
```

Implementación: `c.SFTP.Open(remotePath)`; si falla con `os.IsNotExist`
devolver `("", os.ErrNotExist)` para que el caller pueda distinguir
"no existe" sin parsear strings. Si abre OK, `io.Copy` a un
`sha256.New()` en chunks (no carga el archivo entero en memoria) y
devolver `hex.EncodeToString(h.Sum(nil))`.

### `cmd/status.go` — nuevo

```go
var statusPending bool

var statusCmd = &cobra.Command{
    Use:   "status [profile]",
    Short: " compare local files against the remote",
    Args:  cobra.MaximumNArgs(1),
    RunE:  runStatus,
}

func init() {
    statusCmd.Flags().BoolVarP(&statusPending, "pending", "p", false,
        "only check files in unpushed commits + dirty working tree")
}
```

`runStatus`:

1. Cargar `localCfg`, resolver `profileName`, `profile` (mismo bloque que `sync`).
2. Construir `targets []statusTarget`:
   ```go
   type statusTarget struct {
       Path           string
       ExpectAbsent   bool // true para deletes de commits en modo pending
   }
   ```
   - Modo full: `git.TrackedFiles()` → cada uno con `ExpectAbsent=false`.
   - Modo pending: union de
     - `git.CommitsAhead()` → `git.FilesInCommits(shas)`: para `'D'` agregar con `ExpectAbsent=true`, resto con `false`
     - `git.ChangedFiles()` → `ExpectAbsent=false`
     - `git.UntrackedFiles()` si `localCfg.SyncUntracked` → `ExpectAbsent=false`
     - Dedupe por path; si el mismo path aparece como D y luego como modified, gana modified (el commit más reciente o el working tree manda — la regla coincide con beam).
   - Si no hay upstream en modo pending, advertir pero seguir con changed+untracked (no fallar como beam).
3. Conectar SSH/SFTP.
4. Para cada target en serie (paralelizar queda para iteración futura):
   - Si `ExpectAbsent`: chequear que el remoto NO tenga el archivo.
     `RemoteSHA256` con `errors.Is(err, os.ErrNotExist)` → OK. Si existe → drift `--`.
   - Si no: calcular hash local (si el archivo no está en disco, marcar como deletion local — improbable en full mode si `git ls-files` lo lista, pero defensivo). Calcular `RemoteSHA256`. Comparar.
5. Reportar.

Imprimir indicador de progreso simple (un dot por cada 50 archivos
chequeados, vía `fmt.Fprintf(os.Stderr, ".")`) para que comandos largos
no parezcan colgados. Sin TUI bubbletea — overkill.

Helper `localSHA256(path string) (string, error)` en el mismo archivo:
abre, `io.Copy` a `sha256.New()`, hex-encode.

Estilo del output:
- Header `Status against <host>:<path>` en `lipgloss` neutro
- Línea de resumen en `dimStyle`
- Marcador `!=` y `??` y `--` con colores: `!=` orange (214), `??` red (203), `--` red (203)
- Caso happy path: línea verde con check `✓` (color 82)

Reusar tokens de color ya definidos donde sea posible; no inventar
nuevos.

### `cmd/root.go` — registrar

`rootCmd.AddCommand(statusCmd)`.

### `cmd/help.go` — actualizar

Agregar `status` a la lista de comandos.

### Documentación

- `CHANGELOG.md` bajo *Unreleased*:
  `[ADD] status: verify local files match the remote via SHA256 over SFTP`
- `context/progress-tracker.md`: agregar Unit 05 a Completed.

## Dependencies

- ninguna nueva (`crypto/sha256`, `encoding/hex`, `io` ya están en stdlib).

## Verify when done

- [ ] `teleport status` sin flags compara todos los `git ls-files`
      contra el remoto y reporta drift.
- [ ] `teleport status --pending` compara solo files-in-CommitsAhead +
      ChangedFiles + (untracked si está habilitado).
- [ ] Sin upstream, el modo `--pending` advierte y sigue con
      changed+untracked, no aborta.
- [ ] Archivo que coincide byte a byte no aparece en la lista de drift.
- [ ] Archivo que difiere aparece con prefijo `!=`.
- [ ] Archivo local que no existe en remoto aparece con prefijo `??`.
- [ ] En modo `--pending`, un archivo eliminado en algún commit ahead
      que **sigue** existiendo en remoto aparece con prefijo `--`.
- [ ] Cuando todo coincide, output es una sola línea con check verde y
      exit code `0`.
- [ ] Con cualquier drift, exit code `1`.
- [ ] `teleport help` lista `status`.
- [ ] `go build ./...` pasa.
