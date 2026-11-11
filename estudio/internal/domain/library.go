package domain

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"time"
)

// Errores de dominio comunes.
var (
	ErrNotFound = errors.New("no encontrado")
	ErrConflict = errors.New("conflicto con datos existentes")
)

// ValidationError describe un dato de entrada inválido (Field = campo del formulario).
type ValidationError struct{ Field, Msg string }

func (e *ValidationError) Error() string { return e.Field + ": " + e.Msg }

// Invalid crea un *ValidationError.
func Invalid(field, msg string) error { return &ValidationError{Field: field, Msg: msg} }

// CategoryKind indica para qué sirve una categoría (cambia cómo se programa y se reporta).
type CategoryKind string

const (
	KindMusic     CategoryKind = "music"     // canciones (rotación con separación de artista/título)
	KindJingle    CategoryKind = "jingle"    // jingles, IDs, cortinas
	KindSpot      CategoryKind = "spot"      // cuñas (vienen del panel web)
	KindVoice     CategoryKind = "voice"     // locuciones de hora/temperatura
	KindEmergency CategoryKind = "emergency" // suena si hay silencio
	KindFiller    CategoryKind = "filler"    // relleno para completar la hora
)

// CategoryKinds en el orden de la interfaz.
var CategoryKinds = []CategoryKind{KindMusic, KindJingle, KindSpot, KindVoice, KindEmergency, KindFiller}

// Valid indica si el tipo es conocido.
func (k CategoryKind) Valid() bool {
	for _, x := range CategoryKinds {
		if x == k {
			return true
		}
	}
	return false
}

// IsMusic indica si las pistas de este tipo son canciones (se informan como "lo que suena").
func (k CategoryKind) IsMusic() bool { return k == KindMusic || k == KindFiller || k == KindEmergency }

// Category agrupa pistas para la rotación. ParentID permite subcategorías (0 = raíz).
type Category struct {
	ID       int64        `json:"id"`
	Name     string       `json:"name"`
	ParentID int64        `json:"parent_id"`
	Kind     CategoryKind `json:"kind"`
	Color    string       `json:"color"` // #RRGGBB para la interfaz
}

// DefaultCategories son las categorías iniciales del plan.
var DefaultCategories = []Category{
	{Name: "Éxitos", Kind: KindMusic, Color: "#FF1E1E"},
	{Name: "Retro", Kind: KindMusic, Color: "#FF8A00"},
	{Name: "Salsa", Kind: KindMusic, Color: "#E6B800"},
	{Name: "Cumbia", Kind: KindMusic, Color: "#2EAD5B"},
	{Name: "Baladas", Kind: KindMusic, Color: "#B455D6"},
	{Name: "Jingles", Kind: KindJingle, Color: "#223CB8"},
	{Name: "Cortinas", Kind: KindJingle, Color: "#3D7BFF"},
	{Name: "IDs", Kind: KindJingle, Color: "#5AA9FF"},
	{Name: "Cuñas", Kind: KindSpot, Color: "#9AA3D0"},
	{Name: "Hora", Kind: KindVoice, Color: "#00B3A4"},
	{Name: "Temperatura", Kind: KindVoice, Color: "#00C2D1"},
	{Name: "Emergencia", Kind: KindEmergency, Color: "#8B0000"},
	{Name: "Relleno", Kind: KindFiller, Color: "#6B7280"},
}

