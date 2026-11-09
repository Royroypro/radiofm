package onair

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"radioclub25/estudio/internal/audio"
	"radioclub25/estudio/internal/domain"
	"radioclub25/estudio/internal/service/timeannounce"
)

// --- falsos ---

type memLog struct {
	mu    sync.Mutex
	items []domain.LogItem
}

func (m *memLog) Upcoming(_ context.Context, _ time.Time, limit int) ([]domain.LogItem, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []domain.LogItem
	for _, it := range m.items {
		if it.Status == domain.LogPending || it.Status == domain.LogPlaying {
			out = append(out, it)
		}
	}
	return out, nil
}

func (m *memLog) SetStatus(_ context.Context, id int64, s domain.LogStatus) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := range m.items {
		if m.items[i].ID == id {
			m.items[i].Status = s
		}
	}
	return nil
}

func (m *memLog) status(id int64) domain.LogStatus {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, it := range m.items {
		if it.ID == id {
			return it.Status
		}
	}
	return ""
}

type memTracks struct{ byID map[int64]domain.Track }

func (m memTracks) Get(_ context.Context, id int64) (*domain.Track, error) {
	t, ok := m.byID[id]
	if !ok {
		return nil, domain.ErrNotFound
	}
	return &t, nil
}

func (m memTracks) ByCategory(_ context.Context, cat int64) ([]domain.Track, error) {
	var out []domain.Track
	for _, t := range m.byID {
		if t.CategoryID == cat {
			out = append(out, t)
		}
	}
	return out, nil
}

type noCats struct{}

func (noCats) List(context.Context) ([]domain.Category, error) {
	return []domain.Category{{ID: 1, Kind: domain.KindMusic}, {ID: 5, Kind: domain.KindJingle, Name: "Cortinas"}}, nil
}

type memSpots struct{ spots map[int64][]domain.Spot }

func (m memSpots) TandaSpots(_ context.Context, id int64) ([]domain.Spot, error) {
	return m.spots[id], nil
}
func (m memSpots) CortinaCategory(context.Context) int64 { return 5 }

type memPlays struct {
	mu    sync.Mutex
	plays []domain.Play
}

func (m *memPlays) Add(_ context.Context, p *domain.Play) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	p.ID = int64(len(m.plays) + 1)
	m.plays = append(m.plays, *p)
	return nil
}

func (m *memPlays) Finish(_ context.Context, id int64, d time.Duration, complete bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.plays[id-1].Duration, m.plays[id-1].Complete = d, complete
	return nil
}
func (m *memPlays) Since(context.Context, time.Time) ([]domain.Play, error) { return nil, nil }
func (m *memPlays) Between(context.Context, time.Time, time.Time) ([]domain.Play, error) {
	return nil, nil
}

func (m *memPlays) titles() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []string
	for _, p := range m.plays {
		out = append(out, p.Title)
	}
	return out
}

// decodeSine: cada "archivo" es un seno de 1,5 s (las duraciones las fija Length).
func decodeSine(context.Context, string, time.Duration) (audio.Source, error) {
	return audio.NewMemorySource(audio.Sine(440, 0.2, 3)), nil
}

type env struct {
	svc    *Service
	log    *memLog
	plays  *memPlays
	engine *audio.Engine
	cancel context.CancelFunc
}

func newEnv(t *testing.T, items []domain.LogItem, tracks map[int64]domain.Track, spots map[int64][]domain.Spot) *env {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	e := audio.NewEngine(audio.Config{LimiterCeilingDB: -1})
	go audio.RunClock(ctx, e)
	lg, pl := &memLog{items: items}, &memPlays{}
	svc := New(Deps{
		Engine: e, Log: lg, Tracks: memTracks{tracks}, Categories: noCats{}, Spots: memSpots{spots}, Plays: pl,
		Decode: decodeSine, Voice: func() timeannounce.Files { return timeannounce.Files{} },
		Temperature: func(context.Context) (float64, error) { return 20, nil },
		Location:    func() *time.Location { return time.UTC },
		Options:     func() Options { return Options{Crossfade: 300 * time.Millisecond, HardFade: 200 * time.Millisecond} },
	}, Hooks{})
	go svc.Run(ctx)
	t.Cleanup(cancel)
	return &env{svc: svc, log: lg, plays: pl, engine: e, cancel: cancel}
}

func track(id int64, title string, length, segue time.Duration) domain.Track {
	return domain.Track{ID: id, Title: title, Artist: "A", CategoryID: 1, Active: true, Duration: length,
		CueOut: length, SegueAt: segue}
}

