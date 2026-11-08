// Pantalla activa (compartida: el menú lateral y el botón "Menú" de Al aire la cambian).
class Nav {
  page = $state('onair')
  menuOpen = $state(false)

  go(page: string) {
    this.page = page
    this.menuOpen = false
  }
}

export const nav = new Nav()
