//go:build !windows

package updater

import (
	"os"
	"os/exec"
	"syscall"
	"time"
)

func detach(*exec.Cmd) {}

// WaitPID espera (hasta timeout) a que termine el proceso pid.
func WaitPID(pid int, timeout time.Duration) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		p, err := os.FindProcess(pid)
		if err != nil || p.Signal(syscall.Signal(0)) != nil {
			return
		}
		time.Sleep(200 * time.Millisecond)
	}
}
