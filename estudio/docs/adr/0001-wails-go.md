# ADR 0001 — Go + Wails v2 para la aplicación de escritorio

- Estado: aceptada
- Fecha: 2026-11-08

## Contexto
Se necesita un programa de escritorio para Windows con interfaz rica (decks, forma de onda, VU,
botonera, editor de parrilla) y un motor de audio en tiempo real. El servidor ya está en Go y el
equipo lo conoce.

## Decisión
- **Go** para todo el núcleo (motor de audio, programador, sincronización), igual que el servidor.
- **Wails v2** para la ventana: WebView2 nativo de Windows + bindings Go↔TypeScript, sin empaquetar
  un navegador (a diferencia de Electron). Frontend en **Svelte 5 + TypeScript + Vite**.
- `main.go` vive en la raíz de `estudio/` porque Wails v2 compila el paquete main de la raíz del
  proyecto y embebe `frontend/dist` desde ahí. Es solo raíz de composición.

## Consecuencias
- + Un solo lenguaje en el backend del servidor y de la PC; ejecutable único (~12 MB).
- + La interfaz se itera rápido con `wails dev` (recarga en caliente).
- − WebView2 debe estar instalado (viene con Windows 11; el instalador NSIS lo puede incluir).
- − Wails v3 cambiará la API; migrar será un trabajo acotado a `main.go` e `internal/ui`.
