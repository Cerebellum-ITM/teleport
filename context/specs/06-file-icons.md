# Unit 06: File icons — ampliar el icon map del sync progress

## Goal

Ampliar `fileTypeIcon` en `internal/tui/syncprogress.go` para que los
archivos sincronizados muestren un glyph Nerd Font apropiado a su tipo
en vez de caer al fallback genérico `` (cod-file). Hoy solo 11
extensiones tienen icono (`go, py, js, ts, md, json, yaml, yml, html,
css, rs`); todo lo demás —incluyendo `.xml`, scripts, imágenes,
archivos comprimidos, lenguajes nativos, etc.— se ve idéntico.

Sin cambios de comportamiento: solo más cobertura de extensiones en el
mismo map y mismo fallback cuando no haya match.

## Design

Mismo helper, mismo flujo de render en `RenderSyncProgress`. El cambio
es puramente de datos: ampliar el map `icons` con las extensiones de
los siguientes grupos.

### Grupo 1 — Markup / config / shell

`xml, svg, toml, ini, env, conf, cfg, lock, gitignore, dockerfile,
makefile, sh, bash, zsh, fish, ps1, bat`

### Grupo 2 — Frontend extra

`jsx, tsx, vue, svelte, scss, sass, less`

### Grupo 3 — Lenguajes (nativos / JVM / otros)

`c, cpp, cc, h, hpp, java, kt, swift, rb, php, lua, dart, ex, exs`

### Grupo 4 — Datos / texto / media / archivos

- Datos: `sql, csv, tsv, db, sqlite`
- Texto: `txt, log, rst`
- Imágenes: `png, jpg, jpeg, gif, webp, ico, bmp`
- Comprimidos / binarios: `zip, tar, gz, tgz, 7z, rar, pdf, exe, bin`

### Resolución de glyphs

Para cada extensión, usar el skill `/nerd-fonts` para escoger el glyph
adecuado (preferir Devicon / Seti / Material cuando exista una marca
clara: `nf-dev-*`, `nf-seti-*`, `nf-md-*`). Reglas:

- Familias relacionadas pueden compartir glyph (`scss/sass`, `c/h`,
  `cpp/cc/hpp`, `jpg/jpeg`, `tar/gz/tgz`).
- Si el glyph elegido tiene ancho variable, mantener el patrón actual
  del archivo (todos los iconos del map terminan sin espacio; el
  espacio lo agrega `fmt.Fprintf` en la línea de render).
- No introducir glyphs que requieran codepoint fuera de Nerd Fonts v3.

### Casos especiales por nombre completo

`dockerfile`, `makefile` y `gitignore` no son extensiones: vienen del
basename. `filepath.Ext` los devuelve vacío (o `.gitignore` para el
último). Manejarlos antes del lookup por extensión:

```go
base := strings.ToLower(filepath.Base(path))
switch base {
case "dockerfile":       return ""
case "makefile":         return ""
case ".gitignore":       return ""
}
```

Mantener el `switch` corto y arriba del map para que el flujo siga
siendo lineal.

### Fallback

Sin cambio: si la extensión no está en el map, devolver `` (cod-file).

## Implementation

### `internal/tui/syncprogress.go` — `fileTypeIcon`

Sustituir el cuerpo actual (líneas 165–187) por:

