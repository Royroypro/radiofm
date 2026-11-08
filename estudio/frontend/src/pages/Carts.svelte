<!-- Botonera: configurar los 24 botones de cada página (texto, color, audio, modo). -->
<script lang="ts">
  import { onMount } from 'svelte'
  import { Carts, SaveCart, PickFile } from '../../wailsjs/go/ui/App'
  import { domain } from '../../wailsjs/go/models'
  import { toast } from '../lib/toast.svelte'
  import { onair } from '../lib/onair.svelte'

  let page = $state(0)
  let carts: domain.Cart[] = $state([])
  let edit: any = $state(null)

  async function load() {
    carts = await Carts(page)
  }

  function open(slot: number) {
    const c = carts.find((x) => x.slot === slot)
    edit = c ? { ...c } : { page, slot, label: '', color: '#223CB8', path: '', stop_on_push: true }
  }

  async function pick() {
    const p = await PickFile('Audio del botón (máx. 30 s)')
    if (p) {
      edit.path = p
      if (!edit.label) edit.label = p.split(/[\\/]/).pop()!.replace(/\.[^.]+$/, '').slice(0, 24)
    }
  }

  async function save(clear = false) {
    try {
      await SaveCart(domain.Cart.createFrom({ ...edit, path: clear ? '' : edit.path }))
      edit = null
      await load()
    } catch (e) {
      toast.error(e)
    }
  }

  onMount(load)
</script>

<div class="page">
  <div class="row">
    <h1 class="grow">Botonera</h1>
    {#each Array(10) as _, p}<button class="small" class:ghost={p !== page} onclick={() => { page = p; load() }}>{p + 1}</button>{/each}
  </div>
  <p class="muted small">Haga clic en un botón para configurarlo. Al aire: F1–F12 disparan los botones 1–12 de la página activa,
    el teclado numérico 1–9 y 0 los botones 13–22. Varios pueden sonar a la vez.</p>
  <div class="grid">
    {#each Array(24) as _, i}
      {@const c = carts.find((x) => x.slot === i)}
      <button class="cart" style:background={c ? c.color : 'transparent'} onclick={() => open(i)}>
        <span class="k">{i < 12 ? 'F' + (i + 1) : i < 21 ? 'Num ' + (i - 11) : i === 21 ? 'Num 0' : ''}</span>
        {c?.label ?? '+'}
        {#if c}<span class="small mode">{c.stop_on_push ? 'para al pulsar' : 'reinicia'}</span>{/if}
      </button>
    {/each}
  </div>
  <button class="ghost" style="align-self:start" onclick={() => (onair.cartPage = page)}>Usar esta página al aire</button>
</div>

{#if edit}
  <div class="backdrop">
    <div class="modal">
      <h2>Botón {edit.slot + 1} · página {edit.page + 1}</h2>
      <div class="row"><input class="grow" readonly value={edit.path} placeholder="Archivo de audio" /><button class="ghost" onclick={pick}>Elegir…</button></div>
      <div class="grid2" style="margin-top:10px">
        <label class="field">Texto<input maxlength="24" bind:value={edit.label} /></label>
        <label class="field">Color<input type="color" bind:value={edit.color} /></label>
      </div>
      <label class="check" style="margin-top:8px"><input type="checkbox" bind:checked={edit.stop_on_push} /> Pulsar de nuevo lo detiene (si no, vuelve a empezar)</label>
      <div class="actions">
        <button class="danger" onclick={() => save(true)}>Vaciar</button>
        <span class="grow"></span>
        <button class="ghost" onclick={() => (edit = null)}>Cancelar</button>
        <button class="ok" disabled={!edit.path} onclick={() => save()}>Guardar</button>
      </div>
    </div>
  </div>
{/if}

<style>
  .page { display: flex; flex-direction: column; gap: 10px; height: 100%; }
  .grid { display: grid; grid-template-columns: repeat(6, 1fr); gap: 10px; }
  .cart { height: 90px; position: relative; border: 1px solid var(--line); font-size: 15px; }
  .cart .k { position: absolute; top: 4px; left: 6px; font-size: 10px; opacity: .8; }
  .cart .mode { position: absolute; bottom: 4px; right: 6px; opacity: .7; }
</style>
