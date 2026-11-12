package studio

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"

	"radioclub25/internal/domain"
)

// BackupKeep es cuántas copias de la base del Estudio se guardan por frecuencia.
const BackupKeep = 14

// MaxBackupBytes es el tamaño máximo de una copia (comprimida).
const MaxBackupBytes = 100 << 20 // el nginx del servidor acepta hasta 110 MB

var backupName = regexp.MustCompile(`^estudio-\d{8}\.db$`)

// SaveBackup guarda la copia diaria de la base de la PC del estudio (gzip) en
// <dir>/<frecuencia>/<nombre>.gz y borra las más viejas (quedan BackupKeep).
// Si se corta a mitad, no queda nada a medias (se escribe a un .part).
func SaveBackup(dir string, fid int64, name string, body io.Reader) error {
	if !backupName.MatchString(name) {
		return domain.Invalid("name", "Nombre de copia inválido")
	}
	fdir := filepath.Join(dir, strconv.FormatInt(fid, 10))
	if err := os.MkdirAll(fdir, 0o750); err != nil {
		return err
	}
	dst := filepath.Join(fdir, name+".gz")
	tmp := dst + ".part"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	n, err := io.Copy(f, io.LimitReader(body, MaxBackupBytes+1))
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err == nil && n > MaxBackupBytes {
		err = domain.Invalid("size", fmt.Sprintf("La copia pasa de %d MB", MaxBackupBytes>>20))
	}
	if err == nil && n < 20 {
		err = domain.Invalid("size", "La copia llegó vacía")
	}
	if err != nil {
		_ = os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, dst); err != nil {
		return err
	}
	matches, _ := filepath.Glob(filepath.Join(fdir, "estudio-*.db.gz"))
	sort.Strings(matches)
	for len(matches) > BackupKeep {
		_ = os.Remove(matches[0])
		matches = matches[1:]
	}
	return nil
}

// Backups lista las copias guardadas de una frecuencia (más nuevas primero).
func Backups(dir string, fid int64) []string {
	matches, _ := filepath.Glob(filepath.Join(dir, strconv.FormatInt(fid, 10), "estudio-*.db.gz"))
	sort.Sort(sort.Reverse(sort.StringSlice(matches)))
	out := make([]string, len(matches))
	for i, m := range matches {
		out[i] = filepath.Base(m)
	}
	return out
}
