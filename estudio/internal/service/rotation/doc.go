// Package rotation elige la próxima pista de una categoría: la menos reciente
// que respete la separación de artista y de título, sin repetir intérprete en
// slots seguidos. Si ninguna cumple, relaja las reglas en orden (artista →
// título → cualquiera) y lo informa.
//
// Lógica pura y determinista (recibe el historial y la hora): fase 4.
package rotation
