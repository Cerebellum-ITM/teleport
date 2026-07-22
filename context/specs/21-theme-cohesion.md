# Unit 21: Cohesión visual — paquete `internal/theme` + tags de commit con color semántico

## Goal

Unificar el color de teleport en **una sola fuente de verdad** (`internal/theme`)
y migrar los ~15 archivos que hoy hardcodean números de color sueltos, matando
los duplicados (dos rojos, cinco grises) y dándole a todos los pickers el mismo
tratamiento de cursor/selección. Además, colorear el **tag** de cada commit
(`[ADD]`/`[FIX]`/`[DOC]`…) según su tipo, en el commit picker y en el mirror
picker, para que la lista se lea de un vistazo.

## Design

### El problema actual

Los estilos viven regados: `dimStyle`/`headerStyle` en `dirpicker.go`,
`boldStyle` en `branchpicker.go`, y cada `cmd/*.go` e `internal/tui/*.go`
define sus propios `lipgloss.NewStyle().Foreground(lipgloss.Color("NN"))`.
Consecuencias medidas:

- **Dos rojos**: el `✗` de subida fallida usa `196` (`syncprogress.go`,
  `shipprogress.go`); todo lo demás "malo" usa `203`.
- **Cinco grises** para texto secundario: `241`, `245`, `248`, `252`, `150`.
- **Cursor inconsistente**: commit picker y dir browser pintan el cursor de
  rosa `212`; mirror picker y branch picker lo dejan plano.

`context/ui-context.md` ya documenta tokens, pero nada los impone en código.
Este unit convierte esa tabla en un paquete Go.

### Tokens (la fuente de verdad)

`internal/theme` expone colores semánticos y estilos listos. **Ningún otro
archivo puede volver a escribir `lipgloss.Color("NN")`** (salvo
`internal/highlight`, que es dominio de chroma y queda fuera).

| Token | Color | Rol | Reemplaza |
| --- | --- | --- | --- |
| `Accent` | `212` rosa | cursor/selección en todos los pickers | 212 disperso |
| `Success` | `82` verde | ✓, sent, OK | ya consistente |
| `Warn` | `214` ámbar | toggle sin marcar, avisos | ya consistente |
| `Danger` | `203` rojo | borrar, fallo, error, missing | **mata `196`** |
| `HeaderBg` / `HeaderFg` | `62` / `230` | barras de título | 62/104/60 |
| `Icon` | `116` cian | glyphs de tipo/sección | 116/86 |
| `Path` | `86` cian claro | rutas remotas (dir browser, shell) | 86 |
| `TextBright` | `255` | nombres/valores destacados | 255 |
| `TextDim` | `245` | texto secundario | 248/252 |
| `TextFaint` | `241` | terciario, gutters, separadores | ya domina |
| `Hint` | `150` verde suave | líneas guía/ayuda | ya es rol propio |
| `Gold` | `220` | acentos dorados (ejemplos, ship) | 220/222 |

Estilos derivados exportados (para no repetir `NewStyle().Foreground(...)`):
`Cursor`, `Bold`, `Header` (bg+fg+padding), `Title` (bold fg=HeaderBg),
`Dim`, `Faint`, `OK`, `Fail`, `WarnStyle`, `IconStyle`, `HintStyle`.

### Paleta rotativa por commit

`beamCommitPalette` (16 colores) se **promueve** de `beamfilepicker.go` a
`theme.CommitPalette` sin cambiar los valores (39/45/43/81/220/215/208/209/205/
213/199/171/141/99/147/105). `BeamFileStyles` y el picker la consumen desde
theme. Queda disponible para cualquier vista que quiera teñir por commit.

**Beam no cambia visualmente.** La razón de ser de esta paleta amplia es
distinguir de un vistazo los **muchos commits** que beam puede enviar a la vez
(el file picker agrupa y tiñe los archivos por commit). Ese propósito y ese
comportamiento se conservan intactos: solo cambia de dónde se importa la paleta.
El coloreo semántico del tag (siguiente sección) es **aditivo y ortogonal** — un
chip en el prefijo del subject — y no reemplaza ni altera la paleta rotativa de
beam.

### Tags de commit con color semántico

Un helper parsea el prefijo `^\[([A-Z]+)\]` del subject y devuelve el estilo del
chip. Mapa (tags de CommitCraft que usa el proyecto):

