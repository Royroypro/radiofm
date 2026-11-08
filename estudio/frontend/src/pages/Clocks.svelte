<!--
  Relojes: plantillas de una hora. Cada slot es una categoría (rotación), una
  pista fija, una tanda, la hora o la temperatura locutadas, un cambio de modo
  o un silencio. Los slots "duros" suenan al minuto exacto.
-->
<script lang="ts">
  import { onMount } from 'svelte'
  import { Clocks, SaveClock, DeleteClock, SlotKinds, Categories, Tracks } from '../../wailsjs/go/ui/App'
  import { domain, type ui } from '../../wailsjs/go/models'
  import { slotLabel, modeLabel } from '../lib/labels'
  import { toast } from '../lib/toast.svelte'
  import TrackPicker from '../lib/TrackPicker.svelte'

  let clocks: domain.Clock[] = $state([])
  let kinds: string[] = $state([])
  let cats: ui.CategoryView[] = $state([])
  let edit: any = $state(null)
  let trackNames: Record<number, string> = $state({})
  let pickFor: number | null = $state(null)

  async function load() {
    clocks = await Clocks()
  }

  function open(c: domain.Clock | null) {
    edit = c ? JSON.parse(JSON.stringify(c)) : { id: 0, name: '', color: '#223CB8', slots: [] }
    loadTrackNames()
  }

  async function loadTrackNames() {
    for (const s of edit?.slots ?? []) {
      if (s.kind === 'track' && s.track_id && !trackNames[s.track_id]) {
        const page = await Tracks(domain.TrackFilter.createFrom({ Limit: 5000 }))
        for (const t of page.items) trackNames[t.id] = `${t.artist} - ${t.title}`
        break
      }
    }
  }

  function addSlot(kind = 'category') {
    edit.slots.push({ kind, category_id: cats.find((c) => c.kind === 'music')?.id ?? 0, track_id: 0, hard: false, minute: 0, mode: 'auto', seconds: 5 })
  }

  function move(i: number, d: number) {
    const j = i + d
    if (j < 0 || j >= edit.slots.length) return
    ;[edit.slots[i], edit.slots[j]] = [edit.slots[j], edit.slots[i]]
  }

  async function save() {
    try {
      const c = await SaveClock(domain.Clock.createFrom({
        ...edit,
        slots: edit.slots.map((s: any) => ({ ...s, category_id: Number(s.category_id), minute: Number(s.minute), seconds: Number(s.seconds) })),
      }))
      toast.show('Reloj guardado. Regenere la playlist para aplicarlo a días ya generados.')
      await load()
      open(c)
    } catch (e) {
      toast.error(e)
    }
  }

  async function remove() {
    if (!edit?.id || !confirm(`¿Borrar el reloj «${edit.name}»?`)) return
    try {
      await DeleteClock(edit.id)
      edit = null
      await load()
    } catch (e) {
      toast.error(e)
    }
  }

  const catName = (id: number) => cats.find((c) => c.id === id)?.name ?? '—'

  onMount(async () => {
    await load()
    kinds = await SlotKinds()
    cats = await Categories()
    if (clocks.length) open(clocks[0])
  })
</script>

<div class="clocks">
  <section class="panel">
    <div class="row"><h2 class="grow">Relojes</h2><button class="small" onclick={() => open(null)}>+ Nuevo</button></div>
    {#each clocks as c (c.id)}
      <button class="item" class:active={edit?.id === c.id} onclick={() => open(c)}>
        <span class="dot" style:background={c.color}></span>{c.name}
        <span class="muted small">{c.slots.length} slots</span>
      </button>
    {/each}
  </section>

  <section class="panel editor">
    {#if edit}
      <div class="row">
        <label class="field grow">Nombre<input bind:value={edit.name} /></label>
        <label class="field">Color<input type="color" bind:value={edit.color} /></label>
      </div>
      <div class="scroll grow">
        <table>
          <thead><tr><th>#</th><th>Tipo</th><th>Contenido</th><th>Duro</th><th>Minuto</th><th></th></tr></thead>
          <tbody>
            {#each edit.slots as s, i}
              <tr>
                <td class="muted">{i + 1}</td>
                <td><select bind:value={s.kind}>{#each kinds as k}<option value={k}>{slotLabel[k] ?? k}</option>{/each}</select></td>
                <td>
                  {#if s.kind === 'category'}
                    <select bind:value={s.category_id}>{#each cats as c (c.id)}<option value={c.id}>{c.name} ({c.tracks})</option>{/each}</select>
                  {:else if s.kind === 'track'}
                    <button class="small ghost" onclick={() => (pickFor = i)}>{trackNames[s.track_id] ?? (s.track_id ? `pista ${s.track_id}` : 'Elegir pista…')}</button>
                  {:else if s.kind === 'mode'}
                    <select bind:value={s.mode}>{#each Object.entries(modeLabel) as [v, l]}<option value={v}>{l}</option>{/each}</select>
                  {:else if s.kind === 'silence'}
                    <input type="number" min="1" max="600" style="width:80px" bind:value={s.seconds} /> s
                  {:else}
                    <span class="muted small">—</span>
                  {/if}
                </td>
                <td><input type="checkbox" bind:checked={s.hard} /></td>
                <td>{#if s.hard}<input type="number" min="0" max="59" style="width:64px" bind:value={s.minute} />{/if}</td>
                <td class="acts">
                  <button class="small ghost" onclick={() => move(i, -1)}>↑</button>
                  <button class="small ghost" onclick={() => move(i, 1)}>↓</button>
                  <button class="small danger" onclick={() => edit.slots.splice(i, 1)}>✕</button>
                </td>
              </tr>
            {/each}
          </tbody>
        </table>
        {#if !edit.slots.length}<p class="muted">Sin slots. Agregue categorías y eventos.</p>{/if}
      </div>
      <div class="row">
        <button class="ghost" onclick={() => addSlot('category')}>+ Categoría</button>
        <button class="ghost" onclick={() => addSlot('spots')}>+ Tanda</button>
        <button class="ghost" onclick={() => addSlot('time')}>+ Hora</button>
        <button class="ghost" onclick={() => addSlot('temp')}>+ Temperatura</button>
        <span class="grow"></span>
        {#if edit.id}<button class="danger" onclick={remove}>Borrar</button>{/if}
        <button class="ok" onclick={save}>Guardar</button>
      </div>
      <p class="muted small">Los slots no duros suenan en orden. Un slot duro suena al minuto exacto: el generador rellena antes si sobra
        tiempo y, al aire, funde lo que esté sonando. Al final de la hora se rellena con la categoría «Relleno» (o la última categoría musical).</p>
    {:else}
      <p class="muted">Elija o cree un reloj.</p>
    {/if}
  </section>
</div>

{#if pickFor !== null}
  <TrackPicker oncancel={() => (pickFor = null)} onpick={(t) => { edit.slots[pickFor!].track_id = t.id; trackNames[t.id] = `${t.artist} - ${t.title}`; pickFor = null }} />
{/if}

<style>
  .clocks { display: grid; grid-template-columns: 240px 1fr; gap: 12px; height: 100%; min-height: 0; }
  .item { display: flex; align-items: center; gap: 4px; width: 100%; background: transparent; text-align: left; padding: 7px 8px; }
  .item .small { margin-left: auto; }
  .item.active { background: var(--royal); }
  .editor { display: flex; flex-direction: column; gap: 10px; min-height: 0; min-width: 0; }
  td.acts { text-align: right; width: 130px; }
</style>
