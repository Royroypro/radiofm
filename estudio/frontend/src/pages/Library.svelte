<!--
  Biblioteca: categorías (izquierda), búsqueda e importación (arriba), lista de
  pistas (centro) y editor de la pista seleccionada (derecha).
-->
<script lang="ts">
  import { onMount } from 'svelte'
  import {
    Categories, CategoryKinds, SaveCategory, DeleteCategory, Tracks, UpdateTrack, DeleteTrack,
    ReanalyzeTrack, ResetTrackMarks, SetTrackAttributes, ImportFolder, PickFolder, AnalyzerStatus, CheckMissing,
  } from '../../wailsjs/go/ui/App'
  import { EventsOn } from '../../wailsjs/runtime/runtime'
  import { domain, type ui, type library } from '../../wailsjs/go/models'
  import { dur, secs, fromSecs, dateInput, fromDateInput, db } from '../lib/format'
  import { toast } from '../lib/toast.svelte'
  import CueEditor from '../lib/CueEditor.svelte'

  const PAGE = 200
  const kindLabel: Record<string, string> = {
    music: 'Música', jingle: 'Jingles/IDs', spot: 'Cuñas', voice: 'Locuciones', emergency: 'Emergencia', filler: 'Relleno',
  }

  let cats: ui.CategoryView[] = $state([])
  let kinds: string[] = $state([])
  let catFilter = $state(0)
  let query = $state('')
  let tracks: domain.Track[] = $state([])
  let total = $state(0)
  let offset = $state(0)
  let sel: domain.Track | null = $state(null)
  let edit: any = $state(null) // copia editable de sel (segundos en lugar de ns)
  let analyzer: library.AnalyzerStatus | null = $state(null)

  // diálogos
  let catDlg: any = $state(null)
  let importDlg: { dir: string; cat: number; recursive: boolean; running: boolean; done: number; total: number; file: string } | null = $state(null)

  const catName = (id: number) => cats.find((c) => c.id === id)?.name ?? '—'
  const catColor = (id: number) => cats.find((c) => c.id === id)?.color ?? '#555'

  async function loadCats() {
    cats = (await Categories()) ?? []
  }

  async function search(reset = true) {
    if (reset) offset = 0
    try {
      const page = await Tracks(domain.TrackFilter.createFrom({ CategoryID: catFilter, Query: query, Limit: PAGE, Offset: offset }))
      tracks = page.items
      total = page.total
    } catch (e) {
      toast.error(e)
    }
  }

  let searchTimer: ReturnType<typeof setTimeout>
  function onQuery() {
    clearTimeout(searchTimer)
    searchTimer = setTimeout(() => search(), 250)
  }

  function select(t: domain.Track) {
    sel = t
    edit = {
      ...t,
      cue_in_s: secs(t.cue_in), cue_out_s: secs(t.cue_out), segue_s: secs(t.segue_at), intro_s: secs(t.intro),
      from: dateInput(t.valid_from as any), to: dateInput(t.valid_to as any),
    }
  }

  async function saveTrack() {
    if (!edit) return
    const t = domain.Track.createFrom({
      ...edit,
      cue_in: fromSecs(edit.cue_in_s), cue_out: fromSecs(edit.cue_out_s), segue_at: fromSecs(edit.segue_s),
      intro: fromSecs(edit.intro_s), valid_from: fromDateInput(edit.from), valid_to: fromDateInput(edit.to),
      category_id: Number(edit.category_id), gain_db: Number(edit.gain_db),
      bpm: Number(edit.bpm) || 0, energy: Number(edit.energy) || 0,
    })
    try {
      await UpdateTrack(t)
      toast.show('Pista guardada')
      await Promise.all([search(false), loadCats()])
      const again = tracks.find((x) => x.id === t.id)
      if (again) select(again)
    } catch (e) {
      toast.error(e)
    }
  }

  async function reanalyze() {
    if (!sel) return
    try {
      select(await ReanalyzeTrack(sel.id))
      await search(false)
      toast.show('Pista analizada')
    } catch (e) {
      toast.error(e)
    }
  }

  // --- atributos musicales ---
  const voices: Record<string, string> = { '': '—', male: 'Masculina', female: 'Femenina', group: 'Grupo / dúo', instrumental: 'Instrumental' }
  let bulk: { genre: string; year: string; energy: number; voice: string; language: string; mood: string } | null = $state(null)
  async function applyBulk() {
    if (!bulk) return
    if (!confirm(`¿Aplicar estos atributos a las ${total} pistas del filtro actual?`)) return
    try {
      const n = await SetTrackAttributes(domain.TrackFilter.createFrom({ CategoryID: catFilter, Query: query }),
        domain.TrackAttributes.createFrom({ ...bulk, energy: Number(bulk.energy) || 0 }))
      bulk = null
      toast.show(`${n} pistas actualizadas`)
      await search(false)
    } catch (e) {
      toast.error(e)
    }
  }

  async function resetMarks() {
    if (!sel || !confirm('¿Descartar las marcas puestas a mano y volver a las automáticas?')) return
    try {
      select(await ResetTrackMarks(sel.id))
      await search(false)
      toast.show('Marcas automáticas restablecidas')
    } catch (e) {
      toast.error(e)
    }
  }

  const others = $derived(tracks.filter((t) => t.id !== sel?.id).map((t) => ({ id: t.id, label: t.artist ? `${t.artist} - ${t.title}` : t.title || t.path })))

  async function removeTrack() {
    if (!sel || !confirm(`¿Quitar «${sel.title || sel.path}» de la biblioteca? (el archivo no se borra)`)) return
    try {
      await DeleteTrack(sel.id)
      sel = edit = null
      await Promise.all([search(false), loadCats()])
    } catch (e) {
      toast.error(e)
    }
  }

  async function saveCat() {
    try {
      await SaveCategory(domain.Category.createFrom({ ...catDlg, parent_id: Number(catDlg.parent_id) }))
      catDlg = null
      await loadCats()
    } catch (e) {
      toast.error(e)
    }
  }

  async function removeCat() {
    if (!catDlg?.id || !confirm(`¿Borrar la categoría «${catDlg.name}»?`)) return
    try {
      await DeleteCategory(catDlg.id)
      catDlg = null
      if (catFilter) catFilter = 0
      await loadCats()
      await search()
    } catch (e) {
      toast.error(e)
    }
  }

  async function pickImportFolder() {
    const dir = await PickFolder('Carpeta de música a importar')
    if (dir && importDlg) importDlg.dir = dir
  }

  async function runImport() {
    if (!importDlg?.dir) return
    importDlg.running = true
    try {
      const r = await ImportFolder(importDlg.dir, Number(importDlg.cat), importDlg.recursive)
      toast.show(`Importadas ${r.added} nuevas, ${r.updated} actualizadas` + (r.failed ? `, ${r.failed} con error` : ''), r.failed > 0)
      importDlg = null
      await Promise.all([search(), loadCats()])
    } catch (e) {
      toast.error(e)
      if (importDlg) importDlg.running = false
    }
  }

  async function checkMissing() {
    try {
      const n = await CheckMissing()
      toast.show(n ? `${n} pistas sin archivo` : 'Todos los archivos están en su lugar', n > 0)
      await search(false)
    } catch (e) {
      toast.error(e)
    }
  }

  function status(t: domain.Track): [string, string] {
    if (t.missing) return ['Sin archivo', 'bad']
    if (!t.active) return ['Inactiva', 'warn']
    if (t.analyze_err) return ['Error', 'bad']
    if (!t.analyzed) return ['Analizando…', '']
    return ['Lista', 'ok']
  }

  onMount(() => {
    loadCats()
    CategoryKinds().then((k) => (kinds = k))
    search()
    const off = EventsOn('library:import', (p: { done: number; total: number; file: string }) => {
      if (importDlg) Object.assign(importDlg, { done: p.done, total: p.total, file: p.file })
    })
    const poll = setInterval(async () => {
      analyzer = await AnalyzerStatus()
      if (analyzer.pending > 0) search(false)
    }, 3000)
    return () => {
      off()
      clearInterval(poll)
    }
  })
