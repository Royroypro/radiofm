// Package secret cifra las claves (fuente de Icecast, clave de la TV, token del
// panel) antes de escribirlas en el archivo de configuración.
//
// En Windows usa DPAPI ligado al usuario: el archivo copiado a otra PC u otro
// usuario no se puede descifrar. Fuera de Windows (solo desarrollo y tests) se
// guardan en base64 sin cifrar. No guarda nada por sí mismo: solo transforma.
package secret

import (
	"encoding/base64"
	"strings"
)

// prefix marca un valor cifrado dentro del JSON. Un valor sin prefijo se toma
// como texto plano (por ejemplo, escrito a mano en emisor.json): se acepta y se
// cifra en el próximo guardado.
const prefix = "enc:"

// Seal cifra plain y devuelve el texto para guardar. "" queda "".
func Seal(plain string) (string, error) {
	if plain == "" {
		return "", nil
	}
	enc, err := protect([]byte(plain))
	if err != nil {
		return "", err
	}
	return prefix + base64.StdEncoding.EncodeToString(enc), nil
}

// Open descifra un valor guardado con Seal. Un valor sin prefijo se devuelve tal cual.
func Open(stored string) (string, error) {
	if !strings.HasPrefix(stored, prefix) {
		return stored, nil
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(stored, prefix))
	if err != nil {
		return "", err
	}
	plain, err := unprotect(raw)
	if err != nil {
		return "", err
	}
	return string(plain), nil
}
