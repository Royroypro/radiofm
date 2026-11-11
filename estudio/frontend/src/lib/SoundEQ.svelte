<!--
  Ecualizador gráfico de 10 bandas y ampliación estéreo del procesador de audio.
  Cada cambio se guarda y se aplica al aire al instante (SetSound), para ajustar
  escuchando. Va después del AGC y antes de los compresores (ver audio/processor.go).
-->
<script lang="ts">
  import { SetSound } from '../../wailsjs/go/ui/App'
  import { toast } from './toast.svelte'

  let { audio }: { audio: { eq: number[] | null; stereo_width: number } } = $props()

  const freqs = ['31', '62', '125', '250', '500', '1k', '2k', '4k', '8k', '16k']
  const presets: Record<string, { eq: number[]; width?: number }> = {
    Plano: { eq: [0, 0, 0, 0, 0, 0, 0, 0, 0, 0], width: 100 },
    'Más graves': { eq: [4, 4, 3, 1, 0, 0, 0, 0, 0, 0] },
    'Voz clara': { eq: [-2, -1, 0, -1, 0, 1, 3, 3, 1, 0] },
    Brillo: { eq: [0, 0, 0, 0, 0, 0, 1, 2, 3, 3] },
    'FM con cuerpo': { eq: [3, 3, 2, 0, -1, 0, 1, 2, 2, 1] },
  }

  let timer: ReturnType<typeof setTimeout> | undefined
  function apply() {
    clearTimeout(timer)
    timer = setTimeout(async () => {
      try {
        await SetSound(audio.eq!.map(Number), Number(audio.stereo_width))
      } catch (e) {
        toast.error(e)
      }
    }, 150)
  }

  function preset(name: string) {
    audio.eq = [...presets[name].eq]
    if (presets[name].width != null) audio.stereo_width = presets[name].width!
    apply()
  }

  const fmt = (v: number) => (v > 0 ? '+' : '') + Number(v).toFixed(0)
</script>

<div class="eq">
  <div class="bands">
    {#each audio.eq ?? [] as _, i}
      <label class="band" title="{freqs[i]} Hz">
        <span class="val">{fmt(audio.eq![i])}</span>
        <input type="range" min="-12" max="12" step="1" bind:value={audio.eq![i]} oninput={apply}
          ondblclick={() => { audio.eq![i] = 0; apply() }} />
        <span class="f">{freqs[i]}</span>
      </label>
    {/each}
  </div>
  <div class="side">
    <div class="presets">
      {#each Object.keys(presets) as p}<button class="small ghost" onclick={() => preset(p)}>{p}</button>{/each}
    </div>
    <label class="field">Ampliación estéreo: <strong>{audio.stereo_width} %</strong>
      <input type="range" min="0" max="200" step="5" bind:value={audio.stereo_width} oninput={apply}
        ondblclick={() => { audio.stereo_width = 100; apply() }} />
      <span class="muted small">0 % = mono · 100 % = normal · más de 100 % = sonido más abierto. Recomendado: 110–140 %.</span>
    </label>
    <p class="muted small">Se escucha al instante en el aire y en la web (se guarda solo). Doble clic en un control lo vuelve a 0.
      ±12 dB por banda; con ganancias altas el limitador trabaja más: mejor bajar lo que sobra que subir todo.</p>
  </div>
</div>

<style>
  .eq { display: flex; gap: 18px; flex-wrap: wrap; align-items: flex-start; }
  .bands { display: flex; gap: 6px; padding: 8px; border: 1px solid var(--line); border-radius: 8px; }
  .band { display: flex; flex-direction: column; align-items: center; gap: 4px; width: 34px; }
  .band input { writing-mode: vertical-lr; direction: rtl; height: 140px; width: 24px; }
  .val { font-size: 11px; font-variant-numeric: tabular-nums; }
  .f { font-size: 11px; color: var(--muted); }
  .side { display: flex; flex-direction: column; gap: 10px; flex: 1; min-width: 260px; }
  .presets { display: flex; gap: 6px; flex-wrap: wrap; }
</style>
