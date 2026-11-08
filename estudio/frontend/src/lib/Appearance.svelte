<!-- Apariencia: elegir tema (oscuro, claro, azul Club25 o personalizado) con vista previa al instante. -->
<script lang="ts">
  import { themes, themeVars, customFields } from './theme'
  import { theme } from './themeState.svelte'
  import { toast } from './toast.svelte'

  const options = [...Object.entries(themes).map(([id, t]) => ({ id, label: t.label })), { id: 'custom', label: 'Personalizado' }]

  function pick(id: string) {
    theme.name = id
    if (id === 'custom') {
      // partir de los colores del tema base para que los selectores muestren algo
      const v = themeVars(theme.custom.base).vars
      theme.custom.accent ||= v['--accent']
      theme.custom.bg ||= v['--bg']
      theme.custom.panel ||= v['--panel']
      theme.custom.sidebar ||= v['--sidebar']
      theme.custom.text ||= v['--text']
    }
    theme.apply()
  }

  async function save() {
    try {
      await theme.save()
      toast.show('Tema guardado')
    } catch (e) {
      toast.error(e)
    }
  }

  function swatch(id: string) {
    const v = themeVars(id, theme.custom).vars
    return `background:${v['--bg']};border-color:${v['--line']}`
  }
</script>

<div class="themes">
  {#each options as o}
    <button class="opt" class:active={theme.name === o.id} onclick={() => pick(o.id)}>
      <span class="sw" style={swatch(o.id)}>
        <i style:background={themeVars(o.id, theme.custom).vars['--sidebar']}></i>
        <i style:background={themeVars(o.id, theme.custom).vars['--panel']}></i>
        <i style:background={themeVars(o.id, theme.custom).vars['--accent']}></i>
      </span>
      {o.label}
    </button>
  {/each}
</div>

{#if theme.name === 'custom'}
  <div class="grid3 custom">
    <label class="field">Basado en
      <select bind:value={theme.custom.base} onchange={() => theme.apply()}>
        {#each Object.entries(themes) as [id, t]}<option value={id}>{t.label}</option>{/each}
      </select></label>
    {#each customFields as f}
      <label class="field">{f.label}<input type="color" bind:value={theme.custom[f.key]} oninput={() => theme.apply()} /></label>
    {/each}
  </div>
{/if}
<div class="row" style="margin-top:8px"><button class="primary" onclick={save}>Guardar tema</button>
  <span class="small muted">El cambio se ve al instante; «Guardar tema» lo conserva al reabrir el programa.</span></div>

<style>
  .themes { display: flex; gap: 10px; flex-wrap: wrap; }
  .opt { display: flex; flex-direction: column; align-items: center; gap: 6px; padding: 8px 12px; min-width: 120px; }
  .sw { display: flex; gap: 3px; padding: 6px; border: 1px solid; border-radius: 6px; width: 90px; height: 40px; }
  .sw i { flex: 1; border-radius: 3px; }
  .custom { margin-top: 10px; }
  .custom input[type=color] { height: 32px; padding: 2px; }
</style>