</script>

<div class="lib">
  <section class="panel cats">
    <div class="row"><h2 class="grow">Categorías</h2>
      <button class="small" onclick={() => (catDlg = { id: 0, name: '', kind: 'music', color: '#223CB8', parent_id: 0 })}>+</button></div>
    <button class="cat" class:active={catFilter === 0} onclick={() => { catFilter = 0; search() }}>
      Todas <span class="muted">{cats.reduce((a, c) => a + c.tracks, 0)}</span></button>
    {#each cats as c (c.id)}
      <div class="catrow">
        <button class="cat" class:active={catFilter === c.id} onclick={() => { catFilter = c.id; search() }}>
          <span class="dot" style:background={c.color}></span>{c.parent_id ? '↳ ' : ''}{c.name}
          <span class="muted">{c.tracks}</span>
        </button>
        <button class="small ghost" title="Editar" onclick={() => (catDlg = { ...c })}>✎</button>
      </div>
    {/each}
  </section>

  <section class="panel list">
    <div class="row">
      <input class="grow" placeholder="Buscar por título, artista, álbum o archivo…" bind:value={query} oninput={onQuery} />
      <button onclick={() => (importDlg = { dir: '', cat: catFilter || cats[0]?.id || 0, recursive: true, running: false, done: 0, total: 0, file: '' })}>Importar carpeta…</button>
      <button class="ghost" onclick={checkMissing}>Verificar archivos</button>
      <button class="ghost" title="Género, año, energía, voz, idioma o ánimo para todas las pistas que se ven (categoría + búsqueda)"
        onclick={() => (bulk = { genre: '', year: '', energy: 0, voice: '', language: '', mood: '' })}>Atributos en lote…</button>
    </div>
    <p class="muted small">
      {total} pistas{#if analyzer && (analyzer.pending > 0 || analyzer.current)} · analizando: {analyzer.current} ({analyzer.pending} en cola){/if}
    </p>
    <div class="scroll grow">
      <table>
        <thead><tr><th>Artista</th><th>Título</th><th>Categoría</th><th class="num">Duración</th><th class="num">Loudness</th><th class="num">Ganancia</th><th>Estado</th></tr></thead>
        <tbody>
          {#each tracks as t (t.id)}
            {@const st = status(t)}
            <tr class:sel={sel?.id === t.id} onclick={() => select(t)}>
              <td>{t.artist}</td><td>{t.title}</td>
              <td><span class="dot" style:background={catColor(t.category_id)}></span>{catName(t.category_id)}</td>
              <td class="num">{dur(t.duration)}</td>
              <td class="num">{t.analyzed && !t.analyze_err ? t.loudness.toFixed(1) + ' LUFS' : '—'}</td>
              <td class="num">{t.analyzed ? db(t.gain_db) : '—'}</td>
              <td><span class="pill {st[1]}">{st[0]}</span></td>
            </tr>
          {/each}
        </tbody>
      </table>
      {#if !tracks.length}<p class="muted">No hay pistas. Use «Importar carpeta…».</p>{/if}
    </div>
    {#if total > PAGE}
      <div class="row">
        <button class="small ghost" disabled={offset === 0} onclick={() => { offset = Math.max(0, offset - PAGE); search(false) }}>‹ Anterior</button>
        <span class="muted small">{offset + 1}–{Math.min(offset + PAGE, total)} de {total}</span>
        <button class="small ghost" disabled={offset + PAGE >= total} onclick={() => { offset += PAGE; search(false) }}>Siguiente ›</button>
      </div>
    {/if}
  </section>

  <section class="panel editor">
    {#if edit}
      <h2>Pista</h2>
      <p class="small muted path" title={edit.path}>{edit.path}</p>
      <label class="field">Título<input bind:value={edit.title} /></label>
      <label class="field">Artista<input bind:value={edit.artist} /></label>
      <label class="field">Categoría
        <select bind:value={edit.category_id}>{#each cats as c (c.id)}<option value={c.id}>{c.name}</option>{/each}</select></label>
      <label class="check"><input type="checkbox" bind:checked={edit.active} /> Activa (se programa)</label>
      <h2 style="margin-top:8px">Atributos (reglas de flujo)</h2>
      <div class="grid3">
        <label class="field" title="Estimado al analizar; corríjalo si salió el doble o la mitad">BPM<input type="number" min="0" max="300" step="0.1" bind:value={edit.bpm} /></label>
        <label class="field" title="1 calma … 5 muy movida (estimada; la que ponga a mano se respeta)">Energía
          <select bind:value={edit.energy}><option value={0}>—</option>{#each [1, 2, 3, 4, 5] as e}<option value={e}>{e}</option>{/each}</select></label>
        <label class="field">Voz<select bind:value={edit.voice}>{#each Object.entries(voices) as [k, v]}<option value={k}>{v}</option>{/each}</select></label>
        <label class="field">Género<input bind:value={edit.genre} /></label>
        <label class="field">Año<input bind:value={edit.year} maxlength="4" /></label>
        <label class="field">Idioma<input bind:value={edit.language} placeholder="es" maxlength="10" /></label>
      </div>
      <label class="field">Ánimo<input bind:value={edit.mood} placeholder="alegre, romántica, nostálgica…" maxlength="40" /></label>
      <div class="grid2">
        <label class="field">Vigente desde<input type="date" bind:value={edit.from} /></label>
        <label class="field">Hasta<input type="date" bind:value={edit.to} /></label>
      </div>
      <h2 style="margin-top:12px">Puntos (segundos)
        {#if edit.marks_manual}<span class="badge" title="El análisis no los cambia">a mano</span>{/if}
        {#if edit.ending}<span class="badge" title="Decide cómo entra la siguiente">{edit.ending === 'fade' ? `final con fundido (${secs(edit.fade_out)} s)` : 'final seco'}</span>{/if}
      </h2>
      {#key edit.id}
        <CueEditor trackId={edit.id} envelope={edit.envelope} duration={secs(edit.duration)} {others}
          bind:cueIn={edit.cue_in_s} bind:intro={edit.intro_s} bind:segue={edit.segue_s} bind:cueOut={edit.cue_out_s} />
      {/key}
      <div class="grid2">
        <label class="field">Cue-in<input type="number" step="0.1" min="0" bind:value={edit.cue_in_s} /></label>
        <label class="field">Cue-out (0 = final)<input type="number" step="0.1" min="0" bind:value={edit.cue_out_s} /></label>
        <label class="field">Mezcla (0 = auto)<input type="number" step="0.1" min="0" bind:value={edit.segue_s} /></label>
        <label class="field">Intro<input type="number" step="0.1" min="0" bind:value={edit.intro_s} /></label>
        <label class="field">Ganancia (dB)<input type="number" step="0.1" bind:value={edit.gain_db} /></label>
        <div class="field small muted">Duración {dur(edit.duration)}<br />Sonó {edit.play_count} veces</div>
      </div>
      {#if edit.analyze_err}<p class="error small">{edit.analyze_err}</p>{/if}
      <div class="row" style="margin-top:12px">
        <button class="ok" onclick={saveTrack}>Guardar</button>
        <button class="ghost" onclick={reanalyze}>Reanalizar</button>
        {#if edit.marks_manual}<button class="ghost" onclick={resetMarks} title="Descarta las marcas puestas a mano">Marcas automáticas</button>{/if}
        <button class="danger" onclick={removeTrack}>Quitar</button>
      </div>
    {:else}
      <p class="muted">Seleccione una pista para editarla.</p>
    {/if}
  </section>
</div>

{#if bulk}
  <div class="backdrop">
    <div class="modal">
      <h2>Atributos en lote</h2>
      <p class="muted small">Se aplican a las {total} pistas del filtro actual (categoría y búsqueda). Los campos vacíos no se cambian.</p>
      <div class="grid3">
        <label class="field">Género<input bind:value={bulk.genre} /></label>
        <label class="field">Año<input bind:value={bulk.year} maxlength="4" /></label>
        <label class="field">Energía<select bind:value={bulk.energy}><option value={0}>(no cambiar)</option>{#each [1, 2, 3, 4, 5] as e}<option value={e}>{e}</option>{/each}</select></label>
        <label class="field">Voz<select bind:value={bulk.voice}><option value="">(no cambiar)</option>{#each Object.entries(voices).filter(([k]) => k) as [k, v]}<option value={k}>{v}</option>{/each}</select></label>
        <label class="field">Idioma<input bind:value={bulk.language} placeholder="es" maxlength="10" /></label>
        <label class="field">Ánimo<input bind:value={bulk.mood} maxlength="40" /></label>
      </div>
      <div class="actions"><span class="grow"></span>
        <button class="ghost" onclick={() => (bulk = null)}>Cancelar</button>
        <button class="ok" onclick={applyBulk}>Aplicar</button></div>
    </div>
  </div>
{/if}

{#if catDlg}
  <div class="backdrop">
    <div class="modal">
      <h2>{catDlg.id ? 'Editar categoría' : 'Nueva categoría'}</h2>
      <div class="grid2">
        <label class="field">Nombre<input bind:value={catDlg.name} /></label>
        <label class="field">Tipo<select bind:value={catDlg.kind}>{#each kinds as k}<option value={k}>{kindLabel[k] ?? k}</option>{/each}</select></label>
        <label class="field">Dentro de<select bind:value={catDlg.parent_id}><option value={0}>(ninguna)</option>
          {#each cats.filter((c) => c.id !== catDlg.id && !c.parent_id) as c (c.id)}<option value={c.id}>{c.name}</option>{/each}</select></label>
        <label class="field">Color<input type="color" bind:value={catDlg.color} /></label>
      </div>
      <div class="actions">
        {#if catDlg.id}<button class="danger" onclick={removeCat}>Borrar</button>{/if}
        <span class="grow"></span>
        <button class="ghost" onclick={() => (catDlg = null)}>Cancelar</button>
        <button class="ok" onclick={saveCat}>Guardar</button>
      </div>
    </div>
  </div>
{/if}

{#if importDlg}
  <div class="backdrop">
    <div class="modal">
      <h2>Importar carpeta</h2>
      <div class="row"><input class="grow" readonly value={importDlg.dir} placeholder="Elija una carpeta" />
        <button class="ghost" disabled={importDlg.running} onclick={pickImportFolder}>Examinar…</button></div>
      <div class="grid2" style="margin-top:10px">
        <label class="field">Categoría<select bind:value={importDlg.cat} disabled={importDlg.running}>
          {#each cats as c (c.id)}<option value={c.id}>{c.name}</option>{/each}</select></label>
        <label class="check" style="align-self:end"><input type="checkbox" bind:checked={importDlg.recursive} disabled={importDlg.running} /> Incluir subcarpetas</label>
      </div>
      {#if importDlg.running}
        <p class="small">Leyendo {importDlg.done} de {importDlg.total || '…'} · {importDlg.file}</p>
        <progress max={importDlg.total || 1} value={importDlg.done} style="width:100%"></progress>
      {/if}
      <p class="muted small">Las pistas se analizan después en segundo plano (loudness, cue-in/out y punto de mezcla).</p>
      <div class="actions">
        <button class="ghost" disabled={importDlg.running} onclick={() => (importDlg = null)}>Cancelar</button>
        <button class="ok" disabled={!importDlg.dir || importDlg.running} onclick={runImport}>Importar</button>
      </div>
    </div>
  </div>
{/if}

<style>
  .lib { display: grid; grid-template-columns: 230px 1fr 300px; gap: 12px; height: 100%; min-height: 0; }
  .cats, .editor { overflow: auto; }
  .list { display: flex; flex-direction: column; gap: 8px; min-height: 0; min-width: 0; }
  .cat { display: flex; align-items: center; gap: 4px; width: 100%; background: transparent; text-align: left; padding: 6px 8px; }
  .cat .muted { margin-left: auto; }
  .cat { color: var(--text); border-color: transparent; }
  .cat.active { background: var(--accent); color: var(--accent-text); }
  .catrow { display: flex; gap: 2px; align-items: center; }
  .editor { display: flex; flex-direction: column; gap: 8px; }
  .badge { font-size: 10px; font-weight: 700; padding: 1px 6px; border-radius: 8px; background: var(--panel-2); border: 1px solid var(--line); margin-left: 6px; vertical-align: middle; }
  .path { word-break: break-all; margin: 0; }
</style>
