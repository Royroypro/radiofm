package ffmpeg

import (
	"bufio"
	"bytes"
	"math"
	"regexp"
	"strconv"
	"strings"
)

// ---------- dispositivos ----------

// Device es un dispositivo DirectShow.
type Device struct {
	Name string `json:"name"`
	Kind string `json:"kind"` // "audio" o "video"
}

// ListDevicesArgs es el comando que lista los dispositivos DirectShow (la lista sale por stderr).
func ListDevicesArgs() []string {
	return []string{"-hide_banner", "-list_devices", "true", "-f", "dshow", "-i", "dummy"}
}

var deviceLine = regexp.MustCompile(`\]\s+"(.+)"\s+\((audio|video|none)\)\s*$`)

// ParseDevices lee la salida de ListDevicesArgs. Ignora los "Alternative name" y
// los dispositivos sin tipo ("none": cámaras virtuales sin formato).
// Acepta también el formato antiguo de ffmpeg (encabezados "DirectShow video devices").
func ParseDevices(out string) []Device {
	var list []Device
	section := ""
	seen := map[string]bool{}
	sc := bufio.NewScanner(strings.NewReader(out))
	for sc.Scan() {
		line := strings.TrimRight(sc.Text(), "\r")
		switch {
		case strings.Contains(line, "DirectShow video devices"):
			section = "video"
			continue
		case strings.Contains(line, "DirectShow audio devices"):
			section = "audio"
			continue
		case strings.Contains(line, "Alternative name"):
			continue
		}
		if m := deviceLine.FindStringSubmatch(line); m != nil {
			if m[2] != "none" {
				add(&list, seen, Device{Name: m[1], Kind: m[2]})
			}
			continue
		}
		// Formato antiguo: `[dshow @ …]  "Nombre"` bajo un encabezado.
		if section != "" {
			if i := strings.Index(line, `"`); i >= 0 {
				if j := strings.LastIndex(line, `"`); j > i {
					add(&list, seen, Device{Name: line[i+1 : j], Kind: section})
				}
			}
		}
	}
	return list
}

func add(list *[]Device, seen map[string]bool, d Device) {
	key := d.Kind + "\x00" + d.Name
	if !seen[key] {
		seen[key] = true
		*list = append(*list, d)
	}
}

// ---------- codificadores ----------

// ParseEncoders lee `ffmpeg -hide_banner -encoders` y devuelve los nombres disponibles.
func ParseEncoders(out string) map[string]bool {
	set := map[string]bool{}
	sc := bufio.NewScanner(strings.NewReader(out))
	started := false
	for sc.Scan() {
		f := strings.Fields(sc.Text())
		if len(f) == 1 && f[0] == "------" {
			started = true
			continue
		}
		if started && len(f) >= 2 && len(f[0]) == 6 {
			set[f[1]] = true
		}
	}
	return set
}

// ---------- progreso ----------

// Progress es el último bloque de -progress de ffmpeg.
type Progress struct {
	OutTimeSec  float64 // segundos ya enviados
	TotalBytes  int64
	BitrateKbps float64 // -1 si ffmpeg aún no lo sabe
	Speed       float64 // 1.0 = tiempo real
	Frame       int64
	FPS         float64
	DupFrames   int64
	DropFrames  int64
	End         bool // progress=end: ffmpeg terminó
}

// ProgressParser junta las líneas clave=valor de -progress; cada bloque termina en "progress=…".
// Las líneas que no son de progreso son mensajes de ffmpeg (errores, avisos).
type ProgressParser struct{ cur Progress }

