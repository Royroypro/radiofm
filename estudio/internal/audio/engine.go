package audio

import (
	"errors"
	"math"
	"sync/atomic"
	"time"
)

// Slots de reproductores: 0 y 1 son los decks A/B (música, con ducking);
// del 2 en adelante, carts de la botonera (sin ducking).
const (
	DeckA    = 0
	DeckB    = 1
	NumDecks = 2
	NumCarts = 24
	numSlots = NumDecks + NumCarts
)

// CartSlot devuelve el slot del cart i (0–23).
func CartSlot(i int) int { return NumDecks + i }

// EventType es el tipo de evento que emite el motor.
type EventType int

const (
	EventStarted EventType = iota + 1 // un reproductor empezó a sonar
	EventSegue                        // llegó al punto de mezcla: es hora de lanzar lo siguiente
	EventEnded                        // terminó (fin, cue-out, stop o fundido completo)
	EventSilence                      // el bus lleva Config.SilenceAfter en silencio
)

// Event es un aviso del motor. Tag es el que se pasó en LoadOptions.
type Event struct {
	Type EventType
	Slot int
	Tag  uint64
}

// Config son los parámetros del motor.
type Config struct {
	DuckDepthDB      float64       // −12
	DuckThresholdDB  float64       // umbral de voz del mic, −35
	DuckAttack       time.Duration // 50 ms
	DuckRelease      time.Duration // 500 ms
	LimiterCeilingDB float64       // −1
	SilenceThreshold float64       // dBFS, −50
	SilenceAfter     time.Duration // 0 = no detectar
	EncoderBuffer    time.Duration // tamaño del buffer hacia el codificador, 2 s
}

// DefaultConfig devuelve la configuración del plan.
func DefaultConfig() Config {
	return Config{DuckDepthDB: -12, DuckThresholdDB: -35, DuckAttack: 50 * time.Millisecond,
		DuckRelease: 500 * time.Millisecond, LimiterCeilingDB: -1, SilenceThreshold: -50,
		SilenceAfter: 15 * time.Second, EncoderBuffer: 2 * time.Second}
}

type cmdKind int

const (
	cmdLoad cmdKind = iota + 1
	cmdPlay
	cmdFade
	cmdStop
	cmdMasterGain
	cmdMicGain
	cmdProcessor
)

// ProcScope es a qué salida se aplica el procesador.
type ProcScope int

const (
	ProcBoth   ProcScope = iota // FM (tarjeta) y stream
	ProcStream                  // solo el stream (el transmisor FM ya tiene su procesador)
)

type command struct {
	kind   cmdKind
	slot   int
	item   *playItem
	value  float32
	frames int64
	delay  int64 // cmdPlay: frames de espera antes de empezar
	stop   bool
	proc   *Processor
	scope  ProcScope
}

// slotStatus se publica con atómicos para leerlo desde otras goroutines.
type slotStatus struct {
	tag     atomic.Uint64
	pos     atomic.Int64
	length  atomic.Int64
	playing atomic.Bool
	loaded  atomic.Bool
}

// Engine es el motor de audio. Render es lo único que corre en el hilo de
// audio; el resto de métodos se llaman desde cualquier goroutine y se
// comunican con Render por un canal de comandos que Render vacía sin esperar.
type Engine struct {
	cfg    Config
	cmds   chan command
	events chan Event
	trash  chan *playItem // ítems que Render ya no usa: otra goroutine los cancela

	// estado del hilo de audio
	slots      [numSlots]*playItem
	music, tmp []float32
	carts, mic []float32
	ducker     *Ducker
	limiter    *Limiter
	proc       *Processor // nil = sin procesar
	procScope  ProcScope
	procBuf    []float32 // copia procesada para el stream (ProcStream)
	streamLim  *Limiter  // limitador propio de esa copia
	master     float32
	micGain    float32
	silent     int64        // frames seguidos en silencio
	silentPub  atomic.Int64 // copia de silent para Status (cualquier goroutine)
	silenceSet bool

	// entradas y salidas compartidas
	micRing  *RingBuffer // lo llena el callback de captura
	recRing  *RingBuffer // copia del micrófono para grabar (voice tracking); lo vacía el grabador
	recOn    atomic.Bool
	encRing  *RingBuffer // lo vacía el codificador
	logRing  *RingBuffer // copia de la salida FM para el grabador legal (airlog)
	logTapOn atomic.Bool
	tvRing   *RingBuffer // copia del stream para el modo TV (video)
	tvTapOn  atomic.Bool
	micOn    atomic.Bool
	duckOn   atomic.Bool
	encTapOn atomic.Bool

	// estado publicado
	status      [numSlots]slotStatus
	levels      [4]atomic.Uint32 // PeakL, PeakR, RMSL, RMSR (bits float32)
	duckGain    atomic.Uint32
	limitGain   atomic.Uint32
	procGain    atomic.Uint32 // reducción de los compresores (1 = nada)
	recPeak     atomic.Uint32 // pico del micrófono mientras se graba (bits float32)
	agcGain     atomic.Uint32
	dropouts    atomic.Int64 // frames de silencio por decodificador lento
	encDropped  atomic.Int64 // muestras descartadas hacia el codificador
	logDropped  atomic.Int64 // muestras descartadas hacia el grabador
	renderedFrm atomic.Int64
}

