# Unit 20: `teleport mirror` — reflejo git fiel del commit (mismo hash + mensaje)

## Goal

Un comando nuevo `teleport mirror [profile]` que **avanza la rama del
remoto a los mismos commits que tienes en local**, con **hash y mensaje
idénticos**, dejando el working tree remoto **limpio** en el nuevo HEAD.
A diferencia de `beam` —que copia *contenidos de archivos* por SFTP y deja
el working tree sucio sin tocar la historia git—, `mirror` transfiere los
**objetos git reales** (vía `git bundle`), de modo que el `git log` del
servidor queda idéntico al local. Sube de a poco eligiendo hasta qué commit
avanzar; el tramo siempre es **contiguo** desde el HEAD remoto, y por eso el
hash se preserva por construcción.

## Design

### El insight: transferir objetos, no reconstruir commits

El hash de un commit es el SHA sobre `tree + parent(s) + author + committer
+ mensaje`. Si se transfiere el **objeto git tal cual** (bundle/pack), git
lo almacena bajo su hash original **sin recalcularlo**: hash y mensaje
idénticos salen gratis. El único requisito es que el/los **padre(s)** ya
estén en el remoto (prerrequisito del bundle). Por eso `mirror` **nunca**
reconstruye el commit (re-`git commit` con las mismas fechas sería frágil:
cualquier diferencia de modo de archivo, EOL u orden de árbol rompe el hash).

### Prefijo contiguo ⇒ hash siempre idéntico

Cada commit arrastra a su padre, así que:

```
remoto en P.        Local:  P → A → B → C → D  (commits ahead)
```

- **Avanzar a un prefijo contiguo** desde `P` (hasta A, o B, o C…): **mismo
  hash** para cada commit del tramo. Solo se mueve el puntero de la rama más
  o menos lejos.
- **Con huecos** (aplicar A y C saltándose B): imposible mantener el hash —
  requeriría rebasar C sobre otro padre (cherry-pick ⇒ hash nuevo). Ese caso
  **no** es trabajo de `mirror`; para desplegar subconjuntos de archivos sin
  reflejar la historia git está `beam` (que ya existe y no cambia).

`mirror` **jamás** produce un hash distinto: si algo lo requeriría, no lo
hace. Es su promesa central.

### `mirror` vs `beam` (no hay solape)

| | `beam` (sin cambios) | `mirror` (nuevo) |
| --- | --- | --- |
| Qué sube | contenidos de archivos (SFTP) | objetos git del commit (bundle) |
| Historia remota | intacta (no crea commits) | **avanza** a los mismos SHAs |
| Mismo hash | n/a | **sí** |
| Working tree remoto | queda *sucio* (por eso existe `clean`) | queda *limpio* en el nuevo HEAD |
| Selección con huecos | sí (cherry-pick de archivos) | no (solo prefijo contiguo) |

### El picker: "elegir destino", no multi-select

`mirror` **no** reusa el multi-select de `beam`. Su picker es de **una sola
selección**: eliges *hasta qué commit avanzar* y se refleja el rango
contiguo `remoteHEAD..elegido`. El cursor arranca en HEAD (avanzar todo).

```
teleport mirror
  ● D (HEAD)    enter → refleja P..D (todo)
  ○ C           enter → refleja P..C  (A,B,C con su hash exacto; D queda para después)
  ○ B
  ○ A
  ── P (base = HEAD remoto)
```

### Semántica de la rama destino

`mirror` avanza **la rama que el remoto tenga *checked out*** (su `HEAD`).
**No** hace `git checkout` de otra rama por sí mismo (inseguro y frágil):
la rama destino se elige **una sola vez** dejándola checkouteada en el remoto,
y de ahí en adelante `mirror` solo la avanza (ff) o la resetea (`--force`).

- El **nombre** de la rama no afecta el hash (el hash no incluye el nombre);
  local `main` puede reflejarse sobre una rama remota `deploy` sin problema.
