<!--
  Forma de onda de lo que suena: envolvente de sonoridad (200 puntos en base 36,
  calculada en el análisis de la biblioteca), con lo ya sonado sombreado, la
  zona de intro, el punto de mezcla y marcas de tiempo cada 5 s.
-->
<script lang="ts">
  let {
    envelope = '',
    duration = 0, // ns, duración total del archivo
    cueIn = 0, // ns
    position = 0, // ns desde el cue-in
    intro = 0, // ns desde el inicio del archivo
    segue = 0, // ns desde el cue-in
  }: { envelope?: string; duration?: number; cueIn?: number; position?: number; intro?: number; segue?: number } = $props()

  let canvas: HTMLCanvasElement | undefined = $state()
  let w = $state(600)
  let h = $state(64)

  const NS = 1e9

  $effect(() => {
    if (!canvas) return
    const dpr = window.devicePixelRatio || 1
    canvas.width = w * dpr
    canvas.height = h * dpr
    const g = canvas.getContext('2d')!
    g.setTransform(dpr, 0, 0, dpr, 0, 0)
    g.clearRect(0, 0, w, h)
    const total = duration > 0 ? duration : 1
    const x = (ns: number) => Math.max(0, Math.min(w, (ns / total) * w))
    const played = x(cueIn + position)

    // intro
    if (intro > 0) {
      g.fillStyle = 'rgba(255,138,0,.18)'
      g.fillRect(0, 0, x(intro), h)
    }
    // barras de la envolvente (simétricas, como una forma de onda)
    const n = envelope.length || 1
    const bw = w / n
    for (let i = 0; i < envelope.length; i++) {
      const v = parseInt(envelope[i], 36) / 35
      const bh = Math.max(1, v * (h - 6))
      const bx = i * bw
      g.fillStyle = bx + bw <= played ? '#c81818' : bx < played ? '#e05050' : '#7d86b8'
      g.fillRect(bx, (h - bh) / 2, Math.max(1, bw - 0.6), bh)
    }
    if (!envelope) {
      g.fillStyle = '#26306a'
      g.fillRect(0, h / 2 - 1, w, 2)
      g.fillStyle = '#c81818'
      g.fillRect(0, h / 2 - 1, played, 2)
    }
    // marcas cada 5 s
    g.fillStyle = 'rgba(255,255,255,.55)'
    g.font = '10px Nunito, sans-serif'
    for (let t = 5; t * NS < total; t += 5) {
      const tx = x(t * NS)
      g.fillRect(tx, 0, 1, 5)
      if (t % 15 === 0) g.fillText(`${Math.floor(t / 60)}:${String(t % 60).padStart(2, '0')}`, tx + 2, 11)
    }
    // punto de mezcla y cabezal
    if (segue > 0) {
      g.fillStyle = '#FF8A00'
      g.fillRect(x(cueIn + segue), 0, 2, h)
    }
    g.fillStyle = '#fff'
    g.fillRect(played - 1, 0, 2, h)
  })
</script>

<div class="wave" bind:clientWidth={w} bind:clientHeight={h}>
  <canvas bind:this={canvas} style:width="{w}px" style:height="{h}px"></canvas>
</div>

<style>
  .wave { width: 100%; height: 100%; background: #0a102e; border-top: 1px solid #26306a; overflow: hidden; }
  canvas { display: block; }
</style>
