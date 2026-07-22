# Unit 24: Beam headless completo — selección explícita de commits por flag

## Goal

Cerrar el único hueco headless de `beam`: hoy sin TTY la única resolución es
`-a` (todos los no-enviados) — no hay forma de decir "manda exactamente estos
commits" desde un script. Esta unidad añade un flag repetible
**`--commit <sha>`** que selecciona commits específicos (contiguos o no) sin
abrir el commit picker, con lo que beam queda operable al 100% desde CI y
pipelines: `teleport beam --commit a1b2c3d --commit e4f5a6b --no-input --json`.

## Design

### Flag `--commit` (`-C`)

```
teleport beam --commit <sha> [--commit <sha>]... [profile]
```

- Repetible; cada valor es un commit-ish (SHA corto, SHA completo, o ref como
  `HEAD~2`) que se resuelve con `git rev-parse` a un SHA completo.
- **Cada commit resuelto debe estar en la lista de commits ahead** de la rama
  fuente (lo que hoy alimenta al picker vía `git.CommitsAheadOf`). Un commit
  fuera de esa lista es error `1` inmediato, nombrando el SHA rechazado y
  los SHAs válidos disponibles — **antes** de conectar o subir nada.
- Subconjuntos no contiguos son válidos (beam es file-level; los huecos son
  legales por diseño, a diferencia de mirror).
- Duplicados (dos flags que resuelven al mismo SHA) se deduplican en
  silencio; el orden de envío es el orden topológico de la rama (el mismo
  que hoy produce el picker), **no** el orden de los flags.

### Semántica de selección explícita

- `--commit` es **imperativo**: manda exactamente lo pedido, aunque el commit
  ya esté marcado como enviado en `beamed_commits` (la selección explícita
  gana sobre el tracking, igual que re-seleccionar un commit sent en el
  picker). Tras el envío exitoso, el tracking se actualiza igual que hoy
  (`rememberBeamedCommits`, atribución por path).
- `--commit` y `-a` son **mutuamente excluyentes**: pedir "estos exactos" y
  "todos los no-enviados" a la vez es contradictorio → error de uso
  (exit `1`) antes de hacer nada.
- Funciona **también en modo interactivo**: con `--commit` el picker se
  salta siempre (útil para scriptear desde una terminal real). El file-diff
  viewer conserva su regla actual: se abre solo si `interactive()` — con
  TTY puedes revisar archivos de los commits pedidos; headless se envía
  todo directo, como hoy con `-a`.

### Matriz headless de beam (estado final)

| Invocación headless | Resultado |
| ------------------- | --------- |
| `beam --no-input` | exit `2`, hint: `pass -a or --commit` |
| `beam -a --no-input` | todos los no-enviados (comportamiento Unit 19, sin cambio) |
| `beam --commit X --commit Y --no-input` | exactamente X e Y |
| `beam --commit X -a` | error de uso: flags excluyentes |
| `beam --commit <sha-no-ahead>` | error `1` nombrando el SHA inválido y los válidos |

El hint del `ErrNeedsTTY` existente se actualiza de
`"pass -a to auto-select unsent commits"` a
`"pass -a (unsent commits) or --commit <sha> (explicit commits)"`.

### Sin cambios colaterales

- La rama fuente se sigue resolviendo igual (`--branch` o picker; headless
  multi-rama sigue exigiendo `--branch`, regla existente de `resolveBranch`).
- `-c` (clean), `-s` (then-sync), `-y`, `--json` y el resto del contrato
  headless (Unit 19) componen con `--commit` sin reglas nuevas.
- El shape JSON de `beamResult` no cambia (`commits` ya reporta los shorts
  enviados).

## Implementation

### `internal/git` — resolución y validación

Helper nuevo (o en `cmd/beam.go` si se prefiere no tocar el paquete; decidir
por consistencia con `LocalBranches`/`CommitsAheadOf` que ya viven en git):

```go
// ResolveCommit resolves a commit-ish to its full SHA via rev-parse.
func ResolveCommit(ref string) (string, error)
```

### `cmd/beam.go`

- Flag nuevo:

```go
beamCmd.Flags().StringArrayVarP(&beamCommits, "commit", "C", nil,
    "send exactly this commit (repeatable); skips the picker")
```

- Validación temprana en `runBeam`, antes del guard headless actual:
  - `len(beamCommits) > 0 && beamAuto` → error de uso.
- El guard headless pasa a exigir `beamAuto || len(beamCommits) > 0`, con el
  hint actualizado.
- Rama de selección nueva junto a la de `beamAuto`: resolver cada flag con
  `ResolveCommit`, dedup, mapear contra la lista `commits` (ahead) por SHA
  completo; cualquier no-match → error con el listado de shorts válidos.
  `selectedCommits` se construye filtrando `commits` en su orden original
  (garantiza orden topológico y reusa los `git.Commit` ya poblados).
- El resto del flujo (file picker si interactivo, upload, deletes, tracking,
  `-s`, `emit`) queda intacto — la selección explícita entra por el mismo
  camino que la del picker.

### `cmd/help.go` y docs

- Añadir `--commit` a la ayuda de beam.
- `README.md`: sección headless de beam — ejemplo con `--commit`.
- `CHANGELOG.md` bajo *Unreleased*:
  - `[ADD] beam: --commit flag to send explicit commits headless (repeatable)`
- `context/architecture.md`: actualizar la tabla/nota headless de beam
  (ahora `-a` **o** `--commit`); `context/progress-tracker.md` al cerrar.

## Dependencies

Ninguna nueva. Reusa `git.CommitsAheadOf`, el flujo de `runBeam`, el contrato
headless de Unit 19 y `rev-parse` vía el runner git existente.

## Verify when done

- [ ] `teleport beam --commit <short-sha> --no-input` envía exactamente los
      archivos de ese commit, sin abrir picker ni viewer, y sale `0`.
- [ ] Dos `--commit` no contiguos se envían en orden topológico de la rama,
      no en el orden de los flags.
- [ ] `--commit` con un SHA que no está ahead falla con exit `1` nombrando
      el SHA inválido y listando los válidos, **sin** conectar al remoto.
- [ ] `--commit` de un commit ya marcado sent lo re-envía (explícito gana) y
      el tracking queda actualizado tras el éxito.
- [ ] `--commit HEAD` y el SHA completo equivalente seleccionan el mismo
      commit (resolución rev-parse); duplicados se deduplican.
- [ ] `beam --commit X -a` falla como error de uso sin tocar nada.
- [ ] `beam --no-input` sin `-a` ni `--commit` sigue saliendo `2`, ahora con
      el hint que menciona ambos flags.
- [ ] En terminal interactiva, `--commit` salta el picker pero el file-diff
      viewer sí se abre; `-a` conserva su comportamiento actual exacto.
- [ ] `--commit` compone con `-c`, `-s`, `-y`, `--json` (JSON shape sin
      cambios) y con `--then` si la Unit 23 ya está implementada.
- [ ] `teleport help` documenta `--commit`.
- [ ] `go build ./...` y `go vet ./...` pasan.
