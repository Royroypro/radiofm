package library

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"
	"sync"
	"time"

	"radioclub25/estudio/internal/domain"
	"radioclub25/estudio/internal/platform/ffmpeg"
)

// Media es lo que el servicio necesita de ffmpeg (lo cumple ffmpeg.Tools; los tests usan un falso).
type Media interface {
	Probe(ctx context.Context, path string) (ffmpeg.Info, error)
	Analyze(ctx context.Context, path string) (ffmpeg.Loudness, error)
}

// AudioExtensions son las extensiones que se importan.
var AudioExtensions = map[string]bool{".mp3": true, ".wav": true, ".flac": true, ".ogg": true, ".m4a": true,
	".aac": true, ".wma": true, ".opus": true}

// VideoExtensions son los videos que se importan (modo TV). Un video con el
// mismo nombre que un audio de la misma carpeta queda como videoclip de esa
// pista; uno sin audio gemelo es una pista propia (su audio suena en la radio).
var VideoExtensions = map[string]bool{".mp4": true, ".mkv": true, ".webm": true, ".mov": true, ".avi": true,
	".mpg": true, ".mpeg": true, ".m4v": true, ".ts": true}

// IsVideo dice si la ruta es un video.
func IsVideo(p string) bool { return VideoExtensions[strings.ToLower(filepath.Ext(p))] }

// PairVideos empareja videos con audios del mismo nombre (sin extensión, sin
// distinguir mayúsculas) en la misma carpeta. Devuelve el video de cada audio
// y la lista final a importar (audios + videos sin gemelo).
func PairVideos(audios, videos []string) (map[string]string, []string) {
	key := func(p string) string {
		return strings.ToLower(strings.TrimSuffix(p, filepath.Ext(p)))
	}
	byKey := map[string]string{}
	for _, a := range audios {
		byKey[key(a)] = a
	}
	pair := map[string]string{}
	files := append([]string(nil), audios...)
	for _, v := range videos {
		if a, ok := byKey[key(v)]; ok {
			if _, dup := pair[a]; !dup {
				pair[a] = v
				continue
			}
		}
		pair[v] = v // video suelto: pista propia
		files = append(files, v)
	}
	return pair, files
}

// Service gestiona la biblioteca.
type Service struct {
	tracks domain.TrackRepository
	cats   domain.CategoryRepository
	media  Media
	rules  AnalysisRules
	log    *slog.Logger

	wake      chan struct{} // despierta al analizador
	mu        sync.Mutex
	analyzing string // pista que se analiza ahora
	pending   int
}

// New crea el servicio.
func New(tracks domain.TrackRepository, cats domain.CategoryRepository, media Media, log *slog.Logger) *Service {
	return &Service{tracks: tracks, cats: cats, media: media, rules: DefaultAnalysisRules(), log: log,
		wake: make(chan struct{}, 1)}
}

// ---------- Categorías ----------

// EnsureDefaultCategories crea las categorías del plan si no hay ninguna.
func (s *Service) EnsureDefaultCategories(ctx context.Context) error {
	list, err := s.cats.List(ctx)
	if err != nil || len(list) > 0 {
		return err
	}
	for _, c := range domain.DefaultCategories {
		c := c
		if err := s.cats.Create(ctx, &c); err != nil {
			return err
		}
	}
	return nil
}

// Categories devuelve las categorías.
func (s *Service) Categories(ctx context.Context) ([]domain.Category, error) { return s.cats.List(ctx) }

// SaveCategory crea o modifica una categoría.
func (s *Service) SaveCategory(ctx context.Context, c *domain.Category) error {
	c.Name = strings.TrimSpace(c.Name)
	if c.Name == "" {
		return domain.Invalid("name", "El nombre es obligatorio")
	}
	if !c.Kind.Valid() {
		return domain.Invalid("kind", "Tipo de categoría inválido")
	}
	if c.Color == "" {
		c.Color = "#223CB8"
	}
	if c.ParentID == c.ID && c.ID != 0 {
		return domain.Invalid("parent_id", "Una categoría no puede ser su propia subcategoría")
	}
	var err error
	if c.ID == 0 {
		err = s.cats.Create(ctx, c)
	} else {
		err = s.cats.Update(ctx, c)
	}
	if errors.Is(err, domain.ErrConflict) {
		return domain.Invalid("name", "Ya existe una categoría con ese nombre")
	}
	return err
}

