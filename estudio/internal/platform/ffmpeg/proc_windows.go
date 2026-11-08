//go:build windows

package ffmpeg

import (
	"os/exec"
	"syscall"
)

// createNoWindow evita que cada ffmpeg abra una ventana de consola desde la app gráfica.
const createNoWindow = 0x08000000

func hideWindow(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: createNoWindow}
}

func exeName(name string) string { return name + ".exe" }