// Track es una pista de la biblioteca.
type Track struct {
	ID         int64         `json:"id"`
	Path       string        `json:"path"` // ruta absoluta del archivo
	Title      string        `json:"title"`
	Artist     string        `json:"artist"`
	Album      string        `json:"album"`
	Genre      string        `json:"genre"`
	Composer   string        `json:"composer"`   // compositor (etiqueta del archivo; informe APDAYC)
	VideoPath  string        `json:"video_path"` // video para la TV (videoclip o promo); "" = sin video
	Year       string        `json:"year"`
	CategoryID int64         `json:"category_id"`
	Duration   time.Duration `json:"duration"`
	Active     bool          `json:"active"`
	ValidFrom  *time.Time    `json:"valid_from"` // nil = sin fecha de inicio
	ValidTo    *time.Time    `json:"valid_to"`   // nil = sin fecha de fin

	// Análisis (ver service/library).
	Analyzed   bool          `json:"analyzed"`
	Loudness   float64       `json:"loudness"` // LUFS integrado
	GainDB     float64       `json:"gain_db"`  // ganancia para normalizar
	CueIn      time.Duration `json:"cue_in"`
	CueOut     time.Duration `json:"cue_out"`  // 0 = fin del archivo
	SegueAt    time.Duration `json:"segue_at"` // punto de mezcla desde el inicio del archivo; 0 = CueOut − crossfade
	Intro      time.Duration `json:"intro"`    // duración de la intro (para hablar encima)
	AnalyzeErr string        `json:"analyze_err"`
	Envelope   string        `json:"envelope"` // forma de onda: 200 puntos en base 36 ("0" silencio … "z" máximo)
	Ending     Ending        `json:"ending"`   // cómo termina: fundido o seco (decide la transición)
	FadeOut    time.Duration `json:"fade_out"` // duración del fundido final propio (0 = final seco)
	// MarksManual: el operador ajustó cue-in, intro, mezcla o cue-out a mano;
	// volver a analizar no los pisa (solo actualiza sonoridad y forma de onda).
	MarksManual bool `json:"marks_manual"`

	// Atributos musicales (reglas de flujo de la rotación). 0 / "" = sin dato.
	BPM       float64 `json:"bpm"`        // tempo (estimado en el análisis; corregible)
	Energy    int     `json:"energy"`     // 1 calma … 5 muy movida (estimada; corregible)
	Voice     string  `json:"voice"`      // male | female | group | instrumental
	Language  string  `json:"language"`   // es, en, pt… (texto libre corto)
	Mood      string  `json:"mood"`       // ánimo (texto libre: alegre, romántica, nostálgica…)
	TempoDone bool    `json:"tempo_done"` // ya se intentó estimar el tempo

	Missing      bool       `json:"missing"` // el archivo ya no está
	LastPlayedAt *time.Time `json:"last_played_at"`
	PlayCount    int        `json:"play_count"`
	CreatedAt    time.Time  `json:"created_at"`
}

// End devuelve dónde termina la parte útil (CueOut o la duración).
func (t Track) End() time.Duration {
	if t.CueOut > 0 && t.CueOut <= t.Duration {
		return t.CueOut
	}
	return t.Duration
}

// PlayLength es lo que suena realmente: de CueIn a End.
func (t Track) PlayLength() time.Duration { return max(t.End()-t.CueIn, 0) }

// Segue devuelve el punto de mezcla relativo al inicio de la reproducción (CueIn),
// usando crossfade si la pista no tiene uno analizado.
func (t Track) Segue(crossfade time.Duration) time.Duration {
	s := t.SegueAt
	if s <= t.CueIn || s > t.End() {
		s = t.End() - crossfade
	}
	return max(s-t.CueIn, 0)
}

// Display devuelve "Artista - Título" o el nombre del archivo.
func (t Track) Display() string {
	switch {
	case t.Artist != "" && t.Title != "":
		return t.Artist + " - " + t.Title
	case t.Title != "":
		return t.Title
	}
	return strings.TrimSuffix(filepath.Base(t.Path), filepath.Ext(t.Path))
}

// Available indica si la pista puede programarse en el instante t.
func (t Track) Available(at time.Time) bool {
	if !t.Active || t.Missing {
		return false
	}
	if t.ValidFrom != nil && at.Before(*t.ValidFrom) {
		return false
	}
	if t.ValidTo != nil && !at.Before(*t.ValidTo) {
		return false
	}
	return true
}

// TitleFromFilename deduce artista y título de "Artista - Título.mp3".
func TitleFromFilename(path string) (artist, title string) {
	name := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	name = strings.ReplaceAll(name, "_", " ")
	if a, b, ok := strings.Cut(name, " - "); ok {
		return strings.TrimSpace(a), strings.TrimSpace(b)
	}
	return "", strings.TrimSpace(name)
}

