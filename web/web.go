// Package web contiene las plantillas HTML (templates/) y los archivos
// estáticos (static/) embebidos en el binario. En desarrollo se pueden leer
// del disco poniendo WEB_DIR=./web para no recompilar al editar HTML/CSS/JS.
package web

import "embed"

// FS contiene templates/ y static/.
//
//go:embed templates static
var FS embed.FS
