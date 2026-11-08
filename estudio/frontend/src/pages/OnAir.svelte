<!--
  Al aire: la pantalla principal del operador. Decks A/B (lo que suena, cuenta
  atrás, intro), lo que sigue, vúmetros, modos, micrófono, botonera y estado de
  la emisión. Atajos: Espacio = siguiente · F1–F12 = botones · Ctrl+M = micrófono.
-->
<script lang="ts">
  import { onMount } from 'svelte'
  import { OnAirStart, OnAirStop, OnAirMode, SetDuck, StopCarts, Carts } from '../../wailsjs/go/ui/App'
  import type { domain, onair as oa } from '../../wailsjs/go/models'
  import { onair } from '../lib/onair.svelte'
  import { dur } from '../lib/format'
  import { slotIcon, clock } from '../lib/labels'
  import { toast } from '../lib/toast.svelte'
  import Meter from '../lib/Meter.svelte'

  let carts: domain.Cart[] = $state([])
  let duck = $state(false)
  const v = $derived(onair.view)
  const modes = [
    { id: 'auto', label: 'Automático' },
    { id: 'assisted', label: 'Asistido' },
    { id: 'live', label: 'En vivo' },
  ]

  async function loadCarts() {
    carts = await Carts(onair.cartPage)
  }

  async function run(f: () => Promise<any>) {
    try {
      await f()
    } catch (e) {
      toast.error(e)
    }
  }

  function deckClass(d: oa.DeckView) {
    if (!d.segment) return 'idle'
    return d.current ? 'front' : 'back'
  }

  function pct(d: oa.DeckView) {
    const seg = d.segment
    if (!seg) return 0
    const total = seg.segue > 0 ? seg.segue : seg.length
    return total > 0 ? Math.min(100, (d.position / total) * 100) : 0
  }

  const cartAt = (slot: number) => carts.find((c) => c.slot === slot)
  const cartOn = (slot: number) => v?.air.carts_on?.includes(slot) ?? false

  onMount(() => {
    onair.start()
    loadCarts()
  })
</script>

