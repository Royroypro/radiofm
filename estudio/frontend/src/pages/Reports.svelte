<!-- Reportes: lo emitido (as-run), el comprobante de cuñas, el informe musical (APDAYC)
     y la grabación legal de la emisión (escuchar y exportar tramos). -->
<script lang="ts">
  import { onMount } from 'svelte'
  import { ReportAsRun, ReportSpots, ReportMusic, ExportReport, Today, LoggerFiles, LoggerStatus, LoggerExport, OpenRecordings } from '../../wailsjs/go/ui/App'
  import type { domain, reports, airlog } from '../../wailsjs/go/models'
  import { dur } from '../lib/format'
  import { toast } from '../lib/toast.svelte'

  const kindLabel: Record<string, string> = { music: 'Música', jingle: 'Jingle', spot: 'Cuña', voice: 'Locución', cart: 'Botonera', relay: 'Enlace a cadena' }

  let tab: 'asrun' | 'spots' | 'music' | 'logger' = $state('asrun')
  let music: reports.MusicRow[] = $state([])
  let files: airlog.File[] = $state([])
  let lstatus: airlog.Status | null = $state(null)
  let playing = $state('')
  let expFrom = $state('')
  let expTo = $state('')
  let from = $state('')
  let to = $state('')
  let advertiser = $state('')
  let asrun: domain.Play[] = $state([])
  let proof: reports.ProofRow[] = $state([])

  const dt = (iso: any) => new Date(iso).toLocaleString('es-PE', { hour12: false })

  async function load() {
    try {
      if (tab === 'asrun') asrun = await ReportAsRun(from, to)
      else if (tab === 'spots') proof = await ReportSpots(from, to, advertiser)
      else if (tab === 'music') music = await ReportMusic(from, to)
      else {
        files = (await LoggerFiles()) ?? []
        lstatus = await LoggerStatus()
      }
    } catch (e) {
      toast.error(e)
    }
  }

  async function exportCSV() {
    try {
      const p = await ExportReport(tab, from, to, advertiser)
      if (p) toast.show('Guardado en ' + p)
    } catch (e) {
      toast.error(e)
    }
  }

  const mb = (b: number) => (b / 1048576).toFixed(1) + ' MB'
  const hhmm = (iso: any) => new Date(iso).toLocaleString('es-PE', { hour12: false, dateStyle: 'short', timeStyle: 'short' })
  async function exportClip() {
    try {
      const p = await LoggerExport(expFrom, expTo)
      if (p) toast.show('Tramo guardado en ' + p)
    } catch (e) {
      toast.error(e)
    }
  }

  onMount(async () => {
    from = to = await Today()
    load()
  })
</script>

