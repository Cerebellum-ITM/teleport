# Unit 09: SSH password auth — fallback a contraseña cuando no hay llave

## Goal

Cuando `teleport` no encuentra ningún método de autenticación por
llave (sin agente SSH activo y sin archivos de llave en `~/.ssh/`),
en lugar de abortar con un error críptico debe detectar esa condición
y pedir la contraseña del servidor de forma interactiva, reintentar
la conexión con `ssh.Password`, y continuar el flujo normal.

El objetivo es que un usuario sin llaves SSH configuradas pueda usar
la herramienta simplemente ingresando su contraseña.

## Design

### Flujo de autenticación

El flujo actual en `buildAuthMethods` es:

```
SSH_AUTH_SOCK → key files → error "no auth methods available"
```

El nuevo flujo:

```
SSH_AUTH_SOCK → key files → [ninguno disponible] → pedir contraseña → ssh.Password(pw)
```

Si hay métodos de llave disponibles, el comportamiento es idéntico al
actual. La contraseña solo entra en juego cuando **ningún** método de
llave fue encontrado.

### Prompt de contraseña

- Se muestra con `huh.NewForm` + `huh.NewInput().EchoMode(huh.EchoModePassword)`.
- Título: `"Password for <user>@<host>"`.
- Si el usuario cancela el form (presiona `Ctrl-C`/`Esc`), devolver
  el mismo error que antes (`"no SSH auth methods available — cancelled"`).
- El prompt ocurre en la capa `cmd/`, **no** dentro de
  `internal/ssh/`. Ver arquitectura más abajo.

### Arquitectura — cómo pasar el password sin mezclar capas

`internal/ssh.buildAuthMethods` (privada) no debe importar `huh` ni
hacer I/O interactivo. En cambio:

1. Añadir un centinela exportado en `internal/ssh/`:

   ```go
   // ErrNoAuthMethods se retorna cuando no hay agente ni key files.
   var ErrNoAuthMethods = errors.New("no SSH auth methods available")
   ```

2. `buildAuthMethods` retorna `ErrNoAuthMethods` (en lugar del error
   formateado actual) cuando el slice de signers queda vacío.

3. `Connect` propaga ese error sin modificarlo.

4. En `cmd/`, un helper compartido `connectToProfile` (ya existe en
   `cmd/clean.go`; extraer a `cmd/ssh_helpers.go` si no lo está)
   atrapa `errors.Is(err, sshpkg.ErrNoAuthMethods)` y ejecuta el
   prompt de contraseña:

   ```go
   client, err := sshpkg.Connect(host)
   if errors.Is(err, sshpkg.ErrNoAuthMethods) {
       pw, err := promptPassword(host)
       if err != nil {
           return nil, err
       }
       client, err = sshpkg.ConnectWithPassword(host, pw)
   }
   ```

5. Añadir `ConnectWithPassword(host Host, pw string) (*Client, error)`
   en `internal/ssh/client.go` — idéntica a `Connect` pero usa
   `ssh.Password(pw)` como único auth method.

### Cambios por archivo

- `internal/ssh/client.go`:
  - Exportar `ErrNoAuthMethods`.
  - `buildAuthMethods` retorna `ErrNoAuthMethods` cuando
    `len(signers) == 0`.
  - Nueva función `ConnectWithPassword(host Host, pw string) (*Client, error)`.

- `cmd/ssh_helpers.go` (nuevo, o agrandar `cmd/clean.go` si ya está):
  - Mover/definir `resolveProfile` y `connectToProfile` aquí si no
    están ya en un lugar compartido.
  - `connectToProfile` atrapa `ErrNoAuthMethods`, llama a
    `promptPassword`, reintenta con `ConnectWithPassword`.
  - `promptPassword(host sshpkg.Host) (string, error)` — usa `huh`.

- `cmd/sync.go`, `cmd/beam.go`, `cmd/status.go`:
  - Si todavía llaman a `sshpkg.Connect` directamente, migrar a
    `connectToProfile` para heredar el fallback de password.
  - `cmd/clean.go` ya usa `connectToProfile`; verificar que baste con
    actualizar ese helper.