// DeleteCategory borra una categoría vacía.
func (s *Service) DeleteCategory(ctx context.Context, id int64) error {
	err := s.cats.Delete(ctx, id)
	if errors.Is(err, domain.ErrConflict) {
		return domain.Invalid("category", "La categoría tiene pistas o subcategorías: muévalas antes de borrarla")
	}
	return err
}

// ---------- Pistas ----------

// Tracks lista pistas con filtro y paginación.
func (s *Service) Tracks(ctx context.Context, f domain.TrackFilter) ([]domain.Track, int, error) {
	return s.tracks.List(ctx, f)
}

// Track devuelve una pista.
func (s *Service) Track(ctx context.Context, id int64) (*domain.Track, error) {
	return s.tracks.Get(ctx, id)
}

// UpdateTrack guarda la edición manual de una pista (categoría, estado, vigencia, cues).
func (s *Service) UpdateTrack(ctx context.Context, t *domain.Track) error {
	old, err := s.tracks.Get(ctx, t.ID)
	if err != nil {
		return err
	}
	t.Title, t.Artist = strings.TrimSpace(t.Title), strings.TrimSpace(t.Artist)
	switch {
	case t.CueIn < 0 || (old.Duration > 0 && t.CueIn >= old.Duration):
		return domain.Invalid("cue_in", "El cue-in debe estar dentro de la pista")
	case t.CueOut != 0 && (t.CueOut <= t.CueIn || (old.Duration > 0 && t.CueOut > old.Duration)):
		return domain.Invalid("cue_out", "El cue-out debe ir después del cue-in y antes del final")
	case t.SegueAt != 0 && t.SegueAt <= t.CueIn:
		return domain.Invalid("segue_at", "El punto de mezcla debe ir después del cue-in")
	case t.ValidFrom != nil && t.ValidTo != nil && !t.ValidTo.After(*t.ValidFrom):
		return domain.Invalid("valid_to", "La fecha de fin debe ser posterior al inicio")
	case t.GainDB < -30 || t.GainDB > 12:
		return domain.Invalid("gain_db", "La ganancia debe estar entre −30 y +12 dB")
	case t.Energy < 0 || t.Energy > 5:
		return domain.Invalid("energy", "La energía va de 1 a 5")
	case t.BPM < 0 || t.BPM > 300:
		return domain.Invalid("bpm", "Tempo fuera de rango")
	}
	// Si cambió alguna marca, queda como ajuste manual (el análisis ya no la pisa).
	if t.CueIn != old.CueIn || t.CueOut != old.CueOut || t.SegueAt != old.SegueAt || t.Intro != old.Intro {
		t.MarksManual = true
	}
	return s.tracks.Update(ctx, t)
}

// ResetMarks descarta las marcas manuales y vuelve a analizar la pista.
func (s *Service) ResetMarks(ctx context.Context, id int64) error {
	t, err := s.tracks.Get(ctx, id)
	if err != nil {
		return err
	}
	t.MarksManual = false
	if err := s.tracks.Update(ctx, t); err != nil {
		return err
	}
	a := s.AnalyzeTrack(ctx, *t)
	if a.Err != "" {
		return fmt.Errorf("análisis: %s", a.Err)
	}
	return nil
}

// DeleteTrack quita una pista de la biblioteca (el archivo no se borra).
func (s *Service) DeleteTrack(ctx context.Context, id int64) error { return s.tracks.Delete(ctx, id) }

