package audio

import "sync/atomic"

// RingBuffer es una cola circular de muestras float32 para UN productor y UN
// consumidor (SPSC), sin bloqueos ni reservas de memoria: se puede usar desde
// el callback de audio. Diseño inspirado en ma_pcm_rb de miniaudio (MIT-0).
//
// Write y Read nunca esperan: escriben/leen lo que cabe/hay y devuelven cuánto.
type RingBuffer struct {
	buf  []float32
	mask uint64
	r    atomic.Uint64 // posición de lectura (solo la avanza el consumidor)
	w    atomic.Uint64 // posición de escritura (solo la avanza el productor)
}

// NewRingBuffer crea un buffer con capacidad para al menos minSamples muestras
// (se redondea a potencia de 2).
func NewRingBuffer(minSamples int) *RingBuffer {
	n := 1
	for n < minSamples {
		n <<= 1
	}
	return &RingBuffer{buf: make([]float32, n), mask: uint64(n - 1)}
}

// Cap devuelve la capacidad en muestras.
func (b *RingBuffer) Cap() int { return len(b.buf) }

// Len devuelve las muestras disponibles para leer.
func (b *RingBuffer) Len() int { return int(b.w.Load() - b.r.Load()) }

// Free devuelve el espacio libre para escribir.
func (b *RingBuffer) Free() int { return len(b.buf) - b.Len() }

// Write copia hasta len(p) muestras y devuelve cuántas cupieron.
func (b *RingBuffer) Write(p []float32) int {
	w, r := b.w.Load(), b.r.Load()
	free := uint64(len(b.buf)) - (w - r)
	n := uint64(len(p))
	if n > free {
		n = free
	}
	for i := uint64(0); i < n; i++ {
		b.buf[(w+i)&b.mask] = p[i]
	}
	b.w.Store(w + n)
	return int(n)
}

// Read copia hasta len(p) muestras y devuelve cuántas había.
func (b *RingBuffer) Read(p []float32) int {
	r, w := b.r.Load(), b.w.Load()
	avail := w - r
	n := uint64(len(p))
	if n > avail {
		n = avail
	}
	for i := uint64(0); i < n; i++ {
		p[i] = b.buf[(r+i)&b.mask]
	}
	b.r.Store(r + n)
	return int(n)
}

// Discard descarta hasta n muestras (lo usa el consumidor para corregir deriva).
func (b *RingBuffer) Discard(n int) int {
	r, w := b.r.Load(), b.w.Load()
	avail := w - r
	if uint64(n) > avail {
		n = int(avail)
	}
	b.r.Store(r + uint64(n))
	return n
}
