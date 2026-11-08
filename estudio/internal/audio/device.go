//go:build cgo

package audio

import (
	"fmt"
	"strings"

	"github.com/gen2brain/malgo"
)

// ListDevices enumera los dispositivos de salida y de captura del sistema.
func ListDevices() ([]Device, error) {
	ctx, err := malgo.InitContext(nil, malgo.ContextConfig{}, nil)
	if err != nil {
		return nil, fmt.Errorf("audio: iniciar miniaudio: %w", err)
	}
	defer func() {
		_ = ctx.Uninit()
		ctx.Free()
	}()
	kinds := []struct {
		kind DeviceKind
		typ  malgo.DeviceType
	}{{Playback, malgo.Playback}, {Capture, malgo.Capture}}
	var out []Device
	for _, k := range kinds {
		infos, err := ctx.Devices(k.typ)
		if err != nil {
			return nil, fmt.Errorf("audio: listar dispositivos (%s): %w", k.kind, err)
		}
		for _, d := range infos {
			out = append(out, Device{ID: d.ID.String(), Name: strings.TrimSpace(d.Name()), Kind: k.kind,
				IsDefault: d.IsDefault != 0})
		}
	}
	return out, nil
}
