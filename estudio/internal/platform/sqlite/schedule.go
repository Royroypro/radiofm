package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"radioclub25/estudio/internal/domain"
)

// ---------- Relojes ----------

// ClockRepo implementa domain.ClockRepository.
type ClockRepo struct{ db *sql.DB }

// NewClockRepo crea el repositorio.
func NewClockRepo(db *sql.DB) *ClockRepo { return &ClockRepo{db: db} }

func scanClock(r rowScanner) (*domain.Clock, error) {
	var c domain.Clock
	var slots string
	if err := r.Scan(&c.ID, &c.Name, &c.Color, &slots); err != nil {
		return nil, err
	}
	if err := json.Unmarshal([]byte(slots), &c.Slots); err != nil {
		return nil, fmt.Errorf("clocks: slots de %q: %w", c.Name, err)
	}
	if c.Slots == nil {
		c.Slots = []domain.ClockSlot{}
	}
	return &c, nil
}

// List devuelve los relojes por nombre.
func (r *ClockRepo) List(ctx context.Context) ([]domain.Clock, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT id, name, color, slots_json FROM clocks ORDER BY name COLLATE NOCASE`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.Clock
	for rows.Next() {
		c, err := scanClock(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *c)
	}
	return out, rows.Err()
}

// Get busca un reloj.
func (r *ClockRepo) Get(ctx context.Context, id int64) (*domain.Clock, error) {
	c, err := scanClock(r.db.QueryRowContext(ctx, `SELECT id, name, color, slots_json FROM clocks WHERE id=?`, id))
	return c, notFound(err)
}

// Save crea o actualiza un reloj.
func (r *ClockRepo) Save(ctx context.Context, c *domain.Clock) error {
	b, err := json.Marshal(c.Slots)
	if err != nil {
		return err
	}
	if c.ID == 0 {
		res, err := r.db.ExecContext(ctx, `INSERT INTO clocks (name, color, slots_json) VALUES (?,?,?)`, c.Name, c.Color, string(b))
		if isUnique(err) {
			return domain.ErrConflict
		}
		if err != nil {
			return err
		}
		c.ID, _ = res.LastInsertId()
		return nil
	}
	err = checkAffected(r.db.ExecContext(ctx, `UPDATE clocks SET name=?, color=?, slots_json=? WHERE id=?`,
		c.Name, c.Color, string(b), c.ID))
	if isUnique(err) {
		return domain.ErrConflict
	}
	return err
}

// Delete borra un reloj que no esté en la parrilla ni en excepciones.
func (r *ClockRepo) Delete(ctx context.Context, id int64) error {
	var n int
	if err := r.db.QueryRowContext(ctx, `SELECT (SELECT COUNT(*) FROM grid WHERE clock_id=?) +
		(SELECT COUNT(*) FROM grid_exceptions WHERE clock_id=?)`, id, id).Scan(&n); err != nil {
		return err
	}
	if n > 0 {
		return domain.ErrConflict
	}
	return checkAffected(r.db.ExecContext(ctx, `DELETE FROM clocks WHERE id=?`, id))
}

// ---------- Parrilla ----------

// GridRepo implementa domain.GridRepository.
type GridRepo struct{ db *sql.DB }

// NewGridRepo crea el repositorio.
func NewGridRepo(db *sql.DB) *GridRepo { return &GridRepo{db: db} }

// Cells devuelve las celdas con reloj asignado.
func (r *GridRepo) Cells(ctx context.Context) ([]domain.GridCell, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT weekday, hour, clock_id FROM grid ORDER BY weekday, hour`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.GridCell
	for rows.Next() {
		var c domain.GridCell
		if err := rows.Scan(&c.Weekday, &c.Hour, &c.ClockID); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// SetCells asigna relojes a celdas (ClockID 0 = quitar).
func (r *GridRepo) SetCells(ctx context.Context, cells []domain.GridCell) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	for _, c := range cells {
		if c.ClockID == 0 {
			_, err = tx.ExecContext(ctx, `DELETE FROM grid WHERE weekday=? AND hour=?`, c.Weekday, c.Hour)
		} else {
			_, err = tx.ExecContext(ctx, `INSERT INTO grid (weekday, hour, clock_id) VALUES (?,?,?)
				ON CONFLICT(weekday, hour) DO UPDATE SET clock_id=excluded.clock_id`, c.Weekday, c.Hour, c.ClockID)
		}
		if err != nil {
			return err
		}
	}
	return tx.Commit()
}

// Exceptions devuelve las excepciones desde fromDate (inclusive).
func (r *GridRepo) Exceptions(ctx context.Context, fromDate string) ([]domain.GridException, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT date, hour, clock_id FROM grid_exceptions WHERE date >= ? ORDER BY date, hour`, fromDate)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.GridException
	for rows.Next() {
		var e domain.GridException
		if err := rows.Scan(&e.Date, &e.Hour, &e.ClockID); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// SetException crea, cambia o (con ClockID 0) borra una excepción.
func (r *GridRepo) SetException(ctx context.Context, e domain.GridException) error {
	if e.ClockID == 0 {
		_, err := r.db.ExecContext(ctx, `DELETE FROM grid_exceptions WHERE date=? AND hour=?`, e.Date, e.Hour)
		return err
	}
	_, err := r.db.ExecContext(ctx, `INSERT INTO grid_exceptions (date, hour, clock_id) VALUES (?,?,?)
		ON CONFLICT(date, hour) DO UPDATE SET clock_id=excluded.clock_id`, e.Date, e.Hour, e.ClockID)
	return err
}

// ---------- Playlist ----------

// LogRepo implementa domain.LogRepository.
type LogRepo struct{ db *sql.DB }

// NewLogRepo crea el repositorio.
func NewLogRepo(db *sql.DB) *LogRepo { return &LogRepo{db: db} }

const logCols = `id, day, hour, seq, kind, track_id, category_id, hard, at, length_ms, optional, locked, status,
	title, artist, mode, seconds, note, url`

func scanLog(r rowScanner) (*domain.LogItem, error) {
	var l domain.LogItem
	var kind, at, status string
	var length int64
	var hard, optional, locked int
	if err := r.Scan(&l.ID, &l.Day, &l.Hour, &l.Seq, &kind, &l.TrackID, &l.CategoryID, &hard, &at, &length, &optional,
		&locked, &status, &l.Title, &l.Artist, &l.Mode, &l.Seconds, &l.Note, &l.URL); err != nil {
		return nil, err
	}
	l.Kind, l.Status, l.At, l.Length = domain.SlotKind(kind), domain.LogStatus(status), parseTime(at), dur(length)
	l.Hard, l.Optional, l.Locked = hard == 1, optional == 1, locked == 1
	return &l, nil
}

// Day devuelve la playlist de un día en orden.
func (r *LogRepo) Day(ctx context.Context, day string) ([]domain.LogItem, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT `+logCols+` FROM log_items WHERE day=? ORDER BY seq, id`, day)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.LogItem
	for rows.Next() {
		l, err := scanLog(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *l)
	}
	return out, rows.Err()
}

