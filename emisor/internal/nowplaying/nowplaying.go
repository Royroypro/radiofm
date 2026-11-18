// Package nowplaying averigua qué canción suena para mandar su nombre a Icecast:
//
//   - Jazler RadioStar 2: lee el archivo que Jazler publica en
//     C:\Jazler RadioStar 2\Exports\NowOnAir.xml (o NowOnAir.txt) cada vez que
//     cambia de tema. Las cuñas, jingles y pausas cuentan como «sin canción».
//   - Otro programa (RadioDJ, ZaraRadio, mAirList…): un archivo de texto cuya
//     primera línea es lo que suena.
//   - Lista de canciones: un .m3u/.m3u8/.pls/.txt cargado a mano.
//
// Es lógica pura (bytes entran, títulos salen); quien vigila el archivo y
// envía el título es internal/ui. No escribe nada en Jazler.
package nowplaying

import (
	"bytes"
	"encoding/xml"
	"io"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

// Track es lo que suena ahora.
type Track struct {
	Title  string `json:"title"`   // «Artista - Título» listo para Icecast; "" si no suena una canción
	IsSong bool   `json:"is_song"` // false: cuña, jingle, pausa o Jazler detenido
	Kind   string `json:"kind"`    // tipo de evento según Jazler (song, spot, jingle, idle…) o "texto"
}

// jazlerIdle son textos que Jazler escribe cuando no está emitiendo música.
var jazlerIdle = []string{"sorry, service not available"}

// Parse interpreta el contenido de un archivo de «lo que suena» (XML de Jazler o texto).
func Parse(data []byte) (Track, bool) {
	text := Decode(data)
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return Track{}, false
	}
	if strings.HasPrefix(trimmed, "<?xml") || strings.HasPrefix(trimmed, "<Schedule") {
		return parseJazlerXML(trimmed)
	}
	line := strings.TrimSpace(strings.SplitN(trimmed, "\n", 2)[0])
	low := strings.ToLower(line)
	for _, idle := range jazlerIdle {
		if strings.HasPrefix(low, idle) {
			return Track{Kind: "idle"}, true
		}
	}
	return Track{Title: line, IsSong: true, Kind: "texto"}, true
}

type jazlerSchedule struct {
	Event struct {
		Type string `xml:"eventType,attr"`
		Song struct {
			Title  string `xml:"title,attr"`
			Artist struct {
				Name string `xml:"name,attr"`
			} `xml:"Artist"`
		} `xml:"Song"`
	} `xml:"Event"`
}

func parseJazlerXML(s string) (Track, bool) {
	var sch jazlerSchedule
	dec := xml.NewDecoder(strings.NewReader(s))
	dec.CharsetReader = func(_ string, r io.Reader) (io.Reader, error) {
		return r, nil // ya se pasó a UTF-8 en Decode
	}
	if err := dec.Decode(&sch); err != nil {
		return Track{}, false // archivo a medio escribir: se reintenta en la próxima vuelta
	}
	ev := sch.Event
	kind := strings.ToLower(strings.TrimSpace(ev.Type))
	title := strings.TrimSpace(ev.Song.Title)
	artist := strings.TrimSpace(ev.Song.Artist.Name)
	if title == "" || (kind != "" && kind != "song") {
		if kind == "" {
			kind = "idle"
		}
		return Track{Kind: kind}, true
	}
	full := title
	if artist != "" {
		full = artist + " - " + title
	}
	return Track{Title: full, IsSong: true, Kind: "song"}, true
}

// Decode pasa a UTF-8: quita el BOM y, si no es UTF-8 válido, lo lee como
// Windows-1252 (programas viejos como Jazler escriben así los acentos y la ñ).
func Decode(b []byte) string {
	b = bytes.TrimPrefix(b, []byte("\xef\xbb\xbf"))
	if utf8.Valid(b) {
		return strings.ReplaceAll(string(b), "\r\n", "\n")
	}
	var sb strings.Builder
	for _, c := range b {
		if c >= 0x80 && c <= 0x9f {
			sb.WriteRune(cp1252[c-0x80])
		} else {
			sb.WriteRune(rune(c)) // el resto de Windows-1252 coincide con Latin-1
		}
	}
	return strings.ReplaceAll(sb.String(), "\r\n", "\n")
}

// cp1252 son los caracteres de 0x80–0x9F en Windows-1252.
var cp1252 = [32]rune{'€', '\u0081', '‚', 'ƒ', '„', '…', '†', '‡', 'ˆ', '‰', 'Š', '‹', 'Œ', '\u008d', 'Ž', '\u008f',
	'\u0090', '‘', '’', '“', '”', '•', '–', '—', '˜', '™', 'š', '›', 'œ', '\u009d', 'ž', 'Ÿ'}

// ---------- listas de canciones ----------

var audioExt = map[string]bool{".mp3": true, ".wav": true, ".aac": true, ".m4a": true, ".ogg": true, ".opus": true,
	".flac": true, ".wma": true, ".mp2": true}

// ParsePlaylist lee una lista de canciones y devuelve los títulos en orden:
//   - .m3u/.m3u8: el texto de #EXTINF o, si no hay, el nombre del archivo;
//   - .pls: TitleN o, si no hay, FileN;
//   - .txt/.csv u otro: una canción por línea (si la línea es un archivo de audio, su nombre).
func ParsePlaylist(name string, data []byte) []string {
	text := Decode(data)
	ext := strings.ToLower(filepath.Ext(name))
	var out []string
	add := func(s string) {
		if s = strings.TrimSpace(s); s != "" {
			out = append(out, s)
		}
	}
	switch ext {
	case ".pls":
		titles, files := map[string]string{}, map[string]string{}
		var order []string
		for _, line := range strings.Split(text, "\n") {
			k, v, ok := strings.Cut(strings.TrimSpace(line), "=")
			if !ok {
				continue
			}
			lk := strings.ToLower(k)
			switch {
			case strings.HasPrefix(lk, "title"):
				titles[lk[5:]] = v
			case strings.HasPrefix(lk, "file"):
				files[lk[4:]] = v
				order = append(order, lk[4:])
			}
		}
		for _, n := range order {
			if t := titles[n]; t != "" {
				add(t)
			} else {
				add(titleFromFile(files[n]))
			}
		}
	default:
		pending := ""
		for _, line := range strings.Split(text, "\n") {
			line = strings.TrimSpace(line)
			switch {
			case line == "" || line == "#EXTM3U":
			case strings.HasPrefix(line, "#EXTINF:"):
				if _, t, ok := strings.Cut(line, ","); ok {
					pending = strings.TrimSpace(t)
				}
			case strings.HasPrefix(line, "#"):
			default:
				if pending != "" {
					add(pending)
					pending = ""
				} else if looksLikeFile(line) {
					add(titleFromFile(line))
				} else {
					add(line)
				}
			}
		}
	}
	return out
}

func looksLikeFile(s string) bool {
	return audioExt[strings.ToLower(filepath.Ext(s))]
}

// titleFromFile convierte «D:\Música\Grupo 5 - Motor y motivo.mp3» en «Grupo 5 - Motor y motivo».
func titleFromFile(p string) string {
	p = strings.ReplaceAll(p, "\\", "/")
	base := p[strings.LastIndex(p, "/")+1:]
	base = strings.TrimSuffix(base, filepath.Ext(base))
	return strings.TrimSpace(strings.ReplaceAll(base, "_", " "))
}