- Lo que importa es la **ancestría**: el HEAD remoto debe ser ancestro del tip
  elegido (fast-forward); si divergió → `--force`.
- **Setup del remoto (una vez):** un repo git (aunque sea `git init` +
  `git checkout -b <rama>`) con la rama destino checkouteada, idealmente en un
  commit que esté en la historia local (base = ancestro).
- **Rama unborn (repo recién iniciado, sin commits):** `git rev-parse HEAD`
  falla; `mirror` lo detecta (`remoteHEAD == ""`), empaqueta la **historia
  completa** del tip y aplica con `reset --hard` — no destructivo (no hay nada
  que perder). Es el camino de bootstrap para un server vacío.

### Solo fast-forward; `--force` es opt-in y destructivo

Por defecto `mirror` **solo avanza en fast-forward**: nunca descarta commits
del remoto. Si el remoto **divergió** (su HEAD no es ancestro del tip
elegido) o su **working tree está sucio**, `mirror` **rechaza** con un
mensaje claro. `--force`/`-f` habilita el camino destructivo (`reset --hard`
al tip elegido, descartando commits y cambios remotos), detrás de una
confirmación TUI. `--clean`/`-c` encadena `cleanRemote` (Unit 07) antes de
reflejar, para el caso de working tree sucio sin divergencia de historia.

### Sin sent-tracking propio

`beam` recuerda qué commits envió por profile (Unit 15/17) porque su modelo
de archivos no deja rastro en git. `mirror` **no lo necesita**: el propio
`HEAD` del remoto es la fuente de verdad, y cada corrida lo relee. Cero
estado nuevo en el config.

### Integración con headless (Unit 19)

`mirror` respeta el guard `interactive()` y el contrato de exit codes:

- Sin `-a` y headless (sin TTY o `--no-input`) → el target picker no puede
  abrirse → `ErrNeedsTTY` (exit `2`, hint `pass -a`). No transfiere nada.
- Varias ramas locales y headless sin `--branch` → `ErrNeedsTTY` (reusa
  `resolveBranch`, hint `pass --branch`).
- `--force` en headless requiere `-y`/`--no-input` para auto-confirmar el
  `reset --hard`; sin eso → `ErrNeedsTTY` (exit `2`, hint `pass -y`). Misma
  regla que la confirmación de `clean`.
- La fase `--clean` sigue la regla de `clean`: auto-confirma solo con
  `-y`/`--no-input`.
- `--json` emite **un** objeto en `stdout` y manda progreso/decoración a
  `stderr` (reusa `emit`/`useTUI` de Unit 19). Un `ErrNeedsTTY` bajo `--json`
  se serializa `{"error":…,"hint":…}` en `stderr` + exit `2`.

### Salida `--json`

```json
{
  "command": "mirror",
  "target": "staging:/var/www/myproj",
  "branch": "main",
  "from": "80a7ff8",
  "to": "e4f5a6b",
  "commits": ["a1b2c3d", "e4f5a6b"],
  "fast_forward": true,
  "forced": false
}
```

(`from` = HEAD remoto previo; `to` = tip elegido; `commits` = SHAs cortos del
tramo reflejado, del más viejo al más nuevo; `forced` = si se usó `--force`.)

## Implementation

### `internal/git/git.go` — empaquetar el tramo

```go
// BundleTo writes a git bundle at dst containing exactly the commits in
// base..tip (tip inclusive), recording tip under a temporary ref so the
// remote can `git fetch <bundle> refs/teleport/mirror`. When base is empty,
// the bundle carries tip's full history (for a diverged/unrelated remote
// under --force). The temp ref is created and deleted locally around the
// bundle write; it never survives the call.
func BundleTo(base, tip, dst string) error
```

Implementación:
1. `git update-ref refs/teleport/mirror <tip>` (nombra el tip para que el
   bundle almacene un ref; un SHA pelado no queda como ref fetch-eable).
2. `git bundle create <dst> <base>..refs/teleport/mirror` (o, si `base==""`,
   `git bundle create <dst> refs/teleport/mirror` → historia completa).
