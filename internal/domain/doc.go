// Package domain contiene las entidades del negocio (usuarios, anuncios,
// contratos, pagos, licencia, ajustes, programación) y las interfaces de los
// repositorios que las persisten.
//
// Reglas de este paquete:
//   - Es Go puro: NO importa paquetes de base de datos, HTTP ni de otros
//     paquetes internos. Cualquier capa puede importar domain; domain no
//     importa a nadie.
//   - Las interfaces de repositorio se definen aquí y se implementan en
//     internal/repository/sqlite.
//   - La lógica que solo depende de los datos de una entidad (por ejemplo,
//     si un anuncio está vigente en una fecha) puede vivir aquí como método.
//     Los casos de uso que combinan varias entidades van en internal/service.
package domain
