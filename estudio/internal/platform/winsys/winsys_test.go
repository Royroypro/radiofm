package winsys

import "testing"

func TestSingleInstance(t *testing.T) {
	release, err := SingleInstance("Club25EstudioTest")
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	if _, err := SingleInstance("Club25EstudioTest"); err != ErrAlreadyRunning {
		// fuera de Windows SingleInstance no bloquea
		if err == nil && !isWindows {
			return
		}
		t.Fatalf("la segunda instancia debe rechazarse: %v", err)
	}
}
