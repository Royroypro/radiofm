// Estado compartido del aire: se consulta a Go varias veces por segundo y lo
// leen la pantalla Al aire, la cabecera y los atajos de teclado.
import { OnAirStatus, TriggerCart, OnAirNext, SetMic } from '../../wailsjs/go/ui/App'
import type { ui } from '../../wailsjs/go/models'
import { toast } from './toast.svelte'

class OnAirState {
  view: ui.OnAirView | null = $state(null)
  cartPage = $state(0)
  mic = $state(false)
  private timer: ReturnType<typeof setInterval> | undefined

  start() {
    if (this.timer) return
    const poll = async () => {
      try {
        this.view = await OnAirStatus()
        this.mic = this.view.air.engine.mic_on
      } catch {
        /* la ventana se está cerrando */
      }
    }
    poll()
    this.timer = setInterval(poll, 150)
  }

  async cart(slot: number) {
    try {
      await TriggerCart(this.cartPage, slot)
    } catch (e) {
      toast.error(e)
    }
  }

  async toggleMic() {
    this.mic = !this.mic
    await SetMic(this.mic)
  }

  async next() {
    try {
      await OnAirNext()
    } catch (e) {
      toast.error(e)
    }
  }
}

export const onair = new OnAirState()

/** Atajos globales: Espacio = siguiente, F1–F12 = botones 1–12, Ctrl+M = micrófono. */
export function handleKey(e: KeyboardEvent) {
  const t = e.target as HTMLElement
  const typing = t && (t.tagName === 'INPUT' || t.tagName === 'TEXTAREA' || t.tagName === 'SELECT')
  if (/^F([1-9]|1[0-2])$/.test(e.key)) {
    e.preventDefault()
    onair.cart(Number(e.key.slice(1)) - 1)
    return
  }
  if (typing) return
  if (e.code === 'Space') {
    e.preventDefault()
    onair.next()
  } else if (e.ctrlKey && e.key.toLowerCase() === 'm') {
    e.preventDefault()
    onair.toggleMic()
  } else if (/^Numpad[0-9]$/.test(e.code)) {
    // teclado numérico: 1–9 = botones 13–21, 0 = botón 22
    const n = Number(e.code.slice(6))
    onair.cart(n === 0 ? 21 : 11 + n)
  }
}