| Tag | Color | | Tag | Color |
| --- | --- | --- | --- | --- |
| `ADD` | verde `41` | | `DOC` | azul `39` |
| `FIX` | rojo `209` | | `MERGE` | cian `45` |
| `IMP` / `REF` | púrpura `141` | | `REL` | oro `220` |
| `DEL` / `REM` | rojo `203` | | otros / sin tag | gris `245` |

```go
// TagChip splits a commit subject into a colored [TAG] chip and the rest.
// ok is false when the subject has no leading [TAG]; then chip is empty and
// rest is the whole subject.
func TagChip(subject string) (chip string, rest string, ok bool)
```

`chip` ya viene renderizado (fg = color del tag, **bold**, incluye los
corchetes). El chip **no** usa background para no chocar con la fila
seleccionada. Aplicación:

- **Commit picker** (`internal/tui/commitpicker.go`): la fila de cada commit
  muestra `<chip> <resto del subject>` en vez del subject plano.
- **Mirror picker** (`internal/tui/mirrortargetpicker.go`): igual, y además el
  cursor `▶` pasa a `theme.Cursor` (rosa) y el short SHA a `theme.Bold` —
  quedando visualmente hermano del commit picker.

Comportamiento sin tag: se muestra el subject tal cual (sin chip).

## Implementation

### `internal/theme/theme.go` (nuevo)

- Definir los tokens de color y los estilos derivados de la tabla.
- `CommitPalette []lipgloss.Style` (movida verbatim de beam).
- `TagChip(subject string) (chip, rest string, ok bool)` con el mapa de tags
  (`var tagColors = map[string]lipgloss.Color{…}`, default `TextDim`) y regex
  `^\[([A-Z]+)\]\s*`.
- Solo importa `charm.land/lipgloss/v2` y `regexp`. Sin ciclos (theme no importa
  tui ni cmd).

### Migración (mecánica, sin cambio de comportamiento)

Reemplazar las definiciones locales por `theme.*` en:

- `internal/tui/`: `dirpicker.go` (mueve `dimStyle`/`headerStyle` a theme),
  `branchpicker.go` (`boldStyle`→`theme.Bold`), `commitpicker.go`,
  `mirrortargetpicker.go`, `beamfilepicker.go` (paleta→`theme.CommitPalette`,
  `203`→`Danger`), `beamfileviewer.go`, `filepicker.go`, `hostpicker.go`,
  `cleanconfirm.go`, `syncprogress.go` (**`196`→`Danger`**), `shipprogress.go`
  (**`196`→`Danger`**).
- `cmd/`: `clean.go`, `pull.go`, `status.go`, `ship.go`, `mirror.go`,
  `help.go` (mapear lo que corresponda; los acentos únicos de ayuda —oro de
  ejemplos, blanco de nombres— usan `Gold`/`TextBright`).

Los `headerStyle`/`dimStyle`/`boldStyle` compartidos que hoy viven en
`dirpicker.go`/`branchpicker.go` se borran de ahí y pasan a theme; todos los
callers apuntan a `theme.Header`/`theme.Dim`/`theme.Bold`.

### `context/ui-context.md`

Actualizar la tabla de tokens para que refleje el paquete `internal/theme`
(nombres de token = símbolos Go), documentar `TagChip` y la regla nueva:
"todo color se define en `internal/theme`; ningún `lipgloss.Color("NN")` fuera
de ahí (excepto `internal/highlight`)".

## Dependencies

Ninguna nueva. `charm.land/lipgloss/v2` (ya presente) y `regexp` (stdlib).

## Verify when done

- [ ] `internal/theme` es el único sitio con `lipgloss.Color("NN")` fuera de
      `internal/highlight` (grep lo confirma).
- [ ] El `✗` de subida fallida (sync/ship) usa el mismo rojo (`Danger`) que
      delete/status/pull — un solo rojo en toda la app.
- [ ] El commit picker y el mirror picker muestran el tag `[ADD]`/`[FIX]`/… con
      su color semántico; un subject sin tag se muestra plano.
- [ ] El mirror picker usa el cursor rosa `Accent` y el short SHA en bold, igual
      que el commit picker.
- [ ] La paleta por-commit de beam sigue viéndose idéntica (valores intactos,
      solo movida a theme).
- [ ] Cero cambios de comportamiento funcional; solo estilo.
- [ ] `go build ./...`, `go vet ./...`, `gofmt -l` y `go test ./...` limpios.
