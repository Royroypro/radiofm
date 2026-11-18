package nowplaying

import (
	"reflect"
	"testing"
)

// jazlerIdleXML es el NowOnAir.xml real de esta PC con Jazler detenido.
const jazlerIdleXML = "<?xml version=\"1.0\" encoding=\"utf-8\"?>\r\n<Schedule System=\"Jazler\">\r\n" +
	" <Event status=\"happening\" startTime=\"12:56:53\" eventType=\"idle\">\r\n" +
	"    <Announcement Display=\"Sorry, service not available. Try again later.\"/>\r\n" +
	"  <Song title=\"\">\r\n   <Artist name=\"\">\r\n   </Artist>\r\n    <Jazler ID=\"\"/>\r\n" +
	"    <PlayLister ID=\"\"/>\r\n    <Media runTime=\"\"/>\r\n    <Expire Time=\"\"/>\r\n  </Song>\r\n </Event>\r\n</Schedule>\r\n"

func jazlerSong(kind, title, artist string) []byte {
	return []byte(`<?xml version="1.0" encoding="utf-8"?>
<Schedule System="Jazler">
 <Event status="happening" startTime="20:15:02" eventType="` + kind + `">
  <Announcement Display="Ahora: ` + artist + `"/>
  <Song title="` + title + `">
   <Artist name="` + artist + `"></Artist>
   <Jazler ID="1234"/><PlayLister ID=""/><Media runTime="3:45"/><Expire Time="20:18:47"/>
  </Song>
 </Event>
</Schedule>`)
}

func TestParseJazler(t *testing.T) {
	if tr, ok := Parse([]byte(jazlerIdleXML)); !ok || tr.IsSong || tr.Title != "" || tr.Kind != "idle" {
		t.Fatalf("Jazler detenido: %+v %v", tr, ok)
	}
	tr, ok := Parse(jazlerSong("song", "Motor y motivo", "Grupo 5"))
	if !ok || !tr.IsSong || tr.Title != "Grupo 5 - Motor y motivo" {
		t.Fatalf("canción: %+v", tr)
	}
	// Las cuñas y jingles no son canciones (se usa el texto de pausa).
	for _, k := range []string{"spot", "jingle", "sweeper"} {
		if tr, ok := Parse(jazlerSong(k, "Cuña Ferretería", "")); !ok || tr.IsSong || tr.Kind != k {
			t.Fatalf("%s: %+v", k, tr)
		}
	}
	// Sin artista: solo el título.
	if tr, _ := Parse(jazlerSong("song", "Himno de Huacho", "")); tr.Title != "Himno de Huacho" {
		t.Fatalf("sin artista: %+v", tr)
	}
	// Archivo a medio escribir: no se entiende (se reintenta), no es «pausa».
	if _, ok := Parse([]byte(`<?xml version="1.0"?><Schedule><Event eventType="song"><Song title="Me`)); ok {
		t.Fatal("XML cortado no debe dar resultado")
	}
}

func TestParseText(t *testing.T) {
	if tr, ok := Parse([]byte("\xef\xbb\xbfAgua Marina - Tu amor fue una mentira\r\n")); !ok || tr.Title != "Agua Marina - Tu amor fue una mentira" {
		t.Fatalf("texto: %+v", tr)
	}
	// NowOnAir.txt de Jazler detenido.
	if tr, ok := Parse([]byte("Sorry, service not available. Try again later.\r\n")); !ok || tr.IsSong {
		t.Fatalf("Jazler detenido (txt): %+v", tr)
	}
	if _, ok := Parse([]byte("  \r\n")); ok {
		t.Fatal("vacío = sin dato")
	}
}

func TestDecodeWindows1252(t *testing.T) {
	// «Corazón Serrano – Mía» en Windows-1252 (ó=F3, –=96, í=ED), como lo escribe un programa viejo.
	b := []byte{'C', 'o', 'r', 'a', 'z', 0xF3, 'n', ' ', 'S', 'e', 'r', 'r', 'a', 'n', 'o', ' ', 0x96, ' ', 'M', 0xED, 'a'}
	if got := Decode(b); got != "Corazón Serrano – Mía" {
		t.Fatalf("Decode = %q", got)
	}
	x := append([]byte(`<?xml version="1.0" encoding="utf-8"?><Schedule><Event eventType="song"><Song title="`),
		0xD1, 'o', 0xF1, 'o')
	x = append(x, []byte(`"><Artist name="Los Pap"></Artist></Song></Event></Schedule>`)...)
	if tr, _ := Parse(x); tr.Title != "Los Pap - Ñoño" {
		t.Fatalf("XML con acentos en Windows-1252: %+v", tr)
	}
}

func TestParsePlaylist(t *testing.T) {
	m3u := "#EXTM3U\r\n#EXTINF:215,Grupo 5 - Motor y motivo\r\nD:\\Musica\\grupo5.mp3\r\n" +
		"D:\\Musica\\Agua_Marina - Paloma Ajena.mp3\r\n#EXTINF:180,Corazón Serrano - Mía\r\nhttp://x/y.mp3\r\n"
	want := []string{"Grupo 5 - Motor y motivo", "Agua Marina - Paloma Ajena", "Corazón Serrano - Mía"}
	if got := ParsePlaylist("lista.m3u", []byte(m3u)); !reflect.DeepEqual(got, want) {
		t.Fatalf("m3u = %q", got)
	}
	pls := "[playlist]\nFile1=C:\\a\\Los Mirlos - Sonido amazónico.mp3\nFile2=C:\\a\\b.mp3\nTitle2=Armonía 10 - Fruta prohibida\nNumberOfEntries=2\n"
	if got := ParsePlaylist("x.PLS", []byte(pls)); !reflect.DeepEqual(got, []string{"Los Mirlos - Sonido amazónico", "Armonía 10 - Fruta prohibida"}) {
		t.Fatalf("pls = %q", got)
	}
	txt := "Hermanos Yaipén - Te amo\n\n  Chacalón - Soy provinciano  \nE:\\pistas\\Juaneco y su combo - Vacilando con ayahuasca.wav\n"
	if got := ParsePlaylist("canciones.txt", []byte(txt)); !reflect.DeepEqual(got, []string{"Hermanos Yaipén - Te amo",
		"Chacalón - Soy provinciano", "Juaneco y su combo - Vacilando con ayahuasca"}) {
		t.Fatalf("txt = %q", got)
	}
}