// Feed recibe una línea. Si cierra un bloque devuelve el progreso y done=true.
// Si es un mensaje de ffmpeg devuelve isLog=true.
func (p *ProgressParser) Feed(line string) (pr Progress, done, isLog bool) {
	line = strings.TrimSpace(line)
	if line == "" {
		return pr, false, false
	}
	k, v, ok := strings.Cut(line, "=")
	if !ok || strings.ContainsAny(k, " []:") {
		return pr, false, true
	}
	switch k {
	case "out_time_us", "out_time_ms": // las dos están en microsegundos
		if n, err := strconv.ParseInt(v, 10, 64); err == nil && n >= 0 {
			p.cur.OutTimeSec = float64(n) / 1e6
		}
	case "total_size":
		p.cur.TotalBytes, _ = strconv.ParseInt(v, 10, 64)
	case "bitrate":
		if f, err := strconv.ParseFloat(strings.TrimSuffix(v, "kbits/s"), 64); err == nil {
			p.cur.BitrateKbps = f
		} else {
			p.cur.BitrateKbps = -1
		}
	case "speed":
		p.cur.Speed, _ = strconv.ParseFloat(strings.TrimSuffix(v, "x"), 64)
	case "frame":
		p.cur.Frame, _ = strconv.ParseInt(v, 10, 64)
	case "fps":
		p.cur.FPS, _ = strconv.ParseFloat(v, 64)
	case "dup_frames":
		p.cur.DupFrames, _ = strconv.ParseInt(v, 10, 64)
	case "drop_frames":
		p.cur.DropFrames, _ = strconv.ParseInt(v, 10, 64)
	case "progress":
		out := p.cur
		out.End = v == "end"
		// Hacia Icecast ffmpeg informa "bitrate=N/A": se calcula con bytes y tiempo.
		if out.BitrateKbps <= 0 && out.OutTimeSec > 0 && out.TotalBytes > 0 {
			out.BitrateKbps = float64(out.TotalBytes) * 8 / 1000 / out.OutTimeSec
		}
		return out, true, false
	case "out_time", "stream_0_0_q", "stream_0_1_q", "stream_1_0_q":
	default:
		if strings.HasPrefix(k, "stream_") {
			return pr, false, false
		}
		// Una línea "algo=valor" desconocida: mejor mostrarla que perderla.
		return pr, false, true
	}
	return pr, false, false
}

// ---------- niveles ----------

// Level es el nivel de un canal en dBFS (-inf → Silence).
type Level struct {
	PeakDB float64 `json:"peak"`
	RMSDB  float64 `json:"rms"`
}

// Silence es el nivel que se informa cuando no hay señal (en lugar de -inf).
const Silence = -90.0

// LevelParser junta las líneas de ametadata: "frame:N …" abre un bloque y
// "lavfi.astats.<canal>.Peak_level=<dB>" lo llena.
type LevelParser struct {
	cur  []Level
	open bool
}

// Feed recibe una línea de stdout. Cuando empieza un bloque nuevo devuelve el anterior completo.
func (p *LevelParser) Feed(line string) (levels []Level, done bool) {
	line = strings.TrimSpace(line)
	if strings.HasPrefix(line, "frame:") {
		levels, done = p.flush()
		p.open = true
		return levels, done
	}
	rest, ok := strings.CutPrefix(line, "lavfi.astats.")
	if !ok || !p.open {
		return nil, false
	}
	key, val, ok := strings.Cut(rest, "=")
	if !ok {
		return nil, false
	}
	chStr, metric, ok := strings.Cut(key, ".")
	ch, err := strconv.Atoi(chStr)
	if !ok || err != nil || ch < 1 || ch > 8 {
		return nil, false
	}
	for len(p.cur) < ch {
		p.cur = append(p.cur, Level{PeakDB: Silence, RMSDB: Silence})
	}
	db := parseDB(val)
	switch metric {
	case "Peak_level":
		p.cur[ch-1].PeakDB = db
	case "RMS_level":
		p.cur[ch-1].RMSDB = db
	}
	return nil, false
}

// Flush devuelve el bloque pendiente (al terminar el proceso).
func (p *LevelParser) Flush() ([]Level, bool) { return p.flush() }

func (p *LevelParser) flush() ([]Level, bool) {
	if !p.open || len(p.cur) == 0 {
		return nil, false
	}
	out := p.cur
	p.cur, p.open = nil, false
	return out, true
}

func parseDB(v string) float64 {
	f, err := strconv.ParseFloat(strings.TrimSpace(v), 64)
	if err != nil || math.IsInf(f, 0) || math.IsNaN(f) || f < Silence {
		return Silence
	}
	return f
}

// SplitLines es un bufio.SplitFunc que corta en \n y en \r (ffmpeg usa \r en algunas líneas).
func SplitLines(data []byte, atEOF bool) (advance int, token []byte, err error) {
	if i := bytes.IndexAny(data, "\r\n"); i >= 0 {
		return i + 1, data[:i], nil
	}
	if atEOF && len(data) > 0 {
		return len(data), data, nil
	}
	return 0, nil, nil
}