// NewEngine crea el motor. No abre ningún dispositivo: ver Output y RunClock.
func NewEngine(cfg Config) *Engine {
	if cfg.EncoderBuffer <= 0 {
		cfg.EncoderBuffer = 2 * time.Second
	}
	e := &Engine{
		cfg:       cfg,
		cmds:      make(chan command, 256),
		events:    make(chan Event, 256),
		trash:     make(chan *playItem, numSlots*4),
		music:     make([]float32, maxBlockFrames*Channels),
		tmp:       make([]float32, maxBlockFrames*Channels),
		carts:     make([]float32, maxBlockFrames*Channels),
		mic:       make([]float32, maxBlockFrames*Channels),
		ducker:    NewDucker(cfg.DuckDepthDB, cfg.DuckThresholdDB, cfg.DuckAttack, cfg.DuckRelease),
		limiter:   NewLimiter(cfg.LimiterCeilingDB, 1500*time.Microsecond, 80*time.Millisecond),
		procBuf:   make([]float32, maxBlockFrames*Channels),
		streamLim: NewLimiter(cfg.LimiterCeilingDB, 1500*time.Microsecond, 80*time.Millisecond),
		master:    1,
		micGain:   1,
		micRing:   NewRingBuffer(int(DurationToFrames(time.Second)) * Channels),
		recRing:   NewRingBuffer(int(DurationToFrames(4*time.Second)) * Channels),
		encRing:   NewRingBuffer(int(DurationToFrames(cfg.EncoderBuffer)) * Channels),
		logRing:   NewRingBuffer(int(DurationToFrames(5*time.Second)) * Channels),
		tvRing:    NewRingBuffer(int(DurationToFrames(2*time.Second)) * Channels),
	}
	e.duckGain.Store(math.Float32bits(1))
	e.limitGain.Store(math.Float32bits(1))
	e.procGain.Store(math.Float32bits(1))
	e.agcGain.Store(math.Float32bits(1))
	e.encTapOn.Store(true)
	go e.collectTrash()
	return e
}

func (e *Engine) collectTrash() {
	for it := range e.trash {
		it.cancel()
	}
}

// Events devuelve el canal de eventos. Si nadie lo lee y se llena, los eventos se pierden.
func (e *Engine) Events() <-chan Event { return e.events }

// ErrBusy indica que la cola de comandos está llena (el hilo de audio no corre).
var ErrBusy = errors.New("audio: cola de comandos llena")

func (e *Engine) send(c command) error {
	select {
	case e.cmds <- c:
		return nil
	default:
		if c.item != nil {
			c.item.cancel()
		}
		return ErrBusy
	}
}

// Load carga src en un slot (empieza a decodificar y llenar el buffer en
// segundo plano) sin reproducirlo. Si el slot tenía algo, se descarta.
func (e *Engine) Load(slot int, src Source, o LoadOptions) error {
	if slot < 0 || slot >= numSlots {
		src.Close()
		return errors.New("audio: slot inválido")
	}
	it := newPlayItem(src, o)
	return e.send(command{kind: cmdLoad, slot: slot, item: it})
}

// Play empieza a sonar el slot, con fundido de entrada opcional.
func (e *Engine) Play(slot int, fadeIn time.Duration) error { return e.PlayAfter(slot, fadeIn, 0) }

// PlayAfter empieza a sonar el slot tras esperar delay (con precisión de un
// bloque, 10 ms). Se usa para que una tanda entre justo cuando terminó de
// fundirse la música, sin pisarla.
func (e *Engine) PlayAfter(slot int, fadeIn, delay time.Duration) error {
	return e.send(command{kind: cmdPlay, slot: slot, frames: DurationToFrames(fadeIn), delay: DurationToFrames(delay)})
}

