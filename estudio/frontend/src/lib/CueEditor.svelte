<!--
  Editor de marcas de una pista (Biblioteca): forma de onda con cue-in, intro,
  punto de mezcla y cue-out arrastrables, pre-escucha por la salida
  predeterminada de Windows (audífonos del operador; no toca el aire) y
  «probar transición» con otra pista usando las mismas reglas que el aire.

  Los valores se manejan en segundos (como el formulario de la pista).
  intro y segue son desde el inicio del archivo; cueOut 0 = hasta el final.
-->
<script lang="ts">
  import { onDestroy } from 'svelte'
  import { PreviewTransition } from '../../wailsjs/go/ui/App'
  import { toast } from './toast.svelte'

  type Other = { id: number; label: string }
  let {
    trackId,
    envelope = '',
    duration = 0, // segundos
    cueIn = $bindable(0),
    intro = $bindable(0),
    segue = $bindable(0),
    cueOut = $bindable(0),
    others = [] as Other[],
  }: {
    trackId: number; envelope?: string; duration?: number
    cueIn?: number; intro?: number; segue?: number; cueOut?: number; others?: Other[]
  } = $props()

  type Mark = 'cueIn' | 'intro' | 'segue' | 'cueOut'
  const marks: { key: Mark; label: string; color: string }[] = [
    { key: 'cueIn', label: 'Cue-in', color: '#2ead5b' },
    { key: 'intro', label: 'Intro', color: '#e6b800' },
    { key: 'segue', label: 'Mezcla', color: '#ff8a00' },
    { key: 'cueOut', label: 'Cue-out', color: '#ff1e1e' },
  ]

  let zoom: 'all' | 'start' | 'end' = $state('all')
  let w = $state(600)
  const h = 90
  let canvas: HTMLCanvasElement | undefined = $state()
  let playhead = $state(-1)
  let nextId = $state(0)
  let testing = $state(false)

  const end = $derived(cueOut > 0 ? cueOut : duration)
  const view = $derived.by(() => {
    if (zoom === 'start') return [0, Math.min(duration, 30)]
    if (zoom === 'end') return [Math.max(0, end - 30), duration]
    return [0, duration || 1]
  })
  const xOf = (t: number) => ((t - view[0]) / (view[1] - view[0] || 1)) * w
  const tOf = (x: number) => view[0] + (x / w) * (view[1] - view[0])
  function value(k: Mark): number {
    if (k === 'cueIn') return cueIn
    if (k === 'intro') return intro
    if (k === 'segue') return segue
    return cueOut > 0 ? cueOut : duration
  }
  function setValue(k: Mark, t: number) {
    t = Math.round(Math.max(0, Math.min(duration, t)) * 10) / 10
    if (k === 'cueIn') cueIn = Math.min(t, end - 1)
    else if (k === 'intro') intro = t <= cueIn ? 0 : t
    else if (k === 'segue') segue = t <= cueIn ? 0 : Math.min(t, end)
    else cueOut = t >= duration - 0.15 ? 0 : Math.max(t, cueIn + 1)
  }

  // --- dibujo ---
  $effect(() => {
    if (!canvas) return
    const dpr = window.devicePixelRatio || 1
    canvas.width = w * dpr
    canvas.height = h * dpr
    const css = getComputedStyle(document.documentElement)
    const bar = css.getPropertyValue('--wave-bar').trim() || '#6c7a96'
    const text = css.getPropertyValue('--text').trim() || '#fff'
    const g = canvas.getContext('2d')!
    g.setTransform(dpr, 0, 0, dpr, 0, 0)
    g.clearRect(0, 0, w, h)
    // fuera de cue-in / cue-out: sombreado
    g.fillStyle = 'rgba(0,0,0,.35)'
    g.fillRect(0, 0, Math.max(0, xOf(cueIn)), h)
    g.fillRect(Math.min(w, xOf(end)), 0, w, h)
    // envolvente
    const n = envelope.length
    if (n && duration > 0) {
      g.fillStyle = bar
      for (let i = 0; i < n; i++) {
        const t0 = (i / n) * duration, t1 = ((i + 1) / n) * duration
        if (t1 < view[0] || t0 > view[1]) continue
        const v = parseInt(envelope[i], 36) / 35
        const bh = Math.max(1, v * (h - 14))
        const x0 = xOf(t0), x1 = xOf(t1)
        g.fillRect(x0, (h - bh) / 2 + 6, Math.max(1, x1 - x0 - 0.6), bh)
      }
    }
    // escala de tiempo
    g.fillStyle = text
    g.globalAlpha = 0.55
    g.font = '10px Nunito, sans-serif'
    const span = view[1] - view[0]
    const step = span > 120 ? 30 : span > 40 ? 10 : 5
    for (let t = Math.ceil(view[0] / step) * step; t <= view[1]; t += step) {
      const x = xOf(t)
      g.fillRect(x, 0, 1, 4)
      g.fillText(`${Math.floor(t / 60)}:${String(Math.floor(t % 60)).padStart(2, '0')}`, x + 2, 11)
    }
    g.globalAlpha = 1
    // cabezal de la pre-escucha
    if (playhead >= 0) {
      g.fillStyle = text
      g.fillRect(xOf(playhead) - 1, 0, 2, h)
    }
  })

  // --- arrastre de marcas ---
  let dragging: Mark | null = null
  function down(e: PointerEvent, k: Mark) {
    dragging = k
    ;(e.currentTarget as Element).setPointerCapture(e.pointerId)
    e.stopPropagation()
  }
  function move(e: PointerEvent) {
    if (!dragging) return
    const r = (e.currentTarget as HTMLElement).getBoundingClientRect()
    setValue(dragging, tOf(e.clientX - r.left))
  }
  function up() { dragging = null }
  function clickWave(e: MouseEvent) {
    if (dragging) return
    const r = (e.currentTarget as HTMLElement).getBoundingClientRect()
    playFrom(tOf(e.clientX - r.left))
  }

  // --- pre-escucha ---
  let audio: HTMLAudioElement | null = null
  let audioB: HTMLAudioElement | null = null
  let raf = 0
  let stopAt = 0
  function stop() {
    for (const a of [audio, audioB]) { if (a) { a.pause(); a.removeAttribute('src'); a.load() } }
    audio = audioB = null
    cancelAnimationFrame(raf)
    playhead = -1
    testing = false
  }
  function tick() {
    if (audio) {
      playhead = audio.currentTime
      if (stopAt > 0 && audio.currentTime >= stopAt) { stop(); return }
    }
    raf = requestAnimationFrame(tick)
  }
  function newAudio(id: number, from: number): HTMLAudioElement {
    const a = new Audio(`/preview/track/${id}`)
    a.currentTime = from
    a.onerror = () => toast.error('No se pudo escuchar el archivo')
    return a
  }
  function playFrom(t: number, until = 0) {
    stop()
    audio = newAudio(trackId, Math.max(0, t))
    stopAt = until
    audio.play().catch((e) => toast.error(e))
    raf = requestAnimationFrame(tick)
  }
  function markHere(k: Mark) {
    if (playhead < 0) { toast.error('Primero ponga a sonar la pista (clic en la onda)'); return }
    setValue(k, playhead)
  }

  // «probar transición»: unos segundos antes del punto de mezcla de esta pista
  // hasta que entra la otra, con el fundido que aplicaría el aire.
  async function testTransition() {
    if (!nextId) { toast.error('Elija la pista que sigue'); return }
    let p
    try { p = await PreviewTransition(trackId, nextId) } catch (e) { toast.error(e); return }
    stop()
    testing = true
    const sec = (ns: number) => ns / 1e9
    const startAt = sec(p.start_at), outFade = sec(p.out_fade), delay = sec(p.delay), aEnd = sec(p.a_end)
    const a = newAudio(trackId, Math.max(0, startAt - 6))
    audio = a
    stopAt = 0
    await a.play().catch((e) => toast.error(e))
    raf = requestAnimationFrame(tick)
    const startB = () => {
      if (!testing) return
      const b = newAudio(nextId, sec(p.b_from))
      audioB = b
      b.play().catch(() => {})
      setTimeout(() => { if (audioB === b) stop() }, 8000)
    }
    const watch = () => {
      if (!testing || audio !== a) return
      if (a.currentTime >= startAt) {
        if (outFade >= 0) fadeOut(a, outFade)
        else setTimeout(() => a.pause(), Math.max(0, (aEnd - a.currentTime) * 1000))
        setTimeout(startB, delay * 1000)
        return
      }
      requestAnimationFrame(watch)
    }
    requestAnimationFrame(watch)
  }
  function fadeOut(a: HTMLAudioElement, secs: number) {
    const t0 = performance.now()
    const step = () => {
      const x = Math.min(1, (performance.now() - t0) / (Math.max(secs, 0.05) * 1000))
      a.volume = Math.cos((x * Math.PI) / 2) // misma curva equal-power que el aire
      if (x < 1 && testing) requestAnimationFrame(step)
      else if (x >= 1) a.pause()
    }
    requestAnimationFrame(step)
  }

  onDestroy(stop)
  $effect(() => { void trackId; return () => stop() })
