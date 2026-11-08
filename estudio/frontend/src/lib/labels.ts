// Textos de la interfaz para valores del dominio.

export const slotLabel: Record<string, string> = {
  category: 'Categoría',
  track: 'Pista fija',
  spots: 'Tanda de cuñas',
  time: 'Hora locutada',
  temp: 'Temperatura',
  mode: 'Cambio de modo',
  silence: 'Silencio',
}

export const slotIcon: Record<string, string> = {
  category: '♪',
  track: '♫',
  spots: '$',
  time: '🕒',
  temp: '🌡',
  mode: '⇄',
  silence: '…',
}

export const modeLabel: Record<string, string> = { auto: 'Automático', assisted: 'Asistido', live: 'En vivo' }

export const statusLabel: Record<string, string> = {
  pending: 'Pendiente',
  playing: 'Sonando',
  played: 'Sonó',
  skipped: 'Saltado',
}

export const weekdays = ['Dom', 'Lun', 'Mar', 'Mié', 'Jue', 'Vie', 'Sáb']

/** Hora local HH:MM:SS de una fecha ISO. */
export function clock(iso: string | any): string {
  const d = new Date(iso)
  return d.toLocaleTimeString('es-PE', { hour12: false })
}
