//go:build bindings

package main

// generatingBindings es true cuando Wails compila el programa solo para generar
// los bindings de TypeScript (wails build / wails generate). En ese modo el
// programa no debe abrir la base, el audio ni comprobar la instancia única: si
// Club25 Estudio ya está abierto en la PC, el aviso modal colgaba la compilación.
const generatingBindings = true
