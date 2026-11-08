<!--
  Pantalla inicial de Club25 Estudio (fase 0): marca, versión y las tarjetas de
  sonido que ve miniaudio. Las pantallas reales (Al aire, Biblioteca, Relojes,
  Parrilla, Cuñas, Botonera, Reportes, Ajustes) llegan en las fases 3–6.

  Las funciones Go se importan de ../wailsjs/go/ui/App (los genera Wails a partir
  de internal/ui; no editar a mano).
-->
<script lang="ts">
  import { onMount } from 'svelte'
  import { Info, Devices } from '../wailsjs/go/ui/App'
  import type { audio, ui } from '../wailsjs/go/models'
  import isotipo from './assets/brand/isotipo.svg'

  let info: ui.AppInfo | null = $state(null)
  let devices: audio.Device[] = $state([])
  let error = $state('')
  let now = $state(new Date())

  const outputs = $derived(devices.filter((d) => d.kind === 'playback'))
  const inputs = $derived(devices.filter((d) => d.kind === 'capture'))

  onMount(() => {
    Info().then((i) => (info = i))
    Devices()
      .then((d) => (devices = d ?? []))
      .catch((e) => (error = String(e)))
    const t = setInterval(() => (now = new Date()), 1000)
    return () => clearInterval(t)
  })
</script>

<main>
  <header>
    <img src={isotipo} alt="" class="logo" />
    <div>
      <h1>{info?.name ?? 'Club25 Estudio'}</h1>
      <p class="muted">Automatización de radio · versión {info?.version ?? '…'}</p>
    </div>
    <time>{now.toLocaleTimeString('es-PE', { hour12: false })}</time>
  </header>

  <section class="note">
    Esqueleto de la fase 0. El motor de audio, la programación y la pantalla «Al aire» llegan en las
    próximas fases (ver <code>docs/estudio/plan.md</code>).
  </section>

  {#if error}
    <p class="error">{error}</p>
  {/if}

  <div class="cols">
    <section class="panel">
      <h2>Salidas ({outputs.length})</h2>
      <ul>
        {#each outputs as d (d.id)}
          <li class:default={d.is_default}>{d.name}{#if d.is_default}<span class="tag">predeterminada</span>{/if}</li>
        {/each}
      </ul>
    </section>
    <section class="panel">
      <h2>Entradas ({inputs.length})</h2>
      <ul>
        {#each inputs as d (d.id)}
          <li class:default={d.is_default}>{d.name}{#if d.is_default}<span class="tag">predeterminada</span>{/if}</li>
        {/each}
      </ul>
    </section>
  </div>
</main>

<style>
  main { padding: 24px 32px; max-width: 1200px; margin: 0 auto; }
  header { display: flex; align-items: center; gap: 16px; border-bottom: 2px solid var(--red); padding-bottom: 16px; }
  .logo { width: 56px; height: 56px; }
  h1 { margin: 0; font-size: 26px; }
  h2 { margin: 0 0 12px; font-size: 16px; color: var(--muted); text-transform: uppercase; letter-spacing: .06em; }
  time { margin-left: auto; font-size: 32px; font-variant-numeric: tabular-nums; }
  .muted { color: var(--muted); margin: 4px 0 0; }
  .note { margin: 20px 0; padding: 12px 16px; border-left: 4px solid var(--orange); background: var(--panel); border-radius: 6px; }
  .error { color: var(--red); }
  .cols { display: grid; grid-template-columns: 1fr 1fr; gap: 20px; }
  .panel { background: var(--panel); border: 1px solid var(--line); border-radius: 10px; padding: 16px; }
  ul { list-style: none; margin: 0; padding: 0; }
  li { padding: 6px 0; border-bottom: 1px solid var(--line); font-size: 14px; }
  li.default { color: #fff; font-weight: 700; }
  .tag { margin-left: 8px; font-size: 11px; background: var(--royal); padding: 2px 6px; border-radius: 4px; }
  code { color: var(--orange); }
</style>
