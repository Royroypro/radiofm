<!--
  Estructura de Club25 Estudio: cabecera con marca y reloj, navegación lateral
  y la pantalla activa. Cada pantalla vive en src/pages/. Las funciones Go se
  importan de ../wailsjs/go/ui/App (las genera Wails desde internal/ui).
-->
<script lang="ts">
  import { onMount } from 'svelte'
  import { Info } from '../wailsjs/go/ui/App'
  import type { ui } from '../wailsjs/go/models'
  import isotipo from './assets/brand/isotipo.svg'
  import Library from './pages/Library.svelte'
  import Settings from './pages/Settings.svelte'
  import Playlist from './pages/Playlist.svelte'
  import Clocks from './pages/Clocks.svelte'
  import Grid from './pages/Grid.svelte'
  import Spots from './pages/Spots.svelte'
  import VoiceTrack from './pages/VoiceTrack.svelte'
  import Reports from './pages/Reports.svelte'
  import OnAir from './pages/OnAir.svelte'
  import Carts from './pages/Carts.svelte'
  import { onair, handleKey } from './lib/onair.svelte'
  import Placeholder from './pages/Placeholder.svelte'
  import { toast } from './lib/toast.svelte'
  import { nav } from './lib/nav.svelte'
  import { theme } from './lib/themeState.svelte'

  type Page = { id: string; label: string; phase?: string }
  const pages: Page[] = [
    { id: 'onair', label: 'Al aire' },
    { id: 'log', label: 'Playlist' },
    { id: 'library', label: 'Biblioteca' },
    { id: 'clocks', label: 'Relojes' },
    { id: 'grid', label: 'Parrilla' },
    { id: 'voicetrack', label: 'Voice tracking' },
    { id: 'spots', label: 'Cuñas' },
    { id: 'carts', label: 'Botonera' },
    { id: 'reports', label: 'Reportes' },
    { id: 'settings', label: 'Ajustes' },
  ]

  // Al aire usa toda la ventana: la barra lateral se abre con su botón "Menú".
  const page = $derived(nav.page)
  const sidebar = $derived(page !== 'onair' || nav.menuOpen)
  let info: ui.AppInfo | null = $state(null)
  let now = $state(new Date())

  onMount(() => {
    theme.load()
    onair.start()
    Info().then((i) => (info = i))
    const t = setInterval(() => (now = new Date()), 500)
    return () => clearInterval(t)
  })

  const current = $derived(pages.find((p) => p.id === page)!)
</script>

<svelte:window onkeydown={handleKey} />

<div class="shell" class:full={!sidebar}>
  {#if sidebar}
  <aside class:overlay={page === 'onair'}>
    <div class="brand">
      <img src={isotipo} alt="" />
      <div>
        <strong>Club25 Estudio</strong>
        <span class="muted small">v{info?.version ?? '…'}</span>
      </div>
    </div>
    <nav>
      {#each pages as p (p.id)}
        <button class="nav" class:active={page === p.id} onclick={() => nav.go(p.id)}>{p.label}</button>
      {/each}
    </nav>
    {#if onair.view}
      <div class="air small">
        <span class="pill" class:ok={onair.view.air.running}>{onair.view.air.running ? 'AL AIRE' : 'detenido'}</span>
        {#if onair.mic}<span class="pill bad">MIC</span>{/if}
        {#if onair.view.stream.enabled}<span class="pill" class:ok={onair.view.stream.encoder.connected} class:bad={!onair.view.stream.encoder.connected}>WEB</span>{/if}
      </div>
    {/if}
    <time>{now.toLocaleTimeString('es-PE', { hour12: false })}</time>
    {#if info && !info.ffmpeg_ok}
      <p class="error small">ffmpeg no encontrado: la biblioteca no puede analizar ni reproducir.</p>
    {/if}
  </aside>
  {/if}

  <main>
    {#if page === 'onair'}
      <OnAir />
    {:else if page === 'carts'}
      <Carts />
    {:else if page === 'library'}
      <Library />
    {:else if page === 'log'}
      <Playlist />
    {:else if page === 'clocks'}
      <Clocks />
    {:else if page === 'grid'}
      <Grid />
    {:else if page === 'spots'}
      <Spots />
    {:else if page === 'voicetrack'}
      <VoiceTrack />
    {:else if page === 'reports'}
      <Reports />
    {:else if page === 'settings'}
      <Settings {info} />
    {:else}
      <Placeholder title={current.label} phase={current.phase ?? ''} />
    {/if}
  </main>
</div>

{#if toast.text}
  <div class="toast" class:err={toast.isError}>{toast.text}</div>
{/if}

<style>
  .shell { display: grid; grid-template-columns: 190px 1fr; height: 100vh; }
  .shell.full { grid-template-columns: 1fr; }
  .shell.full main { padding: 6px; }
  aside.overlay { position: fixed; left: 0; top: 0; bottom: 0; width: 190px; z-index: 40; box-shadow: 4px 0 20px rgba(0,0,0,.6); }
  .shell:has(aside.overlay) { grid-template-columns: 1fr; }
  .shell:has(aside.overlay) main { padding: 6px; }
  aside { background: var(--sidebar); color: var(--sidebar-text); border-right: 1px solid var(--line); display: flex; flex-direction: column; padding: 14px 10px; gap: 14px; }
  .brand { display: flex; align-items: center; gap: 10px; padding: 0 6px; }
  .brand img { width: 40px; height: 40px; }
  .brand div { display: flex; flex-direction: column; }
  nav { display: flex; flex-direction: column; gap: 4px; }
  .nav { text-align: left; background: transparent; border-color: transparent; border-radius: 6px; padding: 9px 12px; color: var(--sidebar-text); }
  .nav.active { background: var(--accent); color: var(--accent-text); }
  .air { display: flex; gap: 4px; justify-content: center; margin-top: auto; }
  time { font-size: 28px; text-align: center; font-variant-numeric: tabular-nums; }
  main { overflow: hidden; padding: 16px; display: flex; flex-direction: column; min-width: 0; }
</style>
