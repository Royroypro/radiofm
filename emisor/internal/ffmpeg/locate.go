package ffmpeg

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
)

// Locate busca ffmpeg.exe en este orden:
//  1. la ruta configurada (si no está vacía),
//  2. junto al programa (exeDir\ffmpeg.exe o exeDir\ffmpeg\bin\ffmpeg.exe), que es como se empaqueta,
//  3. el PATH de Windows.
func Locate(configured, exeDir string) (string, error) {
	if configured != "" {
		if isFile(configured) {
			return configured, nil
		}
		return "", errors.New("No se encontró ffmpeg en la ruta configurada: " + configured)
	}
	for _, p := range []string{
		filepath.Join(exeDir, exeName),
		filepath.Join(exeDir, "ffmpeg", "bin", exeName),
		filepath.Join(exeDir, "ffmpeg", exeName),
	} {
		if exeDir != "" && isFile(p) {
			return p, nil
		}
	}
	if p, err := exec.LookPath("ffmpeg"); err == nil {
		return p, nil
	}
	return "", errors.New("No se encontró ffmpeg. Póngalo junto al programa (ffmpeg.exe) o instálelo (ver README).")
}

func isFile(p string) bool {
	st, err := os.Stat(p)
	return err == nil && !st.IsDir()
}
