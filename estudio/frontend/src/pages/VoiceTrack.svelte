<!--
  Voice tracking: el locutor graba su intervención entre dos canciones de la
  playlist. Elija el hueco (después de qué canción), grabe con el micrófono de
  Ajustes → Audio (no sale al aire), escuche cómo queda la transición y guárdela:
  se agrega a la biblioteca (categoría «Voice tracking») y queda bloqueada en la
  playlist (sobrevive a la regeneración).
-->
<script lang="ts">
  import { onMount, onDestroy } from 'svelte'
  import { Today, LogDay, TrackByID, VTStart, VTStop, VTDiscard, VTSave, VTStatus, RemoveLogItem } from '../../wailsjs/go/ui/App'
  import type { domain, ui, voicetrack } from '../../wailsjs/go/models'
  import { dur } from '../lib/format'
  import { clock } from '../lib/labels'
  import { toast } from '../lib/toast.svelte'
  import Wave from '../lib/Wave.svelte'

  let day = $state('')
  let items: domain.LogItem[] = $state([])
  let gap: number | null = $state(null) // índice de A en items
  let a: domain.Track | null = $state(null)
  let b: domain.Track | null = $state(null)
  let status: ui.VoiceTrackStatus | null = $state(null)
  let take: voicetrack.Take | null = $state(null)
  let overlap = $state(1.5) // s: la siguiente entra esto antes del final de la locución
  let title = $state('')
  let playing = $state(false)

  const isMusic = (it: domain.LogItem) => (it.kind === 'category' || it.kind === 'track') && it.track_id > 0
  const isVoice = (it: domain.LogItem) => it.artist === 'Locutor'

  async function load() {
    items = ((await LogDay(day)) ?? []).filter((it) => it.status === 'pending')
  }

  async function pick(i: number) {
    stopPreview()
    gap = i
    a = b = null
    const A = items[i]
    const B = items.slice(i + 1).find(isMusic)
    try {
      a = await TrackByID(A.track_id)
      if (B) b = await TrackByID(B.track_id)
    } catch (e) {
      toast.error(e)
    }
  }

  // --- grabación ---
  let poll: ReturnType<typeof setInterval> | undefined
  async function record() {
    try {
      if (take) await VTDiscard(take.file).catch(() => {})
      take = null
      await VTStart()
      poll = setInterval(async () => (status = await VTStatus()), 150)
    } catch (e) {
      toast.error(e)
    }
  }
  async function stopRec() {
    clearInterval(poll)
    try {
      take = await VTStop()
      status = await VTStatus()
    } catch (e) {
      toast.error(e)
    }
  }
  async function discard() {
    stopPreview()
    if (take) await VTDiscard(take.file).catch(() => {})
    take = null
  }
  async function save() {
    if (!take || gap === null) return
    try {
      await VTSave(take.file, items[gap].id, Math.round(overlap * 1000), title)
      toast.show('Voice track guardado en la playlist')
      take = null
      title = ''
      await load()
    } catch (e) {
      toast.error(e)
    }
  }
  async function remove(it: domain.LogItem) {
    if (!confirm(`¿Quitar «${it.title}» de la playlist?`)) return
    try {
      await RemoveLogItem(it.id)
      await load()
    } catch (e) {
      toast.error(e)
    }
  }

  // --- pre-escucha (audífonos de la PC; no sale al aire) ---
  const sec = (ns: number) => ns / 1e9
  let audios: HTMLAudioElement[] = []
  let timers: ReturnType<typeof setTimeout>[] = []
  function stopPreview() {
    for (const x of audios) { x.pause(); x.removeAttribute('src'); x.load() }
    for (const t of timers) clearTimeout(t)
    audios = []
    timers = []
    playing = false
  }
  function play(url: string, from: number, vol = 1): HTMLAudioElement {
    const x = new Audio(url)
    x.currentTime = from
    x.volume = vol
    x.play().catch((e) => toast.error(e))
    audios.push(x)
    return x
  }
  function fade(x: HTMLAudioElement, secs: number) {
    const t0 = performance.now()
    const step = () => {
      const k = Math.min(1, (performance.now() - t0) / (secs * 1000))
      x.volume = Math.cos((k * Math.PI) / 2)
      if (k < 1 && playing) requestAnimationFrame(step)
    }
    requestAnimationFrame(step)
  }
  // final de A → (punto de mezcla de A) locución con A fundiéndose → B entra «solape» antes del final de la locución
  function listen() {
    if (!a) return
    stopPreview()
    playing = true
    const aMix = sec(a.segue_at || a.cue_out || a.duration)
    const lead = 6
    const xa = play(`/preview/track/${a.id}`, Math.max(0, aMix - lead))
    const voiceLen = take ? sec(take.duration) : 0
    timers.push(setTimeout(() => {
      fade(xa, 2.5)
      if (take) play(`/preview/take/${take.file}`, 0)
      const bAt = take ? Math.max(0, voiceLen - overlap) : 0
      timers.push(setTimeout(() => {
        if (b) play(`/preview/track/${b.id}`, sec(b.cue_in))
        timers.push(setTimeout(stopPreview, 8000))
      }, bAt * 1000))
    }, lead * 1000))
  }

  onMount(() => {
    Today().then((d) => {
      day = d
      load()
    })
    VTStatus().then((s) => (status = s))
  })
  onDestroy(() => {
    clearInterval(poll)
    stopPreview()
  })