3. `git update-ref -d refs/teleport/mirror` (siempre, incluso en error).

Helpers adicionales que ya existen y se reusan: `LocalHEAD()`,
`CommitsAheadOf(branch)`, `resolveBranch()` (en `cmd/beam.go`),
`LocalBranches()`.

Nuevo helper de comprobación de ancestro (para decidir fast-forward):

```go
// IsAncestor reports whether maybeAncestor is an ancestor of (or equal to)
// descendant, via `git merge-base --is-ancestor` (exit 0 = yes, 1 = no).
func IsAncestor(maybeAncestor, descendant string) (bool, error)

// MergeBase returns the best common ancestor of a and b, or "" if none
// (unrelated histories). Used to size the bundle under --force.
func MergeBase(a, b string) (string, error)
```

### `cmd/mirror.go` — el comando

```go
var (
    mirrorBranch string // --branch / -b : source branch (default: current)
    mirrorAuto   bool   // --auto   / -a : advance to HEAD, skip target picker
    mirrorClean  bool   // --clean  / -c : run cleanRemote before mirroring
    mirrorForce  bool   // --force  / -f : allow non-ff (reset --hard)
    mirrorYes    bool   // --yes    / -y : skip the --force / clean confirmation
)

var mirrorCmd = &cobra.Command{
    Use:   "mirror [profile]",
    Short: "󰜘 advance the remote branch to your local commits (same hash)",
    Args:  cobra.MaximumNArgs(1),
    RunE:  runMirror,
}
```

Wiring: `rootCmd.AddCommand(mirrorCmd)` en `cmd/root.go`; registrar flags en
`init()`. (No se agrega un flag de atajo en el `rootCmd` raíz como `-b` de
beam; `mirror` es solo subcomando.)

`runMirror(cmd, args)`:

1. `profile, profileName, err := resolveProfile(args)`.
2. Guard headless del target picker: `if !interactive() && !mirrorAuto {
   return errNeedsTTY("pass -a to mirror all unsent commits") }` — falla
   **antes de conectar**, no transfiere nada.
3. `branch, err := resolveBranch(mirrorBranch)` (ya trae el guard headless de
   `--branch`; devuelve la rama actual si hay una sola).
4. `localTip, err := git.LocalHEAD()` (tip de la rama resuelta; si `branch`
   != rama checked-out, usar `git rev-parse <branch>` — añadir
   `git.BranchHEAD(branch)` si hace falta).
5. Conectar: `client, err := connectToProfile(profile)` (defer Close).
6. **Fase clean opcional** (`-c`): `cleanRemote(client, profile, mirrorYes ||
   noInput, false)`; si `counts.Skipped` → abortar como `clean`.
7. Verificar que el remoto es git y leer su HEAD:
   - `git -C <dir> rev-parse --is-inside-work-tree` (mismo error/hint que
     `clean` si falla).
   - `remoteHEAD := git -C <dir> rev-parse HEAD`.
8. **Working tree sucio** (si no se hizo `-c`): `git -C <dir> status
   --porcelain` no vacío → error con hint `pass -c to clean the remote first,
   or -f to force`. (Con `-f` se salta este chequeo; el `reset --hard`
   limpia.)
9. **Commits ahead + target picker**:
   - `commits, err := git.CommitsAheadOf(branch)` (newest-first). Si vacío →
     `"Nothing to mirror — remote already at <localTip short>."` y salir `0`.
   - `sin -a`: `chosen, err := tui.RunMirrorTargetPicker(commits,
     remoteHEAD)` → el commit tip elegido. Cursor default en el más nuevo.
   - `con -a`: `chosen = commits[0]` (HEAD).
   - `chosenSHA := chosen.SHA`.
