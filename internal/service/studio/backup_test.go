package studio

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSaveBackupKeepsLast(t *testing.T) {
	dir := t.TempDir()
	body := strings.Repeat("x", 100)
	for d := 1; d <= BackupKeep+3; d++ {
		if err := SaveBackup(dir, 2, fmt.Sprintf("estudio-202611%02d.db", d), strings.NewReader(body)); err != nil {
			t.Fatal(err)
		}
	}
	list := Backups(dir, 2)
	if len(list) != BackupKeep || list[0] != fmt.Sprintf("estudio-202611%02d.db.gz", BackupKeep+3) {
		t.Fatalf("deben quedar las %d más nuevas (la primera, la última): %v", BackupKeep, list)
	}
	for _, bad := range []string{"../x.db", "estudio-2026.db", "otra-20261101.db", ""} {
		if err := SaveBackup(dir, 2, bad, strings.NewReader(body)); err == nil {
			t.Fatalf("nombre %q debe rechazarse", bad)
		}
	}
	if err := SaveBackup(dir, 2, "estudio-20261231.db", strings.NewReader("")); err == nil {
		t.Fatal("una copia vacía debe rechazarse")
	}
	if _, err := os.Stat(filepath.Join(dir, "2", "estudio-20261231.db.gz.part")); err == nil {
		t.Fatal("no debe quedar un archivo a medias")
	}
}
