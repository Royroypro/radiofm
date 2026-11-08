//go:build !windows

package ffmpeg

import "os/exec"

func hideWindow(*exec.Cmd) {}

func exeName(name string) string { return name }
