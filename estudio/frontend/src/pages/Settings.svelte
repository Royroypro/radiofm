<!-- Ajustes: dispositivos, mezcla, Icecast, panel web, emisora, rotación, locuciones y seguridad. -->
<script lang="ts">
  import { onMount } from 'svelte'
  import { Settings, SaveSettings, Devices, PickFolder, CheckLocutions, CurrentTemperature, SetIcecastPassword, RestartAudio, RestartStreaming, SetServerConnection, SyncStatus, ToolsStatus, DownloadTools } from '../../wailsjs/go/ui/App'
  import { EventsOn } from '../../wailsjs/runtime/runtime'
  import { config, type audio, type ui } from '../../wailsjs/go/models'
  import { toast } from '../lib/toast.svelte'
  import Appearance from '../lib/Appearance.svelte'

  let { info }: { info: ui.AppInfo | null } = $props()
  let s: config.Settings | null = $state(null)
  let devices: audio.Device[] = $state([])
  let devErr = $state('')
  let dirty = $state(false)

  const outputs = $derived(devices.filter((d) => d.kind === 'playback'))
  const inputs = $derived(devices.filter((d) => d.kind === 'capture'))

  onMount(async () => {
    s = await Settings()
    sync = await SyncStatus()
    checks = await ToolsStatus()
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
  let checks: any[] = $state([])
  let dl: { name: string; done: number; total: number } | null = $state(null)

  async function downloadTools() {
    dl = { name: 'ffmpeg', done: 0, total: 0 }
    const off = EventsOn('tools:progress', (p: any) => (dl = p))
    try {
      const got = await DownloadTools()
      toast.show(got ? 'Herramientas descargadas: reinicie el programa para usarlas' : 'No faltaba nada')
      checks = await ToolsStatus()
    } catch (e) {
      toast.error(e)
    } finally {
      off()
      dl = null
    }
  }
  let token = $state('')
  let sync: any = $state(null)

  async function saveServer() {
    try {
      await SetServerConnection(s!.server.url, token)
      token = ''
      dirty = false
      toast.show('Conexión guardada; sincronizando…')
      setTimeout(async () => (sync = await SyncStatus()), 3000)
    } catch (e) {
      toast.error(e)
    }
  }

  async function savePassword() {
    try {
      await SetIcecastPassword(icePass)
      icePass = ''
      toast.show('Contraseña guardada (cifrada) y emisión reiniciada')
    } catch (e) {
      toast.error(e)
    }
  }

  function addRecording() {
    if (!s) return
    s.logger.recordings = [...(s.logger.recordings ?? []), { name: '', start: '06:00', minutes: 120, days: 62 } as any]
  }
  function removeRecording(i: number) {
    if (s) s.logger.recordings = s.logger.recordings.filter((_: any, k: number) => k !== i)
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
      <h2>Apariencia</h2>
      <Appearance />
    </section>

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
        <label class="field" title="Iguala el nivel entre canciones (AGC) y comprime en 3 bandas antes del limitador">Procesador de audio
          <select bind:value={s.audio.proc_preset}>
            <option value="natural">Natural (suave)</option><option value="fm">FM comercial</option>
            <option value="voice">Hablado</option><option value="off">Sin procesar</option></select></label>
        <label class="field" title="Si el transmisor FM ya tiene su propio procesador, elija «solo stream»">Aplicar a
          <select bind:value={s.audio.proc_scope}>
            <option value="both">FM y stream</option><option value="stream">Solo stream (el transmisor procesa la FM)</option></select></label>
        <label class="field" title="La música se funde en este tiempo y la tanda entra al terminar, sin pisarse">Fundido al entrar una tanda (ms)<input type="number" min="0" max="5000" step="100" bind:value={s.audio.break_fade_ms} /></label>
        <p class="muted small" style="grid-column: 1 / -1; margin: 0">Transiciones automáticas: entre canciones se respeta cómo termina cada una (con fundido se cruza sobre el fundido; con final seco no se corta el golpe). Los jingles entran sobre el final de la canción y la música entra bajo la cola del jingle.</p>
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
        <h2>Panel web (sincronización)</h2>
        <label class="field">URL del sitio<input bind:value={s.server.url} /></label>
        <label class="field" style="margin-top:6px">Token de Admin → Estudio (PC)
          <input type="password" placeholder={sync?.configured ? '(guardado; escriba otro para cambiarlo)' : 'c25s_…'} bind:value={token} /></label>
        <div class="row" style="margin-top:6px">
          <button onclick={saveServer}>Guardar y sincronizar</button>
          {#if sync}
            <span class="pill" class:ok={sync.online} class:bad={!sync.online && sync.configured}>
              {sync.online ? 'conectado' : sync.suspended ? 'servicio suspendido' : sync.unauthorized ? 'token inválido' : sync.configured ? 'sin conexión' : 'sin configurar'}</span>
            <span class="small muted">{sync.spots} cuñas · {sync.queue} emisiones en cola</span>
          {/if}
        </div>
        {#if sync?.last_error}<p class="error small">{sync.last_error}</p>{/if}
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
        <p class="muted small" style="margin:6px 0 2px">Flujo (usa los atributos de la Biblioteca; 0 = no aplicar). Si ninguna canción cumple, se relajan de abajo hacia arriba.</p>
        <label class="check"><input type="checkbox" bind:checked={s.rotation.no_same_voice} /> No dos voces femeninas (o masculinas) seguidas</label>
        <label class="field">Salto máx. de energía (1–4)<input type="number" min="0" max="4" bind:value={s.rotation.max_energy_jump} /></label>
        <label class="field">Salto máx. de tempo (%)<input type="number" min="0" max="200" step="5" bind:value={s.rotation.max_tempo_jump_pct} /></label>
        <label class="field">Máx. seguidas del mismo género<input type="number" min="0" max="20" bind:value={s.rotation.max_genre_run} /></label>
        <label class="field">Máx. seguidas de la misma década<input type="number" min="0" max="20" bind:value={s.rotation.max_era_run} /></label>
        <h2 style="margin-top:10px">Tandas</h2>
        <label class="field" title="La PC no pone más cuñas que esto en una tanda; lo que no entra se repone en otra">Máx. cuñas por tanda (0 = sin tope)<input type="number" min="0" max="40" bind:value={s.schedule.break_max_spots} /></label>
        <label class="field" title="Duración máxima de las cuñas de una tanda, sin contar cortinas">Máx. segundos por tanda (0 = sin tope)<input type="number" min="0" max="1800" step="10" bind:value={s.schedule.break_max_seconds} /></label>
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

    <section class="panel grid2">
      <div>
        <h2>Grabador de la emisión</h2>
        <label class="check"><input type="checkbox" bind:checked={s.logger.enabled} /> Grabar todo lo que sale al aire (archivo por hora)</label>
        <label class="field">Carpeta (vacío = carpeta de datos\grabaciones)<input bind:value={s.logger.dir} /></label>
        <div class="grid3">
          <label class="field" title="Al pasarse se borra lo más viejo">Máx. MB (0 = sin límite)<input type="number" min="0" step="256" bind:value={s.logger.max_mb} /></label>
          <label class="field" title="El reglamento pide al menos 30 días">Máx. días (0 = sin límite)<input type="number" min="0" bind:value={s.logger.max_days} /></label>
          <label class="field">Calidad (kbps, mono)<input type="number" min="32" max="192" step="16" bind:value={s.logger.kbps} /></label>
        </div>
        <h3 style="margin:8px 0 4px">Grabaciones programadas</h3>
        <p class="muted small" style="margin:0 0 4px">Al terminar la franja se recorta de la grabación y se guarda en «programas».</p>
        {#each s.logger.recordings ?? [] as r, i}
          <div class="row" style="gap:4px;margin-bottom:4px">
            <input style="flex:1" placeholder="Nombre del programa" bind:value={r.name} />
            <input type="time" bind:value={r.start} />
            <input type="number" min="1" max="720" style="width:70px" bind:value={r.minutes} title="Minutos" />
            <select bind:value={r.days} title="Días">
              <option value={0}>Todos</option><option value={62}>Lun–Vie</option><option value={65}>Sáb y Dom</option>
              {#each ['Dom', 'Lun', 'Mar', 'Mié', 'Jue', 'Vie', 'Sáb'] as d, k}<option value={1 << k}>Solo {d}</option>{/each}
            </select>
            <button class="small danger" onclick={() => removeRecording(i)}>✕</button>
          </div>
        {/each}
        <button class="small ghost" onclick={addRecording}>+ Grabación programada</button>
      </div>
      <div>
        <h2>RDS (pantalla del radio)</h2>
        <label class="check"><input type="checkbox" bind:checked={s.rds.enabled} /> Enviar nombre y canción al codificador RDS</label>
        <div class="grid3">
          <label class="field">Conexión<select bind:value={s.rds.transport}><option value="tcp">TCP</option><option value="udp">UDP</option><option value="serial">Puerto serie</option></select></label>
          <label class="field">{s.rds.transport === 'serial' ? 'Puerto (COM3)' : 'IP:puerto'}<input bind:value={s.rds.address} placeholder={s.rds.transport === 'serial' ? 'COM3' : '192.168.1.50:7000'} /></label>
          {#if s.rds.transport === 'serial'}<label class="field">Baudios<input type="number" bind:value={s.rds.baud} /></label>{/if}
        </div>
        <div class="grid3">
          <label class="field" title="8 caracteres">Nombre (PS)<input maxlength="8" bind:value={s.rds.ps} /></label>
          <label class="field" title="{'{artist}'}, {'{title}'}, {'{station}'}">Texto<input bind:value={s.rds.rt_format} /></label>
          <label class="field">Mín. segundos entre envíos<input type="number" min="0" bind:value={s.rds.min_seconds} /></label>
        </div>
        <div class="grid2">
          <label class="field" title="{'{ps}'} = nombre; \r\n = fin de línea">Comando PS<input bind:value={s.rds.ps_template} /></label>
          <label class="field" title="{'{text}'} = texto; \r\n = fin de línea">Comando RadioText<input bind:value={s.rds.rt_template} /></label>
        </div>
        <label class="check"><input type="checkbox" bind:checked={s.rds.skip_spots} /> En cuñas, jingles y locuciones mostrar el nombre de la radio</label>
        <p class="muted small">Cada codificador usa su sintaxis: revise el manual del equipo y ajuste los comandos (por defecto «PS=…» y «RT=…»). Sin tildes: se quitan solas.</p>
      </div>
    </section>

    <section class="panel">
      <h2>Sistema</h2>
      <p class="small">Carpeta de datos: <code>{info?.data_dir}</code></p>
      <p class="small">ffmpeg: <code class:error={!info?.ffmpeg_ok}>{info?.ffmpeg}</code></p>
      {#each checks as c}
        <div class="small"><span class="pill" class:ok={c.ok} class:bad={!c.ok}>{c.ok ? 'OK' : 'FALTA'}</span> {c.name}: <span class="muted">{c.detail}</span></div>
      {/each}
      {#if sync && Math.abs(sync.clock_offset) > 5}
        <p class="error small">El reloj de esta PC está {sync.clock_offset > 0 ? 'adelantado' : 'atrasado'} {Math.abs(sync.clock_offset).toFixed(0)} s respecto al servidor:
          la hora locutada y los eventos duros saldrán corridos. Sincronice la hora de Windows.</p>
      {/if}
      <div class="row" style="margin-top:6px">
        <button class="ghost small" disabled={!!dl} onclick={downloadTools}>Revisar y descargar herramientas del servidor</button>
        {#if dl}<progress max={dl.total || 1} value={dl.done}></progress><span class="small muted">{dl.name} {(dl.done / 1048576).toFixed(0)} / {(dl.total / 1048576).toFixed(0)} MB</span>{/if}
      </div>
    </section>
  </div>
{/if}

<style>
  .settings { display: flex; flex-direction: column; gap: 12px; height: 100%; padding-right: 6px; }
  .top { position: sticky; top: 0; background: var(--bg); padding-bottom: 6px; z-index: 2; }
  section h2 { margin-bottom: 8px; }
  code { user-select: text; }
</style>