10. **Decidir fast-forward**:
    - `ff, _ := git.IsAncestor(remoteHEAD, chosenSHA)`.
    - Si `ff` → `base = remoteHEAD`.
    - Si `!ff` y `!mirrorForce` → error: `"remote diverged (HEAD <short> not
      an ancestor of <chosen short>); pass -f to overwrite"` → exit `1`.
    - Si `!ff` y `mirrorForce` → confirmación destructiva:
      - `autoConfirm := mirrorYes || noInput`.
      - `if !autoConfirm && !interactive() → errNeedsTTY("pass -y")`.
      - `if !autoConfirm → tui.RunMirrorForceConfirm(...)` (reusar el estilo
        de `RunCleanConfirm`); si el usuario cancela → `"aborted, no changes
        made"`.
      - `base = git.MergeBase(remoteHEAD, chosenSHA)` (puede ser `""` →
        historias no relacionadas → bundle completo).
11. **Empaquetar + transferir**:
    - `tmp, _ := os.CreateTemp("", "teleport-*.bundle")` (borrar al final).
    - `git.BundleTo(base, chosenSHA, tmp.Name())`.
    - `remoteBundle := filepath.Join(profile.Path, ".git",
      "teleport-mirror.bundle")` (dentro de `.git/` para que `git status` no
      lo vea; se borra siempre).
    - `client.UploadFile(tmp.Name(), remoteBundle)`.
12. **Aplicar en el remoto** (vía `client.RunCommand`, `dir =
    ShellQuote(profile.Path)`):
    - `git -C <dir> bundle verify <remoteBundle>` (confirma prerrequisitos).
    - `git -C <dir> fetch <remoteBundle> refs/teleport/mirror` → FETCH_HEAD =
      chosenSHA.
    - Fast-forward: `git -C <dir> merge --ff-only FETCH_HEAD` (avanza la rama
      checked-out + working tree).
    - Force: `git -C <dir> reset --hard FETCH_HEAD` (descarta commits/cambios
      remotos).
    - Siempre: `rm -f <remoteBundle>` (defer / al final, tolerante a error).
13. `config.TouchLastSync()`.
14. `emit(mirrorResult{…}, printMirrorSummary)` (patrón Unit 19). El humano
    imprime algo como:
    `✓ mirrored <host>:<path> → <chosen short> (N commit(s), fast-forward)`.

`mirrorResult` (struct JSON):

```go
type mirrorResult struct {
    Command     string   `json:"command"`      // "mirror"
    Target      string   `json:"target"`       // host:path
    Branch      string   `json:"branch"`
    From        string   `json:"from"`         // remoteHEAD short
    To          string   `json:"to"`           // chosen short
    Commits     []string `json:"commits"`      // short SHAs, oldest→newest
    FastForward bool     `json:"fast_forward"`
    Forced      bool     `json:"forced"`
}
```

### `internal/tui/mirrortargetpicker.go` — picker de selección única

Modelo bubbletea ligero, single-select, modelado sobre `branchpicker.go`
(que ya es single-select) pero rindiendo filas de commit al estilo del
commit picker (short SHA + subject, con la base remota marcada). `↑/↓`/`j/k`
mueven el cursor, `enter` confirma el tip, `ctrl+c`/`esc` cancela. Muestra
una línea guía indicando que se reflejará el tramo contiguo desde el HEAD
remoto hasta el commit bajo el cursor. Cursor inicial en el índice 0 (HEAD).

```go
// RunMirrorTargetPicker shows the commits ahead (newest first) and returns
// the commit chosen as the new remote tip. remoteHEAD is shown as the base
// marker. Cancelling returns an error the caller treats as a no-op abort.
func RunMirrorTargetPicker(commits []git.Commit, remoteHEAD string) (git.Commit, error)
```

`RunMirrorForceConfirm(plan)` puede reusar `tui.RunCleanConfirm` o un
confirm análogo describiendo cuántos commits del remoto se descartarán.

Los uploaders no aplican aquí: la transferencia es un único `UploadFile` del
bundle + comandos SSH, sin barra de progreso por archivo (a diferencia de
`sync`/`beam`). Un `log.Info`/`fmt.Fprintf(os.Stderr, …)` de una línea basta;
bajo `--json` va a `stderr`.

