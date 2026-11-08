<!--
  Parrilla semanal: qué reloj suena en cada hora (7 × 24). Elija un reloj en
  la paleta y haga clic (o arrastre) sobre las celdas para pintarlas.
  Excepciones: otro reloj para una hora de una fecha concreta.
-->
<script lang="ts">
  import { onMount } from 'svelte'
  import { Clocks, Grid, SetGrid, Exceptions, SetException, Today } from '../../wailsjs/go/ui/App'
  import { domain } from '../../wailsjs/go/models'
  import { weekdays } from '../lib/labels'
  import { toast } from '../lib/toast.svelte'

  let clocks: domain.Clock[] = $state([])
  let cells: Record<string, number> = $state({}) // "d-h" → clock id
  let brush = $state(0)
  let painting = $state(false)
  let dirty = $state(false)
  let exceptions: domain.GridException[] = $state([])
  let exc = $state({ date: '', hour: 0, clock_id: 0 })

  const key = (d: number, h: number) => `${d}-${h}`
  const clockOf = (id: number) => clocks.find((c) => c.id === id)

  async function load() {
    clocks = await Clocks()
    const list = await Grid()
    const m: Record<string, number> = {}
    for (const c of list) m[key(c.weekday, c.hour)] = c.clock_id
    cells = m
    exceptions = await Exceptions()
    if (!brush && clocks.length) brush = clocks[0].id
    dirty = false
  }

  function paint(d: number, h: number) {
    if (cells[key(d, h)] !== brush) {
      cells[key(d, h)] = brush
      dirty = true
    }
  }

  async function save() {
    const list: domain.GridCell[] = []
    for (let d = 0; d < 7; d++)
      for (let h = 0; h < 24; h++) list.push(domain.GridCell.createFrom({ weekday: d, hour: h, clock_id: cells[key(d, h)] ?? 0 }))
    try {
      await SetGrid(list)
      dirty = false
      toast.show('Parrilla guardada. Se aplica a los días que se generen desde ahora (o regenere en Playlist).')
    } catch (e) {
      toast.error(e)
    }
  }

  async function addException() {
    try {
      await SetException(domain.GridException.createFrom({ ...exc, hour: Number(exc.hour), clock_id: Number(exc.clock_id) }))
      exceptions = await Exceptions()
    } catch (e) {
      toast.error(e)
    }
  }

  async function removeException(e: domain.GridException) {
    await SetException(domain.GridException.createFrom({ ...e, clock_id: 0 }))
    exceptions = await Exceptions()
  }

  onMount(async () => {
    await load()
    exc.date = await Today()
    exc.clock_id = clocks[0]?.id ?? 0
  })
</script>

<svelte:window onmouseup={() => (painting = false)} />

<div class="page">
  <div class="row">
    <h1 class="grow">Parrilla semanal</h1>
    <span class="muted small">Pincel:</span>
    <button class="chip" class:active={brush === 0} onclick={() => (brush = 0)}>(vacío)</button>
    {#each clocks as c (c.id)}
      <button class="chip" class:active={brush === c.id} style:border-color={c.color} onclick={() => (brush = c.id)}>
        <span class="dot" style:background={c.color}></span>{c.name}</button>
    {/each}
    <button class="ok" disabled={!dirty} onclick={save}>Guardar</button>
  </div>

  <div class="panel scroll">
    <table class="grid">
      <thead><tr><th></th>{#each weekdays as w}<th>{w}</th>{/each}</tr></thead>
      <tbody>
        {#each Array(24) as _, h}
          <tr>
            <th class="h">{String(h).padStart(2, '0')}:00</th>
            {#each Array(7) as _, d}
              {@const c = clockOf(cells[key(d, h)] ?? 0)}
              <td class="cell" style:background={c ? c.color + '55' : 'transparent'} style:border-left-color={c?.color ?? 'transparent'}
                onmousedown={() => { painting = true; paint(d, h) }} onmouseenter={() => painting && paint(d, h)}>
                {c?.name ?? ''}
              </td>
            {/each}
          </tr>
        {/each}
      </tbody>
    </table>
  </div>

  <section class="panel">
    <h2>Excepciones por fecha</h2>
    <div class="row">
      <input type="date" bind:value={exc.date} />
      <select bind:value={exc.hour}>{#each Array(24) as _, h}<option value={h}>{String(h).padStart(2, '0')}:00</option>{/each}</select>
      <select bind:value={exc.clock_id}>{#each clocks as c (c.id)}<option value={c.id}>{c.name}</option>{/each}</select>
      <button onclick={addException}>Agregar</button>
    </div>
    {#each exceptions as e}
      <div class="row small"><span>{e.date} {String(e.hour).padStart(2, '0')}:00 → {clockOf(e.clock_id)?.name ?? e.clock_id}</span>
        <button class="small ghost" onclick={() => removeException(e)}>Quitar</button></div>
    {/each}
  </section>
</div>

<style>
  .page { display: flex; flex-direction: column; gap: 10px; height: 100%; min-height: 0; }
  .page > .scroll { flex: 1; min-height: 0; }
  .chip { background: transparent; border: 1px solid var(--line); padding: 4px 10px; font-size: 12px; }
  .chip.active { background: var(--panel-2); outline: 2px solid var(--orange); }
  table.grid td.cell { border-left: 3px solid transparent; font-size: 11px; cursor: crosshair; height: 22px; padding: 2px 6px; }
  table.grid th { text-align: center; }
  th.h { width: 60px; font-variant-numeric: tabular-nums; }
</style>