// Fade lleva el volumen del slot a gain (0–1) en d; si stop, al terminar se detiene.
func (e *Engine) Fade(slot int, gain float32, d time.Duration, stop bool) error {
	return e.send(command{kind: cmdFade, slot: slot, value: gain, frames: DurationToFrames(d), stop: stop})
}

// Stop detiene el slot al instante.
func (e *Engine) Stop(slot int) error { return e.send(command{kind: cmdStop, slot: slot}) }

// SetMasterGain fija la ganancia general (lineal).
func (e *Engine) SetMasterGain(g float32) error {
	return e.send(command{kind: cmdMasterGain, value: g})
}

// SetProcessor cambia el procesador del bus (nil = sin procesar) y a qué salida
// se aplica. El procesador se crea fuera del hilo de audio (NewProcessor reserva memoria).
func (e *Engine) SetProcessor(p *Processor, scope ProcScope) error {
	return e.send(command{kind: cmdProcessor, proc: p, scope: scope})
}

// SetMicGain fija la ganancia del micrófono (lineal).
func (e *Engine) SetMicGain(g float32) error { return e.send(command{kind: cmdMicGain, value: g}) }

// SetMic abre o cierra el micrófono en la mezcla.
func (e *Engine) SetMic(on bool) { e.micOn.Store(on) }

// SetDuck fuerza el ducking de la música (botón).
func (e *Engine) SetDuck(on bool) { e.duckOn.Store(on) }

// SetEncoderTap activa o desactiva la copia del bus hacia el codificador
// (se desactiva cuando la fuente de Icecast es la entrada de captura).
func (e *Engine) SetEncoderTap(on bool) { e.encTapOn.Store(on) }

// SetRecording activa la copia del micrófono hacia RecordOutput (voice
// tracking). Graba aunque el micrófono no esté al aire.
func (e *Engine) SetRecording(on bool) {
	if on {
		e.recRing.Discard(e.recRing.Len()) // empezar sin restos de una toma anterior
	}
	e.recOn.Store(on)
}

// RecordOutput es el buffer con lo que capta el micrófono mientras se graba.
func (e *Engine) RecordOutput() *RingBuffer { return e.recRing }

// MicInput es el buffer donde el callback de captura escribe el micrófono.
func (e *Engine) MicInput() *RingBuffer { return e.micRing }

// SetLoggerTap activa la copia de la salida FM hacia el grabador de la emisión.
func (e *Engine) SetLoggerTap(on bool) { e.logTapOn.Store(on) }

// LoggerOutput es el buffer del que lee el grabador de la emisión (lo que sale
// por la tarjeta, después del procesador y el limitador).
func (e *Engine) LoggerOutput() *RingBuffer { return e.logRing }

// SetTVTap activa la copia del stream hacia el modo TV.
func (e *Engine) SetTVTap(on bool) {
	if on {
		e.tvRing.Discard(e.tvRing.Len())
	}
	e.tvTapOn.Store(on)
}

// TVOutput es el buffer del que lee el modo TV (el mismo audio que va a Icecast).
func (e *Engine) TVOutput() *RingBuffer { return e.tvRing }

// EncoderOutput es el buffer del que lee el codificador de Icecast.
func (e *Engine) EncoderOutput() *RingBuffer { return e.encRing }

// apply ejecuta un comando dentro del hilo de audio.
func (e *Engine) apply(c command) {
	if c.slot < 0 || c.slot >= numSlots {
		return
	}
	st := &e.status[c.slot]
	switch c.kind {
	case cmdLoad:
		e.drop(c.slot)
		e.slots[c.slot] = c.item
		st.tag.Store(c.item.tag)
		st.pos.Store(0)
		st.length.Store(c.item.length)
		st.loaded.Store(true)
	case cmdPlay:
		it := e.slots[c.slot]
		if it == nil || it.playing || it.endSent {
			return
		}
		it.playing = true
		it.wait = c.delay
		if c.frames > 0 {
			it.vol = 0
			it.setFade(1, c.frames, false)
		}
		st.playing.Store(true)
		e.emit(Event{Type: EventStarted, Slot: c.slot, Tag: it.tag})
	case cmdProcessor:
		e.proc, e.procScope = c.proc, c.scope
		if c.proc == nil {
			e.procGain.Store(math.Float32bits(1))
			e.agcGain.Store(math.Float32bits(1))
		}
	case cmdFade:
		if it := e.slots[c.slot]; it != nil && it.playing {
			it.setFade(c.value, c.frames, c.stop)
		}
	case cmdStop:
		if it := e.slots[c.slot]; it != nil {
			wasPlaying := it.playing
			it.playing = false
			st.playing.Store(false)
			if wasPlaying && !it.endSent {
				it.endSent = true
				e.emit(Event{Type: EventEnded, Slot: c.slot, Tag: it.tag})
			}
			e.drop(c.slot)
		}
	case cmdMasterGain:
		e.master = c.value
	case cmdMicGain:
		e.micGain = c.value
	}
}

