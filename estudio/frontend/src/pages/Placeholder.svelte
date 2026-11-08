<!-- Pantalla aún no construida: indica en qué fase del plan llega. -->
<script lang="ts">
  let { title, phase }: { title: string; phase: string } = $props()
</script>

<div class="panel">
  <h1>{title}</h1>
  <p class="muted">Esta pantalla llega en la {phase} del plan (<code>docs/estudio/plan.md</code>).</p>
</div>
