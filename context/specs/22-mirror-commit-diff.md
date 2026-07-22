# Unit 22: Ver los cambios en el mirror picker (`d` = diff del commit)

## Goal

Que el mirror picker deje **inspeccionar los cambios** antes de reflejar: con el
cursor sobre un commit, `d` abre el **diff completo de ese commit**
(`git show <sha>`) en un pager estilo bat/delta con scroll — la misma sensación
que el `d` del beam file picker, pero a nivel de commit. Es solo lectura y
ortogonal a la selección (nunca cambia qué se refleja).

## Design

### Comportamiento

- En `RunMirrorTargetPicker`, `d` sobre el commit resaltado abre un **visor de
  diff** a pantalla del pager; `esc`/`q` regresa al picker con el cursor
  intacto; `enter` sigue confirmando el destino como hoy.
- El visor muestra el diff que el commit introdujo respecto a su primer padre
  (`git show --format= <sha>`), con:
  - **Cabecera**: `󰜘 <short> · <subject> · +adds −dels` (short y subject con el
    color del tag / `theme`; adds en `theme.Success`, dels en `theme.Danger`).
  - **Cuerpo**: el diff con cada archivo separado por una barra de cabecera
    (ruta del archivo), líneas `+`/`−`/contexto coloreadas
    (verde/rojo/faint) y los rangos `@@` en dim. (Resaltado de sintaxis
    por-archivo queda como mejora futura; este unit usa coloreo por-línea, que
    ya basta para revisar.)
  - Placeholder `「empty」` si el commit no tocó archivos (raro).
- Scroll: `j/k ↑/↓`, `ctrl+d`/`ctrl+u`, `g/G` — reusando el keymap del
  `viewport` como en el beam viewer.
- Footer del picker gana el hint: `d=diff`.

### Reuso e I/O (invariante #3)

El TUI no hace I/O. El picker recibe un **loader** (closure) que hace el
git+highlight, igual que `beamViewerLoader` para beam:

```go
// CommitDiffFunc renders a commit's full diff for the mirror viewer.
type CommitDiffFunc func(sha string, width int) (ViewerContent, error)
```

- `git.CommitDiff(sha) ([]byte, error)` — nuevo helper: `git show --format=
  <sha>` (todo el diff del commit).
- `highlight.CommitDiff(raw []byte, width int, profile) (body string, adds,
  dels int)` — nuevo: recorre las secciones `diff --git a/… b/…`, emite una
  barra de cabecera por archivo (estilo `theme`), colorea `+`/`−`/contexto y los
  `@@`, y cuenta adds/dels (sin contar las cabeceras `+++`/`---`). Reusa los
  helpers de conteo que ya tiene `highlight.Diff`.
- El loader vive en `cmd/mirror.go` (`mirrorCommitDiffLoader`), detecta el
  `colorprofile`, llama a `git.CommitDiff` + `highlight.CommitDiff`, y devuelve
  `tui.ViewerContent`. Se pasa a `RunMirrorTargetPicker`.

### Estructura del visor

Reusar el `viewport`-based sub-model del beam viewer donde sea posible. Como el
`beamFileViewer` está atado a `git.FileChange`/`FileContentFunc`, se añade un
sub-model hermano más simple `commitDiffViewer` en
`internal/tui/mirrortargetpicker.go` (o un archivo nuevo
`internal/tui/commitdiffviewer.go`) sobre `bubbles/v2/viewport`, con carga
perezosa (un `tea.Cmd` que llama al loader y devuelve un `msg` con el contenido)
y estados `loading`/`err`. No hay modo file⇄diff (solo diff), así que `tab` no
aplica aquí.

## Implementation

### `internal/git/git.go`

```go
// CommitDiff returns the full diff a commit introduced vs its first parent.
func CommitDiff(sha string) ([]byte, error) // git show --format= <sha>
```

### `internal/highlight/highlight.go`

```go
// CommitDiff renders a whole-commit (multi-file) diff: a header bar per file,
// per-line +/−/context coloring, dim @@ ranges. Returns the body and the total
// adds/dels. Unlike Diff (single file, delta-style syntax highlighting), this
// keeps file separators and does not run a per-file lexer.
func CommitDiff(raw []byte, width int, profile colorprofile.Profile) (string, int, int)
```

### `internal/tui/` — visor + integración en el picker

- Nuevo `commitDiffViewer` (viewport) con `CommitDiffFunc` loader, header
  (short/subject/adds/dels), placeholder `「empty」`, keymap de scroll.
- `mirrorTargetModel` gana campos `load CommitDiffFunc`, `viewing bool`,
  `viewer commitDiffViewer`. `d` abre el visor sobre `commits[cursor]`;
  `updateViewing` maneja `esc`/`q` (cierra) y delega scroll al viewport.
  `RunMirrorTargetPicker(commits, remoteHEAD, load)` gana el parámetro `load`.
- Footer: añadir `d=diff`.

### `cmd/mirror.go`

- `mirrorCommitDiffLoader(sha string, width int) (tui.ViewerContent, error)`
  (closure análogo a `beamViewerLoader`), pasado a `RunMirrorTargetPicker`.

## Dependencies

Ninguna nueva. Reusa `bubbles/v2/viewport`, `internal/highlight` (chroma) y el
patrón de loader de beam (Unit 18).

## Verify when done

- [ ] En el mirror picker, `d` sobre un commit abre su diff completo en el
      pager; `esc`/`q` vuelve con el cursor donde estaba; `enter` sigue
      confirmando el destino.
- [ ] La cabecera del visor muestra `<short> · <subject> · +adds −dels` con
      adds/dels correctos (verifican contra `git show --stat`).
- [ ] El diff separa archivos con una barra de cabecera y colorea `+`/`−`/
      contexto y los `@@`.
- [ ] Un commit sin archivos muestra `「empty」`; scroll (`j/k`, `ctrl+d/u`,
      `g/G`) funciona.
- [ ] El TUI no hace I/O directo (el git+highlight vive en el loader de
      `cmd/mirror.go`).
- [ ] `go build ./...`, `go vet ./...`, `gofmt -l` y `go test ./...` limpios.