</script>

<div class="cue-editor">
  <div class="zoom">
    <button class:active={zoom === 'all'} onclick={() => (zoom = 'all')}>Toda</button>
    <button class:active={zoom === 'start'} onclick={() => (zoom = 'start')}>Inicio</button>
    <button class:active={zoom === 'end'} onclick={() => (zoom = 'end')}>Final</button>
    <span class="grow"></span>
    <span class="small muted">Clic en la onda: escuchar desde ahí · arrastre las marcas</span>
  </div>
  <!-- svelte-ignore a11y_click_events_have_key_events, a11y_no_static_element_interactions -->
  <div class="wave" bind:clientWidth={w} onpointermove={move} onpointerup={up} onclick={clickWave}>
    <canvas bind:this={canvas} style="width:{w}px;height:{h}px"></canvas>
    {#each marks as m (m.key)}
      {@const t = value(m.key)}
      {#if (m.key !== 'intro' && m.key !== 'segue') || t > 0}
        {#if t >= view[0] && t <= view[1]}
          <div class="mark" style="left:{xOf(t)}px;--c:{m.color}" title="{m.label}: {t.toFixed(1)} s"
            onpointerdown={(e) => down(e, m.key)}>
            <span>{m.label}</span>
          </div>
        {/if}
      {/if}
    {/each}
  </div>
  <div class="row wrap">
    <button onclick={() => playFrom(cueIn)}>▶ Desde cue-in</button>
    <button onclick={() => playFrom(cueIn, intro > cueIn ? intro : cueIn + 15)}>▶ Intro</button>
    <button onclick={() => playFrom(Math.max(0, (segue || end) - 8))}>▶ Final</button>
    <button onclick={stop}>■</button>
    <span class="grow"></span>
    {#each marks as m (m.key)}
      <button class="ghost small" style="border-color:{m.color}" onclick={() => markHere(m.key)}>{m.label} aquí</button>
    {/each}
  </div>
  <div class="row wrap">
    <select bind:value={nextId}>
      <option value={0}>Probar transición con…</option>
      {#each others as o (o.id)}<option value={o.id}>{o.label}</option>{/each}
    </select>
    <button onclick={testTransition} disabled={testing}>▶ Probar transición</button>
  </div>
</div>

<style>
  .cue-editor { display: flex; flex-direction: column; gap: 6px; }
  .zoom { display: flex; gap: 4px; align-items: center; }
  .zoom button { padding: 2px 8px; font-size: 12px; }
  .wave { position: relative; height: 90px; background: var(--wave-bg); border-radius: 6px; cursor: crosshair; overflow: hidden; touch-action: none; }
  canvas { display: block; }
  .mark { position: absolute; top: 0; bottom: 0; width: 12px; margin-left: -6px; cursor: ew-resize; }
  .mark::before { content: ''; position: absolute; left: 5px; top: 0; bottom: 0; width: 2px; background: var(--c); }
  .mark span { position: absolute; top: 2px; left: 8px; font-size: 10px; font-weight: 700; color: #111; background: var(--c); padding: 0 4px; border-radius: 3px; white-space: nowrap; pointer-events: none; }
  .row { display: flex; gap: 6px; align-items: center; }
  .wrap { flex-wrap: wrap; }
  .grow { flex: 1; }
  .small { font-size: 11px; }
</style>
