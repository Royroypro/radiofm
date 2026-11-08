//go:build cgo

package audio

import (
	"fmt"
	"strings"
	"unsafe"

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

// Stream es un dispositivo de audio abierto (salida o captura).
type Stream struct {
	ctx    *malgo.AllocatedContext
	dev    *malgo.Device
	Device Device
}

// Close detiene y libera el dispositivo.
func (s *Stream) Close() error {
	if s.dev != nil {
		_ = s.dev.Stop()
		s.dev.Uninit()
	}
	if s.ctx != nil {
		_ = s.ctx.Uninit()
		s.ctx.Free()
	}
	return nil
}

// findDevice busca un dispositivo por ID o por nombre (sin distinguir mayúsculas).
// Con id vacío devuelve nil (= dispositivo predeterminado).
func findDevice(ctx *malgo.AllocatedContext, typ malgo.DeviceType, id string) (*malgo.DeviceInfo, error) {
	if id == "" {
		return nil, nil
	}
	infos, err := ctx.Devices(typ)
	if err != nil {
		return nil, err
	}
	for i := range infos {
		if infos[i].ID.String() == id || strings.EqualFold(strings.TrimSpace(infos[i].Name()), id) {
			return &infos[i], nil
		}
	}
	return nil, fmt.Errorf("audio: no se encontró el dispositivo %q", id)
}

// OpenOutput abre la tarjeta de salida (id vacío = predeterminada) y la
// conecta al motor: su callback llama a render en cada bloque. render suele
// ser Engine.Render; la pre-escucha puede usar otra función.
func OpenOutput(id string, render func([]float32)) (*Stream, error) {
	return open(malgo.Playback, id, func(out, _ []byte) {
		render(bytesAsFloats(out))
	})
}

// OpenCapture abre una entrada (micrófono o línea) y escribe lo capturado en dst.
// Si dst está lleno, lo que no cabe se descarta.
func OpenCapture(id string, dst *RingBuffer) (*Stream, error) {
	return open(malgo.Capture, id, func(_, in []byte) {
		dst.Write(bytesAsFloats(in))
	})
}

func open(typ malgo.DeviceType, id string, cb func(out, in []byte)) (*Stream, error) {
	ctx, err := malgo.InitContext(nil, malgo.ContextConfig{}, nil)
	if err != nil {
		return nil, fmt.Errorf("audio: iniciar miniaudio: %w", err)
	}
	s := &Stream{ctx: ctx}
	info, err := findDevice(ctx, typ, id)
	if err != nil {
		s.Close()
		return nil, err
	}
	cfg := malgo.DefaultDeviceConfig(typ)
	cfg.SampleRate = SampleRate
	cfg.PeriodSizeInMilliseconds = 10
	cfg.Periods = 3
	if typ == malgo.Playback {
		cfg.Playback.Format, cfg.Playback.Channels = malgo.FormatF32, Channels
	} else {
		cfg.Capture.Format, cfg.Capture.Channels = malgo.FormatF32, Channels
	}
	kind := Playback
	if typ == malgo.Capture {
		kind = Capture
	}
	if info != nil {
		if typ == malgo.Playback {
			cfg.Playback.DeviceID = info.ID.Pointer()
		} else {
			cfg.Capture.DeviceID = info.ID.Pointer()
		}
		s.Device = Device{ID: info.ID.String(), Name: strings.TrimSpace(info.Name()), Kind: kind}
	} else {
		s.Device = Device{Name: "(predeterminado)", Kind: kind, IsDefault: true}
	}
	dev, err := malgo.InitDevice(ctx.Context, cfg, malgo.DeviceCallbacks{
		Data: func(out, in []byte, _ uint32) { cb(out, in) },
	})
	if err != nil {
		s.Close()
		return nil, fmt.Errorf("audio: abrir %s: %w", s.Device.Name, err)
	}
	s.dev = dev
	if err := dev.Start(); err != nil {
		s.Close()
		return nil, fmt.Errorf("audio: iniciar %s: %w", s.Device.Name, err)
	}
	return s, nil
}

// bytesAsFloats reinterpreta el buffer de miniaudio (FormatF32) como []float32 sin copiar.
func bytesAsFloats(b []byte) []float32 {
	if len(b) < 4 {
		return nil
	}
	return unsafe.Slice((*float32)(unsafe.Pointer(&b[0])), len(b)/4)
}
