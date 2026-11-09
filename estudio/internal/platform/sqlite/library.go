package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"radioclub25/estudio/internal/domain"
)

// ---------- Categorías ----------

// CategoryRepo implementa domain.CategoryRepository.
type CategoryRepo struct{ db *sql.DB }

// NewCategoryRepo crea el repositorio.
func NewCategoryRepo(db *sql.DB) *CategoryRepo { return &CategoryRepo{db: db} }

// List devuelve las categorías ordenadas por nombre.
func (r *CategoryRepo) List(ctx context.Context) ([]domain.Category, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT id, name, parent_id, kind, color FROM categories ORDER BY parent_id, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.Category
	for rows.Next() {
		var c domain.Category
		var kind string
		if err := rows.Scan(&c.ID, &c.Name, &c.ParentID, &kind, &c.Color); err != nil {
			return nil, err
		}
		c.Kind = domain.CategoryKind(kind)
		out = append(out, c)
	}
	return out, rows.Err()
}

// Get busca una categoría.
func (r *CategoryRepo) Get(ctx context.Context, id int64) (*domain.Category, error) {
	var c domain.Category
	var kind string
	err := r.db.QueryRowContext(ctx, `SELECT id, name, parent_id, kind, color FROM categories WHERE id=?`, id).
		Scan(&c.ID, &c.Name, &c.ParentID, &kind, &c.Color)
	c.Kind = domain.CategoryKind(kind)
	return &c, notFound(err)
}

// Create inserta una categoría.
func (r *CategoryRepo) Create(ctx context.Context, c *domain.Category) error {
	res, err := r.db.ExecContext(ctx, `INSERT INTO categories (name, parent_id, kind, color) VALUES (?,?,?,?)`,
		c.Name, c.ParentID, string(c.Kind), c.Color)
	if isUnique(err) {
		return domain.ErrConflict
	}
	if err != nil {
		return err
	}
	c.ID, _ = res.LastInsertId()
	return nil
}

// Update modifica una categoría.
func (r *CategoryRepo) Update(ctx context.Context, c *domain.Category) error {
	err := checkAffected(r.db.ExecContext(ctx, `UPDATE categories SET name=?, parent_id=?, kind=?, color=? WHERE id=?`,
		c.Name, c.ParentID, string(c.Kind), c.Color, c.ID))
	if isUnique(err) {
		return domain.ErrConflict
	}
	return err
}

// Delete borra una categoría sin pistas ni subcategorías.
func (r *CategoryRepo) Delete(ctx context.Context, id int64) error {
	var n int
	if err := r.db.QueryRowContext(ctx, `SELECT (SELECT COUNT(*) FROM tracks WHERE category_id=?) +
		(SELECT COUNT(*) FROM categories WHERE parent_id=?)`, id, id).Scan(&n); err != nil {
		return err
	}
	if n > 0 {
		return domain.ErrConflict
	}
	return checkAffected(r.db.ExecContext(ctx, `DELETE FROM categories WHERE id=?`, id))
}

// ---------- Pistas ----------

// TrackRepo implementa domain.TrackRepository.
type TrackRepo struct{ db *sql.DB }

// NewTrackRepo crea el repositorio.
func NewTrackRepo(db *sql.DB) *TrackRepo { return &TrackRepo{db: db} }

const trackCols = `id, path, title, artist, album, genre, year, category_id, duration_ms, active, valid_from, valid_to,
	analyzed, loudness, gain_db, cue_in_ms, cue_out_ms, segue_ms, intro_ms, analyze_err, missing, last_played_at,
	play_count, created_at, envelope, ending, fade_out_ms, marks_manual`

func scanTrack(r rowScanner) (*domain.Track, error) {
	var t domain.Track
	var durMs, cueIn, cueOut, segue, intro, fadeOut int64
	var active, analyzed, missing, manual int
	var ending string
	var from, to, last sql.NullString
	var created string
	if err := r.Scan(&t.ID, &t.Path, &t.Title, &t.Artist, &t.Album, &t.Genre, &t.Year, &t.CategoryID, &durMs, &active,
		&from, &to, &analyzed, &t.Loudness, &t.GainDB, &cueIn, &cueOut, &segue, &intro, &t.AnalyzeErr, &missing, &last,
		&t.PlayCount, &created, &t.Envelope, &ending, &fadeOut, &manual); err != nil {
		return nil, err
	}
	t.Ending, t.FadeOut, t.MarksManual = domain.Ending(ending), dur(fadeOut), manual == 1
	t.Duration, t.CueIn, t.CueOut, t.SegueAt, t.Intro = dur(durMs), dur(cueIn), dur(cueOut), dur(segue), dur(intro)
	t.Active, t.Analyzed, t.Missing = active == 1, analyzed == 1, missing == 1
	t.ValidFrom, t.ValidTo, t.LastPlayedAt = parseTimePtr(from), parseTimePtr(to), parseTimePtr(last)
	t.CreatedAt = parseTime(created)
	return &t, nil
}

