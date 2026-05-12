# Unit 03: Sync flag defaults per working directory

## Goal

Permitir que cada working directory recuerde sus banderas de sincronización
preferidas (por ahora: `--untracked`) en su `LocalConfig`, expuesto vía un
subcomando `teleport config`. Además, cuando se ejecute `teleport sync` /
`teleport -s` sin `-u` y existan archivos untracked reales, emitir un
warning informativo para que el usuario no los pierda silenciosamente.

## Design

Dos cambios visibles, ambos no intrusivos:

1. **Subcomando `teleport config`** — interfaz dedicada y explícita para
   leer/escribir defaults locales. Sigue el patrón ya usado por `init` y
   `profiles`. No hay prompts automáticos; el usuario opta por persistir
   ejecutando `teleport config set sync-untracked true`.

2. **Warning de untracked** — sólo se imprime cuando el sync corre sin
   `-u` y `git.UntrackedFiles()` devuelve al menos un archivo. Usa
   `log.Warn` con el conteo y una sugerencia, sin bloquear el flujo:

   ```
   WARN  3 untracked file(s) not included. Use -u to sync them, or
         `teleport config set sync-untracked true` to enable by default.
   ```

   Si el wd ya tiene `sync-untracked = true` persistido, el warning no
   aplica porque `-u` queda implícito.

**Resolución de la bandera efectiva** (en `runSync`):

```
effectiveUntracked = flagUntracked || localCfg.SyncUntracked
```

El flag de CLI siempre puede activar; nunca desactivar lo guardado salvo
sobrescribiendo con `teleport config set sync-untracked false`. Esto
preserva el principio de "el wd recuerda mi preferencia" sin volver el
flag confuso.

## Implementation

### `internal/config/config.go` — extender `LocalConfig`

Agregar un campo opcional:

```go
type LocalConfig struct {
    DefaultProfile string `toml:"default_profile"`
    SyncUntracked  bool   `toml:"sync_untracked,omitempty"`
}
```

`omitempty` mantiene los archivos existentes sin ruido cuando la opción
no se ha tocado. No requiere migración: TOML decodifica el campo
faltante como `false`.

### `cmd/config.go` — nuevo archivo

Comando padre `configCmd` con subcomandos:

- `teleport config get [key]`
  - Sin `key`: imprime todo el `LocalConfig` actual en formato
    `key = value` (una línea por campo).
  - Con `key`: imprime sólo ese valor. Keys válidas: `sync-untracked`,
    `default-profile`.
- `teleport config set <key> <value>`
  - Valida `key` ∈ {`sync-untracked`, `default-profile`}.
  - Para `sync-untracked`: parsea `value` con `strconv.ParseBool` y
    rechaza valores inválidos con mensaje claro.
  - Para `default-profile`: acepta cualquier string no vacío (no
    valida existencia del profile aquí; el sync ya falla con mensaje
    útil si no existe).
  - Carga `LocalConfig`, muta el campo, llama `SaveLocal`.
  - Imprime confirmación: `sync-untracked = true (saved for this wd)`.
- `teleport config unset <key>`
  - Resetea el campo a su zero value y guarda.

Registrar `configCmd` en `rootCmd` dentro de `cmd/root.go` (junto a los
demás `AddCommand`).

Mapeo `key` ↔ campo struct centralizado en una función pequeña
`applyConfigKey(cfg *config.LocalConfig, key, value string) error` para
mantener get/set/unset coherentes.

### `cmd/sync.go` — usar default y emitir warning

Reemplazar el bloque actual:

```go
if includeUntracked {
    untracked, err := git.UntrackedFiles()
    ...
}
```

por:

```go
effectiveUntracked := includeUntracked || localCfg.SyncUntracked

if effectiveUntracked {
    untracked, err := git.UntrackedFiles()
    if err != nil {
        log.Warn("Could not list untracked files", "err", err)
    } else {
        changed = append(changed, untracked...)
    }
} else {
    untracked, err := git.UntrackedFiles()
    if err == nil && len(untracked) > 0 {
        log.Warn(
            fmt.Sprintf("%d untracked file(s) not included", len(untracked)),
            "hint", "use -u, or `teleport config set sync-untracked true`",
        )
    }
}
```

Nota: `localCfg` ya está cargado antes de este bloque (línea 29). No se
agrega ninguna lectura nueva.

### `cmd/help.go` — actualizar texto

Agregar `config` a la lista de comandos disponibles y mencionar
`sync-untracked` como key conocida. Mantener el estilo existente del
help personalizado.

### `cmd/root.go` — sin cambios funcionales

Solo añadir `rootCmd.AddCommand(configCmd)` si no queda hecho desde
`cmd/config.go`'s `init()` (preferir el patrón usado por los demás
subcomandos del proyecto — revisar `syncCmd`/`initCmd` y seguirlo).

### `CHANGELOG.md`

Anotar bajo *Unreleased*:

- `[ADD] config: persist per-wd sync flags via teleport config get/set/unset`
- `[ADD] sync: warn when untracked files exist and -u was not used`

## Dependencies

- ninguna (cobra, BurntSushi/toml, charmbracelet/log ya están en `go.mod`)

## Verify when done

- [ ] `teleport config set sync-untracked true` crea o actualiza el
      `~/.config/teleport/projects/<hash>.toml` con `sync_untracked = true`
- [ ] `teleport config get sync-untracked` imprime `true`
- [ ] `teleport config get` (sin key) lista todos los campos del wd
- [ ] `teleport config unset sync-untracked` deja el campo en `false`
      (omitido del TOML por `omitempty`)
- [ ] `teleport config set sync-untracked maybe` falla con mensaje claro
      y no modifica el archivo
- [ ] Con `sync_untracked = false` y untracked reales, `teleport sync`
      imprime WARN con conteo y hint, y NO incluye los archivos
- [ ] Con `sync_untracked = false` y sin untracked, no aparece WARN
- [ ] Con `sync_untracked = true`, `teleport sync` incluye los untracked
      sin imprimir WARN y sin necesidad de `-u`
- [ ] `teleport sync -u` sigue funcionando aunque el default sea `false`
- [ ] `go build ./...` pasa sin errores
- [ ] `teleport help` muestra el nuevo subcomando `config`