func waitFor(t *testing.T, d time.Duration, cond func() bool) bool {
	t.Helper()
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		if cond() {
			return true
		}
		time.Sleep(20 * time.Millisecond)
	}
	return false
}

func TestPlaysListWithTanda(t *testing.T) {
	now := time.Now()
	tracks := map[int64]domain.Track{
		1: track(1, "uno", 1200*time.Millisecond, 900*time.Millisecond),
		2: track(2, "dos", 1200*time.Millisecond, 900*time.Millisecond),
		9: {ID: 9, Title: "cortina", CategoryID: 5, Active: true, Duration: 300 * time.Millisecond},
	}
	spots := map[int64][]domain.Spot{11: {{ID: 1, Advertiser: "Ferretería", Name: "cuña", Path: "c.mp3", Duration: 400 * time.Millisecond}}}
	items := []domain.LogItem{
		{ID: 10, Kind: domain.SlotCategory, TrackID: 1, At: now, Length: time.Second, Status: domain.LogPending},
		{ID: 11, Kind: domain.SlotSpots, At: now.Add(time.Second), Length: time.Second, Status: domain.LogPending},
		{ID: 12, Kind: domain.SlotCategory, TrackID: 2, At: now.Add(2 * time.Second), Length: time.Second, Status: domain.LogPending},
	}
	e := newEnv(t, items, tracks, spots)
	if err := e.svc.Start(); err != nil {
		t.Fatal(err)
	}
	ok := waitFor(t, 8*time.Second, func() bool { return e.log.status(12) == domain.LogPlayed })
	got := e.plays.titles()
	want := []string{"uno", "cortina", "cuña", "cortina", "dos"}
	// al acabarse la playlist el aire pone relleno (no queda en silencio): solo se compara lo programado
	if !ok || len(got) < len(want) {
		t.Fatalf("emisiones = %v, quería %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("emisiones = %v, quería %v", got, want)
		}
	}
	if e.log.status(10) != domain.LogPlayed || e.log.status(11) != domain.LogPlayed {
		t.Fatal("los elementos deben quedar como emitidos")
	}
	e.plays.mu.Lock()
	defer e.plays.mu.Unlock()
	for _, p := range e.plays.plays[:len(want)] {
		if !p.Complete {
			t.Fatalf("%q debe quedar completa (duración %v)", p.Title, p.Duration)
		}
	}
	if e.plays.plays[2].SpotID != 1 || e.plays.plays[2].Kind != domain.PlaySpot {
		t.Fatalf("la cuña debe registrarse como cuña: %+v", e.plays.plays[2])
	}
}

func TestHardEventCutsAndLiveMode(t *testing.T) {
	now := time.Now()
	tracks := map[int64]domain.Track{
		1: track(1, "larga", 20*time.Second, 19*time.Second),
		2: track(2, "evento", 2*time.Second, 0),
		3: track(3, "después", 20*time.Second, 19*time.Second),
	}
	items := []domain.LogItem{
		{ID: 1, Kind: domain.SlotCategory, TrackID: 1, At: now, Length: 20 * time.Second, Status: domain.LogPending},
		{ID: 2, Kind: domain.SlotTrack, TrackID: 2, Hard: true, At: now.Add(1500 * time.Millisecond), Length: 2 * time.Second, Status: domain.LogPending},
		{ID: 3, Kind: domain.SlotCategory, TrackID: 3, At: now.Add(3500 * time.Millisecond), Length: 20 * time.Second, Status: domain.LogPending},
	}
	e := newEnv(t, items, tracks, nil)
	_ = e.svc.Start()
	if !waitFor(t, 5*time.Second, func() bool { return e.log.status(2) == domain.LogPlaying }) {
		t.Fatalf("el evento duro debe arrancar a su hora; emisiones %v", e.plays.titles())
	}
	elapsed := time.Since(now)
	if elapsed < 1300*time.Millisecond || elapsed > 2500*time.Millisecond {
		t.Fatalf("el evento duro arrancó a los %v (quería ~1,5 s)", elapsed)
	}
	if !waitFor(t, 2*time.Second, func() bool {
		e.plays.mu.Lock()
		defer e.plays.mu.Unlock()
		return e.plays.plays[0].Duration > 0
	}) {
		t.Fatal("la pista cortada debe cerrarse")
	}
	e.plays.mu.Lock()
	cut := e.plays.plays[0]
	e.plays.mu.Unlock()
	if cut.Complete {
		t.Fatalf("la pista fundida por el evento duro no está completa: %+v", cut)
	}
	// al terminar el evento sigue "después"; pasar a en vivo funde la música
	if !waitFor(t, 5*time.Second, func() bool { return e.log.status(3) == domain.LogPlaying }) {
		t.Fatalf("tras el evento debe seguir la playlist: %v", e.plays.titles())
	}
	if err := e.svc.SetMode(domain.ModeLive); err != nil {
		t.Fatal(err)
	}
	if !waitFor(t, 3*time.Second, func() bool {
		st := e.engine.Status()
		return !st.Decks[0].Playing && !st.Decks[1].Playing
	}) {
		t.Fatal("en vivo la música debe pararse")
	}
	if snap := e.svc.Snapshot(); snap.Mode != domain.ModeLive || !snap.Running {
		t.Fatalf("snapshot = %+v", snap)
	}
}

