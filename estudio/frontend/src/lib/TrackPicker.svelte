<!-- Diálogo para buscar y elegir una pista de la biblioteca. onpick recibe la pista elegida. -->
<script lang="ts">
  import { onMount } from 'svelte'
  import { Tracks } from '../../wailsjs/go/ui/App'
  import { domain } from '../../wailsjs/go/models'
  import { dur } from './format'

  let { title = 'Elegir pista', onpick, oncancel }: {
    title?: string
    onpick: (t: domain.Track) => void
    oncancel: () => void
  } = $props()

  let query = $state('')
  let items: domain.Track[] = $state([])
  let timer: ReturnType<typeof setTimeout>

  async function search() {
    const page = await Tracks(domain.TrackFilter.createFrom({ Query: query, OnlyActive: true, Limit: 100 }))
    items = page.items
  }

  function onInput() {
    clearTimeout(timer)
    timer = setTimeout(search, 200)
  }

  onMount(search)
</script>

<div class="backdrop">
  <div class="modal picker">
    <h2>{title}</h2>
    <!-- svelte-ignore a11y_autofocus -->
    <input autofocus style="width:100%" placeholder="Buscar…" bind:value={query} oninput={onInput} />
    <div class="scroll list">
      <table>
        <tbody>
          {#each items as t (t.id)}
            <tr onclick={() => onpick(t)}><td>{t.artist}</td><td>{t.title}</td><td class="num">{dur(t.duration)}</td></tr>
          {/each}
        </tbody>
      </table>
      {#if !items.length}<p class="muted">Sin resultados.</p>{/if}
    </div>
    <div class="actions"><button class="ghost" onclick={oncancel}>Cancelar</button></div>
  </div>
</div>

<style>
  .picker { width: 620px; }
  .list { height: 360px; margin-top: 10px; }
</style>
