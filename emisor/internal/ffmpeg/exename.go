package ffmpeg

import "runtime"

// exeName es el nombre del ejecutable de ffmpeg en este sistema.
var exeName = func() string {
	if runtime.GOOS == "windows" {
		return "ffmpeg.exe"
	}
	return "ffmpeg"
}()