// drop saca el ítem del slot y lo manda a cancelar fuera del hilo de audio.
func (e *Engine) drop(slot int) {
	if it := e.slots[slot]; it != nil {
		e.slots[slot] = nil
		e.status[slot].loaded.Store(false)
		e.status[slot].playing.Store(false)
		select {
		case e.trash <- it:
		default: // no debería pasar; se pierde la cancelación y la goroutine termina sola al llenar
		}
	}
}

func (e *Engine) emit(ev Event) {
	select {
	case e.events <- ev:
	default:
	}
}

// Render produce el siguiente bloque de la mezcla en out (entrelazado). Es lo
// que llama el callback de la tarjeta (o RunClock). No bloquea ni reserva memoria.
func (e *Engine) Render(out []float32) {
	for len(out) > 0 {
		n := min(len(out), maxBlockFrames*Channels)
		e.renderBlock(out[:n])
		out = out[n:]
	}
}

func (e *Engine) renderBlock(out []float32) {
	// 1. comandos pendientes
	for i := 0; i < 64; i++ {
		select {
		case c := <-e.cmds:
			e.apply(c)
			continue
		default:
		}
		break
	}
	n := len(out)
	frames := n / Channels
	music, carts, mic, tmp := e.music[:n], e.carts[:n], e.mic[:n], e.tmp[:n]
	clear(music)
	clear(carts)

	// 2. reproductores
	var dropped int64
	for s := 0; s < numSlots; s++ {
		it := e.slots[s]
		if it == nil {
			continue
		}
		dst := music
		if s >= NumDecks {
			dst = carts
		}
		segue, ended, drop := it.render(dst, tmp)
		dropped += int64(drop)
		e.status[s].pos.Store(it.pos)
		if segue {
			e.emit(Event{Type: EventSegue, Slot: s, Tag: it.tag})
		}
		if ended {
			e.status[s].playing.Store(false)
			e.emit(Event{Type: EventEnded, Slot: s, Tag: it.tag})
		}
	}
	if dropped > 0 {
		e.dropouts.Add(dropped)
	}

	// 3. micrófono (con corrección de deriva: si se acumula más de 200 ms, se descarta el exceso)
	got := e.micRing.Read(mic)
	clear(mic[got:])
	if extra := e.micRing.Len() - int(DurationToFrames(200*time.Millisecond))*Channels; extra > 0 {
		e.micRing.Discard(extra - int(DurationToFrames(100*time.Millisecond))*Channels)
	}
	var micPeak float32
	micOn := e.micOn.Load()
	if rec := e.recOn.Load(); micOn || rec {
		for i := range mic {
			mic[i] *= e.micGain
		}
		micPeak = peakOf(mic)
		if rec {
			e.recRing.Write(mic) // si el grabador no da abasto, se pierde (nunca frena el aire)
			e.recPeak.Store(math.Float32bits(micPeak))
		}
	}
	if !micOn {
		micPeak = 0 // grabando sin el mic al aire: no activa el ducking
	}

	// 4. ducking y suma
	g := e.ducker.Process(frames, e.duckOn.Load(), micPeak)
	for i := range out {
		v := music[i]*g + carts[i]
		if micOn {
			v += mic[i]
		}
		out[i] = v * e.master
	}

	// 5. procesador (AGC + 3 bandas), limitador, medidores, silencio
	stream := out // lo que va al codificador
	if p := e.proc; p != nil {
		if e.procScope == ProcStream {
			stream = e.procBuf[:n]
			copy(stream, out)
			p.Process(stream)
			e.streamLim.Process(stream)
		} else {
			p.Process(out)
		}
		e.procGain.Store(math.Float32bits(p.Reduction))
		e.agcGain.Store(math.Float32bits(p.AGC))
	}
	e.limiter.Process(out)
	lv := measure(out)
	e.levels[0].Store(math.Float32bits(lv.PeakL))
	e.levels[1].Store(math.Float32bits(lv.PeakR))
	e.levels[2].Store(math.Float32bits(lv.RMSL))
	e.levels[3].Store(math.Float32bits(lv.RMSR))
	e.duckGain.Store(math.Float32bits(g))
	e.limitGain.Store(math.Float32bits(e.limiter.Reduction))
	if e.cfg.SilenceAfter > 0 {
		if float64(max(lv.PeakL, lv.PeakR)) < float64(DBToGain(e.cfg.SilenceThreshold)) {
			e.silent += int64(frames)
			if !e.silenceSet && e.silent >= DurationToFrames(e.cfg.SilenceAfter) {
				e.silenceSet = true
				e.emit(Event{Type: EventSilence, Slot: -1})
			}
		} else {
			e.silent, e.silenceSet = 0, false
		}
		e.silentPub.Store(e.silent)
	}

	// 6. copia hacia el codificador (si se atasca, se descarta: nunca frena la salida)
	if e.logTapOn.Load() { // grabador legal: siempre lo que sale por la tarjeta
		if w := e.logRing.Write(out); w < n {
			e.logDropped.Add(int64(n - w))
		}
	}
	if e.encTapOn.Load() {
		if w := e.encRing.Write(stream); w < n {
			e.encDropped.Add(int64(n - w))
		}
	}
	if e.tvTapOn.Load() {
		e.tvRing.Write(stream) // si la TV se atrasa, se descarta
	}
	e.renderedFrm.Add(int64(frames))
}

