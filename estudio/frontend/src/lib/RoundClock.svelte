<!-- Reloj de estudio: día, hora grande, fecha y anillo de 60 puntos que marca los segundos. -->
<script lang="ts">
  let { now }: { now: Date } = $props()
  const dots = Array.from({ length: 60 }, (_, i) => i)
  const sec = $derived(now.getSeconds())
</script>

<div class="clock">
  <svg viewBox="0 0 200 200">
    {#each dots as i}
      {@const a = (i / 60) * Math.PI * 2 - Math.PI / 2}
      <circle cx={100 + Math.cos(a) * 90} cy={100 + Math.sin(a) * 90} r={i % 5 === 0 ? 3.2 : 2.2}
        fill={i <= sec ? '#FF1E1E' : '#26306a'} />
    {/each}
  </svg>
  <div class="txt">
    <div class="day">{now.toLocaleDateString('es-PE', { weekday: 'long' })}</div>
    <div class="time">{now.toLocaleTimeString('es-PE', { hour12: false })}</div>
    <div class="date">{now.toLocaleDateString('es-PE', { day: 'numeric', month: 'long', year: 'numeric' })}</div>
  </div>
</div>

<style>
  .clock { position: relative; width: 100%; aspect-ratio: 1; max-height: 100%; }
  svg { position: absolute; inset: 0; width: 100%; height: 100%; }
  .txt { position: absolute; inset: 0; display: flex; flex-direction: column; align-items: center; justify-content: center; }
  .day, .date { font-size: 11px; color: var(--muted); }
  .time { font-size: 24px; font-weight: 700; font-variant-numeric: tabular-nums; letter-spacing: .5px; }
</style>
