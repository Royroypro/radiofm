package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"radioclub25/internal/domain"
)

// FMSpotRepo implementa domain.FMSpotRepository.
type FMSpotRepo struct{ db *sql.DB }

// NewFMSpotRepo crea el repositorio de cuñas FM.
func NewFMSpotRepo(db *sql.DB) *FMSpotRepo { return &FMSpotRepo{db: db} }

const fmSpotCols = `id, advertiser, name, file_url, sha256, duration_ms, starts_at, ends_at, plays_per_day,
	hour_from, hour_to, active, created_at, updated_at`

func scanFMSpot(r rowScanner) (*domain.FMSpot, error) {
	var s domain.FMSpot
	var starts, ends sql.NullString
	var created, updated string
	var active int
	if err := r.Scan(&s.ID, &s.Advertiser, &s.Name, &s.FileURL, &s.SHA256, &s.DurationMs, &starts, &ends,
		&s.PlaysPerDay, &s.HourFrom, &s.HourTo, &active, &created, &updated); err != nil {
		return nil, err
	}
	s.StartsAt, s.EndsAt, s.Active = parseTimePtr(starts), parseTimePtr(ends), active == 1
	s.CreatedAt, s.UpdatedAt = parseTime(created), parseTime(updated)
	return &s, nil
}

