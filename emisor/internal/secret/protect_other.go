//go:build !windows

package secret

// Fuera de Windows no hay DPAPI: solo para desarrollo y tests.
func protect(plain []byte) ([]byte, error) { return append([]byte(nil), plain...), nil }

func unprotect(enc []byte) ([]byte, error) { return append([]byte(nil), enc...), nil }