func TestAssistedWaitsForOperator(t *testing.T) {
	now := time.Now()
	tracks := map[int64]domain.Track{1: track(1, "a", 800*time.Millisecond, 600*time.Millisecond), 2: track(2, "b", 800*time.Millisecond, 600*time.Millisecond)}
	items := []domain.LogItem{
		{ID: 1, Kind: domain.SlotCategory, TrackID: 1, At: now, Length: time.Second, Status: domain.LogPending},
		{ID: 2, Kind: domain.SlotCategory, TrackID: 2, At: now.Add(time.Second), Length: time.Second, Status: domain.LogPending},
	}
	e := newEnv(t, items, tracks, nil)
	_ = e.svc.SetMode(domain.ModeAssisted)
	_ = e.svc.Start()
	if !waitFor(t, 3*time.Second, func() bool { return e.svc.Snapshot().Waiting }) {
		t.Fatal("en asistido debe quedar esperando al terminar")
	}
	if e.log.status(2) != domain.LogPending {
		t.Fatal("no debe avanzar solo en asistido")
	}
	_ = e.svc.Next()
	if !waitFor(t, 2*time.Second, func() bool { return e.log.status(2) == domain.LogPlaying || e.log.status(2) == domain.LogPlayed }) {
		t.Fatal("Siguiente debe arrancar el próximo elemento")
	}
}

// Caso real en la PC de la radio: tras reiniciar, lo programado antes de una
// tanda dura de las 19:30 había quedado atrasado y no quedaba nada antes de
// ella. El aire no debe quedarse en silencio esperando: adelanta lo que sigue
// después y la tanda corta a su hora.
func TestPlaysAheadInsteadOfWaitingForHardEvent(t *testing.T) {
	now := time.Now()
	tracks := map[int64]domain.Track{
		2: track(2, "tanda", 2*time.Second, 0),
		3: track(3, "insertada", 20*time.Second, 19*time.Second),
	}
	items := []domain.LogItem{
		{ID: 2, Kind: domain.SlotTrack, TrackID: 2, Hard: true, At: now.Add(2 * time.Second), Length: 2 * time.Second, Status: domain.LogPending},
		{ID: 3, Kind: domain.SlotTrack, TrackID: 3, Locked: true, At: now.Add(4 * time.Second), Length: 20 * time.Second, Status: domain.LogPending},
	}
	e := newEnv(t, items, tracks, nil)
	_ = e.svc.Start()
	if !waitFor(t, time.Second, func() bool { return e.log.status(3) == domain.LogPlaying }) {
		t.Fatalf("debe adelantar la pista en vez de esperar en silencio; emisiones %v", e.plays.titles())
	}
	if !waitFor(t, 3*time.Second, func() bool { return e.log.status(2) == domain.LogPlaying }) {
		t.Fatalf("la tanda dura debe cortar a su hora; emisiones %v", e.plays.titles())
	}
	if el := time.Since(now); el < 1700*time.Millisecond {
		t.Fatalf("la tanda no debe adelantarse: arrancó a los %v", el)
	}
}

// decodeMissing falla con los archivos que empiezan por "falta" (un archivo
// que existe pero ffmpeg no puede abrir).
func decodeMissing(ctx context.Context, path string, start time.Duration) (audio.Source, error) {
	if strings.HasPrefix(path, "falta") {
		return nil, errors.New("no se pudo abrir")
	}
	return decodeSine(ctx, path, start)
}

func withPath(t domain.Track, p string) domain.Track { t.Path = p; return t }

func warnings(e *env) string {
	var out []string
	for _, w := range e.svc.Snapshot().Warnings {
		out = append(out, w.Message)
	}
	return strings.Join(out, " | ")
}