// TrackFilter filtra la lista de pistas. Los campos vacíos no filtran.
type TrackFilter struct {
	CategoryID int64
	Query      string // busca en título, artista, álbum y ruta
	OnlyActive bool
	Limit      int
	Offset     int
}

// Analysis es el resultado del análisis de una pista.
type Analysis struct {
	Loudness float64
	GainDB   float64
	CueIn    time.Duration
	CueOut   time.Duration
	SegueAt  time.Duration
	Envelope string
	Ending   Ending
	FadeOut  time.Duration
	Err      string
}

// Ending es cómo termina una pista.
type Ending string

const (
	EndingFade Ending = "fade" // fundido: la siguiente entra encima del fundido
	EndingCold Ending = "cold" // seco (golpe final): la siguiente entra casi al final, sin pisarlo
)

// TrackRepository persiste las pistas.
type TrackRepository interface {
	// Upsert crea la pista o, si la ruta ya existe, actualiza sus etiquetas y duración.
	// Devuelve true si era nueva.
	Upsert(ctx context.Context, t *Track) (bool, error)
	Get(ctx context.Context, id int64) (*Track, error)
	List(ctx context.Context, f TrackFilter) ([]Track, int, error) // lista + total sin paginar
	Update(ctx context.Context, t *Track) error
	Delete(ctx context.Context, id int64) error
	PendingAnalysis(ctx context.Context, limit int) ([]Track, error)
	SetAnalysis(ctx context.Context, id int64, a Analysis) error
	SetMissing(ctx context.Context, id int64, missing bool) error
	ByCategory(ctx context.Context, categoryID int64) ([]Track, error)
	CountByCategory(ctx context.Context) (map[int64]int, error)
	// SetTempo guarda el tempo estimado (y la energía si no tenía) y marca el intento.
	SetTempo(ctx context.Context, id int64, bpm float64, energy int) error
	// SetAttributes aplica a varias pistas los atributos no vacíos de a (edición masiva).
	SetAttributes(ctx context.Context, ids []int64, a TrackAttributes) (int, error)
}

// TrackAttributes son los atributos de una edición masiva (los vacíos no se tocan).
type TrackAttributes struct {
	Genre    string `json:"genre"`
	Year     string `json:"year"`
	Energy   int    `json:"energy"`
	Voice    string `json:"voice"`
	Language string `json:"language"`
	Mood     string `json:"mood"`
}

// CategoryRepository persiste las categorías.
type CategoryRepository interface {
	List(ctx context.Context) ([]Category, error)
	Get(ctx context.Context, id int64) (*Category, error)
	Create(ctx context.Context, c *Category) error
	Update(ctx context.Context, c *Category) error
	Delete(ctx context.Context, id int64) error // falla con ErrConflict si tiene pistas
}

// PlayKind clasifica lo que sonó.
type PlayKind string

const (
	PlayMusic  PlayKind = "music"
	PlayJingle PlayKind = "jingle"
	PlaySpot   PlayKind = "spot"
	PlayVoice  PlayKind = "voice"
	PlayCart   PlayKind = "cart"
	PlayRelay  PlayKind = "relay" // enlace a cadena (stream externo)
)

// Play es una emisión registrada (as-run).
type Play struct {
	ID        int64         `json:"id"`
	TrackID   int64         `json:"track_id"` // 0 si no venía de la biblioteca (cuña del panel)
	SpotID    int64         `json:"spot_id"`  // cuña del panel web (0 si no)
	Kind      PlayKind      `json:"kind"`
	Artist    string        `json:"artist"`
	Title     string        `json:"title"`
	StartedAt time.Time     `json:"started_at"`
	Duration  time.Duration `json:"duration"` // lo que sonó
	Complete  bool          `json:"complete"`
	UUID      string        `json:"uuid"`
}

// PlayRepository guarda el historial de emisiones.
type PlayRepository interface {
	Add(ctx context.Context, p *Play) error
	Finish(ctx context.Context, id int64, d time.Duration, complete bool) error
	Since(ctx context.Context, since time.Time) ([]Play, error)
	Between(ctx context.Context, from, to time.Time) ([]Play, error)
}
