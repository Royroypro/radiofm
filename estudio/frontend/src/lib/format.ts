// Utilidades de formato compartidas por las pantallas.
// Las duraciones de Go (time.Duration) llegan en nanosegundos.

export const NS = 1_000_000_000

/** mm:ss (o h:mm:ss) a partir de nanosegundos. */
export function dur(ns: number | null | undefined): string {
  if (!ns || ns < 0) return '0:00'
  const total = Math.floor(ns / NS)
  const h = Math.floor(total / 3600)
  const m = Math.floor((total % 3600) / 60)
  const s = total % 60
  const ss = String(s).padStart(2, '0')
  return h > 0 ? `${h}:${String(m).padStart(2, '0')}:${ss}` : `${m}:${ss}`
}

/** Segundos con un decimal a partir de nanosegundos (para editar cues). */
export function secs(ns: number): number {
  return Math.round((ns / NS) * 10) / 10
}

/** Nanosegundos a partir de segundos. */
export function fromSecs(s: number): number {
  return Math.round((Number(s) || 0) * NS)
}

/** Fecha ISO (RFC3339) → valor para <input type=date> (YYYY-MM-DD) en hora local. */
export function dateInput(iso: string | null | undefined): string {
  if (!iso) return ''
  const d = new Date(iso)
  const p = (n: number) => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())}`
}

/** <input type=date> → RFC3339 a medianoche local, o null. */
export function fromDateInput(v: string): string | null {
  if (!v) return null
  return new Date(v + 'T00:00:00').toISOString()
}

/** dB con signo y un decimal. */
export function db(v: number): string {
  return (v > 0 ? '+' : '') + v.toFixed(1) + ' dB'
}

/** Mensaje legible de un error de Wails (la promesa se rechaza con un string). */
export function errMsg(e: unknown): string {
  const s = typeof e === 'string' ? e : (e as Error)?.message ?? String(e)
  // los errores de validación de Go llegan como "campo: mensaje": se muestra solo el mensaje
  return s.replace(/^[a-z_]+: /, '')
}
