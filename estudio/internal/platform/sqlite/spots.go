package sqlite

import (
	"context"
	"database/sql"
	"strings"
	"time"

	"radioclub25/estudio/internal/domain"
)

// SpotRepo implementa domain.SpotRepository.
type SpotRepo struct{ db *sql.DB }

// NewSpotRepo crea el repositorio.
func NewSpotRepo(db *sql.DB) *SpotRepo { return &SpotRepo{db: db} }

const spotCols = `id, remote_id, advertiser, name, path, sha256, duration_ms, starts_at, ends_at, plays_per_day,
	hour_from, hour_to, active, updated_at, campaign_id, weight, days_mask, position, priority, sector`

func scanSpot(r rowScanner) (*domain.Spot, error) {
	var s domain.Spot
	var d int64
	var starts, ends sql.NullString
	var active int
	var updated string
	if err := r.Scan(&s.ID, &s.RemoteID, &s.Advertiser, &s.Name, &s.Path, &s.SHA256, &d, &starts, &ends,
		&s.PlaysPerDay, &s.HourFrom, &s.HourTo, &active, &updated, &s.CampaignID, &s.Weight, &s.DaysMask, &s.Position,
		&s.Priority, &s.Sector); err != nil {
		return nil, err
	}
	s.Duration, s.StartsAt, s.EndsAt, s.Active, s.UpdatedAt = dur(d), parseTimePtr(starts), parseTimePtr(ends), active == 1, parseTime(updated)
	return &s, nil
}

// List devuelve las cuñas por anunciante y nombre.
func (r *SpotRepo) List(ctx context.Context) ([]domain.Spot, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT `+spotCols+` FROM spots ORDER BY advertiser COLLATE NOCASE, name COLLATE NOCASE`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.Spot
	for rows.Next() {
		s, err := scanSpot(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *s)
	}
	return out, rows.Err()
}

// Get busca una cuña.
func (r *SpotRepo) Get(ctx context.Context, id int64) (*domain.Spot, error) {
	s, err := scanSpot(r.db.QueryRowContext(ctx, `SELECT `+spotCols+` FROM spots WHERE id=?`, id))
	return s, notFound(err)
}

// ByRemoteID busca una cuña del panel web por su id remoto.
func (r *SpotRepo) ByRemoteID(ctx context.Context, remoteID int64) (*domain.Spot, error) {
	s, err := scanSpot(r.db.QueryRowContext(ctx, `SELECT `+spotCols+` FROM spots WHERE remote_id=? AND remote_id>0`, remoteID))
	return s, notFound(err)
}

// Save crea o actualiza una cuña.
func (r *SpotRepo) Save(ctx context.Context, s *domain.Spot) error {
	s.UpdatedAt = time.Now()
	if s.ID == 0 {
		res, err := r.db.ExecContext(ctx, `INSERT INTO spots (remote_id, advertiser, name, path, sha256, duration_ms,
			starts_at, ends_at, plays_per_day, hour_from, hour_to, active, updated_at, campaign_id, weight, days_mask,
			position, priority, sector) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
			s.RemoteID, s.Advertiser, s.Name, s.Path, s.SHA256, ms(s.Duration), fmtTimePtr(s.StartsAt), fmtTimePtr(s.EndsAt),
			s.PlaysPerDay, s.HourFrom, s.HourTo, b2i(s.Active), fmtTime(s.UpdatedAt), s.CampaignID, max(s.Weight, 1),
			s.DaysMask, posOr(s.Position), s.Prio(), s.Sector)
		if err != nil {
			return err
		}
		s.ID, _ = res.LastInsertId()
		return nil
	}
	return checkAffected(r.db.ExecContext(ctx, `UPDATE spots SET remote_id=?, advertiser=?, name=?, path=?, sha256=?,
		duration_ms=?, starts_at=?, ends_at=?, plays_per_day=?, hour_from=?, hour_to=?, active=?, updated_at=?,
		campaign_id=?, weight=?, days_mask=?, position=?, priority=?, sector=? WHERE id=?`,
		s.RemoteID, s.Advertiser, s.Name, s.Path, s.SHA256, ms(s.Duration), fmtTimePtr(s.StartsAt), fmtTimePtr(s.EndsAt),
		s.PlaysPerDay, s.HourFrom, s.HourTo, b2i(s.Active), fmtTime(s.UpdatedAt), s.CampaignID, max(s.Weight, 1),
		s.DaysMask, posOr(s.Position), s.Prio(), s.Sector, s.ID))
}

// Delete borra una cuña (y la quita de las tandas planificadas).
func (r *SpotRepo) Delete(ctx context.Context, id int64) error {
	if _, err := r.db.ExecContext(ctx, `DELETE FROM tanda_spots WHERE spot_id=?`, id); err != nil {
		return err
	}
	return checkAffected(r.db.ExecContext(ctx, `DELETE FROM spots WHERE id=?`, id))
}

// TandaRepo implementa domain.TandaRepository.
type TandaRepo struct{ db *sql.DB }

// NewTandaRepo crea el repositorio.
func NewTandaRepo(db *sql.DB) *TandaRepo { return &TandaRepo{db: db} }

// ForItems devuelve las cuñas planificadas de cada tanda, en orden.
func (r *TandaRepo) ForItems(ctx context.Context, ids []int64) (map[int64][]int64, error) {
	out := map[int64][]int64{}
	if len(ids) == 0 {
		return out, nil
	}
	args := make([]any, len(ids))
	for i, id := range ids {
		args[i] = id
	}
	rows, err := r.db.QueryContext(ctx, `SELECT log_item_id, spot_id FROM tanda_spots WHERE log_item_id IN (?`+
		strings.Repeat(",?", len(ids)-1)+`) ORDER BY log_item_id, position`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var item, spot int64
		if err := rows.Scan(&item, &spot); err != nil {
			return nil, err
		}
		out[item] = append(out[item], spot)
	}
	return out, rows.Err()
}

// Set reemplaza el contenido de una tanda.
func (r *TandaRepo) Set(ctx context.Context, logItemID int64, spotIDs []int64) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `DELETE FROM tanda_spots WHERE log_item_id=?`, logItemID); err != nil {
		return err
	}
	for i, id := range spotIDs {
		if _, err := tx.ExecContext(ctx, `INSERT INTO tanda_spots (log_item_id, position, spot_id) VALUES (?,?,?)`,
			logItemID, i, id); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func posOr(p string) string {
	if p == "first" || p == "last" {
		return p
	}
	return "any"
}
