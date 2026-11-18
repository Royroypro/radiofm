//go:build !windows

package runner

import "os/exec"

func hideWindow(*exec.Cmd) {}

// HideWindow no hace nada fuera de Windows.
func HideWindow(*exec.Cmd) {}

// killTree fuera de Windows mata solo el proceso directo.
func killTree(cmd *exec.Cmd) error {
	if cmd.Process == nil {
		return nil
	}
	return cmd.Process.Kill()
}
