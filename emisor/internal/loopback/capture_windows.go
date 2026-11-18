//go:build windows && cgo

package loopback

import (
	"errors"
	"fmt"
	"sync"

	"github.com/gen2brain/malgo"
)

// Available dice si este programa se compiló con captura de altavoces.
const Available = true

var (
	ctxOnce sync.Once
	ctxErr  error
	mctx    *malgo.AllocatedContext
)

// audioContext abre una sola vez el contexto WASAPI de miniaudio.
func audioContext() (*malgo.AllocatedContext, error) {
	ctxOnce.Do(func() {
		mctx, ctxErr = malgo.InitContext([]malgo.Backend{malgo.BackendWasapi}, malgo.ContextConfig{}, nil)
	})
	return mctx, ctxErr
}

// ListPlayback devuelve los nombres de los adaptadores de reproducción
// (altavoces, auriculares, HDMI, cables virtuales…).
func ListPlayback() ([]string, error) {
	c, err := audioContext()
	if err != nil {
		return nil, err
	}
	infos, err := c.Devices(malgo.Playback)
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(infos))
	for i := range infos {
		names = append(names, infos[i].Name())
	}
	return names, nil
}

// Capture empieza a capturar lo que suena en el adaptador name ("" = el
// predeterminado de Windows) y entrega el audio a onData. Devuelve la función
// para detenerla.
func Capture(name string, onData func([]byte)) (stop func(), err error) {
	c, err := audioContext()
	if err != nil {
		return nil, err
	}
	cfg := malgo.DefaultDeviceConfig(malgo.Loopback)
	cfg.Capture.Format = malgo.FormatS16
	cfg.Capture.Channels = Channels
	cfg.SampleRate = SampleRate
	cfg.PeriodSizeInMilliseconds = 10
	if name != "" {
		infos, err := c.Devices(malgo.Playback)
		if err != nil {
			return nil, err
		}
		found := false
		for i := range infos {
			if infos[i].Name() == name {
				cfg.Capture.DeviceID = infos[i].ID.Pointer()
				found = true
				break
			}
		}
		if !found {
			return nil, fmt.Errorf("no se encontró la salida de audio %q (¿se desconectó?)", name)
		}
	}
	dev, err := malgo.InitDevice(c.Context, cfg, malgo.DeviceCallbacks{
		Data: func(_, in []byte, _ uint32) {
			if len(in) > 0 {
				onData(append([]byte(nil), in...)) // miniaudio reutiliza el búfer
			}
		},
	})
	if err != nil {
		return nil, fmt.Errorf("no se pudo abrir la salida de audio: %w", err)
	}
	if err := dev.Start(); err != nil {
		dev.Uninit()
		return nil, errors.Join(errors.New("no se pudo iniciar la captura de la salida de audio"), err)
	}
	var once sync.Once
	return func() {
		once.Do(func() {
			_ = dev.Stop()
			dev.Uninit()
		})
	}, nil
}
