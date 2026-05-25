# Unit 08: Config polish — ayuda estilizada e info contextual en `config get`

## Goal

Mejorar el comando `teleport config` en dos ejes:

1. **Ayuda estilizada**: `teleport config` (y sus subcomandos `get`,
   `set`, `unset`) actualmente usan la ayuda por defecto de cobra
   (texto plano). Debe replicar el estilo de la ayuda principal
   ([cmd/help.go](cmd/help.go)) — badge de título, secciones con
   `sectionStyle`, iconos Nerd Font y bloque de `Examples`.
2. **`config get` informativo**: cuando se invoca sin argumentos,
   además de los valores actuales debe mostrar el profile activo con
   su `host:path` resuelto y la última vez que se sincronizó con ese
   remoto. `config get <key>` mantiene su salida actual (solo el
   valor) para no romper scripts.

Para soportar el "última sync" se persiste un timestamp en el config
local; se actualiza desde `sync`, `beam` y `clean` al terminar
correctamente.

## Design

### `teleport config` — ayuda estilizada

- Extraer un helper reutilizable en [cmd/help.go](cmd/help.go):

  ```go
  type helpDoc struct {
      Title       string         // ej. " teleport config "
      Tagline     []string       // 1-2 líneas en descStyle
      Commands    []cmdEntry     // opcional
      Flags       []flagEntry    // opcional
      Examples    [][]string     // opcional
      KeysTable   []keyEntry     // nuevo: para config (key, type, default, desc)
  }

  func renderHelp(doc helpDoc) string
  ```

  El `printHelp` actual pasa a construir un `helpDoc` y llamar a
  `renderHelp`. No se cambia el output del help principal — debe
  quedar **byte-idéntico** salvo donde haya bug.

- Definir `keyEntry`:

  ```go
  type keyEntry struct {
      icon    rune
      name    string   // "sync-untracked"
      typ     string   // "bool"
      def     string   // "false"
      desc    string   // "incluir untracked files al hacer sync"
  }
  ```

  `KeysTable` se renderiza como cuarta sección debajo de `Examples`
  con `sectionStyle.Render("Config keys")`. Layout:

  ```
      <icon>  <name:18>  <typ:6>  <def:10>  <desc>
  ```

  Usar `iconGear` para todas las keys.

- `configCmd.SetHelpFunc(func(*cobra.Command, []string){ ... })` para
  imprimir el `helpDoc` propio cuando se hace `teleport config -h` o
  `teleport config --help`. Lo mismo para `configGetCmd`,
  `configSetCmd`, `configUnsetCmd` (cada uno con su propio `helpDoc`
  — title `" teleport config get "`, tagline, ejemplos específicos y
  la misma `KeysTable`).

- Iconos para el título: `iconGear` (` `) ya está disponible en
  [cmd/help.go:11](cmd/help.go:11). Reutilizar `titleStyle`,
  `sectionStyle`, `iconStyle`, `nameStyle`, `descStyle`,
  `exampleCmdStyle`. No introducir colores nuevos — todo desde la
  paleta ya definida en [context/ui-context.md](context/ui-context.md)
  / [cmd/help.go](cmd/help.go).

- Tagline para `teleport config`:
  ```
  Per-directory defaults for teleport (saved in ~/.config/teleport/projects/).
  ```

- Ejemplos sugeridos para `teleport config`:
  ```
  teleport config get                       show all values + active profile + last sync
  teleport config get default-profile       print one value (scriptable)
  teleport config set sync-untracked true   remember -u for this wd
  teleport config unset default-profile     fall back to global default
  ```

### `teleport config get` (sin args) — output enriquecido

Nueva salida (los colores siguen la paleta `descStyle`/`sectionStyle`
de help.go; el header en `titleStyle`; usar `iconStyle` para iconos):

```
   teleport config

  default-profile  = staging
  sync-untracked   = false

   profile staging
    host  =  deploy@vps-1.example.com
    path  =  /srv/app

   last sync  =  2026-05-25 09:42:10 -06 (hace 4 horas)
```

Reglas:

- Si `default-profile` está vacío en el local config, leer del
  global (`GlobalConfig.Profiles`) cuando exista exactamente uno;
  marcarlo como `(from global)`. Si hay varios y ninguno seleccionado,
  imprimir `default-profile  = <unset>` y omitir el bloque de profile.
- Si el profile resuelto no existe en `GlobalConfig.Profiles`,
  imprimir el bloque de profile como:
  ```
  profile staging  (not found in global config)
  ```
  y omitir host/path.
- `last sync`:
  - leer `LastSync` (ver más abajo) del local config;
  - si es zero-value: `last sync  = never`;
  - si tiene valor: imprimir RFC3339 truncado a segundos + paréntesis
    relativo en español usando un helper local
    `humanizeSince(t time.Time) string` con cortes:
    `<1 min → "hace unos segundos"`,
    `<60 min → "hace N minutos"`,
    `<24 h → "hace N horas"`,
    `<7 d → "hace N días"`,
    resto → `"hace MMM DD"` (formato `2 ene`).

