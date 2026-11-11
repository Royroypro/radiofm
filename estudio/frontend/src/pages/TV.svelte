<!--
  Modo TV: monitor del programa, encendido, escenas, capas de grafismo y
  configuración de la emisión de video a la web (internal/video).
-->
<script lang="ts">
  import { onMount, onDestroy } from 'svelte'
  import { Settings, SaveSettings, TVStatus, TVSetScene, TVSetLayers, TVOnAir, TVApply, SetTVPassword, HasTVPassword, PickImage } from '../../wailsjs/go/ui/App'
  import { config, type video } from '../../wailsjs/go/models'
  import { toast } from '../lib/toast.svelte'

  let s: config.Settings | null = $state(null)
  let st: video.Status | null = $state(null)
  let hasPass = $state(false)
  let pass = $state('')
  let snap = $state('')
  let dirty = $state(false)
  let timer: ReturnType<typeof setInterval> | undefined

  onMount(async () => {
    s = await Settings()
    hasPass = await HasTVPassword()
    await refresh()
    timer = setInterval(refresh, 400)
  })
  onDestroy(() => clearInterval(timer))

  async function refresh() {
    st = await TVStatus()
    snap = st?.running ? `/preview/tv.jpg?t=${Date.now()}` : ''
  }

  async function onAir(on: boolean) {
    try {
      await TVOnAir(on)
      s = await Settings()
      toast.show(on ? 'TV al aire' : 'TV apagada')
    } catch (e) {
      toast.error(e)
    }
  }

  function layers() {
    if (!s) return
    TVSetLayers(s.tv.show_logo, s.tv.show_now, s.tv.show_clock, s.tv.show_ticker)
    dirty = true
  }

  async function save() {
    if (!s) return
    try {
      s.tv.fps = Number(s.tv.fps)
      s.tv.bitrate_kbps = Number(s.tv.bitrate_kbps)
      await SaveSettings(config.Settings.createFrom(s))
      await TVApply()
      dirty = false
      toast.show('Ajustes de TV guardados y aplicados')
    } catch (e) {
      toast.error(e)
    }
  }

  async function savePass() {
    try {
      await SetTVPassword(pass)
      pass = ''
      hasPass = true
      toast.show('Clave de la TV guardada')
    } catch (e) {
      toast.error(e)
    }
  }

  async function pick(field: 'logo' | 'background' | 'plate', title: string) {
    const p = await PickImage(title)
    if (p && s) {
      s.tv[field] = p
      dirty = true
    }
  }

  const ago = (t: any) => (t ? new Date(t).toLocaleTimeString('es-PE', { hour12: false }) : '')
</script>