// CountByCategory cuenta pistas por categoría.
func (s *Service) CountByCategory(ctx context.Context) (map[int64]int, error) {
	return s.tracks.CountByCategory(ctx)
}

// ---------- Importación ----------

// ImportResult resume una importación.
type ImportResult struct {
	Added   int      `json:"added"`
	Updated int      `json:"updated"`
	Failed  int      `json:"failed"`
	Errors  []string `json:"errors"` // las primeras 20
}

// ImportProgress se informa durante la importación.
type ImportProgress struct {
	Done  int    `json:"done"`
	Total int    `json:"total"`
	File  string `json:"file"`
}

// ImportFolder importa los archivos de audio de dir (y subcarpetas si recursive)
// en la categoría indicada. progress puede ser nil.
func (s *Service) ImportFolder(ctx context.Context, dir string, categoryID int64, recursive bool,
	progress func(ImportProgress)) (ImportResult, error) {
	var res ImportResult
	if _, err := s.cats.Get(ctx, categoryID); err != nil {
		return res, domain.Invalid("category", "Elija una categoría válida")
	}
	audios, videos, err := listMedia(dir, recursive)
	videoOf, files := PairVideos(audios, videos)
	if err != nil {
		return res, domain.Invalid("dir", "No se pudo leer la carpeta: "+err.Error())
	}
	type out struct {
		path string
		info ffmpeg.Info
		err  error
	}
	jobs := make(chan string)
	results := make(chan out)
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ { // 4 ffprobe en paralelo
		wg.Add(1)
		go func() {
			defer wg.Done()
			for p := range jobs {
				info, err := s.media.Probe(ctx, p)
				results <- out{p, info, err}
			}
		}()
	}
	go func() {
		defer close(jobs)
		for _, f := range files {
			select {
			case jobs <- f:
			case <-ctx.Done():
				return
			}
		}
	}()
	go func() { wg.Wait(); close(results) }()

	done := 0
	for r := range results {
		done++
		if progress != nil {
			progress(ImportProgress{Done: done, Total: len(files), File: filepath.Base(r.path)})
		}
		if r.err != nil {
			res.Failed++
			if len(res.Errors) < 20 {
				res.Errors = append(res.Errors, filepath.Base(r.path)+": "+r.err.Error())
			}
			continue
		}
		t := &domain.Track{Path: r.path, Title: r.info.Title, Artist: r.info.Artist, Album: r.info.Album,
			Genre: r.info.Genre, Year: r.info.Year, Composer: r.info.Composer, CategoryID: categoryID, Duration: r.info.Duration,
			Active: true, VideoPath: videoOf[r.path]}
		if t.Title == "" {
			t.Artist, t.Title = domain.TitleFromFilename(r.path)
			if t.Artist == "" {
				t.Artist = r.info.Artist
			}
		}
		isNew, err := s.tracks.Upsert(ctx, t)
		switch {
		case err != nil:
			res.Failed++
			if len(res.Errors) < 20 {
				res.Errors = append(res.Errors, filepath.Base(r.path)+": "+err.Error())
			}
		case isNew:
			res.Added++
		default:
			res.Updated++
		}
	}
	s.WakeAnalyzer()
	return res, ctx.Err()
}

func listAudio(dir string, recursive bool) ([]string, error) {
	files, _, err := listMedia(dir, recursive)
	return files, err
}

// listMedia devuelve los audios y los videos de la carpeta.
func listMedia(dir string, recursive bool) ([]string, []string, error) {
	var files, videos []string
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // carpetas sin permiso: se saltan
		}
		if d.IsDir() {
			if p != dir && !recursive {
				return filepath.SkipDir
			}
			return nil
		}
		ext := strings.ToLower(filepath.Ext(p))
		if AudioExtensions[ext] || VideoExtensions[ext] {
			abs, err := filepath.Abs(p)
			if err == nil {
				if VideoExtensions[ext] {
					videos = append(videos, abs)
				} else {
					files = append(files, abs)
				}
			}
		}
		return nil
	})
	return files, videos, err
}

