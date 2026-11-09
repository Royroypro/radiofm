<!--
  Al aire: pantalla principal del operador, todo en una vista.

  ┌ cabecera ─ Siguiente │ AHORA (cuenta atrás) │ SIGUE │ transporte │ vúmetro │ reloj │ modo/menú ┐
  │            forma de onda de lo que suena                                                   │
  ├ playlist ─┬ biblioteca (pestañas, búsqueda, insertar) ┬ botonera ─┬ historial / tanda ──────┤
  └───────────┴──────────────────────────────────────────┴───────────┴─────────────────────────┘

  Atajos: Espacio = siguiente · F1–F12 = botonera · Ctrl+M = micrófono.
-->
<script lang="ts">
  import { onMount } from 'svelte'
  import {
    OnAirStart, OnAirStop, OnAirMode, OnAirStopAfter, StopCarts, Carts, History, Categories, Tracks,
    InsertLogTrack, RemoveLogItem,
  } from '../../wailsjs/go/ui/App'
  import { domain, type ui } from '../../wailsjs/go/models'
  import { onair, win, toggleFullscreen } from '../lib/onair.svelte'
  import { nav } from '../lib/nav.svelte'
  import { dur } from '../lib/format'
  import { clock } from '../lib/labels'
  import { toast } from '../lib/toast.svelte'
  import Meter from '../lib/Meter.svelte'
  import Wave from '../lib/Wave.svelte'
  import RoundClock from '../lib/RoundClock.svelte'
  import isotipo from '../assets/brand/isotipo.svg'
  import { theme } from '../lib/themeState.svelte'

  const v = $derived(onair.view)
  const air = $derived(v?.air)
  let now = $state(new Date())

  // ---- AHORA / SIGUE ----
  const front = $derived(air?.decks.find((d) => d.current && d.segment) ?? air?.decks.find((d) => d.segment) ?? null)
  const nextItem = $derived(air?.next?.[0] ?? null)
  const NS = 1e9
  function count(ns: number) {
    const s = Math.max(0, ns / NS)
    return s < 60 ? s.toFixed(1) + 's' : dur(ns)
  }
  const kindLabel: Record<string, string> = { music: 'Canción', jingle: 'Jingle', spot: 'Cuña', voice: 'Locución', cart: 'Botonera' }
  const itemIcon: Record<string, string> = { category: '♪', track: '♫', spots: '$', time: '🕒', temp: '🌡', mode: '⇄', silence: '…' }

  async function run(f: () => Promise<any>) {
    try {
      await f()
    } catch (e) {
      toast.error(e)
    }
  }

  // ---- playlist ----
  let selItem: number | null = $state(null)
  const playlist = $derived([...(air?.current ? [air.current] : []), ...(air?.next ?? [])])

  // ---- biblioteca ----
  let cats: ui.CategoryView[] = $state([])
  let libKind = $state('music')
  let libQuery = $state('')
  let libTracks: domain.Track[] = $state([])
  let libSel: number | null = $state(null)
  const libKinds = [
    { id: 'music', icon: '♪', label: 'Música' },
    { id: 'jingle', icon: '🔔', label: 'Jingles' },
    { id: 'spot', icon: '$', label: 'Cuñas' },
    { id: 'voice', icon: '🎙', label: 'Locuciones' },
    { id: 'filler', icon: '⋯', label: 'Relleno' },
    { id: 'all', icon: '★', label: 'Todo' },
  ]
  let libTimer: ReturnType<typeof setTimeout>
  async function loadLibrary() {
    const ids = libKind === 'all' ? [0] : cats.filter((c) => c.kind === libKind).map((c) => c.id)
    const lists = await Promise.all(ids.map((id) => Tracks(domain.TrackFilter.createFrom({ CategoryID: id, Query: libQuery, OnlyActive: true, Limit: 300 }))))
    libTracks = lists.flatMap((p) => p.items).sort((a, b) => (a.artist + a.title).localeCompare(b.artist + b.title))
  }
  function onLibQuery() {
    clearTimeout(libTimer)
    libTimer = setTimeout(loadLibrary, 200)
  }
  async function insertNext(trackID: number) {
    const after = selItem ?? air?.current?.id ?? air?.next?.[0]?.id
    if (!after) return toast.show('No hay playlist en curso para insertar', true)
    await run(async () => {
      await InsertLogTrack(after, trackID)
      toast.show('Insertada en la playlist')
    })
  }
  async function removeSelected() {
    if (!selItem || selItem === air?.current?.id) return
    await run(() => RemoveLogItem(selItem!))
    selItem = null
  }

  // ---- botonera ----
  let carts: domain.Cart[] = $state([])
  async function loadCarts() {
    carts = await Carts(onair.cartPage)
  }
  const cartAt = (slot: number) => carts.find((c) => c.slot === slot)
  const cartOn = (slot: number) => air?.carts_on?.includes(slot) ?? false

  // ---- historial ----
  let history: domain.Play[] = $state([])
  const breakLeft = $derived(air?.next_break ? Math.max(0, new Date(air.next_break as any).getTime() - now.getTime()) * 1e6 : 0)

  onMount(() => {
    onair.start()
    loadCarts()
    Categories().then((c) => {
      cats = c
      loadLibrary()
    })
    const t = setInterval(() => (now = new Date()), 200)
    const h = setInterval(async () => (history = await History(40)), 3000)
    History(40).then((x) => (history = x))
    return () => {
      clearInterval(t)
      clearInterval(h)
    }
  })