- `config get <key>` **no cambia**: sigue imprimiendo solo el valor
  para no romper scripts (`teleport config get default-profile`).

### Persistencia de `last sync`

- Añadir campo en [internal/config/config.go](internal/config/config.go:26):
  ```go
  type LocalConfig struct {
      DefaultProfile string    `toml:"default_profile"`
      SyncUntracked  bool      `toml:"sync_untracked,omitempty"`
      LastSync       time.Time `toml:"last_sync,omitempty"`
  }
  ```
  `BurntSushi/toml` ya serializa `time.Time` como RFC3339 nativo.

- Helper nuevo en el mismo paquete:
  ```go
  // TouchLastSync sets LastSync = time.Now() and persists the local config.
  // Safe to call on a fresh dir (creates the file).
  func TouchLastSync() error
  ```

- Invocar `config.TouchLastSync()` al final de un run exitoso en:
  - `cmd/sync.go` — después de imprimir el resumen, solo si **al menos
    un archivo se subió OK y no hubo errores fatales**.
  - `cmd/beam.go` — después de aplicar commits remotos
    correctamente; si `--then-sync/-s` también encadena sync, basta
    con la llamada de sync (no duplicar).
  - `cmd/clean.go` — `clean` también cuenta como "tocar el remoto",
    actualizar `LastSync` cuando termina sin error.

  Si `TouchLastSync` falla, **no abortar el comando** — log
  `warn: could not update last sync timestamp: %v` y continuar.

### Validación de cobra

- `configCmd` actualmente no tiene `RunE`: al hacer `teleport config`
  cobra muestra el `Short`+ usage por defecto. Tras este cambio,
  `teleport config` (sin subcomando) debe imprimir el `helpDoc`
  estilizado (mismo que `-h`). Usar:
  ```go
  configCmd.Run = func(c *cobra.Command, _ []string) { c.Help() }
  ```

## Implementation

### Files to modify

- [cmd/help.go](cmd/help.go) — refactor: extraer `helpDoc`,
  `keyEntry`, `renderHelp`. `printHelp` pasa a construir su `helpDoc`
  y delegar. Añadir `iconKey` si se quiere distinguir keys de
  comandos (opcional; reutilizar `iconGear` está bien).
- [cmd/config.go](cmd/config.go) —
  - `SetHelpFunc` en `configCmd`, `configGetCmd`, `configSetCmd`,
    `configUnsetCmd` con sus respectivos `helpDoc`.
  - `Run` para `configCmd` que llama a `c.Help()`.
  - Reescribir `runConfigGet` para el caso sin argumentos: cargar
    `LoadLocal` + `LoadGlobal`, resolver profile activo, formatear
    el bloque enriquecido. El caso con `args` queda igual.
  - Helper `humanizeSince(t time.Time) string` (privado al paquete
    `cmd`).
- [internal/config/config.go](internal/config/config.go) —
  - Añadir `LastSync time.Time` a `LocalConfig`.
  - Añadir `TouchLastSync() error`.
- [cmd/sync.go](cmd/sync.go), [cmd/beam.go](cmd/beam.go),
  [cmd/clean.go](cmd/clean.go) — invocar `config.TouchLastSync()` al
  finalizar correctamente (ver reglas arriba).

### Files NOT to modify

- [internal/tui/*](internal/tui) — sin cambios.
- [cmd/root.go](cmd/root.go) — sigue ruteando `-h` al `printHelp`
  general; no tocar.

## Dependencies

Ninguna nueva. `time`, `BurntSushi/toml`, `charmbracelet/lipgloss/v2`
y `cobra` ya están en uso.

## Verify when done

- [ ] `teleport config -h` y `teleport config` (sin args) imprimen
  el `helpDoc` estilizado, con badge `titleStyle`, secciones,
  iconos y bloque `Config keys`.
- [ ] `teleport config get -h`, `teleport config set -h`,
  `teleport config unset -h` muestran ayuda estilizada propia
  (no la de cobra por defecto).
- [ ] `teleport -h` produce **el mismo output byte-a-byte** que
  antes del refactor (validar con `diff`).
- [ ] `teleport config get` (sin args) imprime los valores +
  bloque `profile` con host/path resueltos + `last sync` en formato
  absoluto y relativo.
- [ ] `teleport config get default-profile` sigue imprimiendo solo
  el valor (apto para `$()` en scripts).
- [ ] Tras un `teleport sync` exitoso, `teleport config get`
  refleja un `last sync` reciente ("hace unos segundos").
- [ ] `teleport beam` y `teleport clean` también actualizan
  `last_sync`.
- [ ] Si `~/.config/teleport/projects/<hash>.toml` no existe,
  `teleport config get` muestra `last sync = never` sin crashear.
- [ ] Si el profile activo no existe en el global, se muestra
  `(not found in global config)` en vez de host/path falsos.
- [ ] `go build -o teleport .` compila sin errores; `go vet ./...`
  limpio.
