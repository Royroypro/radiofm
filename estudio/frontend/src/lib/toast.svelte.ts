// Aviso flotante compartido: toast.show('Guardado') / toast.error(err).
import { errMsg } from './format'

class Toast {
  text = $state('')
  isError = $state(false)
  private timer: ReturnType<typeof setTimeout> | undefined

  show(text: string, isError = false) {
    this.text = text
    this.isError = isError
    clearTimeout(this.timer)
    this.timer = setTimeout(() => (this.text = ''), isError ? 6000 : 3000)
  }

  error(e: unknown) {
    this.show(errMsg(e), true)
  }
}

export const toast = new Toast()
