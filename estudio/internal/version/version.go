// Package version expone la versión del programa. Se fija al compilar con
//
//	-ldflags "-X radioclub25/estudio/internal/version.Version=0.1.0"
//
// (lo hace Taskfile.yml). Sin ldflags vale "dev".
package version

// Version es la versión semántica de Club25 Estudio.
var Version = "dev"

// Name es el nombre del producto.
const Name = "Club25 Estudio"
