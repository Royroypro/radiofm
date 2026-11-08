<!-- Vúmetro vertical: pico y RMS en dBFS (escala −60…0). -->
<script lang="ts">
  let { peak = 0, rms = 0, label = '' }: { peak: number; rms: number; label?: string } = $props()
  const pct = (v: number) => {
    if (v <= 0) return 0
    const db = 20 * Math.log10(v)
    return Math.max(0, Math.min(100, ((db + 60) / 60) * 100))
  }
</script>

<div class="meter" title="{label} {peak > 0 ? (20 * Math.log10(peak)).toFixed(1) : '-∞'} dBFS">
  <div class="bar rms" style:height="{pct(rms)}%"></div>
  <div class="bar peak" style:bottom="{pct(peak)}%"></div>
  <span>{label}</span>
</div>

<style>
  .meter { position: relative; width: 18px; height: 100%; background: #0d1640; border: 1px solid var(--line); border-radius: 3px; overflow: hidden; }
  .bar.rms { position: absolute; left: 0; right: 0; bottom: 0; background: linear-gradient(to top, #2ead5b, #e6b800 85%, #ff1e1e); z-index: 1; }
  .bar.peak { position: absolute; left: 0; right: 0; height: 2px; background: #fff; z-index: 2; }
  span { position: absolute; bottom: 2px; left: 0; right: 0; text-align: center; font-size: 10px; z-index: 3; }
</style>
