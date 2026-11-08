<!-- Reportes: lo emitido (as-run) y el comprobante de cuñas, con exportación a CSV. -->
<script lang="ts">
  import { onMount } from 'svelte'
  import { ReportAsRun, ReportSpots, ExportReport, Today } from '../../wailsjs/go/ui/App'
  import type { domain, reports } from '../../wailsjs/go/models'
  import { dur } from '../lib/format'
  import { toast } from '../lib/toast.svelte'

  const kindLabel: Record<string, string> = { music: 'Música', jingle: 'Jingle', spot: 'Cuña', voice: 'Locución', cart: 'Botonera' }

  let tab: 'asrun' | 'spots' = $state('asrun')
  let from = $state('')
  let to = $state('')
  let advertiser = $state('')
  let asrun: domain.Play[] = $state([])
  let proof: reports.ProofRow[] = $state([])

  const dt = (iso: any) => new Date(iso).toLocaleString('es-PE', { hour12: false })

  async function load() {
    try {
      if (tab === 'asrun') asrun = await ReportAsRun(from, to)
      else proof = await ReportSpots(from, to, advertiser)
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
  </div>
  <div class="row panel">
    <label class="field">Desde<input type="date" bind:value={from} /></label>
    <label class="field">Hasta<input type="date" bind:value={to} /></label>
    {#if tab === 'spots'}<label class="field">Anunciante (opcional)<input bind:value={advertiser} /></label>{/if}
    <button style="align-self:end" onclick={load}>Ver</button>
    <span class="grow"></span>
    <button class="ghost" style="align-self:end" onclick={exportCSV}>Exportar CSV…</button>
  </div>
  <div class="panel scroll grow">
    {#if tab === 'asrun'}
      <table>
        <thead><tr><th>Fecha y hora</th><th>Tipo</th><th>Artista / anunciante</th><th>Título</th><th class="num">Duración</th><th>Completa</th></tr></thead>
        <tbody>{#each asrun as p (p.id)}<tr><td>{dt(p.started_at)}</td><td>{kindLabel[p.kind] ?? p.kind}</td><td>{p.artist}</td><td>{p.title}</td>
          <td class="num">{dur(p.duration)}</td><td>{p.complete ? 'Sí' : 'No'}</td></tr>{/each}</tbody>
      </table>
      {#if !asrun.length}<p class="muted">Sin emisiones en el rango.</p>{/if}
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
