//go:build windows

package runner

import (
	"os/exec"
	"strconv"
	"syscall"
)

// createNoWindow evita que ffmpeg abra una consola negra en la PC.
const createNoWindow = 0x08000000

func hideWindow(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: createNoWindow}
}

// HideWindow aplica lo mismo a otros comandos auxiliares (listar dispositivos, códecs).
func HideWindow(cmd *exec.Cmd) { hideWindow(cmd) }

// killTree mata el proceso y todos sus hijos (taskkill /T). Si taskkill falla,
// al menos mata el proceso directo.
func killTree(cmd *exec.Cmd) error {
	if cmd.Process == nil {
		return nil
	}
	tk := exec.Command("taskkill", "/T", "/F", "/PID", strconv.Itoa(cmd.Process.Pid))
	hideWindow(tk)
	if err := tk.Run(); err != nil {
		return cmd.Process.Kill()
	}
	return nil
}

// KillTree mata un proceso y sus hijos (para otros procesos auxiliares, como la captura de audio).
func KillTree(cmd *exec.Cmd) error { return killTree(cmd) }
