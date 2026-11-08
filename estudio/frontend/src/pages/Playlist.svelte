<!--
  Playlist del día: lo que sonará, hora por hora, generado desde la parrilla y
  los relojes. Se puede mover, quitar, bloquear (sobrevive a la regeneración)
  e insertar pistas a mano.
-->
<script lang="ts">
  import { onMount } from 'svelte'
  import { Today, LogDay, GenerateDay, MoveLogItem, RemoveLogItem, ToggleLogLock, InsertLogTrack } from '../../wailsjs/go/ui/App'
  import type { domain } from '../../wailsjs/go/models'
  import { dur } from '../lib/format'
  import { slotIcon, statusLabel, clock } from '../lib/labels'
  import { toast } from '../lib/toast.svelte'
  import TrackPicker from '../lib/TrackPicker.svelte'

  let day = $state('')
  let today = $state('')
  let items: domain.LogItem[] = $state([])
  let sel: number | null = $state(null)
  let picking = $state(false)
  let busy = $state(false)

  const hours = $derived.by(() => {
    const groups: { hour: number; items: domain.LogItem[]; total: number }[] = []
    for (const it of items) {
      let g = groups[groups.length - 1]
      if (!g || g.hour !== it.hour) {
        g = { hour: it.hour, items: [], total: 0 }
        groups.push(g)
      }
      g.items.push(it)
      g.total += it.length
    }
    return groups
  })

  async function load() {
    try {
      items = (await LogDay(day)) ?? []
    } catch (e) {
      toast.error(e)
    }
  }

  function shift(days: number) {
    const d = new Date(day + 'T12:00:00')
    d.setDate(d.getDate() + days)
    day = d.toISOString().slice(0, 10)
    load()
  }

  async function act(f: () => Promise<void>) {
    busy = true
    try {
      await f()
      await load()
    } catch (e) {
      toast.error(e)
    } finally {
      busy = false
    }
  }

  async function regenerate() {
    if (!confirm('¿Regenerar la playlist de este día? Se conserva lo bloqueado y lo que ya sonó.')) return
    await act(() => GenerateDay(day))
    toast.show('Playlist regenerada')
  }

  onMount(async () => {
    today = day = await Today()
    load()
  })
</script>

<div class="page">
  <div class="row">
    <h1 class="grow">Playlist</h1>
    <button class="ghost" onclick={() => shift(-1)}>‹</button>
    <input type="date" bind:value={day} onchange={load} />
    <button class="ghost" onclick={() => shift(1)}>›</button>
    <button class="ghost" disabled={day === today} onclick={() => { day = today; load() }}>Hoy</button>
    <button onclick={regenerate} disabled={busy}>Regenerar día</button>
  </div>

  <div class="panel scroll grow">
    {#if !items.length}
      <p class="muted">No hay playlist para este día. Use «Regenerar día» (necesita relojes en la parrilla y pistas en sus categorías).</p>
    {/if}
    {#each hours as g (g.hour)}
      <h3 class="hour">{String(g.hour).padStart(2, '0')}:00 <span class="muted small">{g.items.length} elementos · {dur(g.total)}</span></h3>
      <table>
        <tbody>
          {#each g.items as it (it.id)}
            <tr class:sel={sel === it.id} class:done={it.status !== 'pending'} class:opt={it.optional} onclick={() => (sel = it.id)}>
              <td class="t">{clock(it.at)}</td>
              <td class="k" title={it.kind}>{slotIcon[it.kind] ?? '?'}</td>
              <td class="name">{it.artist ? it.artist + ' - ' : ''}{it.title}
                {#if it.note}<span class="muted small"> · {it.note}</span>{/if}</td>
              <td class="num">{dur(it.length)}</td>
              <td class="flags">
                {#if it.hard}<span class="pill warn" title="Evento duro: suena a la hora exacta">duro</span>{/if}
                {#if it.optional}<span class="pill" title="Se salta si el aire va tarde">opcional</span>{/if}
                {#if it.locked}<span class="pill ok" title="Bloqueado: se conserva al regenerar">🔒</span>{/if}
                {#if it.status !== 'pending'}<span class="pill">{statusLabel[it.status]}</span>{/if}
              </td>
              <td class="acts">
                {#if sel === it.id && it.status === 'pending'}
                  <button class="small ghost" title="Subir" disabled={busy} onclick={() => act(() => MoveLogItem(it.id, -1))}>↑</button>
                  <button class="small ghost" title="Bajar" disabled={busy} onclick={() => act(() => MoveLogItem(it.id, 1))}>↓</button>
                  <button class="small ghost" title="Bloquear" disabled={busy} onclick={() => act(() => ToggleLogLock(it.id))}>🔒</button>
                  <button class="small ghost" title="Insertar pista después" disabled={busy} onclick={() => (picking = true)}>＋</button>
                  <button class="small danger" title="Quitar" disabled={busy} onclick={() => act(() => RemoveLogItem(it.id))}>✕</button>
                {/if}
              </td>
            </tr>
          {/each}
        </tbody>
      </table>
    {/each}
  </div>
</div>

{#if picking && sel}
  <TrackPicker title="Insertar pista después del elemento elegido"
    oncancel={() => (picking = false)}
    onpick={(t) => { picking = false; act(() => InsertLogTrack(sel!, t.id)) }} />
{/if}

<style>
  .page { display: flex; flex-direction: column; gap: 10px; height: 100%; min-height: 0; }
  .hour { margin: 14px 0 4px; color: var(--orange); }
  td.t { width: 78px; font-variant-numeric: tabular-nums; color: var(--muted); }
  td.k { width: 28px; text-align: center; }
  td.name { max-width: 560px; }
  td.flags { width: 190px; }
  td.acts { width: 210px; text-align: right; }
  tr.done td { opacity: .5; }
  tr.opt td.name { font-style: italic; }
</style>