<div class="onair">
  <!-- barra superior: modos y control -->
  <div class="row top">
    {#each modes as m}
      <button class="mode" class:active={v?.air.mode === m.id} onclick={() => run(() => OnAirMode(m.id))}>{m.label}</button>
    {/each}
    <span class="grow"></span>
    {#if v?.air.running}
      <button class="warn" onclick={() => run(OnAirStop)}>■ Detener</button>
    {:else}
      <button class="ok" onclick={() => run(OnAirStart)}>▶ Iniciar</button>
    {/if}
    <button class:pulse={v?.air.waiting} onclick={() => onair.next()} title="Espacio">⏭ Siguiente</button>
    <button class="mic" class:on={onair.mic} onclick={() => onair.toggleMic()} title="Ctrl+M">🎙 Mic {onair.mic ? 'abierto' : 'cerrado'}</button>
    <button class:ghost={!duck} onclick={() => { duck = !duck; SetDuck(duck) }}>Bajar música</button>
  </div>

  <div class="main">
    <!-- decks -->
    <section class="decks">
      {#each v?.air.decks ?? [] as d, i}
        <div class="deck panel {deckClass(d)}">
          <div class="row"><span class="tag">{i === 0 ? 'A' : 'B'}</span>
            {#if d.segment}<span class="pill">{d.segment.kind}</span>{/if}
            <span class="grow"></span>
            {#if d.intro_left > 0}<span class="intro">INTRO {Math.ceil(d.intro_left / 1e9)}</span>{/if}
          </div>
          {#if d.segment}
            <div class="title">{d.segment.title || '—'}</div>
            <div class="artist muted">{d.segment.artist}</div>
            <div class="bar"><div style:width="{pct(d)}%"></div></div>
            <div class="row times"><span>{dur(d.position)}</span><span class="grow"></span>
              <span class="remain" class:low={d.remaining > 0 && d.remaining < 10e9}>−{dur(d.remaining)}</span></div>
          {:else}
            <div class="title muted">(vacío)</div>
          {/if}
        </div>
      {/each}
      {#if v?.air.waiting}<div class="panel waiting">Asistido: pulse <b>Espacio</b> o «Siguiente» para continuar</div>{/if}
      {#if v && v.air.silence_left > 0}<div class="panel waiting">Silencio programado: {dur(v.air.silence_left)}</div>{/if}
    </section>

    <!-- vúmetros -->
    <section class="meters panel">
      <Meter label="L" peak={v?.air.engine.levels.PeakL ?? 0} rms={v?.air.engine.levels.RMSL ?? 0} />
      <Meter label="R" peak={v?.air.engine.levels.PeakR ?? 0} rms={v?.air.engine.levels.RMSR ?? 0} />
      <div class="small muted info">
        {#if v && v.air.engine.duck_gain < 0.95}<div class="pill warn">ducking {(20 * Math.log10(v.air.engine.duck_gain)).toFixed(0)} dB</div>{/if}
        {#if v && v.air.engine.limiter_gain < 0.97}<div class="pill">lim {(20 * Math.log10(v.air.engine.limiter_gain)).toFixed(1)} dB</div>{/if}
      </div>
    </section>

    <!-- lo que sigue -->
    <section class="next panel">
      <h2>Lo que sigue</h2>
      {#each v?.air.next ?? [] as it (it.id)}
        <div class="nx" class:hard={it.hard}>
          <span class="t">{clock(it.at).slice(0, 5)}</span><span>{slotIcon[it.kind] ?? ''}</span>
          <span class="grow name">{it.artist ? it.artist + ' - ' : ''}{it.title}</span><span class="muted">{dur(it.length)}</span>
        </div>
      {:else}
        <p class="muted small">No hay nada programado. Revise la playlist.</p>
      {/each}
    </section>
  </div>

  <!-- botonera -->
  <section class="carts panel">
    <div class="row">
      <h2 class="grow">Botonera · página {onair.cartPage + 1}</h2>
      {#each Array(10) as _, p}<button class="small" class:ghost={p !== onair.cartPage} onclick={() => { onair.cartPage = p; loadCarts() }}>{p + 1}</button>{/each}
      <button class="small danger" onclick={() => run(StopCarts)}>Parar todo</button>
    </div>
    <div class="cartgrid">
      {#each Array(24) as _, i}
        {@const c = cartAt(i)}
        <button class="cart" class:on={cartOn(i)} class:empty={!c} style:background={c ? c.color : 'transparent'}
          onclick={() => c && onair.cart(i)} title={c?.path ?? 'Configúrelo en Botonera'}>
          <span class="k">{i < 12 ? 'F' + (i + 1) : ''}</span>{c?.label ?? ''}
        </button>
      {/each}
    </div>
  </section>

  <!-- estado -->
  <div class="status row small">
    <span>Salida: {v?.audio.output ?? '…'}</span>
    {#if v?.audio.mic}<span>· Mic: {v.audio.mic}</span>{/if}
    <span class="grow"></span>
    {#if v?.stream.enabled}
      <span class="pill" class:ok={v.stream.encoder.connected} class:bad={!v.stream.encoder.connected}>
        Icecast {v.stream.encoder.connected ? 'conectado' : 'desconectado'} · {v.stream.encoder.kbps} kbps
        {#if v.stream.encoder.retries} · reintentos {v.stream.encoder.retries}{/if}</span>
      {#if v.stream.error || v.stream.encoder.last_error}<span class="error">{v.stream.error || v.stream.encoder.last_error}</span>{/if}
    {:else}
      <span class="pill">Icecast desactivado</span>
    {/if}
  </div>
  {#if v?.audio.error}<div class="error small">{v.audio.error}</div>{/if}
  {#if v?.air.warnings?.length}<div class="warnline small" title={v.air.warnings.map((w) => w.message).join('\n')}>⚠ {v.air.warnings[0].message}</div>{/if}
</div>

<style>
  .onair { display: flex; flex-direction: column; gap: 10px; height: 100%; min-height: 0; }
  .mode { background: transparent; border: 1px solid var(--line); }
  .mode.active { background: var(--royal); border-color: var(--royal); }
  .mic { background: #3a2a2a; }
  .mic.on { background: var(--red); animation: blink 1s infinite; }
  .pulse { animation: blink 1s infinite; background: var(--orange); color: #111; }
  @keyframes blink { 50% { filter: brightness(1.4); } }
  .main { display: grid; grid-template-columns: 1fr 70px 380px; gap: 10px; flex: 1; min-height: 0; }
  .decks { display: flex; flex-direction: column; gap: 10px; min-width: 0; }
  .deck { display: flex; flex-direction: column; gap: 6px; transition: opacity .3s; }
  .deck.back { opacity: .55; }
  .deck.front { border-color: var(--red); box-shadow: 0 0 0 1px var(--red) inset; }
  .deck .tag { background: var(--panel-2); border-radius: 6px; padding: 2px 8px; font-weight: 700; }
  .title { font-size: 24px; font-weight: 700; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
  .artist { font-size: 16px; }
  .bar { height: 8px; background: var(--panel-2); border-radius: 4px; overflow: hidden; }
  .bar div { height: 100%; background: var(--royal); }
  .times { font-variant-numeric: tabular-nums; font-size: 18px; }
  .remain { font-size: 30px; font-weight: 700; }
  .remain.low { color: var(--red); }
  .intro { background: var(--orange); color: #111; font-weight: 700; padding: 2px 8px; border-radius: 6px; }
  .waiting { border-color: var(--orange); }
  .meters { display: flex; gap: 6px; justify-content: center; position: relative; padding: 10px 8px 40px; }
  .meters .info { position: absolute; bottom: 4px; left: 2px; right: 2px; display: flex; flex-direction: column; gap: 2px; align-items: center; }
  .next { overflow: auto; }
  .nx { display: flex; gap: 8px; padding: 5px 0; border-bottom: 1px solid #1c2554; font-size: 13px; }
  .nx.hard { color: var(--orange); }
  .nx .t { font-variant-numeric: tabular-nums; color: var(--muted); }
  .nx .name { white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
  .cartgrid { display: grid; grid-template-columns: repeat(12, 1fr); gap: 6px; margin-top: 6px; }
  .cart { height: 46px; position: relative; font-size: 12px; padding: 4px; border: 1px solid var(--line); overflow: hidden; }
  .cart.empty { opacity: .35; cursor: default; }
  .cart.on { outline: 3px solid #fff; animation: blink .8s infinite; }
  .cart .k { position: absolute; top: 2px; left: 4px; font-size: 9px; opacity: .8; }
  .status { color: var(--muted); }
  .warnline { color: var(--orange); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
</style>