```go
func fileTypeIcon(path string) string {
    base := strings.ToLower(filepath.Base(path))
    switch base {
    case "dockerfile":
        return "<glyph-docker>"
    case "makefile":
        return "<glyph-make>"
    case ".gitignore":
        return "<glyph-git>"
    }

    ext := strings.ToLower(filepath.Ext(path))
    if len(ext) > 1 {
        ext = ext[1:]
    }

    icons := map[string]string{
        // existentes
        "go":   "",
        "py":   "",
        "js":   "",
        "ts":   "",
        "md":   "",
        "json": "",
        "yaml": "",
        "yml":  "",
        "html": "",
        "css":  "",
        "rs":   "",

        // grupo 1 — markup/config/shell
        "xml":  "<glyph>",
        "svg":  "<glyph>",
        "toml": "<glyph>",
        "ini":  "<glyph>",
        "env":  "<glyph>",
        "conf": "<glyph>",
        "cfg":  "<glyph>",
        "lock": "<glyph>",
        "sh":   "<glyph>",
        "bash": "<glyph>",
        "zsh":  "<glyph>",
        "fish": "<glyph>",
        "ps1":  "<glyph>",
        "bat":  "<glyph>",

        // grupo 2 — frontend
        "jsx":    "<glyph>",
        "tsx":    "<glyph>",
        "vue":    "<glyph>",
        "svelte": "<glyph>",
        "scss":   "<glyph>",
        "sass":   "<glyph>",
        "less":   "<glyph>",

        // grupo 3 — lenguajes
        "c":    "<glyph>",
        "cpp":  "<glyph>",
        "cc":   "<glyph>",
        "h":    "<glyph>",
        "hpp":  "<glyph>",
        "java": "<glyph>",
        "kt":   "<glyph>",
        "swift":"<glyph>",
        "rb":   "<glyph>",
        "php":  "<glyph>",
        "lua":  "<glyph>",
        "dart": "<glyph>",
        "ex":   "<glyph>",
        "exs":  "<glyph>",

        // grupo 4 — datos/texto/media/archivos
        "sql":    "<glyph>",
        "csv":    "<glyph>",
        "tsv":    "<glyph>",
        "db":     "<glyph>",
        "sqlite": "<glyph>",
        "txt":    "<glyph>",
        "log":    "<glyph>",
        "rst":    "<glyph>",
        "png":    "<glyph>",
        "jpg":    "<glyph>",
        "jpeg":   "<glyph>",
        "gif":    "<glyph>",
        "webp":   "<glyph>",
        "ico":    "<glyph>",
        "bmp":    "<glyph>",
        "zip":    "<glyph>",
        "tar":    "<glyph>",
        "gz":     "<glyph>",
        "tgz":    "<glyph>",
        "7z":     "<glyph>",
        "rar":    "<glyph>",
        "pdf":    "<glyph>",
        "exe":    "<glyph>",
        "bin":    "<glyph>",
    }
    if icon, ok := icons[ext]; ok {
        return icon
    }
    return "" // cod-file fallback
}
```

Los placeholders `<glyph-…>` y `<glyph>` se resuelven en el paso de
implementación corriendo `/nerd-fonts` por familia (no inventar
codepoints en el spec). Si un grupo no tiene un glyph claro de marca,
escoger uno neutro de Material/Codicons (p. ej. `nf-md-file_image`
para imágenes, `nf-md-zip_box` para comprimidos).

### Resto del archivo

Sin cambios. `RenderSyncProgress` ya pasa el path completo a
`fileTypeIcon`; el helper sigue siendo puro y testeable.

### Documentación

- `CHANGELOG.md` bajo *Unreleased*:
  `[ADD] tui: expand sync progress icon map (xml, shell, images, archives, more)`
- `context/progress-tracker.md`: agregar Unit 06 a Completed.

## Dependencies

Ninguna nueva. `strings` y `filepath` ya importados.

## Verify when done

- [ ] `.xml` muestra un glyph dedicado (no el fallback genérico).
- [ ] Un archivo `Dockerfile` (sin extensión) muestra el glyph de
      Docker, no el fallback.
- [ ] Un archivo `Makefile` muestra el glyph de make.
- [ ] `.gitignore` muestra el glyph de git.
- [ ] Archivos shell (`.sh`, `.bash`, `.zsh`, `.fish`) muestran glyph
      de terminal/shell.
- [ ] Imágenes (`.png`, `.jpg`, `.svg`) muestran glyph de imagen
      (o el específico de SVG en su caso).
- [ ] Comprimidos (`.zip`, `.tar.gz`) muestran glyph de archivo
      comprimido.
- [ ] Una extensión no contemplada (p. ej. `.xyz`) sigue cayendo al
      fallback `` sin error.
- [ ] La extensión es case-insensitive (`.XML` se resuelve igual que
      `.xml`).
- [ ] `go build ./...` pasa.
- [ ] `go vet ./...` pasa.