// Cuñas sin audio (no descargadas o borradas): se sacan de la tanda y el aire sigue.
func TestSkipsSpotsWithoutAudio(t *testing.T) {
	now := time.Now()
	tracks := map[int64]domain.Track{
		1: withPath(track(1, "uno", 1200*time.Millisecond, 900*time.Millisecond), "uno.mp3"),
		2: withPath(track(2, "dos", 1200*time.Millisecond, 900*time.Millisecond), "dos.mp3"),
		9: {ID: 9, Title: "cortina", Path: "cortina.mp3", CategoryID: 5, Active: true, Duration: 300 * time.Millisecond},
	}
	spots := map[int64][]domain.Spot{
		11: {
			{ID: 1, Advertiser: "A", Name: "borrada", Path: "borrada.mp3", Duration: 400 * time.Millisecond},
			{ID: 2, Advertiser: "B", Name: "no descargada", Path: "", Duration: 400 * time.Millisecond},
			{ID: 3, Advertiser: "C", Name: "buena", Path: "c.mp3", Duration: 400 * time.Millisecond},
		},
		12: {{ID: 4, Advertiser: "D", Name: "sola sin audio", Path: "borrada.mp3"}},
	}
	items := []domain.LogItem{
		{ID: 10, Kind: domain.SlotCategory, TrackID: 1, At: now, Length: time.Second, Status: domain.LogPending},
		{ID: 11, Kind: domain.SlotSpots, At: now.Add(time.Second), Length: time.Second, Status: domain.LogPending},
		{ID: 12, Kind: domain.SlotSpots, At: now.Add(time.Second), Length: time.Second, Status: domain.LogPending},
		{ID: 13, Kind: domain.SlotCategory, TrackID: 2, At: now.Add(2 * time.Second), Length: time.Second, Status: domain.LogPending},
	}
	e := newEnv(t, items, tracks, spots)
	e.svc.d.Exists = func(p string) bool { return p != "borrada.mp3" }
	_ = e.svc.Start()
	if !waitFor(t, 8*time.Second, func() bool { return e.log.status(13) == domain.LogPlayed }) {
		t.Fatalf("se trabó: emisiones %v", e.plays.titles())
	}
	if got := strings.Join(e.plays.titles(), ","); !strings.HasPrefix(got, "uno,cortina,buena,cortina,dos") {
		t.Fatalf("emisiones = %s", got)
	}
	if e.log.status(11) != domain.LogPlayed || e.log.status(12) != domain.LogSkipped {
		t.Fatalf("estados: tanda con una cuña buena = %s, tanda sin audio = %s", e.log.status(11), e.log.status(12))
	}
	if w := warnings(e); !strings.Contains(w, "«borrada» de A sin audio") || !strings.Contains(w, "«no descargada» de B") {
		t.Fatalf("avisos = %s", w)
	}
}

// Un elemento cuyo audio no abre no debe quedar «sonando» para siempre.
func TestItemThatFailsToOpenIsSkipped(t *testing.T) {
	now := time.Now()
	tracks := map[int64]domain.Track{
		1: withPath(track(1, "uno", 1200*time.Millisecond, 900*time.Millisecond), "uno.mp3"),
		2: withPath(track(2, "rota", 1200*time.Millisecond, 900*time.Millisecond), "falta.mp3"),
		3: withPath(track(3, "tres", 1200*time.Millisecond, 900*time.Millisecond), "tres.mp3"),
		9: {ID: 9, Title: "cortina", Path: "falta-cortina.mp3", CategoryID: 5, Active: true, Duration: 300 * time.Millisecond},
	}
	// tanda donde nada abre (cortina y cuña rotas)
	spots := map[int64][]domain.Spot{12: {{ID: 1, Advertiser: "A", Name: "rota", Path: "falta-cuña.mp3"}}}
	items := []domain.LogItem{
		{ID: 10, Kind: domain.SlotCategory, TrackID: 1, At: now, Length: time.Second, Status: domain.LogPending},
		{ID: 11, Kind: domain.SlotTrack, TrackID: 2, At: now.Add(time.Second), Length: time.Second, Status: domain.LogPending},
		{ID: 12, Kind: domain.SlotSpots, At: now.Add(time.Second), Length: time.Second, Status: domain.LogPending},
		{ID: 13, Kind: domain.SlotCategory, TrackID: 3, At: now.Add(2 * time.Second), Length: time.Second, Status: domain.LogPending},
	}
	e := newEnv(t, items, tracks, spots)
	e.svc.d.Decode = decodeMissing
	_ = e.svc.Start()
	if !waitFor(t, 8*time.Second, func() bool { return e.log.status(13) == domain.LogPlayed }) {
		t.Fatalf("se trabó: emisiones %v, estados 11=%s 12=%s", e.plays.titles(), e.log.status(11), e.log.status(12))
	}
	if e.log.status(11) != domain.LogSkipped || e.log.status(12) != domain.LogSkipped {
		t.Fatalf("los elementos sin audio deben quedar saltados: 11=%s 12=%s", e.log.status(11), e.log.status(12))
	}
}