### No incluir en esta unidad

- Persistencia de contraseñas (keychain, netrc, etc.).
- Soporte de `keyboard-interactive` (challenge-response multi-paso).
- Cambio en la lógica de `known_hosts` ni host key verification.

## Implementation

### 1. `internal/ssh/client.go`

```go
// ErrNoAuthMethods indica que no hay llave ni agente disponibles.
var ErrNoAuthMethods = errors.New("no SSH auth methods available")

// buildAuthMethods — cambio: reemplazar el error final por ErrNoAuthMethods
if len(signers) == 0 {
    return nil, ErrNoAuthMethods
}
return []ssh.AuthMethod{ssh.PublicKeys(signers...)}, nil

// ConnectWithPassword — nueva función
func ConnectWithPassword(host Host, password string) (*Client, error) {
    // Misma lógica que Connect pero Auth: []ssh.AuthMethod{ssh.Password(password)}
}
```

### 2. `cmd/ssh_helpers.go` (nuevo archivo)

```go
package cmd

import (
    "errors"
    "fmt"

    "charm.land/huh/v2"
    sshpkg "github.com/pascualchavez/teleport/internal/ssh"
)

func connectToProfile(host sshpkg.Host) (*sshpkg.Client, error) {
    client, err := sshpkg.Connect(host)
    if err == nil {
        return client, nil
    }
    if !errors.Is(err, sshpkg.ErrNoAuthMethods) {
        return nil, err
    }

    pw, err := promptPassword(host)
    if err != nil {
        return nil, err
    }
    return sshpkg.ConnectWithPassword(host, pw)
}

func promptPassword(host sshpkg.Host) (string, error) {
    var pw string
    form := huh.NewForm(huh.NewGroup(
        huh.NewInput().
            Title(fmt.Sprintf("Password for %s@%s", host.User, host.Hostname)).
            EchoMode(huh.EchoModePassword).
            Value(&pw),
    ))
    if err := form.Run(); err != nil {
        return "", fmt.Errorf("password prompt: %w", err)
    }
    return pw, nil
}
```

### 3. Migración de callers

Revisar cada lugar donde se llama `sshpkg.Connect(...)` en `cmd/`:

| Archivo       | Acción                                        |
|---------------|-----------------------------------------------|
| `cmd/init.go` | Reemplazar por `connectToProfile(host)`        |
| `cmd/sync.go` | Reemplazar por `connectToProfile(host)`        |
| `cmd/beam.go` | Reemplazar por `connectToProfile(host)` (ya tiene helper) |
| `cmd/clean.go`| Ya usa helper; actualizar el helper existente  |
| `cmd/status.go`| Reemplazar por `connectToProfile(host)`       |

Si `connectToProfile` ya existe en `cmd/clean.go`, moverla a
`cmd/ssh_helpers.go` y dejar que `clean.go` la importe del mismo
package (mismos paquete `cmd`, no necesita import).

## Dependencies

Sin dependencias nuevas. `huh` y `golang.org/x/crypto/ssh` ya están
en el `go.mod`.

## Verify when done

- [ ] `go build -o teleport .` compila sin errores; `go vet ./...` limpio.
- [ ] Con `SSH_AUTH_SOCK` desactivado y sin archivos de llave, `teleport sync`
  muestra el prompt de contraseña en lugar del error actual.
- [ ] Ingresar una contraseña correcta: la conexión procede y el sync funciona.
- [ ] Ingresar una contraseña incorrecta: SSH retorna error de auth; se muestra
  el error sin reintentar (no loop infinito).
- [ ] Cancelar el prompt (`Ctrl-C`/`Esc`): el comando termina con error claro,
  no panic.
- [ ] Con agente SSH o llaves configuradas, el prompt **no aparece** — el
  comportamiento es idéntico al actual.
- [ ] `teleport init` también usa el fallback (el host picker conecta tras
  seleccionar el host).
- [ ] `teleport status` y `teleport beam` también usan el fallback.