</script>

<div class="page vt">
  <section class="panel scroll list">
    <div class="row"><h2 class="grow">Playlist</h2><input type="date" bind:value={day} onchange={load} /></div>
    <p class="small muted">Elija después de qué canción va la locución.</p>
    {#each items as it, i (it.id)}
      <div class="item" class:sel={gap === i} class:voice={isVoice(it)}>
        <span class="t">{clock(it.at).slice(0, 5)}</span>
        <span class="grow">{it.artist ? `${it.artist} - ` : ''}{it.title}</span>
        {#if isVoice(it)}
          <button class="ghost small" onclick={() => remove(it)}>Quitar</button>
        {:else if isMusic(it)}
          <button class="small" onclick={() => pick(i)}>＋ Locución después</button>
        {/if}
      </div>
    {:else}<p class="muted">No hay elementos pendientes este día.</p>{/each}
  </section>

  <section class="panel editor">
    {#if gap === null}
      <p class="muted">Elija un hueco en la playlist.</p>
    {:else}
      <h2>Después de: {items[gap].title}</h2>
      {#if a}
        <div class="small muted">Sale: {a.artist} - {a.title}</div>
        <div class="wave"><Wave envelope={a.envelope} duration={a.duration} cueIn={a.cue_in} position={0} intro={a.intro} segue={Math.max(0, (a.segue_at || 0) - a.cue_in)} /></div>
      {/if}
      {#if b}
        <div class="small muted">Entra: {b.artist} - {b.title} {#if b.intro}· intro {dur(b.intro)}{/if}</div>
        <div class="wave"><Wave envelope={b.envelope} duration={b.duration} cueIn={b.cue_in} position={0} intro={b.intro} segue={0} /></div>
      {/if}

      {#if status && !status.mic_ready}
        <p class="error small">No hay micrófono abierto: elíjalo en Ajustes → Audio y pulse «Guardar y aplicar audio».</p>
      {/if}

      <div class="rec">
        {#if status?.recording}
          <button class="danger big" onclick={stopRec}>■ Detener ({Math.floor(status.elapsed / 1e9)} s)</button>
          <div class="level"><div style="width:{Math.min(100, (status.level ?? 0) * 100)}%"></div></div>
        {:else}
          <button class="big" onclick={record} disabled={!status?.mic_ready}>● Grabar</button>
        {/if}
      </div>

      {#if take}
        <p>Toma: {dur(take.duration)}</p>
        <div class="grid2">
          <label class="field">La siguiente entra antes del final (s)<input type="number" min="0" max="10" step="0.5" bind:value={overlap} /></label>
          <label class="field">Nombre (opcional)<input bind:value={title} placeholder="Ej.: saludo 8 am" /></label>
        </div>
      {/if}
      <div class="row">
        <button onclick={playing ? stopPreview : listen}>{playing ? '■ Parar' : take ? '▶ Escuchar la transición' : '▶ Escuchar el final'}</button>
        {#if take}
          <button class="ghost" onclick={discard}>Descartar toma</button>
          <span class="grow"></span>
          <button class="ok" onclick={save}>Guardar en la playlist</button>
        {/if}
      </div>
      <p class="small muted">La locución entra en el punto de mezcla de la canción que sale (que se funde debajo) y la siguiente canción entra {overlap} s antes de que termine. Se escucha por los audífonos de la PC; nada sale al aire.</p>
    {/if}
  </section>
</div>

<style>
  .vt { display: grid; grid-template-columns: minmax(0, 1fr) minmax(0, 1.2fr); gap: 10px; height: 100%; }
  .list { overflow: auto; }
  .item { display: flex; gap: 8px; align-items: center; padding: 4px 6px; border-bottom: 1px solid var(--line-soft); }
  .item.sel { background: var(--sel); }
  .item.voice { color: var(--accent); font-weight: 700; }
  .t { font-variant-numeric: tabular-nums; width: 3.2em; color: var(--muted); }
  .editor { display: flex; flex-direction: column; gap: 8px; overflow: auto; }
  .wave { height: 56px; }
  .rec { display: flex; gap: 10px; align-items: center; }
  .big { font-size: 16px; padding: 10px 18px; }
  .level { flex: 1; height: 10px; background: var(--panel-2); border-radius: 5px; overflow: hidden; }
  .level div { height: 100%; background: var(--green); transition: width .1s linear; }
  .row { display: flex; gap: 8px; align-items: center; }
  .grow { flex: 1; }
  .small { font-size: 11px; }
</style>
