<!-- Ajustes: dispositivos, mezcla, Icecast, panel web, emisora, rotación, locuciones y seguridad. -->
<script lang="ts">
  import { onMount } from 'svelte'
  import { Settings, SaveSettings, Devices, PickFolder, CheckLocutions, CurrentTemperature, SetIcecastPassword, RestartAudio, RestartStreaming } from '../../wailsjs/go/ui/App'
  import { config, type audio, type ui } from '../../wailsjs/go/models'
  import { toast } from '../lib/toast.svelte'

  let { info }: { info: ui.AppInfo | null } = $props()
  let s: config.Settings | null = $state(null)
  let devices: audio.Device[] = $state([])
  let devErr = $state('')
  let dirty = $state(false)

  const outputs = $derived(devices.filter((d) => d.kind === 'playback'))
  const inputs = $derived(devices.filter((d) => d.kind === 'capture'))

  onMount(async () => {
    s = await Settings()
    try {
      devices = (await Devices()) ?? []
    } catch (e) {
      devErr = String(e)
    }
  })

  async function save() {
    if (!s) return
    try {
      await SaveSettings(config.Settings.createFrom(s))
      dirty = false
      toast.show('Ajustes guardados. Algunos cambios de audio se aplican al reiniciar el motor.')
    } catch (e) {
      toast.error(e)
    }
  }

  let icePass = $state('')

  async function savePassword() {
    try {
      await SetIcecastPassword(icePass)
      icePass = ''
      toast.show('Contraseña guardada (cifrada) y emisión reiniciada')
    } catch (e) {
      toast.error(e)
    }
  }

  async function applyAudio() {
    await save()
    const st = await RestartAudio()
    toast.show(st.error ? st.error : `Audio aplicado: ${st.output}${st.mic ? ' · mic ' + st.mic : ''}`, !!st.error)
  }

  async function applyStream() {
    await save()
    await RestartStreaming()
    toast.show('Emisión reiniciada con los ajustes guardados')
  }

  async function checkLocutions() {
    const missing = await CheckLocutions()
    toast.show(missing.length ? `Faltan ${missing.length} locuciones (p. ej. ${missing.slice(0, 4).join(', ')})` : 'Están todas las locuciones', missing.length > 0)
  }

  async function testTemperature() {
    try {
      toast.show(`Temperatura actual: ${(await CurrentTemperature()).toFixed(1)} °C`)
    } catch (e) {
      toast.error(e)
    }
  }

  async function pickLocutions() {
    const d = await PickFolder('Carpeta de locuciones (HRS/MIN/TMP)')
    if (d && s) {
      s.locutions.dir = d
      dirty = true
    }
  }
</script>

