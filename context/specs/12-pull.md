# Unit 12: Pull — traer cambios del remoto al local

## Goal

Agregar `teleport pull [profile]` que descarga al local los archivos que
alguien modificó directamente en el servidor remoto. El remoto debe estar
en el mismo commit que el local — si no, el comando aborta con un hint.
Usa `git status --porcelain` en el remoto para saber exactamente qué
archivos bajarse: trackeados modificados/añadidos/borrados y untracked
(`??`). Sin SHA256, sin iterar todos los archivos.

## Design

### Subcomando

```
teleport pull [profile]
```

- `profile` opcional; si se omite usa `default-profile` del local config.

### Pre-checks (en orden, antes de abrir SFTP)

1. **Working tree local limpio.** Si `git status --porcelain` local
   retorna cualquier línea, abortar:
   ```
   error: uncommitted changes in working tree
   hint: commit or stash your changes before pulling
   ```

2. **Mismo commit.** Comparar `git rev-parse HEAD` local con
   `git rev-parse HEAD` en el remoto vía `RunCommand`. Si difieren,
   abortar:
   ```
   error: remote is not at the same commit
   local:  a1b2c3d
   remote: e4f5g6h
   hint: use `git pull` to sync your commits first
   ```

### Qué archivos bajar

Correr en el remoto (vía `RunCommand`):
```
git -C <profile.Path> status --porcelain=v1 -z
```

Parsear la salida NUL-delimitada. Incluir:
- Cualquier archivo con código de estado trackeado en git (columna 1 o 2
  distinta de espacio y distinta de `?`) — modificados, staged, añadidos,
  borrados, renombrados.
- Archivos untracked (`??`).

Para cada archivo en la lista:
- Si el status es `D` (borrado en remoto): borrar el archivo local
  (`os.Remove`). Si no existe localmente, no es error.
- Para el resto (modificados, añadidos, untracked, renombrados al nuevo
  path): descargar vía SFTP.
- Renombrados (`R`): el path viejo aparece como `D`, el nuevo como `A` —
  el parseo natural lo maneja sin caso especial.

### Output

Durante la operación (a stdout):
```
  ✓ src/main.go
  ✓ internal/config/config.go
  - cmd/old.go  (deleted)
  ✗ cmd/sync.go  (error: ...)
```

Summary al finalizar:
```
Pulled 3 file(s) from server.example.com:/var/www/app
```

Si no hay nada que bajar (remoto limpio):
```
Already up to date.
```

Exit code 1 si uno o más archivos fallaron.

### Fuera de scope

- Flag `--force` para saltarse los pre-checks.
- Selector TUI de archivos.
- Soporte para remotos en un commit diferente.

## Implementation

### `internal/git/git.go`

```go
// HasUncommittedChanges reports whether the working tree has any
// staged or unstaged changes according to git status --porcelain.
func HasUncommittedChanges() (bool, error) {
    lines, err := runGit("status", "--porcelain")
    if err != nil {
        return false, err
    }
    return len(lines) > 0, nil
}

// LocalHEAD returns the full SHA of HEAD.
func LocalHEAD() (string, error) {
    lines, err := runGit("rev-parse", "HEAD")
    if err != nil || len(lines) == 0 {
        return "", fmt.Errorf("git rev-parse HEAD: %w", err)
    }
    return strings.TrimSpace(lines[0]), nil
}
```

### `internal/ssh/client.go`

```go
// DownloadFile copies remotePath from the SFTP server to localPath,
// creating local parent directories as needed.
func (c *Client) DownloadFile(remotePath, localPath string) error {
    src, err := c.SFTP.Open(remotePath)
    if err != nil {
        return fmt.Errorf("open remote %s: %w", remotePath, err)
    }
    defer src.Close()

    if err := os.MkdirAll(filepath.Dir(localPath), 0755); err != nil {
        return fmt.Errorf("mkdir %s: %w", filepath.Dir(localPath), err)
    }

    dst, err := os.Create(localPath)
    if err != nil {
        return fmt.Errorf("create local %s: %w", localPath, err)
    }
    defer dst.Close()

    if _, err := io.Copy(dst, src); err != nil {
        os.Remove(localPath) // partial write — clean up
        return fmt.Errorf("download %s: %w", remotePath, err)
    }
    return nil
}
```

`io` ya está importado en `client.go` (usado en `RemoteSHA256`).

### `cmd/pull.go`

Nuevo archivo. Reutiliza `connectToProfile` / `connectToHost` de
`cmd/clean.go`.