// ReplaceDay guarda la playlist de un día en el orden de items (Seq se renumera).
// Los elementos con ID de ese día se actualizan (el ID no cambia: el aire y la
// interfaz los siguen); los de ID 0 se insertan; los que ya no están se borran.
func (r *LogRepo) ReplaceDay(ctx context.Context, day string, items []domain.LogItem) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	existing := map[int64]bool{}
	rows, err := tx.QueryContext(ctx, `SELECT id FROM log_items WHERE day=?`, day)
	if err != nil {
		return err
	}
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		existing[id] = true
	}
	rows.Close()
	ins, err := tx.PrepareContext(ctx, `INSERT INTO log_items (day, hour, seq, kind, track_id, category_id, hard, at,
		length_ms, optional, locked, status, title, artist, mode, seconds, note, url) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`)
	if err != nil {
		return err
	}
	defer ins.Close()
	upd, err := tx.PrepareContext(ctx, `UPDATE log_items SET hour=?, seq=?, kind=?, track_id=?, category_id=?, hard=?,
		at=?, length_ms=?, optional=?, locked=?, status=?, title=?, artist=?, mode=?, seconds=?, note=?, url=? WHERE id=?`)
	if err != nil {
		return err
	}
	defer upd.Close()
	for i := range items {
		it := &items[i]
		it.Day, it.Seq = day, i
		if it.Status == "" {
			it.Status = domain.LogPending
		}
		if it.ID > 0 && existing[it.ID] {
			delete(existing, it.ID)
			if _, err := upd.ExecContext(ctx, it.Hour, i, string(it.Kind), it.TrackID, it.CategoryID, b2i(it.Hard),
				fmtTime(it.At), ms(it.Length), b2i(it.Optional), b2i(it.Locked), string(it.Status), it.Title, it.Artist,
				it.Mode, it.Seconds, it.Note, it.URL, it.ID); err != nil {
				return fmt.Errorf("log_items: actualizar: %w", err)
			}
			continue
		}
		res, err := ins.ExecContext(ctx, day, it.Hour, i, string(it.Kind), it.TrackID, it.CategoryID, b2i(it.Hard),
			fmtTime(it.At), ms(it.Length), b2i(it.Optional), b2i(it.Locked), string(it.Status), it.Title, it.Artist,
			it.Mode, it.Seconds, it.Note, it.URL)
		if err != nil {
			return fmt.Errorf("log_items: insertar: %w", err)
		}
		it.ID, _ = res.LastInsertId()
	}
	for id := range existing {
		if _, err := tx.ExecContext(ctx, `DELETE FROM log_items WHERE id=?`, id); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// Get busca un elemento.
func (r *LogRepo) Get(ctx context.Context, id int64) (*domain.LogItem, error) {
	l, err := scanLog(r.db.QueryRowContext(ctx, `SELECT `+logCols+` FROM log_items WHERE id=?`, id))
	return l, notFound(err)
}

// Update guarda un elemento.
func (r *LogRepo) Update(ctx context.Context, it *domain.LogItem) error {
	return checkAffected(r.db.ExecContext(ctx, `UPDATE log_items SET hour=?, seq=?, kind=?, track_id=?, category_id=?,
		hard=?, at=?, length_ms=?, optional=?, locked=?, status=?, title=?, artist=?, mode=?, seconds=?, note=?, url=? WHERE id=?`,
		it.Hour, it.Seq, string(it.Kind), it.TrackID, it.CategoryID, b2i(it.Hard), fmtTime(it.At), ms(it.Length),
		b2i(it.Optional), b2i(it.Locked), string(it.Status), it.Title, it.Artist, it.Mode, it.Seconds, it.Note, it.URL, it.ID))
}

// Delete borra un elemento.
func (r *LogRepo) Delete(ctx context.Context, id int64) error {
	return checkAffected(r.db.ExecContext(ctx, `DELETE FROM log_items WHERE id=?`, id))
}

// SetStatus cambia el estado de un elemento.
func (r *LogRepo) SetStatus(ctx context.Context, id int64, s domain.LogStatus) error {
	return checkAffected(r.db.ExecContext(ctx, `UPDATE log_items SET status=? WHERE id=?`, string(s), id))
}

// Days devuelve los días con playlist desde from.
func (r *LogRepo) Days(ctx context.Context, from string) ([]string, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT DISTINCT day FROM log_items WHERE day >= ? ORDER BY day`, from)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var d string
		if err := rows.Scan(&d); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}
