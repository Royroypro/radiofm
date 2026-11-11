//go:build windows

package updater

import (
	"os/exec"
	"syscall"
)

const (
	detachedProcess       = 0x00000008
	createNewProcessGroup = 0x00000200
	createNoWindow        = 0x08000000
)

// launchDetached arranca PowerShell con el ayudante, sin ventana y separado del
// programa (sigue vivo cuando el programa se cierra).
func launchDetached(script string, args ...string) error {
	a := append([]string{"-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-WindowStyle", "Hidden", "-File", script}, args...)
	cmd := exec.Command("powershell.exe", a...)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: detachedProcess | createNewProcessGroup | createNoWindow}
	return cmd.Start()
}