func scanTracks(rows *sql.Rows) ([]domain.Track, error) {
	defer rows.Close()
	var out []domain.Track
	for rows.Next() {
		t, err := scanTrack(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *t)
	}
	return out, rows.Err()
}

// Upsert crea la pista o actualiza etiquetas y duración si la ruta ya existe
// (sin tocar categoría, cues manuales ni estado). Si la duración cambió, se reanaliza.
func (r *TrackRepo) Upsert(ctx context.Context, t *domain.Track) (bool, error) {
	var id, oldDur int64
	err := r.db.QueryRowContext(ctx, `SELECT id, duration_ms FROM tracks WHERE path=?`, t.Path).Scan(&id, &oldDur)
	if err == sql.ErrNoRows {
		if t.CreatedAt.IsZero() {
			t.CreatedAt = time.Now()
		}
		res, err := r.db.ExecContext(ctx, `INSERT INTO tracks (path, title, artist, album, genre, year, category_id,
			duration_ms, active, created_at) VALUES (?,?,?,?,?,?,?,?,?,?)`,
			t.Path, t.Title, t.Artist, t.Album, t.Genre, t.Year, t.CategoryID, ms(t.Duration), b2i(t.Active), fmtTime(t.CreatedAt))
		if err != nil {
			return false, fmt.Errorf("tracks: insertar: %w", err)
		}
		t.ID, _ = res.LastInsertId()
		return true, nil
	}
	if err != nil {
		return false, err
	}
	t.ID = id
	reanalyze := ""
	if oldDur != ms(t.Duration) {
		reanalyze = ", analyzed=0"
	}
	_, err = r.db.ExecContext(ctx, `UPDATE tracks SET title=?, artist=?, album=?, genre=?, year=?, duration_ms=?, missing=0`+
		reanalyze+` WHERE id=?`, t.Title, t.Artist, t.Album, t.Genre, t.Year, ms(t.Duration), id)
	return false, err
}

// Get busca una pista.
func (r *TrackRepo) Get(ctx context.Context, id int64) (*domain.Track, error) {
	t, err := scanTrack(r.db.QueryRowContext(ctx, `SELECT `+trackCols+` FROM tracks WHERE id=?`, id))
	return t, notFound(err)
}

// List filtra y pagina las pistas (por artista y título). Devuelve también el total.
func (r *TrackRepo) List(ctx context.Context, f domain.TrackFilter) ([]domain.Track, int, error) {
	var where []string
	var args []any
	if f.CategoryID > 0 {
		where, args = append(where, "category_id=?"), append(args, f.CategoryID)
	}
	if f.OnlyActive {
		where = append(where, "active=1 AND missing=0")
	}
	if q := strings.TrimSpace(f.Query); q != "" {
		like := "%" + strings.NewReplacer("%", `\%`, "_", `\_`).Replace(q) + "%"
		where = append(where, `(title LIKE ? ESCAPE '\' OR artist LIKE ? ESCAPE '\' OR album LIKE ? ESCAPE '\' OR path LIKE ? ESCAPE '\')`)
		args = append(args, like, like, like, like)
	}
	cond := ""
	if len(where) > 0 {
		cond = " WHERE " + strings.Join(where, " AND ")
	}
	var total int
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM tracks`+cond, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	limit := f.Limit
	if limit <= 0 || limit > 5000 {
		limit = 500
	}
	rows, err := r.db.QueryContext(ctx, `SELECT `+trackCols+` FROM tracks`+cond+
		` ORDER BY artist COLLATE NOCASE, title COLLATE NOCASE LIMIT ? OFFSET ?`, append(args, limit, max(f.Offset, 0))...)
	if err != nil {
		return nil, 0, err
	}
	list, err := scanTracks(rows)
	return list, total, err
}

// Update guarda los campos editables de una pista.
func (r *TrackRepo) Update(ctx context.Context, t *domain.Track) error {
	return checkAffected(r.db.ExecContext(ctx, `UPDATE tracks SET title=?, artist=?, album=?, genre=?, year=?,
		category_id=?, active=?, valid_from=?, valid_to=?, gain_db=?, cue_in_ms=?, cue_out_ms=?, segue_ms=?, intro_ms=?,
		marks_manual=? WHERE id=?`,
		t.Title, t.Artist, t.Album, t.Genre, t.Year, t.CategoryID, b2i(t.Active), fmtTimePtr(t.ValidFrom),
		fmtTimePtr(t.ValidTo), t.GainDB, ms(t.CueIn), ms(t.CueOut), ms(t.SegueAt), ms(t.Intro), b2i(t.MarksManual), t.ID))
}

// Delete borra una pista de la biblioteca (no el archivo).
func (r *TrackRepo) Delete(ctx context.Context, id int64) error {
	return checkAffected(r.db.ExecContext(ctx, `DELETE FROM tracks WHERE id=?`, id))
}

// PendingAnalysis devuelve pistas sin analizar.
func (r *TrackRepo) PendingAnalysis(ctx context.Context, limit int) ([]domain.Track, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT `+trackCols+` FROM tracks WHERE analyzed=0 AND missing=0 ORDER BY id LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	return scanTracks(rows)
}

// SetAnalysis guarda el resultado del análisis. Si el operador ajustó las
// marcas a mano (marks_manual), se conservan: solo cambian sonoridad, ganancia,
// forma de onda y tipo de final.
func (r *TrackRepo) SetAnalysis(ctx context.Context, id int64, a domain.Analysis) error {
	return checkAffected(r.db.ExecContext(ctx, `UPDATE tracks SET analyzed=1, loudness=?, gain_db=?,
		cue_in_ms=CASE WHEN marks_manual=1 THEN cue_in_ms ELSE ? END,
		cue_out_ms=CASE WHEN marks_manual=1 THEN cue_out_ms ELSE ? END,
		segue_ms=CASE WHEN marks_manual=1 THEN segue_ms ELSE ? END,
		analyze_err=?, envelope=?, ending=?, fade_out_ms=? WHERE id=?`,
		a.Loudness, a.GainDB, ms(a.CueIn), ms(a.CueOut), ms(a.SegueAt), a.Err, a.Envelope, string(a.Ending),
		ms(a.FadeOut), id))
}

// SetMissing marca si el archivo falta.
func (r *TrackRepo) SetMissing(ctx context.Context, id int64, missing bool) error {
	return checkAffected(r.db.ExecContext(ctx, `UPDATE tracks SET missing=? WHERE id=?`, b2i(missing), id))
}

// ByCategory devuelve todas las pistas de una categoría (para la rotación).
func (r *TrackRepo) ByCategory(ctx context.Context, categoryID int64) ([]domain.Track, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT `+trackCols+` FROM tracks WHERE category_id=? ORDER BY id`, categoryID)
	if err != nil {
		return nil, err
	}
	return scanTracks(rows)
}