```go
package cmd

import (
    "fmt"
    "os"
    "path/filepath"
    "strings"

    lipgloss "charm.land/lipgloss/v2"
    "github.com/pascualchavez/teleport/internal/config"
    "github.com/pascualchavez/teleport/internal/git"
    "github.com/spf13/cobra"
)

var pullCmd = &cobra.Command{
    Use:   "pull [profile]",
    Short: " download remote changes to local working tree",
    Args:  cobra.MaximumNArgs(1),
    RunE:  runPull,
}

var (
    pullOKStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("82"))
    pullDeleteStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
    pullFailStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("203"))
)

func init() {
    rootCmd.AddCommand(pullCmd)
}

func runPull(_ *cobra.Command, args []string) error {
    // Pre-check 1: local working tree must be clean
    dirty, err := git.HasUncommittedChanges()
    if err != nil {
        return fmt.Errorf("git status: %w", err)
    }
    if dirty {
        return fmt.Errorf("uncommitted changes in working tree\nhint: commit or stash your changes before pulling")
    }

    // Resolve profile
    localCfg, err := config.LoadLocal()
    if err != nil {
        return fmt.Errorf("load local config: %w", err)
    }
    profileName := localCfg.DefaultProfile
    if len(args) > 0 {
        profileName = args[0]
    }
    if profileName == "" {
        return fmt.Errorf("no profile specified; run `teleport init` first or pass a profile name")
    }
    globalCfg, err := config.LoadGlobal()
    if err != nil {
        return fmt.Errorf("load global config: %w", err)
    }
    profile, ok := globalCfg.Profiles[profileName]
    if !ok {
        return fmt.Errorf("profile %q not found; run `teleport init` to create it", profileName)
    }

    client, err := connectToProfile(profile)
    if err != nil {
        return err
    }
    defer client.Close()

    // Pre-check 2: remote must be at the same commit
    localHEAD, err := git.LocalHEAD()
    if err != nil {
        return err
    }
    remoteHEAD, err := client.RunCommand(
        "git -C " + ShellQuote(profile.Path) + " rev-parse HEAD",
    )
    if err != nil {
        return fmt.Errorf("get remote HEAD: %w", err)
    }
    remoteHEAD = strings.TrimSpace(remoteHEAD)
    if localHEAD != remoteHEAD {
        return fmt.Errorf(
            "remote is not at the same commit\nlocal:  %s\nremote: %s\nhint: use `git pull` to sync your commits first",
            localHEAD[:7], remoteHEAD[:7],
        )
    }

    // Get remote dirty files via git status --porcelain=v1 -z
    raw, err := client.RunCommand(
        "git -C " + ShellQuote(profile.Path) + " status --porcelain=v1 -z",
    )
    if err != nil {
        return fmt.Errorf("remote git status: %w", err)
    }

    type entry struct {
        xy   string
        path string
    }
    var entries []entry
    // porcelain=v1 -z: each record is "XY path\0" (no newlines)
    for _, record := range strings.Split(raw, "\x00") {
        record = strings.TrimSpace(record)
        if len(record) < 4 {
            continue
        }
        xy := record[:2]
        path := record[3:]
        entries = append(entries, entry{xy: xy, path: path})
    }

    if len(entries) == 0 {
        fmt.Println("Already up to date.")
        return nil
    }

    var failed int
    for _, e := range entries {
        local := e.path
        remote := filepath.Join(profile.Path, e.path)

        // Deleted in remote: remove locally
        x, y := string(e.xy[0]), string(e.xy[1])
        isDeleted := x == "D" || y == "D"
        if isDeleted {
            if err := os.Remove(local); err != nil && !os.IsNotExist(err) {
                fmt.Printf("  %s %s  (%s)\n", pullFailStyle.Render("✗"), local, err)
                failed++
            } else {
                fmt.Printf("  %s %s\n", pullDeleteStyle.Render("-"), local)
            }
            continue
        }

        if err := client.DownloadFile(remote, local); err != nil {
            fmt.Printf("  %s %s  (%s)\n", pullFailStyle.Render("✗"), local, err)
            failed++
        } else {
            fmt.Printf("  %s %s\n", pullOKStyle.Render("✓"), local)
        }
    }

    pulled := len(entries) - failed
    fmt.Printf("\nPulled %d file(s) from %s:%s\n", pulled, profile.Host, profile.Path)

    if failed > 0 {
        return fmt.Errorf("%d file(s) failed", failed)
    }
    return nil
}
```

**Nota sobre `ShellQuote`:** ya existe en `internal/ssh/client.go` como
función de paquete. En `cmd/pull.go` se llama como `sshpkg.ShellQuote`
si se importa el paquete, o se puede agregar el import
`sshpkg "github.com/pascualchavez/teleport/internal/ssh"` y usarlo así.
Verificar que el import no genere ciclo (no lo genera: `cmd` → `internal/ssh`
es la dirección permitida).

## Dependencies

Sin dependencias nuevas.

## Verify when done

- [ ] `go build -o teleport .` y `go vet ./...` limpios.
- [ ] Working tree local sucio: aborta con "uncommitted changes", no
  descarga nada.
- [ ] Remoto en commit diferente: aborta mostrando los SHAs cortos y el
  hint de `git pull`.
- [ ] Remoto limpio (sin cambios): imprime "Already up to date."
- [ ] Remoto con archivos modificados/añadidos: descarga correctamente,
  imprime `✓` por archivo y el summary.
- [ ] Remoto con archivo borrado (`D`): el archivo se elimina localmente,
  imprime `-` con el path.
- [ ] Archivo que falla al descargar: imprime `✗` con el error, el resto
  continúa, exit code 1 al final.
- [ ] `teleport pull nonexistent-profile`: sale con error "not found".
- [ ] `teleport pull` sin perfil configurado: sale con "no profile
  specified".
- [ ] `teleport pull` visible en `teleport --help`.