### `cmd/root.go` y `cmd/help.go`

- `rootCmd.AddCommand(mirrorCmd)`.
- Entrada en `printHelp()` → `Commands`:
  `{iconSync, "mirror", "advance the remote branch to your local commits (same hash + message)"}`.
- Ejemplos:
  - `{"teleport mirror", "pick how far to advance the remote branch"}`
  - `{"teleport mirror -a", "advance the remote to HEAD (same hashes)"}`
  - `{"teleport mirror -cf", "clean + force-reset the remote to local"}`

### `context/architecture.md`

Registrar `mirror` como el segundo modelo de deploy (git-fiel) junto a
`beam` (archivos): tabla o nota corta explicando que `mirror` transfiere
objetos git (hash idéntico, historia avanza, working tree limpio) mientras
`beam` copia archivos (historia intacta, working tree sucio). Invariante
nueva: **`mirror` nunca reescribe un commit — solo fast-forward, salvo
`--force` explícito con confirmación**.

## Dependencies

Ninguna nueva. Reusa `git` (subcomandos `bundle`/`fetch`/`merge`/`reset`/
`merge-base` del git ya presente en local y remoto), `ssh.Client`
(`UploadFile`, `RunCommand`, `ShellQuote`), `resolveProfile`/`resolveBranch`/
`connectToProfile`/`cleanRemote` (Units 07/09/13), y `emit`/`interactive`/
`useTUI`/`ErrNeedsTTY` (Unit 19). `encoding/json` (stdlib) para `--json`.

## Out of scope (posible fase 2)

- **Toggle de modo de deploy** (`teleport config set deploy-mode mirror`)
  para que el flujo por defecto / `beam` refleje commits en vez de subir
  archivos. Esta unidad entrega el comando explícito; el toggle es un spec
  aparte.
- Reflejar **con huecos** (subconjuntos no contiguos): imposible sin cambiar
  el hash; queda cubierto por `beam` con su modelo de archivos.
- Mapear rama local → rama remota con nombre distinto: innecesario para el
  hash (los objetos son iguales), y `merge --ff-only` avanza la rama
  checked-out del remoto sea cual sea su nombre.

## Verify when done

- [ ] `teleport mirror -a` avanza el HEAD del remoto exactamente al HEAD
      local: `git -C <remoto> rev-parse HEAD` == `git rev-parse HEAD` local
      (mismo hash, no un re-commit), y el working tree remoto queda limpio
      (`git status --porcelain` vacío).
- [ ] Los mensajes de commit en el remoto coinciden byte-a-byte con los
      locales (`git log` idéntico en el tramo reflejado).
- [ ] `teleport mirror` (interactivo) muestra el target picker; elegir un
      commit intermedio C refleja `remoteHEAD..C` con hashes idénticos y deja
      D+ sin enviar; una segunda corrida elige D y avanza el resto.
- [ ] `teleport mirror -a --no-input` corre headless sin abrir picker y
      termina `0`; `teleport mirror --no-input` (sin `-a`) sale `2` con hint
      `pass -a` y **no** transfiere nada.
- [ ] Con el remoto divergido y sin `-f`, `mirror` sale `1` con un mensaje
      que nombra `-f` y **no** toca el remoto; con `-f` en TTY pide
      confirmación; con `-f --no-input` (sin `-y`) sale `2`.
- [ ] Con working tree remoto sucio y sin `-c`/`-f`, `mirror` rechaza con
      hint; `teleport mirror -c` limpia primero y luego refleja en una sola
      sesión SSH.
- [ ] `teleport mirror -a --json` imprime `{command,target,branch,from,to,
      commits[],fast_forward,forced}` en `stdout` y nada decorativo lo
      contamina; el bundle temporal se borra en local y en remoto.
- [ ] `mirror` no escribe ningún estado nuevo en el config (no hay
      sent-tracking); relee `remoteHEAD` en cada corrida.
- [ ] `teleport help` lista `mirror`.
- [ ] `go build ./...` y `go vet ./...` pasan.