// CountByCategory cuenta pistas por categoría.
func (r *TrackRepo) CountByCategory(ctx context.Context) (map[int64]int, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT category_id, COUNT(*) FROM tracks GROUP BY category_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[int64]int{}
	for rows.Next() {
		var id int64
		var n int
		if err := rows.Scan(&id, &n); err != nil {
			return nil, err
		}
		out[id] = n
	}
	return out, rows.Err()
}

// ---------- Historial ----------

// PlayRepo implementa domain.PlayRepository.
type PlayRepo struct{ db *sql.DB }

// NewPlayRepo crea el repositorio.
func NewPlayRepo(db *sql.DB) *PlayRepo { return &PlayRepo{db: db} }

// Add registra el inicio de una emisión y actualiza la pista (última vez y contador).
func (r *PlayRepo) Add(ctx context.Context, p *domain.Play) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	res, err := tx.ExecContext(ctx, `INSERT INTO plays (track_id, spot_id, kind, artist, title, started_at, duration_ms,
		complete, uuid) VALUES (?,?,?,?,?,?,?,?,?)`, p.TrackID, p.SpotID, string(p.Kind), p.Artist, p.Title,
		fmtTime(p.StartedAt), ms(p.Duration), b2i(p.Complete), p.UUID)
	if err != nil {
		return fmt.Errorf("plays: insertar: %w", err)
	}
	p.ID, _ = res.LastInsertId()
	if p.TrackID > 0 {
		if _, err := tx.ExecContext(ctx, `UPDATE tracks SET last_played_at=?, play_count=play_count+1 WHERE id=?`,
			fmtTime(p.StartedAt), p.TrackID); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// Finish guarda cuánto sonó y si fue completa.
func (r *PlayRepo) Finish(ctx context.Context, id int64, d time.Duration, complete bool) error {
	return checkAffected(r.db.ExecContext(ctx, `UPDATE plays SET duration_ms=?, complete=? WHERE id=?`, ms(d), b2i(complete), id))
}

const playCols = `id, track_id, spot_id, kind, artist, title, started_at, duration_ms, complete, uuid`

func (r *PlayRepo) query(ctx context.Context, q string, args ...any) ([]domain.Play, error) {
	rows, err := r.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.Play
	for rows.Next() {
		var p domain.Play
		var kind, started string
		var d int64
		var complete int
		if err := rows.Scan(&p.ID, &p.TrackID, &p.SpotID, &kind, &p.Artist, &p.Title, &started, &d, &complete, &p.UUID); err != nil {
			return nil, err
		}
		p.Kind, p.StartedAt, p.Duration, p.Complete = domain.PlayKind(kind), parseTime(started), dur(d), complete == 1
		out = append(out, p)
	}
	return out, rows.Err()
}

// Since devuelve las emisiones desde since, en orden cronológico.
func (r *PlayRepo) Since(ctx context.Context, since time.Time) ([]domain.Play, error) {
	return r.query(ctx, `SELECT `+playCols+` FROM plays WHERE started_at >= ? ORDER BY started_at`, fmtTime(since))
}

// Between devuelve las emisiones en [from, to).
func (r *PlayRepo) Between(ctx context.Context, from, to time.Time) ([]domain.Play, error) {
	return r.query(ctx, `SELECT `+playCols+` FROM plays WHERE started_at >= ? AND started_at < ? ORDER BY started_at`,
		fmtTime(from), fmtTime(to))
}