<div class="page">
  <div class="row">
    <h1 class="grow">Reportes</h1>
    <button class:ghost={tab !== 'asrun'} onclick={() => { tab = 'asrun'; load() }}>Emisiones</button>
    <button class:ghost={tab !== 'spots'} onclick={() => { tab = 'spots'; load() }}>Comprobante de cuñas</button>
    <button class:ghost={tab !== 'music'} onclick={() => { tab = 'music'; load() }}>Informe musical</button>
    <button class:ghost={tab !== 'logger'} onclick={() => { tab = 'logger'; load() }}>Grabación de la emisión</button>
  </div>
  {#if tab === 'logger'}
  <div class="row panel">
    <div class="grow small">
      {#if lstatus}
        {#if !lstatus.enabled}<span class="pill warn">Grabador apagado (Ajustes → Grabador)</span>
        {:else if lstatus.recording}<span class="pill ok">● Grabando</span>
        {:else}<span class="pill warn">No está grabando</span>{/if}
        {lstatus.used_mb} MB de {lstatus.max_mb || '∞'} MB · máx. {lstatus.max_days || '∞'} días
        {#if lstatus.error}<div class="error">{lstatus.error}</div>{/if}
      {/if}
    </div>
    <label class="field">Desde<input type="datetime-local" bind:value={expFrom} /></label>
    <label class="field">Hasta<input type="datetime-local" bind:value={expTo} /></label>
    <button style="align-self:end" onclick={exportClip}>Exportar tramo…</button>
    <button class="ghost" style="align-self:end" onclick={() => OpenRecordings()}>Programas grabados</button>
  </div>
  {:else}
  <div class="row panel">
    <label class="field">Desde<input type="date" bind:value={from} /></label>
    <label class="field">Hasta<input type="date" bind:value={to} /></label>
    {#if tab === 'spots'}<label class="field">Anunciante (opcional)<input bind:value={advertiser} /></label>{/if}
    <button style="align-self:end" onclick={load}>Ver</button>
    <span class="grow"></span>
    <button class="ghost" style="align-self:end" onclick={exportCSV}>Exportar CSV…</button>
  </div>
  {/if}
  <div class="panel scroll grow">
    {#if tab === 'asrun'}
      <table>
        <thead><tr><th>Fecha y hora</th><th>Tipo</th><th>Artista / anunciante</th><th>Título</th><th class="num">Duración</th><th>Completa</th></tr></thead>
        <tbody>{#each asrun as p (p.id)}<tr><td>{dt(p.started_at)}</td><td>{kindLabel[p.kind] ?? p.kind}</td><td>{p.artist}</td><td>{p.title}</td>
          <td class="num">{dur(p.duration)}</td><td>{p.complete ? 'Sí' : 'No'}</td></tr>{/each}</tbody>
      </table>
      {#if !asrun.length}<p class="muted">Sin emisiones en el rango.</p>{/if}
    {:else if tab === 'music'}
      <table>
        <thead><tr><th>Título</th><th>Intérprete</th><th>Compositor</th><th>Año</th><th class="num">Veces</th><th class="num">Al aire</th></tr></thead>
        <tbody>{#each music as r}<tr><td>{r.title}</td><td>{r.artist}</td><td>{r.composer || '—'}</td><td>{r.year}</td>
          <td class="num">{r.plays}</td><td class="num">{dur(r.aired)}</td></tr>{/each}</tbody>
      </table>
      {#if !music.length}<p class="muted">Sin música emitida en el rango.</p>{/if}
      <p class="muted small">Para la sociedad de autores (APDAYC). El compositor sale de la etiqueta del archivo; si falta, complétela en las etiquetas y vuelva a importar.</p>
    {:else if tab === 'logger'}
      <table>
        <thead><tr><th>Desde</th><th class="num">Tamaño</th><th></th></tr></thead>
        <tbody>{#each files as f (f.name)}<tr><td>{hhmm(f.start)}</td><td class="num">{mb(f.size)}</td>
          <td>{#if playing === f.name}<audio src={`/preview/airlog/${f.name}`} controls autoplay></audio>
            {:else}<button class="small ghost" onclick={() => (playing = f.name)}>▶ Escuchar</button>{/if}</td></tr>{/each}</tbody>
      </table>
      {#if !files.length}<p class="muted">Todavía no hay grabaciones.</p>{/if}
      <p class="muted small">Graba lo que sale por la tarjeta (FM) en un archivo por hora; lo más viejo se borra al pasar el límite. Se escucha por los audífonos de la PC.</p>
    {:else}
      <table>
        <thead><tr><th>Fecha y hora</th><th>Anunciante</th><th>Cuña</th><th class="num">Duración</th><th>Completa</th></tr></thead>
        <tbody>{#each proof as r (r.uuid)}<tr><td>{dt(r.at)}</td><td>{r.advertiser}</td><td>{r.spot}</td><td class="num">{dur(r.duration)}</td><td>{r.complete ? 'Sí' : 'No'}</td></tr>{/each}</tbody>
      </table>
      {#if !proof.length}<p class="muted">Sin cuñas emitidas en el rango.</p>{/if}
      <p class="muted small">El comprobante oficial para el anunciante está en el panel web (Cuñas FM → Comprobante de emisión).</p>
    {/if}
  </div>
</div>

<style>
  .page { display: flex; flex-direction: column; gap: 10px; height: 100%; min-height: 0; }
</style>
