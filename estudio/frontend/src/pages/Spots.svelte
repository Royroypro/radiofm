<!--
  Cuñas: las que llegan del panel web (solo lectura aquí) y las locales.
  Debajo, las tandas del día con las cuñas que les tocaron (reparto automático).
-->
<script lang="ts">
  import { onMount } from 'svelte'
  import { Spots, SaveSpot, DeleteSpot, ReplanDay, DayTandas, Today, PickFile } from '../../wailsjs/go/ui/App'
  import { EventsOn } from '../../wailsjs/runtime/runtime'
  import { domain, type ui, type spots } from '../../wailsjs/go/models'
  import { dur, dateInput, fromDateInput } from '../lib/format'
  import { clock } from '../lib/labels'
  import { toast } from '../lib/toast.svelte'

  let list: ui.SpotView[] = $state([])
  let day = $state('')
  let tandas: spots.TandaView[] = $state([])
  let edit: any = $state(null)

  async function load() {
    list = (await Spots()) ?? []
    tandas = (await DayTandas(day)) ?? []
  }

  function open(s: ui.SpotView | null) {
    edit = s
      ? { ...s, from: dateInput(s.starts_at as any), to: dateInput(s.ends_at as any) }
      : { id: 0, advertiser: '', name: '', path: '', plays_per_day: 6, hour_from: 6, hour_to: 22, active: true, from: '', to: '' }
  }

  async function pick() {
    const p = await PickFile('Audio de la cuña')
    if (p) edit.path = p
  }

  async function save() {
    try {
      await SaveSpot(domain.Spot.createFrom({
        ...edit, plays_per_day: Number(edit.plays_per_day), hour_from: Number(edit.hour_from), hour_to: Number(edit.hour_to),
        starts_at: fromDateInput(edit.from), ends_at: fromDateInput(edit.to),
      }))
      edit = null
      toast.show('Cuña guardada; las tandas se replanifican.')
      setTimeout(load, 800)
    } catch (e) {
      toast.error(e)
    }
  }

  async function remove() {
    if (!edit?.id || !confirm(`¿Borrar la cuña «${edit.name}»?`)) return
    try {
      await DeleteSpot(edit.id)
      edit = null
      setTimeout(load, 800)
    } catch (e) {
      toast.error(e)
    }
  }

  async function replan() {
    try {
      const r = await ReplanDay(day)
      const faltan = Object.values(r.missing ?? {}).reduce((a: number, b: any) => a + Number(b), 0)
      toast.show(faltan ? `Replanificado; ${faltan} emisiones no caben en las tandas del día` : 'Tandas replanificadas', faltan > 0)
      await load()
    } catch (e) {
      toast.error(e)
    }
  }

  const range = (s: domain.Spot) => (s.hour_from === 0 && s.hour_to === 24 ? 'Todo el día' : `${String(s.hour_from).padStart(2, '0')}–${String(s.hour_to).padStart(2, '0')} h`)

  onMount(() => {
    Today().then((d) => {
      day = d
      load()
    })
    return EventsOn('log:changed', () => load())
  })
</script>

<div class="page">
  <div class="row">
    <h1 class="grow">Cuñas</h1>
    <button class="ghost" onclick={() => open(null)}>+ Cuña local</button>
  </div>

  <section class="panel scroll list">
    <table>
      <thead><tr><th>Anunciante</th><th>Cuña</th><th class="num">Duración</th><th class="num">Veces/día</th><th>Franja</th><th>Vigencia</th><th class="num">Hoy</th><th>Origen</th></tr></thead>
      <tbody>
        {#each list as s (s.id)}
          <tr onclick={() => (s.from_web ? toast.show('Esta cuña viene del panel web: edítela allí.') : open(s))}>
            <td>{s.advertiser}</td><td>{s.name}</td><td class="num">{dur(s.duration)}</td><td class="num">{s.plays_per_day}</td>
            <td>{range(s)}</td>
            <td class="small">{s.starts_at ? dateInput(s.starts_at as any) : '—'} → {s.ends_at ? dateInput(s.ends_at as any) : 'sin fin'}</td>
            <td class="num">{s.aired_today}</td>
            <td>{#if s.from_web}<span class="pill ok">Panel web</span>{:else}<span class="pill">Local</span>{/if}
              {#if !s.active}<span class="pill warn">inactiva</span>{/if}</td>
          </tr>
        {/each}
      </tbody>
    </table>
    {#if !list.length}<p class="muted">No hay cuñas. Llegan solas del panel web al configurar la sincronización, o cree una local.</p>{/if}
  </section>

  <section class="panel scroll tandas">
    <div class="row"><h2 class="grow">Tandas del día</h2>
      <input type="date" bind:value={day} onchange={load} />
      <button onclick={replan}>Replanificar</button></div>
    <div class="cards">
      {#each tandas as t (t.log_item_id)}
        <div class="card" class:done={t.status !== 'pending'}>
          <strong>{clock(t.at).slice(0, 5)}</strong>
          {#each t.spots as s (s.id)}<div class="small">{s.advertiser} · {s.name}</div>{:else}<div class="small muted">sin cuñas</div>{/each}
        </div>
      {/each}
    </div>
    {#if !tandas.length}<p class="muted small">Este día no tiene tandas (agregue slots «Tanda de cuñas» a los relojes).</p>{/if}
  </section>
</div>

{#if edit}
  <div class="backdrop">
    <div class="modal">
      <h2>{edit.id ? 'Editar cuña local' : 'Nueva cuña local'}</h2>
      <div class="grid2">
        <label class="field">Anunciante<input bind:value={edit.advertiser} /></label>
        <label class="field">Nombre<input bind:value={edit.name} /></label>
      </div>
      <div class="row" style="margin-top:8px"><input class="grow" readonly value={edit.path} placeholder="Archivo de audio" /><button class="ghost" onclick={pick}>Elegir…</button></div>
      <div class="grid3" style="margin-top:8px">
        <label class="field">Veces por día<input type="number" min="1" max="96" bind:value={edit.plays_per_day} /></label>
        <label class="field">Desde (h)<input type="number" min="0" max="23" bind:value={edit.hour_from} /></label>
        <label class="field">Hasta (h, sin incluir)<input type="number" min="1" max="24" bind:value={edit.hour_to} /></label>
        <label class="field">Vigente desde<input type="date" bind:value={edit.from} /></label>
        <label class="field">Hasta<input type="date" bind:value={edit.to} /></label>
        <label class="check" style="align-self:end"><input type="checkbox" bind:checked={edit.active} /> Activa</label>
      </div>
      <div class="actions">
        {#if edit.id}<button class="danger" onclick={remove}>Borrar</button>{/if}
        <span class="grow"></span>
        <button class="ghost" onclick={() => (edit = null)}>Cancelar</button>
        <button class="ok" onclick={save}>Guardar</button>
      </div>
    </div>
  </div>
{/if}

<style>
  .page { display: flex; flex-direction: column; gap: 10px; height: 100%; min-height: 0; }
  .list { flex: 1; min-height: 120px; }
  .tandas { flex: 1; min-height: 160px; }
  .cards { display: grid; grid-template-columns: repeat(auto-fill, minmax(170px, 1fr)); gap: 8px; }
  .card { background: var(--panel-2); border-radius: 8px; padding: 8px 10px; border-left: 3px solid var(--orange); }
  .card.done { opacity: .5; }
</style>