// Create inserta una cuña.
func (r *FMSpotRepo) Create(ctx context.Context, s *domain.FMSpot) error {
	now := time.Now()
	s.CreatedAt, s.UpdatedAt = now, now
	res, err := r.db.ExecContext(ctx, `INSERT INTO fm_spots (advertiser, name, file_url, sha256, duration_ms,
		starts_at, ends_at, plays_per_day, hour_from, hour_to, active, created_at, updated_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		s.Advertiser, s.Name, s.FileURL, s.SHA256, s.DurationMs, formatTimePtr(s.StartsAt), formatTimePtr(s.EndsAt),
		s.PlaysPerDay, s.HourFrom, s.HourTo, boolInt(s.Active), formatTime(now), formatTime(now))
	if err != nil {
		return fmt.Errorf("fm_spots: create: %w", err)
	}
	s.ID, _ = res.LastInsertId()
	return nil
}

// Get busca una cuña por ID.
func (r *FMSpotRepo) Get(ctx context.Context, id int64) (*domain.FMSpot, error) {
	s, err := scanFMSpot(r.db.QueryRowContext(ctx, `SELECT `+fmSpotCols+` FROM fm_spots WHERE id=?`, id))
	return s, notFound(err)
}

// List devuelve las cuñas ordenadas por anunciante y nombre.
func (r *FMSpotRepo) List(ctx context.Context, onlyActive bool) ([]domain.FMSpot, error) {
	q := `SELECT ` + fmSpotCols + ` FROM fm_spots`
	if onlyActive {
		q += ` WHERE active = 1`
	}
	q += ` ORDER BY advertiser COLLATE NOCASE, name COLLATE NOCASE, id`
	rows, err := r.db.QueryContext(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("fm_spots: list: %w", err)
	}
	defer rows.Close()
	var out []domain.FMSpot
	for rows.Next() {
		s, err := scanFMSpot(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *s)
	}
	return out, rows.Err()
}

// Update modifica una cuña.
func (r *FMSpotRepo) Update(ctx context.Context, s *domain.FMSpot) error {
	s.UpdatedAt = time.Now()
	return checkAffected(r.db.ExecContext(ctx, `UPDATE fm_spots SET advertiser=?, name=?, file_url=?, sha256=?,
		duration_ms=?, starts_at=?, ends_at=?, plays_per_day=?, hour_from=?, hour_to=?, active=?, updated_at=?
		WHERE id=?`,
		s.Advertiser, s.Name, s.FileURL, s.SHA256, s.DurationMs, formatTimePtr(s.StartsAt), formatTimePtr(s.EndsAt),
		s.PlaysPerDay, s.HourFrom, s.HourTo, boolInt(s.Active), formatTime(s.UpdatedAt), s.ID))
}

// Delete borra una cuña. Sus emisiones se conservan (spot_id pasa a NULL).
func (r *FMSpotRepo) Delete(ctx context.Context, id int64) error {
	return checkAffected(r.db.ExecContext(ctx, `DELETE FROM fm_spots WHERE id=?`, id))
}

// AirplayRepo implementa domain.AirplayRepository.
type AirplayRepo struct{ db *sql.DB }

// NewAirplayRepo crea el repositorio de emisiones de cuñas.
func NewAirplayRepo(db *sql.DB) *AirplayRepo { return &AirplayRepo{db: db} }

// Record inserta las emisiones nuevas en una sola transacción. Las que ya
// existen (mismo play_uuid) o cuya cuña no existe se ignoran. Devuelve cuántas
// filas se insertaron.
func (r *AirplayRepo) Record(ctx context.Context, plays []domain.Airplay) (int, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()
	stmt, err := tx.PrepareContext(ctx, `INSERT INTO fm_spot_airplays
		(spot_id, advertiser, spot_name, aired_at, duration_ms, complete, play_uuid, created_at)
		SELECT id, advertiser, name, ?, ?, ?, ?, ? FROM fm_spots WHERE id = ?
		ON CONFLICT(play_uuid) DO NOTHING`)
	if err != nil {
		return 0, fmt.Errorf("fm_spot_airplays: preparar: %w", err)
	}
	defer stmt.Close()
	now := formatTime(time.Now())
	inserted := 0
	for _, p := range plays {
		res, err := stmt.ExecContext(ctx, formatTime(p.AiredAt), p.DurationMs, boolInt(p.Complete), p.PlayUUID, now, p.SpotID)
		if err != nil {
			return 0, fmt.Errorf("fm_spot_airplays: insertar: %w", err)
		}
		n, _ := res.RowsAffected()
		inserted += int(n)
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return inserted, nil
}

// List devuelve las emisiones del filtro en orden cronológico.
func (r *AirplayRepo) List(ctx context.Context, f domain.AirplayFilter) ([]domain.Airplay, error) {
	where := []string{"aired_at >= ?", "aired_at < ?"}
	args := []any{formatTime(f.From), formatTime(f.To)}
	if f.Advertiser != "" {
		where, args = append(where, "advertiser = ? COLLATE NOCASE"), append(args, f.Advertiser)
	}
	if f.SpotID > 0 {
		where, args = append(where, "spot_id = ?"), append(args, f.SpotID)
	}
	rows, err := r.db.QueryContext(ctx, `SELECT id, COALESCE(spot_id, 0), advertiser, spot_name, aired_at, duration_ms,
		complete, play_uuid, created_at FROM fm_spot_airplays WHERE `+strings.Join(where, " AND ")+
		` ORDER BY aired_at, id`, args...)
	if err != nil {
		return nil, fmt.Errorf("fm_spot_airplays: list: %w", err)
	}
	defer rows.Close()
	var out []domain.Airplay
	for rows.Next() {
		var a domain.Airplay
		var aired, created string
		var complete int
		if err := rows.Scan(&a.ID, &a.SpotID, &a.Advertiser, &a.SpotName, &aired, &a.DurationMs, &complete,
			&a.PlayUUID, &created); err != nil {
			return nil, err
		}
		a.AiredAt, a.CreatedAt, a.Complete = parseTime(aired), parseTime(created), complete == 1
		out = append(out, a)
	}
	return out, rows.Err()
}

// CountSince cuenta las emisiones por cuña desde since.
func (r *AirplayRepo) CountSince(ctx context.Context, since time.Time) (map[int64]int, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT spot_id, COUNT(*) FROM fm_spot_airplays
		WHERE spot_id IS NOT NULL AND aired_at >= ? GROUP BY spot_id`, formatTime(since))
	if err != nil {
		return nil, fmt.Errorf("fm_spot_airplays: contar: %w", err)
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