// SlotState es el estado de un reproductor.
type SlotState struct {
	Loaded   bool          `json:"loaded"`
	Playing  bool          `json:"playing"`
	Tag      uint64        `json:"tag"`
	Position time.Duration `json:"position"`
	Length   time.Duration `json:"length"` // 0 = desconocida
}

// Status es una foto del motor para la interfaz.
type Status struct {
	Decks       [NumDecks]SlotState `json:"decks"`
	CartsOn     []int               `json:"carts_on"` // carts sonando
	Levels      Levels              `json:"levels"`
	DuckGain    float32             `json:"duck_gain"`
	LimiterGain float32             `json:"limiter_gain"`
	ProcGain    float32             `json:"proc_gain"` // reducción de los compresores del procesador (1 = nada)
	AGCGain     float32             `json:"agc_gain"`  // corrección del AGC (lineal)
	RecPeak     float32             `json:"rec_peak"`  // nivel del micrófono al grabar un voice track
	MicOn       bool                `json:"mic_on"`
	Dropouts    int64               `json:"dropouts"`
	EncDropped  int64               `json:"enc_dropped"`
	Rendered    time.Duration       `json:"rendered"`
	SilentFor   time.Duration       `json:"silent_for"` // tiempo seguido en silencio en la salida (0 = suena)
}

// Slot devuelve el estado de un slot.
func (e *Engine) Slot(slot int) SlotState {
	st := &e.status[slot]
	return SlotState{Loaded: st.loaded.Load(), Playing: st.playing.Load(), Tag: st.tag.Load(),
		Position: FramesToDuration(st.pos.Load()), Length: FramesToDuration(st.length.Load())}
}

// Status devuelve una foto del estado (se puede llamar desde cualquier goroutine).
func (e *Engine) Status() Status {
	var s Status
	for i := 0; i < NumDecks; i++ {
		s.Decks[i] = e.Slot(i)
	}
	for i := 0; i < NumCarts; i++ {
		if e.status[CartSlot(i)].playing.Load() {
			s.CartsOn = append(s.CartsOn, i)
		}
	}
	f := func(a *atomic.Uint32) float32 { return math.Float32frombits(a.Load()) }
	s.Levels = Levels{PeakL: f(&e.levels[0]), PeakR: f(&e.levels[1]), RMSL: f(&e.levels[2]), RMSR: f(&e.levels[3])}
	s.DuckGain, s.LimiterGain = f(&e.duckGain), f(&e.limitGain)
	s.SilentFor = FramesToDuration(e.silentPub.Load())
	s.ProcGain, s.AGCGain = f(&e.procGain), f(&e.agcGain)
	if e.recOn.Load() {
		s.RecPeak = f(&e.recPeak)
	}
	s.MicOn = e.micOn.Load()
	s.Dropouts, s.EncDropped = e.dropouts.Load(), e.encDropped.Load()
	s.Rendered = FramesToDuration(e.renderedFrm.Load())
	return s
}