// Interruptor «Hora locutada»: apagado se salta sin aviso; encendido y sin
// locución se salta con aviso. En ningún caso se detiene el aire.
func TestTimeAnnounceSwitch(t *testing.T) {
	for _, on := range []bool{false, true} {
		now := time.Now()
		tracks := map[int64]domain.Track{
			1: withPath(track(1, "uno", 1200*time.Millisecond, 900*time.Millisecond), "uno.mp3"),
			2: withPath(track(2, "dos", 1200*time.Millisecond, 900*time.Millisecond), "dos.mp3"),
		}
		items := []domain.LogItem{
			{ID: 10, Kind: domain.SlotCategory, TrackID: 1, At: now, Length: time.Second, Status: domain.LogPending},
			{ID: 11, Kind: domain.SlotTime, Hard: true, At: now.Add(time.Second), Status: domain.LogPending},
			{ID: 12, Kind: domain.SlotCategory, TrackID: 2, At: now.Add(time.Second), Length: time.Second, Status: domain.LogPending},
		}
		e := newEnv(t, items, tracks, nil)
		opts := e.svc.d.Options()
		opts.TimeAnnounce = on
		e.svc.d.Options = func() Options { return opts }
		_ = e.svc.Start()
		if !waitFor(t, 6*time.Second, func() bool { return e.log.status(12) == domain.LogPlayed }) {
			t.Fatalf("hora locutada %v: se trabó (emisiones %v)", on, e.plays.titles())
		}
		if e.log.status(11) != domain.LogSkipped {
			t.Fatalf("hora locutada %v: estado %s", on, e.log.status(11))
		}
		if w := warnings(e); strings.Contains(w, "hora locutada") != on {
			t.Fatalf("hora locutada %v: avisos = %s", on, w)
		}
		e.cancel()
	}
}

// Enlace a cadena: suena el stream durante su duración y sigue la playlist; si
// la cadena no responde, se salta y la playlist sigue igual.
func TestRelay(t *testing.T) {
	for _, broken := range []bool{false, true} {
		now := time.Now()
		tracks := map[int64]domain.Track{
			1: withPath(track(1, "uno", 1200*time.Millisecond, 900*time.Millisecond), "uno.mp3"),
			2: withPath(track(2, "dos", 1200*time.Millisecond, 900*time.Millisecond), "dos.mp3"),
		}
		url := "https://cadena.pe/stream"
		if broken {
			url = "falta://cadena-caida"
		}
		items := []domain.LogItem{
			{ID: 10, Kind: domain.SlotCategory, TrackID: 1, At: now, Length: time.Second, Status: domain.LogPending},
			{ID: 11, Kind: domain.SlotRelay, URL: url, Seconds: 1, Title: "Noticiero", At: now.Add(time.Second), Length: time.Second, Status: domain.LogPending},
			{ID: 12, Kind: domain.SlotCategory, TrackID: 2, At: now.Add(2 * time.Second), Length: time.Second, Status: domain.LogPending},
		}
		e := newEnv(t, items, tracks, nil)
		e.svc.d.Decode = decodeMissing
		_ = e.svc.Start()
		if !waitFor(t, 8*time.Second, func() bool { return e.log.status(12) == domain.LogPlayed }) {
			t.Fatalf("caída=%v: se trabó (emisiones %v)", broken, e.plays.titles())
		}
		got := strings.Join(e.plays.titles(), ",")
		if broken {
			if strings.Contains(got, "Noticiero") || e.log.status(11) != domain.LogSkipped {
				t.Fatalf("cadena caída: %s / %s", got, e.log.status(11))
			}
		} else {
			if !strings.HasPrefix(got, "uno,Noticiero,dos") {
				t.Fatalf("emisiones = %s", got)
			}
			e.plays.mu.Lock()
			k := e.plays.plays[1].Kind
			e.plays.mu.Unlock()
			if k != domain.PlayRelay {
				t.Fatalf("el enlace se registra como %q", k)
			}
		}
		e.cancel()
	}
}
