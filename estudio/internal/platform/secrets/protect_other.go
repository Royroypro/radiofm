//go:build !windows

package secrets

// Fuera de Windows no hay DPAPI: los secretos se guardan sin cifrar (solo para
// desarrollo y tests; el programa está pensado para Windows).
func protect(plain []byte) ([]byte, error) { return append([]byte(nil), plain...), nil }

func unprotect(enc []byte) ([]byte, error) { return append([]byte(nil), enc...), nil }