{#if s}
  <div class="settings scroll" oninput={() => (dirty = true)} onchange={() => (dirty = true)}>
    <div class="row top">
      <h1 class="grow">Ajustes</h1>
      <button class="ok" disabled={!dirty} onclick={save}>Guardar cambios</button>
    </div>

    <section class="panel">
      <h2>Audio</h2>
      {#if devErr}<p class="error small">{devErr}</p>{/if}
      <div class="grid3">
        <label class="field">Salida principal (consola / FM)
          <select bind:value={s.audio.output_device}><option value="">(predeterminada de Windows)</option>
            {#each outputs as d (d.id)}<option value={d.name}>{d.name}</option>{/each}</select></label>
        <label class="field">Pre-escucha
          <select bind:value={s.audio.preview_device}><option value="">(sin pre-escucha)</option>
            {#each outputs as d (d.id)}<option value={d.name}>{d.name}</option>{/each}</select></label>
        <label class="field">Micrófono / entrada en vivo
          <select bind:value={s.audio.mic_device}><option value="">(sin micrófono)</option>
            {#each inputs as d (d.id)}<option value={d.name}>{d.name}</option>{/each}</select></label>
        <label class="field">Crossfade (ms)<input type="number" min="0" max="15000" step="100" bind:value={s.audio.crossfade_ms} /></label>
        <label class="field">Fundido de evento duro (ms)<input type="number" min="0" max="10000" step="100" bind:value={s.audio.hard_fade_ms} /></label>
        <div class="row" style="grid-column: 1 / -1"><span class="grow muted small">El ducking se aplica al reiniciar el programa; dispositivos y ganancia, con el botón.</span>
          <button onclick={applyAudio}>Guardar y aplicar audio</button></div>
        <label class="field">Ganancia del mic (dB)<input type="number" min="-20" max="30" step="0.5" bind:value={s.audio.mic_gain_db} /></label>
        <label class="field">Ducking (dB)<input type="number" min="-40" max="0" step="1" bind:value={s.audio.duck_depth_db} /></label>
        <label class="field">Umbral de voz (dBFS)<input type="number" min="-70" max="0" step="1" bind:value={s.audio.duck_threshold_db} /></label>
        <div class="grid2">
          <label class="field">Ataque (ms)<input type="number" min="1" max="2000" bind:value={s.audio.duck_attack_ms} /></label>
          <label class="field">Relajación (ms)<input type="number" min="10" max="5000" bind:value={s.audio.duck_release_ms} /></label>
        </div>
      </div>
    </section>

    <section class="panel">
      <h2>Emisión a la web (Icecast)</h2>
      <label class="check"><input type="checkbox" bind:checked={s.icecast.enabled} /> Emitir a Icecast</label>
      <div class="grid3">
        <label class="field">Servidor<input bind:value={s.icecast.host} /></label>
        <label class="field">Puerto<input type="number" bind:value={s.icecast.port} /></label>
        <label class="field">Punto de montaje<input bind:value={s.icecast.mount} /></label>
        <label class="field">Usuario<input bind:value={s.icecast.user} /></label>
        <label class="field">Formato<select bind:value={s.icecast.format}><option value="mp3">MP3</option><option value="aac">AAC</option></select></label>
        <label class="field">kbps<input type="number" min="32" max="320" bind:value={s.icecast.bitrate_kbps} /></label>
        <label class="field">Fuente del audio
          <select bind:value={s.icecast.source}>
            <option value="bus">Mezcla de la PC (mic en la PC)</option>
            <option value="capture">Entrada de línea (salida de la consola)</option></select></label>
        {#if s.icecast.source === 'capture'}
          <label class="field">Entrada de línea
            <select bind:value={s.icecast.capture_device}><option value="">(predeterminada)</option>
              {#each inputs as d (d.id)}<option value={d.name}>{d.name}</option>{/each}</select></label>
        {/if}
        <label class="check"><input type="checkbox" bind:checked={s.icecast.metadata_music_only} /> Enviar como título solo la música (no cuñas ni jingles)</label>
      </div>
      <div class="row" style="margin-top:8px">
        <input type="password" placeholder="Contraseña de fuente (se guarda cifrada en esta PC)" bind:value={icePass} style="width:340px" />
        <button class="ghost" disabled={!icePass} onclick={savePassword}>Guardar contraseña</button>
        <span class="grow"></span>
        <button onclick={applyStream}>Guardar y reiniciar emisión</button>
      </div>
    </section>

    <section class="panel grid2">
      <div>
        <h2>Panel web</h2>
        <label class="field">URL del sitio<input bind:value={s.server.url} /></label>
      </div>
      <div>
        <h2>Emisora</h2>
        <div class="grid2">
          <label class="field">Nombre<input bind:value={s.station.name} /></label>
          <label class="field">Ciudad (temperatura)<input bind:value={s.station.city} /></label>
          <label class="field">Latitud<input type="number" step="0.0001" bind:value={s.station.latitude} /></label>
          <label class="field">Longitud<input type="number" step="0.0001" bind:value={s.station.longitude} /></label>
        </div>
      </div>
    </section>

    <section class="panel grid3">
      <div>
        <h2>Rotación</h2>
        <label class="field">Separación de artista (min)<input type="number" min="0" bind:value={s.rotation.artist_separation_min} /></label>
        <label class="field">Separación de título (h)<input type="number" min="0" bind:value={s.rotation.title_separation_hours} /></label>
      </div>
      <div>
        <h2>Locuciones</h2>
        <div class="row"><input class="grow" readonly value={s.locutions.dir} placeholder="Carpeta HRS/MIN/TMP" />
          <button class="ghost small" onclick={pickLocutions}>…</button></div>
        <div class="grid3">
          <label class="field">Hora<input bind:value={s.locutions.hour_prefix} /></label>
          <label class="field">Minutos<input bind:value={s.locutions.minute_prefix} /></label>
          <label class="field">Temp.<input bind:value={s.locutions.temp_prefix} /></label>
        </div>
        <div class="row" style="margin-top:6px">
          <button class="small ghost" onclick={checkLocutions}>Verificar archivos</button>
          <button class="small ghost" onclick={testTemperature}>Probar temperatura</button>
        </div>
      </div>
      <div>
        <h2>Seguridad</h2>
        <label class="field">Silencio antes de emergencia (s, 0 = no)<input type="number" min="0" bind:value={s.safety.silence_seconds} /></label>
        <label class="check"><input type="checkbox" bind:checked={s.safety.start_playing} /> Empezar a sonar al abrir</label>
        <label class="check"><input type="checkbox" bind:checked={s.safety.auto_start} /> Arrancar con Windows</label>
      </div>
    </section>

    <section class="panel">
      <h2>Sistema</h2>
      <p class="small">Carpeta de datos: <code>{info?.data_dir}</code></p>
      <p class="small">ffmpeg: <code class:error={!info?.ffmpeg_ok}>{info?.ffmpeg}</code></p>
    </section>
  </div>
{/if}

<style>
  .settings { display: flex; flex-direction: column; gap: 12px; height: 100%; padding-right: 6px; }
  .top { position: sticky; top: 0; background: var(--bg); padding-bottom: 6px; z-index: 2; }
  section h2 { margin-bottom: 8px; }
  code { user-select: text; }
</style>
