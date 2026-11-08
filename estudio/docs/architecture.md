# Arquitectura de Club25 Estudio

## Piezas

```
┌──────────── PC Windows: Club25 Estudio (un .exe, Wails v2) ───────────────┐
│ UI (Svelte 5 + TS, WebView2)  ⇄  bindings Go (internal/ui)                │
│ Programador: parrilla → relojes → playlist del día + eventos/tandas       │
│ Motor de audio (internal/audio): float32 44.1 kHz estéreo, bloques 10 ms  │
│   decks A/B (decodificación ffmpeg en streaming) + carts + mic ─► mixer   │
│   ─► ducking ─► limitador master ─► BUS                                   │
│ BUS ─► malgo/WASAPI ─► tarjeta principal ─► consola/FM   (reloj maestro)  │
│ BUS ─► (opcional) pre-escucha                                             │
│ Fuente Icecast = BUS  ó  CAPTURA (line-in = salida de la consola)         │
│   └► ring buffer no bloqueante ─► ffmpeg.exe (PCM → MP3 128k) ─► Icecast  │
│ Sync (HTTPS + Bearer) ⇄ /api/studio/* del servidor                        │
│ SQLite local en %ProgramData%\Club25Estudio                               │
└───────────────────────────────────────────────────────────────────────────┘
```

## Capas

```
frontend (Svelte) ──► internal/ui ──► internal/service/* ──► internal/domain
                                  │            │                   ▲
                                  │            └──► internal/audio │ (interfaces)
                                  │                                │
main.go (composición) ────────────┴──► internal/platform/* ────────┘
```

| Capa | Paquetes | Puede importar | No puede |
|---|---|---|---|
| Dominio | `internal/domain` | solo la biblioteca estándar | nada interno |
| Servicios | `internal/service/<x>` | `domain`, `audio` (tipos), interfaces | SQL, HTTP directo, Wails |
| Audio | `internal/audio` | malgo, biblioteca estándar | `domain`, BD, servicios |
| Plataforma | `internal/platform/<x>` | `domain` (para implementar sus interfaces) | servicios, `ui` |
| UI | `internal/ui` | servicios, `version` | reglas de negocio |
| Composición | `main.go`, `cmd/estudio-cli` | todo | lógica |

`main.go` está en la raíz del módulo porque Wails v2 compila el paquete main de la raíz y embebe
`frontend/dist` desde allí (ver [ADR 0001](adr/0001-wails-go.md)).

## Flujo del audio (resumen; detalle en `docs/audio-engine.md`, fase 1)

1. El programador entrega al motor la siguiente pista ya resuelta (ruta, cue-in, cue-out, punto de
   mezcla, ganancia por loudness).
2. ~20 s antes, una goroutine arranca `ffmpeg -i pista -f f32le -ar 44100 -ac 2 -` y llena el ring
   buffer del deck libre.
3. El callback de la tarjeta pide N frames: el mixer suma decks, carts y mic, aplica ducking y el
   limitador, y escribe en la tarjeta y en el ring buffer del codificador.
4. El codificador (otro `ffmpeg`) lee de su ring buffer; si Icecast se cae, se descartan muestras y se
   reintenta con espera exponencial. La salida FM nunca se detiene por la red.

## Integración con el servidor

Contrato en [../../docs/api.md](../../docs/api.md) → "API del estudio". La PC:
- descarga configuración, cuñas (verificando sha256) y programación;
- envía heartbeat, "lo que suena" y las emisiones de cuñas (cola local si no hay red).

## Decisiones

Ver [adr/](adr/):
- 0001 Go + Wails v2 para la app de escritorio.
- 0002 ffmpeg como subproceso (no códecs con CGO).
- 0003 Licencia GPL-3.0-or-later.
- 0004 Monorepo: `estudio/` como módulo aparte dentro de `radiofm`.
- 0005 Fuente de Icecast configurable: mezcla interna (bus) o captura (salida de la consola).
