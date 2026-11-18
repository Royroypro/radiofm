//go:build windows

package updater

import (
	"os/exec"
	"syscall"
	"time"

	"golang.org/x/sys/windows"
)

// detach abre el proceso nuevo en su propio grupo, sin consola.
func detach(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: windows.CREATE_NEW_PROCESS_GROUP | windows.DETACHED_PROCESS,
	}
}

// WaitPID espera (hasta timeout) a que termine el proceso pid. Si no existe, vuelve en seguida.
func WaitPID(pid int, timeout time.Duration) {
	h, err := windows.OpenProcess(windows.SYNCHRONIZE, false, uint32(pid))
	if err != nil {
		return // ya terminó
	}
	defer windows.CloseHandle(h)
	_, _ = windows.WaitForSingleObject(h, uint32(timeout/time.Millisecond))
}