// CheckMissing marca las pistas cuyo archivo ya no existe (y desmarca las que volvieron).
func (s *Service) CheckMissing(ctx context.Context) (int, error) {
	list, _, err := s.tracks.List(ctx, domain.TrackFilter{Limit: 5000})
	if err != nil {
		return 0, err
	}
	missing := 0
	for _, t := range list {
		_, err := os.Stat(t.Path)
		gone := err != nil
		if gone {
			missing++
		}
		if gone != t.Missing {
			_ = s.tracks.SetMissing(ctx, t.ID, gone)
		}
	}
	return missing, nil
}

// ---------- Análisis en segundo plano ----------

// WakeAnalyzer avisa al analizador de que hay pistas nuevas.
func (s *Service) WakeAnalyzer() {
	select {
	case s.wake <- struct{}{}:
	default:
	}
}

// AnalyzerStatus es el estado de la cola de análisis.
type AnalyzerStatus struct {
	Current string `json:"current"`
	Pending int    `json:"pending"`
}

// AnalyzerState devuelve qué se analiza y cuántas faltan (aprox.).
func (s *Service) AnalyzerState() AnalyzerStatus {
	s.mu.Lock()
	defer s.mu.Unlock()
	return AnalyzerStatus{Current: s.analyzing, Pending: s.pending}
}

// RunAnalyzer analiza las pistas pendientes de una en una hasta que ctx se cancele.
func (s *Service) RunAnalyzer(ctx context.Context) {
	for ctx.Err() == nil {
		batch, err := s.tracks.PendingAnalysis(ctx, 50)
		if err != nil {
			s.log.Error("analizador: listar pendientes", "err", err)
		}
		s.mu.Lock()
		s.pending = len(batch)
		s.mu.Unlock()
		if len(batch) == 0 {
			select {
			case <-ctx.Done():
				return
			case <-s.wake:
			case <-time.After(time.Minute):
			}
			continue
		}
		for i, t := range batch {
			if ctx.Err() != nil {
				return
			}
			s.mu.Lock()
			s.analyzing, s.pending = t.Display(), len(batch)-i
			s.mu.Unlock()
			s.AnalyzeTrack(ctx, t)
		}
		s.mu.Lock()
		s.analyzing = ""
		s.mu.Unlock()
	}
}

// AnalyzeTrack mide una pista y guarda el resultado (también los errores, para no reintentar sin fin).
//
// Un archivo raro nunca debe tumbar el programa (el analizador corre al
// arrancar y volvería a caerse cada vez): si algo falla de forma inesperada,
// la pista queda marcada con el error y se sigue con la siguiente.
func (s *Service) AnalyzeTrack(ctx context.Context, t domain.Track) (a domain.Analysis) {
	defer func() {
		if p := recover(); p != nil {
			s.log.Error("analizador: error inesperado", "track", t.ID, "path", t.Path, "panic", p, "stack", string(debug.Stack()))
			a = domain.Analysis{Err: fmt.Sprintf("error inesperado al analizar: %v", p)}
			if err := s.tracks.SetAnalysis(context.WithoutCancel(ctx), t.ID, a); err != nil {
				s.log.Error("analizador: guardar", "track", t.ID, "err", err)
			}
		}
	}()
	if _, err := os.Stat(t.Path); err != nil {
		_ = s.tracks.SetMissing(ctx, t.ID, true)
		return a
	}
	if t.Analyzed && !t.TempoDone { // ya analizada: solo falta el tempo
		s.analyzeTempo(ctx, t, t.Loudness)
		return domain.Analysis{Loudness: t.Loudness, GainDB: t.GainDB, CueIn: t.CueIn, CueOut: t.CueOut, SegueAt: t.SegueAt,
			Envelope: t.Envelope, Ending: t.Ending, FadeOut: t.FadeOut, Err: t.AnalyzeErr}
	}
	l, err := s.media.Analyze(ctx, t.Path)
	if err != nil {
		if ctx.Err() != nil {
			return a
		}
		a.Err = err.Error()
	} else {
		a = Derive(l.Integrated, l.Momentary, t.Duration, s.rules)
	}
	if err := s.tracks.SetAnalysis(ctx, t.ID, a); err != nil {
		s.log.Error("analizador: guardar", "track", t.ID, "err", err)
	}
	if a.Err == "" && !t.TempoDone {
		s.analyzeTempo(ctx, t, a.Loudness)
	}
	return a
}