</script>

<div class="air">
  <!-- ================= CABECERA ================= -->
  <header>
    <div class="col-skip">
      <button class="skip" class:pulse={air?.waiting} onclick={() => onair.next()} title="Siguiente (Espacio)">
        <span class="ico">⏭</span><span>Siguiente</span></button>
    </div>

    <div class="nowsec">
      <div class="nownext">
        <div class="card now">
          <div class="tag">AHORA</div>
          <div class="meta">
            <div class="t1">{front?.segment?.artist || front?.segment?.title || '—'}</div>
            <div class="t2">{front?.segment?.artist ? front.segment.title : ''}</div>
            <div class="t3">
              {#if front?.segment}{dur(front.segment.length)} · {kindLabel[front.segment.kind] ?? front.segment.kind}{/if}
              {#if front && front.intro_left > 0}<span class="intro">INTRO {Math.ceil(front.intro_left / NS)}</span>{/if}
            </div>
          </div>
          <div class="big" class:low={front && front.remaining > 0 && front.remaining < 10 * NS}>{front?.segment ? count(front.remaining) : '--'}</div>
        </div>
        <div class="card next">
          <div class="tag">SIGUE</div>
          <div class="meta">
            <div class="t1">{nextItem?.artist || nextItem?.title || '—'}</div>
            <div class="t2">{nextItem?.artist ? nextItem.title : ''}</div>
            <div class="t3">{#if nextItem}{clock(nextItem.at).slice(0, 8)}{#if nextItem.hard} · <b class="hard">DURO</b>{/if}{/if}</div>
          </div>
          <div class="big dim">{nextItem ? count(nextItem.length) : '--'}</div>
        </div>
      </div>
      <div class="wavebox">
        <Wave envelope={front?.segment?.envelope ?? ''} duration={front?.segment?.duration || front?.segment?.length || 0}
          cueIn={front?.segment?.cue_in ?? 0} position={front?.position ?? 0} intro={front?.segment?.intro ?? 0}
          segue={front?.segment?.segue ?? 0} theme={theme.name + JSON.stringify(theme.custom)} />
      </div>
    </div>

    <div class="transport">
      {#if air?.running}
        <button class="tbtn" onclick={() => run(OnAirStop)} title="Detener la automatización"><span class="ico">■</span>Detener</button>
      {:else}
        <button class="tbtn go" onclick={() => run(OnAirStart)}><span class="ico">▶</span>Iniciar</button>
      {/if}
      <button class="tbtn" class:on={air?.stop_after} onclick={() => run(OnAirStopAfter)} title="Parar al terminar lo que suena"><span class="ico">⏯</span>Parar tras</button>
      <button class="tbtn" class:mic={onair.mic} onclick={() => onair.toggleMic()} title="Ctrl+M"><span class="ico">🎙</span>{onair.mic ? 'Mic abierto' : 'Micrófono'}</button>
    </div>

    <div class="vu">
      <Meter label="L" peak={air?.engine.levels.PeakL ?? 0} rms={air?.engine.levels.RMSL ?? 0} />
      <Meter label="R" peak={air?.engine.levels.PeakR ?? 0} rms={air?.engine.levels.RMSR ?? 0} />
    </div>

    <div class="clockbox"><RoundClock {now} /></div>

    <div class="brandbox">
      <div class="modes">
        {#each [['auto', 'Automático'], ['assisted', 'Asistido'], ['live', 'En vivo']] as [id, label]}
          <button class="mode" class:active={air?.mode === id} onclick={() => run(() => OnAirMode(id))}>{label}</button>
        {/each}
      </div>
      <img src={isotipo} alt="Club25" />
      <div class="winbtns">
        <button title="Minimizar" onclick={win.minimise}>▁</button>
        <button title="Maximizar / restaurar" onclick={win.maximise}>▢</button>
        <button title="Pantalla completa (Ctrl+F11)" onclick={toggleFullscreen}>⛶</button>
      </div>
      <button class="menu" onclick={() => (nav.menuOpen = !nav.menuOpen)}>☰ Menú</button>
    </div>
  </header>

  <!-- ================= CUERPO ================= -->
  <div class="body">
    <!-- playlist -->
    <section class="pane plist">
      <div class="pane-h">Playlist</div>
      <div class="scroll items">
        {#each playlist as it, i (it.id)}
          <button class="pitem" class:current={i === 0 && air?.current?.id === it.id} class:sel={selItem === it.id} class:hard={it.hard}
            onclick={() => (selItem = selItem === it.id ? null : it.id)}>
            <div class="pl1"><span class="at">{clock(it.at)}</span> <span class="du">Dur: {dur(it.length)}</span>
              {#if it.locked}<span class="lk" title="Insertada a mano: se conserva al regenerar la playlist">🔒</span>{/if}{#if it.optional}<span class="op">opc.</span>{/if}</div>
            <div class="pl2"><span class="pi">{itemIcon[it.kind] ?? '•'}</span>
              <div class="pt"><div class="a">{it.title}</div><div class="b">{it.artist}</div></div></div>
          </button>
        {:else}
          <p class="muted small pad">No hay nada programado.</p>
        {/each}
      </div>
      <div class="pane-f">
        <div><div class="small muted">Duración restante hoy</div><div class="rem">{dur(air?.remaining_today ?? 0)}</div></div>
        <button class="small ghost" disabled={!selItem || selItem === air?.current?.id} onclick={removeSelected} title="Quitar el elemento elegido">🗑</button>
        <button class="small ghost" onclick={() => nav.go('log')} title="Abrir la playlist completa">☰</button>
      </div>
    </section>

    <!-- biblioteca -->
    <section class="pane lib">
      <div class="pane-h">Biblioteca</div>
      <div class="libtabs">
        {#each libKinds as k}
          <button class="ltab" class:active={libKind === k.id} title={k.label} onclick={() => { libKind = k.id; loadLibrary() }}>
            <span>{k.icon}</span><small>{k.label}</small></button>
        {/each}
      </div>
      <div class="libbody">
        <div class="libactions">
          <button class="act add" disabled={!libSel} onclick={() => libSel && insertNext(libSel)} title="Insertar después del elemento elegido (o del que suena)">＋<small>Insertar</small></button>
          <button class="act" onclick={() => nav.go('library')} title="Abrir la biblioteca completa">📂<small>Abrir</small></button>
        </div>
        <div class="scroll libtable">
          <table>
            <thead><tr><th>Artista</th><th>Título</th><th class="num">Dur.</th></tr></thead>
            <tbody>
              {#each libTracks as t (t.id)}
                <tr class:sel={libSel === t.id} onclick={() => (libSel = t.id)} ondblclick={() => insertNext(t.id)}>
                  <td>{t.artist}</td><td>{t.title}</td><td class="num">{dur(t.duration)}</td></tr>
              {/each}
            </tbody>
          </table>
        </div>
      </div>
      <div class="libsearch">
        <input placeholder="Buscar artista, título o álbum…" bind:value={libQuery} oninput={onLibQuery} />
        <span class="small muted">{libTracks.length} pistas · doble clic = insertar</span>
      </div>
    </section>

    <!-- botonera -->
    <section class="pane fx">
      <div class="pane-h row"><span class="grow">Botonera · página {onair.cartPage + 1}</span>
        <button class="small danger" onclick={() => run(StopCarts)}>Parar</button></div>
      <div class="fxgrid">
        {#each Array(24) as _, i}
          {@const c = cartAt(i)}
          <button class="cart" class:on={cartOn(i)} class:empty={!c} style:background={c ? c.color : 'transparent'}
            onclick={() => c && onair.cart(i)} title={c?.path ?? 'Vacío (configúrelo en Botonera)'}>
            <span class="k">{i < 12 ? 'F' + (i + 1) : ''}</span><span class="lbl">{c?.label ?? ''}</span>
          </button>
        {/each}
      </div>
      <div class="pages">
        {#each Array(10) as _, p}<button class="pg" class:active={p === onair.cartPage} onclick={() => { onair.cartPage = p; loadCarts() }}>{p + 1}</button>{/each}
      </div>
    </section>

    <!-- historial / tanda / estado -->
    <section class="side">
      <div class="pane hist">
        <div class="pane-h">Historial</div>
        <div class="scroll">
          {#each history as p (p.id)}
            <div class="hrow" class:spot={p.kind === 'spot'} class:jingle={p.kind === 'jingle' || p.kind === 'voice'}>
              <span class="ht">{clock(p.started_at).slice(0, 8)}</span>
              <span class="hn">{p.artist ? p.artist + ' - ' : ''}{p.title}</span>
            </div>
          {/each}
        </div>
      </div>
      <div class="pane brk">
        <div class="pane-h">Siguiente tanda</div>
        <div class="brkt">{air?.next_break ? dur(breakLeft) : '--:--'}</div>
        {#if air?.next_break}<div class="small muted center">a las {clock(air.next_break).slice(0, 5)}</div>{/if}
      </div>
      <div class="pane status">
        <div class="pane-h">Estado</div>
        <div class="srow"><span class="dot" class:ok={air?.running}></span>Aire: {air?.running ? (air.waiting ? 'en espera' : 'emitiendo') : 'detenido'}</div>
        <div class="srow"><span class="dot" class:ok={!!v?.audio.output && !v?.audio.error}></span>Salida: {v?.audio.output ?? '…'}</div>
        <div class="srow"><span class="dot" class:ok={v?.stream.encoder.connected} class:off={!v?.stream.enabled}></span>
          Web (Icecast): {v?.stream.enabled ? (v.stream.encoder.connected ? `conectado ${v.stream.encoder.kbps} kbps` : 'desconectado') : 'desactivado'}</div>
        <div class="srow"><span class="dot" class:ok={v?.sync.online} class:off={!v?.sync.configured}></span>
          Panel: {v?.sync.configured ? (v.sync.online ? 'sincronizado' : v.sync.suspended ? 'suspendido' : 'sin conexión') : 'sin configurar'}{v?.sync.queue ? ` · ${v.sync.queue} en cola` : ''}</div>
      </div>
      <div class="pane problems" class:has={(air?.warnings?.length ?? 0) > 0 || !!v?.audio.error || !!v?.stream.error}>
        <div class="pane-h row"><span class="grow">Problemas</span><span>{(air?.warnings?.length ?? 0) + (v?.audio.error ? 1 : 0) + (v?.stream.error ? 1 : 0)}</span></div>
        <div class="scroll small">
          {#if v?.audio.error}<div class="pr">{v.audio.error}</div>{/if}
          {#if v?.stream.error}<div class="pr">{v.stream.error}</div>{/if}
          {#each air?.warnings ?? [] as w}<div class="pr"><span class="muted">{clock(w.at).slice(0, 5)}</span> {w.message}</div>{/each}
        </div>
      </div>
    </section>
  </div>
</div>

<style>
  .air { display: flex; flex-direction: column; height: 100%; min-height: 0; gap: 6px; }
  /* ---------- cabecera ---------- */
  /* Medidas adaptables: de 1024×600 a pantallas grandes. clamp(mínimo, preferido, máximo). */
  header { display: grid; gap: 6px; height: clamp(112px, 19vh, 170px); grid-template-rows: minmax(0, 1fr);
    grid-template-columns: clamp(60px, 5.5vw, 84px) minmax(0, 1fr) clamp(76px, 6.5vw, 100px) 46px clamp(104px, 10.5vw, 160px) clamp(112px, 10.5vw, 160px); }
  .col-skip, .transport { display: flex; flex-direction: column; gap: 6px; }
  .skip { flex: 1; display: flex; flex-direction: column; align-items: center; justify-content: center; gap: 4px; background: var(--panel-2); font-size: 12px; }
  .skip .ico, .tbtn .ico { font-size: clamp(18px, 2vw, 28px); line-height: 1; }
  .nowsec { display: flex; flex-direction: column; min-width: 0; border: 1px solid var(--line); border-radius: 8px; overflow: hidden; background: var(--panel); }
  .nownext { display: grid; grid-template-columns: 1fr 1fr; flex: 1; min-height: 0; }
  .card { display: flex; align-items: stretch; min-width: 0; }
  .card + .card { border-left: 1px solid var(--line); }
  .tag { writing-mode: vertical-rl; transform: rotate(180deg); text-align: center; font-weight: 800; letter-spacing: 2px; font-size: 12px; padding: 4px 3px; }
  .now .tag { background: var(--red); }
  .next .tag { background: var(--orange); color: #111; }
  .meta { flex: 1; min-width: 0; padding: 6px 10px; display: flex; flex-direction: column; justify-content: center; }
  .t1 { font-size: clamp(14px, 1.45vw, 22px); font-weight: 700; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
  .t2 { font-size: clamp(12px, 1.15vw, 17px); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
  .t3 { font-size: 11px; color: var(--muted); margin-top: 2px; }
  .intro { background: var(--orange); color: #111; font-weight: 700; padding: 0 6px; border-radius: 4px; margin-left: 6px; }
  .hard { color: var(--orange); }
  .big { align-self: center; padding: 0 clamp(6px, .9vw, 14px); font-size: clamp(26px, 3.3vw, 52px); font-weight: 800; color: var(--countdown); font-variant-numeric: tabular-nums; }
  .big.low { color: var(--red); animation: blink 1s infinite; }
  .big.dim { color: var(--countdown-dim); font-size: clamp(22px, 2.6vw, 40px); }
  .wavebox { height: clamp(36px, 7vh, 60px); }
  .tbtn { flex: 1; display: flex; flex-direction: column; align-items: center; justify-content: center; font-size: 11px; background: var(--panel-2); padding: 2px; }
  .tbtn.go { background: var(--ok-btn); color: #fff; }
  .tbtn.on { background: var(--orange); color: #111; }
  .tbtn.mic { background: var(--red); animation: blink 1s infinite; }
  header > * { min-height: 0; overflow: hidden; }
  .vu { display: flex; gap: 4px; padding: 4px; border: 1px solid var(--line); border-radius: 8px; background: var(--panel); }
  .clockbox { display: flex; align-items: center; justify-content: center; border: 1px solid var(--line); border-radius: 8px; background: var(--panel); padding: 4px; }
  .brandbox { display: flex; flex-direction: column; gap: 4px; align-items: stretch; border: 1px solid var(--line); border-radius: 8px; background: var(--panel); padding: 6px; }
  .brandbox img { height: clamp(24px, 4vh, 44px); align-self: center; }
  .winbtns { display: grid; grid-template-columns: repeat(3, 1fr); gap: 3px; }
  .winbtns button { padding: 2px 0; font-size: 11px; }
  .modes { display: flex; flex-direction: column; gap: 3px; }
  .mode { padding: 3px; font-size: 11px; background: transparent; border: 1px solid var(--line); color: var(--text); }
  .mode.active { background: var(--ok-btn); border-color: var(--ok-btn); color: #fff; font-weight: 700; }
  .menu { padding: 3px; font-size: 11px; background: var(--panel-2); }
  .pulse { animation: blink 1s infinite; background: var(--orange); color: #111; }
  @keyframes blink { 50% { filter: brightness(1.45); } }

  /* ---------- cuerpo ---------- */
  .body { flex: 1; min-height: 0; display: grid; gap: 6px;
    grid-template-columns: clamp(210px, 21vw, 340px) minmax(0, 1fr) clamp(220px, 21vw, 340px) clamp(200px, 19vw, 300px); }
  .pane { background: var(--panel); border: 1px solid var(--line); border-radius: 8px; display: flex; flex-direction: column; min-height: 0; overflow: hidden; }
  .pane-h { background: var(--panel-2); padding: 5px 10px; font-weight: 700; font-size: 13px; }
  .pane-f { display: flex; align-items: center; gap: 6px; padding: 6px 8px; border-top: 1px solid var(--line); }
  .pane-f > div { flex: 1; }
  .rem { font-size: 22px; font-weight: 700; font-variant-numeric: tabular-nums; }
  .pad { padding: 10px; }
  .center { text-align: center; }

  .items { flex: 1; }
  .pitem { display: block; width: 100%; text-align: left; background: transparent; color: var(--text); border-radius: 0; border-bottom: 1px solid var(--line-soft); padding: 5px 8px; }
  .pitem.current { background: var(--current); border-left: 4px solid var(--red); }
  .pitem.sel { background: var(--sel); }
  .pitem.hard .at { color: var(--orange); }
  .pl1 { font-size: 10px; color: var(--muted); display: flex; gap: 6px; }
  .pl1 .at { color: var(--countdown-dim); font-variant-numeric: tabular-nums; }
  .pl2 { display: flex; gap: 8px; align-items: center; }
  .pi { width: 26px; text-align: center; font-size: 18px; opacity: .8; }
  .pt { min-width: 0; }
  .pt .a { font-size: clamp(13px, 1.05vw, 16px); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
  .pt .b { font-size: 13px; font-style: italic; color: var(--muted); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }

  .lib { min-width: 0; }
  .libtabs { display: grid; grid-template-columns: repeat(6, 1fr); gap: 2px; padding: 4px; }
  .ltab { display: flex; flex-direction: column; align-items: center; padding: 4px 2px; background: var(--panel-2); font-size: 18px; }
  .ltab small { font-size: 10px; }
  .ltab.active { background: var(--accent); color: var(--accent-text); }
  .libbody { flex: 1; min-height: 0; display: grid; grid-template-columns: 70px 1fr; }
  .libactions { display: flex; flex-direction: column; gap: 4px; padding: 4px; }
  .act { display: flex; flex-direction: column; align-items: center; font-size: 22px; padding: 8px 2px; background: var(--panel-2); }
  .act small { font-size: 10px; }
  .act.add { background: var(--ok-btn); color: #fff; }
  .libtable { min-width: 0; }
  .libtable tr.sel td { background: var(--sel); }
  .libsearch { display: flex; gap: 8px; align-items: center; padding: 6px; border-top: 1px solid var(--line); }
  .libsearch input { flex: 1; }

  .fxgrid { flex: 1; display: grid; grid-template-columns: repeat(4, 1fr); grid-auto-rows: 1fr; gap: 4px; padding: 4px; min-height: 0; }
  .cart { position: relative; border: 1px solid var(--line); padding: 2px; font-size: 12px; overflow: hidden; display: flex; align-items: center; justify-content: center; text-align: center; }
  .cart .k { position: absolute; top: 2px; left: 4px; font-size: 9px; opacity: .75; }
  .cart.empty { opacity: .3; cursor: default; }
  .cart.on { outline: 3px solid var(--text); animation: blink .8s infinite; }
  .pages { display: grid; grid-template-columns: repeat(10, 1fr); gap: 2px; padding: 4px; }
  .pg { padding: 4px 0; font-size: 12px; background: var(--panel-2); }
  .pg.active { background: var(--accent); color: var(--accent-text); }

  .side { display: flex; flex-direction: column; gap: 6px; min-height: 0; }
  .hist { flex: 1; }
  .hrow { display: flex; gap: 8px; padding: 4px 8px; border-bottom: 1px solid var(--line-soft); font-size: 12px; }
  .hrow.spot { background: var(--hl-spot); }
  .hrow.jingle { background: var(--hl-jingle); }
  .ht { color: var(--muted); font-variant-numeric: tabular-nums; }
  .hn { white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
  .brkt { font-size: clamp(30px, 3.4vw, 52px); font-weight: 800; text-align: center; font-variant-numeric: tabular-nums; padding: 4px 0 0; }
  .status { padding-bottom: 6px; }
  .srow { font-size: 12px; padding: 3px 10px; display: flex; align-items: center; gap: 6px; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
  .srow .dot { margin: 0; background: var(--red); flex: none; }
  .srow .dot.ok { background: var(--green); }
  .srow .dot.off { background: var(--muted); }
  .problems { max-height: 120px; }

  /* ---------- pantallas pequeñas ---------- */
  @media (max-width: 1280px) {
    .libactions { display: none; }
    .libbody { grid-template-columns: 1fr; }
    .fxgrid { grid-template-columns: repeat(3, 1fr); }
    .pages { grid-template-columns: repeat(5, 1fr); }
  }
  @media (max-width: 1150px) {
    /* la columna derecha pasa a una franja bajo la botonera */
    .body { grid-template-columns: clamp(200px, 24vw, 280px) minmax(0, 1fr) clamp(200px, 26vw, 280px); grid-template-rows: 1fr auto; }
    .side { grid-column: 1 / -1; flex-direction: row; height: 92px; }
    .side .pane { flex: 1; }
    .side .hist { flex: 2; }
    .brkt { font-size: 30px; padding: 0; }
  }
  @media (max-height: 700px) {
    .brandbox img { display: none; }
    .brandbox { gap: 3px; padding: 4px; }
    .transport { gap: 3px; }
    .tbtn { font-size: 10px; }
    .tbtn .ico, .skip .ico { font-size: 16px; }
    .vu { padding: 2px; }
    .t3 { display: none; }
    .pl1 { font-size: 9px; }
    .pitem { padding: 3px 8px; }
    .modes { gap: 1px; }
    .mode { padding: 1px; }
  }
  .problems.has .pane-h { background: var(--problem); color: #fff; }
  .pr { padding: 3px 8px; border-bottom: 1px solid var(--line-soft); color: var(--problem-text); }
</style>
