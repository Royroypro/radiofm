# AGENTS.md — Club25 Estudio (guía para agentes de IA y programadores)

Lea esto antes de tocar `estudio/`. Es corto a propósito; los detalles están en `docs/`.
El plan completo por fases está en [../docs/estudio/plan.md](../docs/estudio/plan.md).

## Qué es

Programa de escritorio para **Windows** que automatiza la radio Radio Club 25 (93.3 FM), al estilo
de un automatizador profesional: biblioteca musical, relojes y parrilla, playlist del día, tandas
de cuñas, botonera de jingles, micrófono con ducking y hora/temperatura locutadas. Saca el audio
a la tarjeta de sonido (consola → transmisor FM) y a Icecast (la web), y se sincroniza con el
panel web del servidor (carpeta raíz de este repositorio) por la API `/api/studio/*`.

- **Go + Wails v2** (ventana con WebView2, frontend Svelte 5 + TypeScript).
- **miniaudio** vía `github.com/gen2brain/malgo` para la E/S de audio (**requiere CGO**).
- **ffmpeg/ffprobe** como subprocesos para decodificar, analizar y codificar.
- **SQLite** (`modernc.org/sqlite`, sin CGO) para la base local.
- Licencia: **GPL-3.0-or-later** (ver `LICENSE`). El servidor de la raíz conserva la suya.

## Estado

Fase 0 (esqueleto): ventana "hola", `estudio-cli version|devices`, scripts y documentación.
Los paquetes de `internal/service` y `internal/platform` solo tienen su `doc.go` con lo que harán.
El avance por fase está en `CHANGELOG.md`.

## Comandos (desde `estudio/`)

```powershell
task setup     # prepara la PC: Wails, Task, gcc (llvm-mingw), PATH, ffmpeg, npm install
task dev       # wails dev: ventana con recarga en caliente del frontend
task build     # build/bin/club25-estudio.exe (con la versión de VERSION en Taskfile.yml)
task test      # go test ./... (CGO activado)
task lint      # gofmt, go vet, staticcheck y svelte-check
task cli -- devices   # ejecutar estudio-cli con argumentos (version, devices, play, render…)
task ffmpeg    # descargar ffmpeg fijado y verificado (third_party/ffmpeg)
task locutions # generar locuciones provisionales con la voz de Windows
```

Sin gcc, `go vet ./...` y `go test ./...` igual funcionan con `CGO_ENABLED=0`: el acceso a la
tarjeta (`audio.ListDevices`) devuelve `audio.ErrNoCGO`. Así un agente puede trabajar en la lógica
pura sin instalar el toolchain C.

## Mapa

```
main.go                  arranque Wails (Wails v2 exige el main en la raíz)
internal/app/            raíz de composición: config, log, base, servicios, tareas en segundo plano
cmd/estudio-cli/         herramientas sin interfaz (version, devices; luego play, encode, analyze…)
internal/domain/         entidades y reglas puras (sin E/S, sin dependencias internas)
internal/service/<x>/    casos de uso: library, rotation, scheduler, events, spots,
                         timeannounce, sync, onair, reports
internal/audio/          motor de audio y E/S con la tarjeta (malgo). No conoce la BD
internal/platform/<x>/   infraestructura: ffmpeg, icecast, apiclient, sqlite, weather, config,
                         logger, secrets (DPAPI), winsys (instancia única, autoarranque…)
internal/ui/             structs que Wails expone al frontend. Sin lógica de negocio
internal/version/        versión (se fija con -ldflags)
frontend/                Svelte 5 + TS + Vite. frontend/wailsjs/ lo genera Wails: no editar a mano
build/windows/           icono, manifest, instalador NSIS
scripts/                 dev-setup.ps1, with-toolchain.ps1 (gcc en el PATH para CGO), get-ffmpeg.ps1,
                         make-locutions.ps1
third_party/ffmpeg/      ffmpeg.exe y ffprobe.exe descargados (fuera de git)
testdata/                audios de prueba generados (nunca música con copyright)
docs/                    setup-windows, architecture, glossary, third-party, adr/
```

## Reglas (no romper)

1. **Capas**: `ui → service → domain ← platform`; `audio` es independiente (recibe pistas ya
   resueltas). `domain` no importa nada interno. `ui` nunca contiene reglas de negocio.
2. **El callback de audio nunca bloquea ni reserva memoria** (nada de mutex largos, canales que
   esperan, `append`, logs ni E/S dentro del callback). Ver `internal/audio/doc.go`.
3. **SQL solo en `internal/platform/sqlite`**, con migraciones nuevas `NNNN_*.sql`; nunca editar una aplicada.
4. **Secretos** (token del panel, clave de Icecast) solo con `platform/secrets` (DPAPI), nunca en texto plano.
5. **Contrato con el servidor**: `../docs/api.md` → "API del estudio". Si cambia, se actualizan en el
   mismo cambio el handler del servidor, el documento y `platform/apiclient` con sus tests.
6. **Código abierto**: se puede reutilizar o portar código GPL-2.0-or-later, GPL-3.0, LGPL, MIT, BSD,
   Apache-2.0. **No** se copia código solo GPL-2.0 ni AGPL (solo ideas). Todo lo tomado se anota en
   `docs/third-party.md` y con `// Basado en <proyecto>/<archivo> (<licencia>)` en el archivo.
7. Identificadores en inglés; comentarios, documentación e interfaz en **español**.
8. Cada paquete tiene `doc.go` con qué hace, qué no hace y sus invariantes.
9. Tests de las reglas puras (rotación, parrilla, eventos, reparto de cuñas) **sin audio**; la mezcla
   se prueba renderizando sin tarjeta y comparando niveles sobre senos generados.

## Recetas

### Añadir un método llamable desde el frontend
1. Método público en `internal/ui/app.go` (o un struct nuevo de `ui` añadido a `Bind` en `main.go`).
   Que solo llame a un servicio y devuelva structs con etiquetas `json`.
2. `task dev` (o `wails generate module`) regenera `frontend/wailsjs/go/...`.
3. Importarlo en Svelte desde `../wailsjs/go/ui/App`.

### Añadir un subcomando a estudio-cli
1. Un `case` en `run` de `cmd/estudio-cli/main.go` que llame a una función propia.
2. Una línea en `usage` y en el comentario del paquete.
3. Caso en `main_test.go`.

### Añadir un servicio
1. Interfaces de repositorio y entidades en `internal/domain`.
2. `internal/service/<x>/` con constructor `New(deps...)` y tests con fakes.
3. Construirlo en `main.go` e inyectarlo en `ui.NewApp`.

## Cómo verificar un cambio

1. `task lint && task test`.
2. `task dev` y probar la pantalla afectada; `task cli -- devices` para el audio.
3. Si toca la sincronización: servidor en local (`make dev` en la raíz, o los comandos de
   `../AGENTS.md`) y token generado en Admin → Estudio (PC).
