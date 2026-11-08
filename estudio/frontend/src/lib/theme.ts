// Temas de color. Cada tema es un juego completo de variables CSS que usan todas
// las pantallas (style.css y los componentes nunca deben fijar colores a mano:
// usan estas variables). "custom" parte de un tema base y cambia algunos colores.

export type ThemeVars = Record<string, string>

const common = {
  '--red': '#e5212b',
  '--orange': '#ff8a00',
  '--green': '#22a05a',
  '--meter-low': '#22a05a',
  '--meter-mid': '#e6b800',
  '--meter-high': '#e5212b',
}

export const themes: Record<string, { label: string; dark: boolean; vars: ThemeVars }> = {
  dark: {
    label: 'Oscuro',
    dark: true,
    vars: {
      ...common,
      '--bg': '#111316',
      '--panel': '#1a1d21',
      '--panel-2': '#25292f',
      '--line': '#30353d',
      '--line-soft': '#23272d',
      '--text': '#eceff3',
      '--muted': '#9aa1ab',
      '--accent': '#d9333d',
      '--accent-text': '#ffffff',
      '--sel': '#33404f',
      '--current': '#3a1f22',
      '--sidebar': '#16181b',
      '--sidebar-text': '#eceff3',
      '--input-bg': '#121417',
      '--wave-bg': '#0e1013',
      '--wave-bar': '#6b7480',
      '--wave-played': '#d9333d',
      '--countdown': '#3ee06b',
      '--countdown-dim': '#9fdcb2',
      '--hl-spot': '#33231a',
      '--hl-jingle': '#22272e',
      '--problem': '#8b1a1a',
      '--problem-text': '#ffb3b3',
      '--ok-btn': '#1f8a3f',
    },
  },
  light: {
    label: 'Claro',
    dark: false,
    vars: {
      ...common,
      '--bg': '#eceef1',
      '--panel': '#ffffff',
      '--panel-2': '#e4e7ec',
      '--line': '#cfd4db',
      '--line-soft': '#e8ebef',
      '--text': '#1b1f24',
      '--muted': '#5f6772',
      '--accent': '#d0232d',
      '--accent-text': '#ffffff',
      '--sel': '#d6e2f5',
      '--current': '#fde3e3',
      '--sidebar': '#ffffff',
      '--sidebar-text': '#1b1f24',
      '--input-bg': '#f7f8fa',
      '--wave-bg': '#f2f4f7',
      '--wave-bar': '#9aa3ae',
      '--wave-played': '#d0232d',
      '--countdown': '#138a3d',
      '--countdown-dim': '#3f8f5a',
      '--hl-spot': '#fff1e0',
      '--hl-jingle': '#eef1f5',
      '--problem': '#c62828',
      '--problem-text': '#b00020',
      '--ok-btn': '#1f8a3f',
    },
  },
  club25: {
    label: 'Azul Club25',
    dark: true,
    vars: {
      ...common,
      '--red': '#ff1e1e',
      '--bg': '#0b1233',
      '--panel': '#131c47',
      '--panel-2': '#1a255a',
      '--line': '#26306a',
      '--line-soft': '#1c2554',
      '--text': '#eef1ff',
      '--muted': '#9aa3d0',
      '--accent': '#223cb8',
      '--accent-text': '#ffffff',
      '--sel': '#24307a',
      '--current': '#2a1430',
      '--sidebar': '#0a1a6b',
      '--sidebar-text': '#eef1ff',
      '--input-bg': '#0d1640',
      '--wave-bg': '#0a102e',
      '--wave-bar': '#7d86b8',
      '--wave-played': '#c81818',
      '--countdown': '#3ee06b',
      '--countdown-dim': '#a7f3bd',
      '--hl-spot': '#2a1430',
      '--hl-jingle': '#16224f',
      '--problem': '#8b1a1a',
      '--problem-text': '#ffb3b3',
      '--ok-btn': '#1f8a3f',
    },
  },
}

export type CustomColors = { base: string; accent: string; bg: string; panel: string; sidebar: string; text: string }

/** Colores que se pueden personalizar y su variable. */
export const customFields: { key: keyof CustomColors; label: string; vars: string[] }[] = [
  { key: 'accent', label: 'Color principal (botones, selección de pestañas)', vars: ['--accent', '--wave-played'] },
  { key: 'bg', label: 'Fondo', vars: ['--bg', '--input-bg', '--wave-bg'] },
  { key: 'panel', label: 'Paneles', vars: ['--panel'] },
  { key: 'sidebar', label: 'Menú lateral', vars: ['--sidebar'] },
  { key: 'text', label: 'Texto', vars: ['--text', '--sidebar-text'] },
]

/** Variables de un tema (con los cambios del personalizado). */
export function themeVars(name: string, custom?: Partial<CustomColors>): { vars: ThemeVars; dark: boolean } {
  if (name !== 'custom') {
    const t = themes[name] ?? themes.dark
    return { vars: t.vars, dark: t.dark }
  }
  const base = themes[custom?.base ?? 'dark'] ?? themes.dark
  const vars = { ...base.vars }
  for (const f of customFields) {
    const v = custom?.[f.key]
    if (v) for (const name of f.vars) vars[name] = v
  }
  return { vars, dark: base.dark }
}

/** Aplica un tema a toda la ventana. */
export function applyTheme(name: string, custom?: Partial<CustomColors>) {
  const { vars, dark } = themeVars(name, custom)
  const root = document.documentElement
  for (const [k, v] of Object.entries(vars)) root.style.setProperty(k, v)
  // compatibilidad con nombres viejos
  root.style.setProperty('--royal', vars['--accent'])
  root.style.setProperty('--navy', vars['--sidebar'])
  root.style.colorScheme = dark ? 'dark' : 'light'
  root.dataset.theme = name
}
