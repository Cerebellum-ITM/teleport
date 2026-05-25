# Unit 10: Remove profile — eliminar un profile del global config

## Goal

Agregar el subcomando `teleport profiles remove <name>` (alias:
`teleport profiles rm <name>`) que elimina un profile del global
config (`~/.config/teleport/config.toml`). Si el profile eliminado
es el `default-profile` del local config del directorio actual,
emite un warning pero no aborta.

## Design

### Subcomando

```
teleport profiles remove <name>
teleport profiles rm <name>          # alias
```

- Argument: nombre del profile. Exactamente 1 (`cobra.ExactArgs(1)`).
- Si el profile no existe en el global config: error claro.
  ```
  error: profile "staging" not found
  hint: run `teleport profiles` to see all configured profiles
  ```
- Si el profile existe: eliminarlo del map y guardar.
- Output en éxito:
  ```
  Profile "staging" removed.
  ```
- Si ese profile era el `default-profile` del local config actual,
  agregar una línea de warning:
  ```
  Profile "staging" removed.
  warn: "staging" was the default profile for this directory — run `teleport config set default-profile <name>` to pick a new one
  ```
  No modificar el local config automáticamente — dejar que el usuario
  lo resuelva.

### No incluir en esta unidad

- Confirmación interactiva (TUI/form) antes de borrar.
- Borrado en cascada del local config.
- `teleport profiles rename`.

## Implementation

### `cmd/profiles.go`

Añadir dos subcomandos al `profilesCmd` existente:

```go
var profilesRemoveCmd = &cobra.Command{
    Use:     "remove <name>",
    Aliases: []string{"rm"},
    Short:   "remove a profile from the global config",
    Args:    cobra.ExactArgs(1),
    RunE:    runProfilesRemove,
}

func init() {
    profilesCmd.AddCommand(profilesRemoveCmd)
}

func runProfilesRemove(_ *cobra.Command, args []string) error {
    name := args[0]

    globalCfg, err := config.LoadGlobal()
    if err != nil {
        return fmt.Errorf("load global config: %w", err)
    }

    if _, ok := globalCfg.Profiles[name]; !ok {
        return fmt.Errorf("profile %q not found\nhint: run `teleport profiles` to see all configured profiles", name)
    }

    delete(globalCfg.Profiles, name)

    if err := config.SaveGlobal(globalCfg); err != nil {
        return fmt.Errorf("save global config: %w", err)
    }

    fmt.Printf("Profile %q removed.\n", name)

    // Warn if the removed profile was the local default
    localCfg, _ := config.LoadLocal()
    if localCfg != nil && localCfg.DefaultProfile == name {
        log.Warn(fmt.Sprintf("%q was the default profile for this directory", name),
            "hint", "run `teleport config set default-profile <name>` to pick a new one")
    }

    return nil
}
```

### `internal/config/config.go`

Añadir helper (opcional pero limpio):

```go
// RemoveProfile deletes name from Profiles. No-op if not present.
func (g *GlobalConfig) RemoveProfile(name string) {
    delete(g.Profiles, name)
}
```

## Dependencies

Sin dependencias nuevas.

## Verify when done

- [ ] `go build -o teleport .` compila sin errores; `go vet ./...` limpio.
- [ ] `teleport profiles remove staging` con un profile existente: imprime
  `Profile "staging" removed.` y el profile desaparece del config.
- [ ] `teleport profiles rm staging` (alias) funciona igual.
- [ ] `teleport profiles remove nonexistent`: muestra error con hint.
- [ ] Si el profile eliminado era el `default-profile` del local config,
  el warning aparece **sin** modificar el local config.
- [ ] `teleport profiles` (listar) después del remove no muestra el profile.
- [ ] `teleport profiles remove` sin argumentos: cobra muestra usage error.