{#if s}
  <div class="tv scroll">
    <div class="row top">
      <h1 class="grow">TV</h1>
      {#if s.tv.enabled}
        <button class="danger" onclick={() => onAir(false)}>■ Apagar TV</button>
      {:else}
        <button class="ok" onclick={() => onAir(true)}>● TV al aire</button>
      {/if}
    </div>

    <div class="cols">
      <section class="panel monitor">
        <div class="screen">
          {#if snap}<img src={snap} alt="Programa de la TV" />{:else}<span class="muted">TV apagada</span>{/if}
          {#if st?.running}<span class="live" class:ok={st.encoder.connected}>{st.encoder.connected ? '● AL AIRE' : 'conectando…'}</span>{/if}
        </div>
        <div class="row wrap">
          <span class="muted small">Escena:</span>
          <button class:ghost={st?.scene !== 'visual'} onclick={() => TVSetScene('visual')}>Visualizador</button>
          <button class:ghost={st?.scene !== 'image'} disabled={!s.tv.plate} title={s.tv.plate ? '' : 'Elija una imagen de placa abajo'}
            onclick={() => TVSetScene('image')}>Placa</button>
        </div>
        <div class="row wrap">
          <span class="muted small">Capas:</span>
          <label class="check"><input type="checkbox" bind:checked={s.tv.show_logo} onchange={layers} /> Logo</label>
          <label class="check"><input type="checkbox" bind:checked={s.tv.show_now} onchange={layers} /> Ahora suena</label>
          <label class="check"><input type="checkbox" bind:checked={s.tv.show_clock} onchange={layers} /> Reloj</label>
          <label class="check"><input type="checkbox" bind:checked={s.tv.show_ticker} onchange={layers} /> Ticker</label>
        </div>
        {#if st?.running}
          <p class="small muted">
            {st.size} · {st.fps.toFixed(1)} fps · {st.encoder.kbps} kbps · {st.encoder.frames} cuadros
            {#if st.encoder.dropped}· <span class="warn">{st.encoder.dropped} descartados</span>{/if}
            {#if st.late}· <span class="warn">{st.late} atrasados (PC lenta: baje la resolución)</span>{/if}
            {#if st.encoder.since}· al aire desde {ago(st.encoder.since)}{/if}
          </p>
          {#if st.encoder.last_error}<p class="error small">{st.encoder.last_error}</p>{/if}
        {/if}
      </section>

      <section class="panel cfg" oninput={() => (dirty = true)}>
        <div class="row"><h2 class="grow">Emisión</h2><button class="ok" disabled={!dirty} onclick={save}>Guardar y aplicar</button></div>
        <label class="field">Servidor (RTMP)<input bind:value={s.tv.url} placeholder="rtmp://radioclub25peru.com:1935/tv" /></label>
        <div class="grid2">
          <label class="field">Usuario<input bind:value={s.tv.user} /></label>
          <label class="field">Clave {hasPass ? '(guardada)' : ''}
            <span class="row"><input type="password" bind:value={pass} placeholder={hasPass ? '••••••' : ''} />
              <button class="small" disabled={!pass} onclick={savePass}>Guardar</button></span></label>
        </div>
        <p class="muted small">El servidor, el usuario y la clave llegan solos del panel web (Ajustes → Streams) al
          sincronizar; si los cambian allá, el Estudio los renueva y se reconecta. Escríbalos aquí solo si la PC no está
          conectada al panel.</p>
        <div class="grid3">
          <label class="field">Resolución<select bind:value={s.tv.resolution}>
            <option value="480p">480p (PC modesta)</option><option value="720p">720p HD</option><option value="1080p">1080p Full HD</option></select></label>
          <label class="field">Cuadros/s<select bind:value={s.tv.fps}><option value={25}>25</option><option value={30}>30</option></select></label>
          <label class="field">Video (kbps)<input type="number" min="300" max="12000" step="100" bind:value={s.tv.bitrate_kbps} /></label>
          <label class="field">Codificador<select bind:value={s.tv.codec}>
            <option value="x264">x264 (procesador)</option><option value="nvenc">NVIDIA (NVENC)</option>
            <option value="qsv">Intel (Quick Sync)</option><option value="amf">AMD (AMF)</option></select></label>
        </div>

        <h2>Grafismo</h2>
        <div class="grid2">
          <div class="field">Logo (PNG con transparencia)
            <span class="row"><button class="small ghost" onclick={() => pick('logo', 'Logo de la TV')}>Elegir…</button>
              <span class="small muted ell">{s.tv.logo || 'nombre de la radio'}</span>
              {#if s.tv.logo}<button class="small ghost" onclick={() => { s!.tv.logo = ''; dirty = true }}>✕</button>{/if}</span></div>
          <label class="field">Posición del logo<select bind:value={s.tv.logo_pos}>
            <option value="tr">Arriba a la derecha</option><option value="tl">Arriba a la izquierda</option>
            <option value="br">Abajo a la derecha</option><option value="bl">Abajo a la izquierda</option></select></label>
          <div class="field">Fondo del visualizador
            <span class="row"><button class="small ghost" onclick={() => pick('background', 'Fondo de la TV')}>Elegir…</button>
              <span class="small muted ell">{s.tv.background || 'degradado de la marca'}</span>
              {#if s.tv.background}<button class="small ghost" onclick={() => { s!.tv.background = ''; dirty = true }}>✕</button>{/if}</span></div>
          <div class="field">Placa (escena de imagen)
            <span class="row"><button class="small ghost" onclick={() => pick('plate', 'Placa de la TV')}>Elegir…</button>
              <span class="small muted ell">{s.tv.plate || '—'}</span>
              {#if s.tv.plate}<button class="small ghost" onclick={() => { s!.tv.plate = ''; dirty = true }}>✕</button>{/if}</span></div>
        </div>
        <div class="grid3">
          <label class="field">Color principal<input type="color" bind:value={s.tv.accent} /></label>
          <label class="field">Color de acento<input type="color" bind:value={s.tv.accent2} /></label>
          <label class="field">Color de fondo<input type="color" bind:value={s.tv.panel} /></label>
        </div>
        <label class="field">Texto del ticker<input bind:value={s.tv.ticker} placeholder="Escríbenos al WhatsApp… · Síguenos en Facebook…" /></label>
        <p class="muted small">Sin video, la TV muestra el espectro del audio que suena, con el logo, el reloj y el cintillo «ahora suena»;
          en las tandas aparece la placa «Publicidad». El audio es el mismo que va a la web.</p>
      </section>
    </div>
  </div>
{/if}

<style>
  .tv { display: flex; flex-direction: column; gap: 12px; height: 100%; padding-right: 6px; }
  .top { position: sticky; top: 0; background: var(--bg); padding-bottom: 6px; z-index: 2; }
  .cols { display: grid; grid-template-columns: minmax(380px, 1.2fr) minmax(340px, 1fr); gap: 12px; align-items: start; }
  .monitor { display: flex; flex-direction: column; gap: 10px; }
  .screen { position: relative; aspect-ratio: 16 / 9; background: #000; border-radius: 8px; overflow: hidden;
    display: grid; place-items: center; }
  .screen img { width: 100%; height: 100%; object-fit: contain; }
  .live { position: absolute; top: 8px; left: 8px; font-size: 12px; font-weight: 700; padding: 2px 8px;
    border-radius: 4px; background: #555; color: #fff; }
  .live.ok { background: #d62828; }
  .wrap { flex-wrap: wrap; gap: 8px; align-items: center; }
  .cfg { display: flex; flex-direction: column; gap: 10px; }
  .ell { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; max-width: 220px; }
  .warn { color: var(--warn, orange); }
  @media (max-width: 1100px) { .cols { grid-template-columns: 1fr; } }
</style>
