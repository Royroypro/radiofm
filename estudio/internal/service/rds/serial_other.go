//go:build !windows

package rds

import "io"

func openSerial(string, int) (io.WriteCloser, error) { return nil, errNoSerial }
