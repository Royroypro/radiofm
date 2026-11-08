// Tema activo: se carga de Ajustes al abrir, se aplica al instante al cambiarlo
// y se guarda en settings.json (ui.theme y ui.custom_*).
import { Settings, SaveSettings } from '../../wailsjs/go/ui/App'
import { config } from '../../wailsjs/go/models'
import { applyTheme, type CustomColors } from './theme'

class ThemeState {
  name = $state('dark')
  custom: CustomColors = $state({ base: 'dark', accent: '', bg: '', panel: '', sidebar: '', text: '' })

  async load() {
    const s = await Settings()
    this.name = s.ui.theme || 'dark'
    this.custom = {
      base: s.ui.custom_base || 'dark', accent: s.ui.custom_accent, bg: s.ui.custom_bg,
      panel: s.ui.custom_panel, sidebar: s.ui.custom_sidebar, text: s.ui.custom_text,
    }
    this.apply()
  }

  /** Aplica sin guardar (vista previa). */
  apply() {
    applyTheme(this.name, this.custom)
  }

  /** Aplica y guarda en Ajustes. */
  async save() {
    this.apply()
    const s = await Settings()
    s.ui.theme = this.name
    s.ui.custom_base = this.custom.base
    s.ui.custom_accent = this.custom.accent
    s.ui.custom_bg = this.custom.bg
    s.ui.custom_panel = this.custom.panel
    s.ui.custom_sidebar = this.custom.sidebar
    s.ui.custom_text = this.custom.text
    await SaveSettings(config.Settings.createFrom(s))
  }
}

export const theme = new ThemeState()