// TempoSource decodifica un tramo a mono (lo cumple ffmpeg.Tools; si el Media
// del servicio no lo implementa, no se estima el tempo).
type TempoSource interface {
	DecodeMono(ctx context.Context, path string, start, dur time.Duration, rate int) ([]float32, error)
}

// analyzeTempo estima el tempo con 60 s del medio de la pista (las intros y
// finales engañan) y la energía si no tenía. Siempre marca el intento.
func (s *Service) analyzeTempo(ctx context.Context, t domain.Track, loudness float64) {
	bpm := 0.0
	if src, ok := s.media.(TempoSource); ok && t.Duration >= 30*time.Second {
		start := max(t.Duration/2-30*time.Second, 0)
		if pcm, err := src.DecodeMono(ctx, t.Path, start, 60*time.Second, TempoRate); err == nil {
			bpm = EstimateBPM(pcm, TempoRate)
		} else if ctx.Err() != nil {
			return
		}
	}
	energy := 0
	if bpm > 0 {
		energy = EnergyFrom(bpm, loudness)
	}
	if err := s.tracks.SetTempo(ctx, t.ID, bpm, energy); err != nil {
		s.log.Error("analizador: guardar tempo", "track", t.ID, "err", err)
	}
}

// SetAttributes aplica atributos a todas las pistas del filtro (edición masiva).
func (s *Service) SetAttributes(ctx context.Context, f domain.TrackFilter, a domain.TrackAttributes) (int, error) {
	if a.Energy < 0 || a.Energy > 5 {
		return 0, domain.Invalid("energy", "La energía va de 1 a 5")
	}
	f.Limit, f.Offset = 5000, 0
	list, _, err := s.tracks.List(ctx, f)
	if err != nil {
		return 0, err
	}
	ids := make([]int64, 0, len(list))
	for _, t := range list {
		ids = append(ids, t.ID)
	}
	return s.tracks.SetAttributes(ctx, ids, a)
}

// EnsureCategory devuelve la categoría con ese nombre o la crea con ese tipo.
func (s *Service) EnsureCategory(ctx context.Context, name string, kind domain.CategoryKind) (int64, error) {
	cats, err := s.cats.List(ctx)
	if err != nil {
		return 0, err
	}
	for _, c := range cats {
		if strings.EqualFold(c.Name, name) {
			return c.ID, nil
		}
	}
	c := &domain.Category{Name: name, Kind: kind, Color: "#7a5cff"}
	if err := s.cats.Create(ctx, c); err != nil {
		return 0, err
	}
	return c.ID, nil
}

// ImportFile agrega (o actualiza) un archivo suelto a la biblioteca y despierta
// al analizador. Devuelve la pista.
func (s *Service) ImportFile(ctx context.Context, path string, categoryID int64) (*domain.Track, error) {
	info, err := s.media.Probe(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("leer %s: %w", filepath.Base(path), err)
	}
	t := &domain.Track{Path: path, Title: info.Title, Artist: info.Artist, Album: info.Album, Genre: info.Genre,
		Year: info.Year, Composer: info.Composer, CategoryID: categoryID, Duration: info.Duration, Active: true}
	if t.Title == "" {
		t.Artist, t.Title = domain.TitleFromFilename(path)
	}
	if _, err := s.tracks.Upsert(ctx, t); err != nil {
		return nil, err
	}
	s.WakeAnalyzer()
	return s.tracks.Get(ctx, t.ID)
}
