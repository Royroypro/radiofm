# Club25 Estudio

Automatización de radio para **Windows** hecha para Radio Club 25 (93.3 FM, Huacho): biblioteca
musical, relojes y parrilla semanal, playlist del día, tandas de cuñas, botonera de jingles,
micrófono con ducking y hora/temperatura locutadas. Saca el audio a la consola/transmisor y a
Icecast (la web), y se sincroniza con el panel web: cuñas, "lo que suena" y comprobante de emisión.

> Estado: **fase 0** (esqueleto). Ver el plan por fases en [../docs/estudio/plan.md](../docs/estudio/plan.md)
> y el avance en [CHANGELOG.md](CHANGELOG.md).

```
Club25 Estudio (PC) ──► tarjeta de sonido ──► consola ──► transmisor 93.3 FM
        │
        ├──► ffmpeg (MP3 128k) ──► Icecast :8010/radio ──► web (oyentes)
        └──► HTTPS /api/studio/* ⇄ panel web (cuñas, lo que suena, comprobante)
```

## Empezar (desarrollo)

Requisitos: Windows 10/11, Go 1.27+, Node 22.12+, PowerShell 7, WebView2 (viene con Windows 11).

```powershell
cd estudio
pwsh -File scripts/dev-setup.ps1   # instala Wails, Task, gcc (llvm-mingw), ffmpeg y dependencias
task dev                           # abre la ventana con recarga en caliente
```

Paso a paso y problemas frecuentes: [docs/setup-windows.md](docs/setup-windows.md).

## Documentación

| Documento | Contenido |
|---|---|
| [AGENTS.md](AGENTS.md) | guía para agentes de IA y programadores: mapa, reglas, comandos, recetas |
| [docs/architecture.md](docs/architecture.md) | piezas, capas y flujo del audio |
| [docs/glossary.md](docs/glossary.md) | términos de radio (cuña, tanda, reloj, segue…) y su nombre en el código |
| [docs/setup-windows.md](docs/setup-windows.md) | preparar la PC de desarrollo |
| [docs/third-party.md](docs/third-party.md) | proyectos de código abierto usados y su licencia |
| [docs/adr/](docs/adr/) | decisiones de arquitectura |
| [../docs/api.md](../docs/api.md) | contrato de la API del servidor (sección "API del estudio") |

## Licencia

GPL-3.0-or-later. Ver [LICENSE](LICENSE). Usa proyectos libres listados en
[docs/third-party.md](docs/third-party.md); ffmpeg se distribuye como ejecutable aparte con su licencia.
