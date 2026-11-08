package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"radioclub25/internal/domain"
)

// AdRepo implementa domain.AdRepository.
type AdRepo struct{ db *sql.DB }

// NewAdRepo crea el repositorio de anuncios.
func NewAdRepo(db *sql.DB) *AdRepo { return &AdRepo{db: db} }

const adCols = `id, name, type, position, file_url, link_url, starts_at, ends_at, weight, skip_after_sec, active, created_at, updated_at`

func scanAd(r rowScanner) (*domain.Ad, error) {
	var a domain.Ad
	var typ, pos, created, updated string
	var starts, ends sql.NullString
	var active int
	if err := r.Scan(&a.ID, &a.Name, &typ, &pos, &a.FileURL, &a.LinkURL, &starts, &ends, &a.Weight, &a.SkipAfterSec,
		&active, &created, &updated); err != nil {
		return nil, err
	}
	a.Type, a.Position, a.Active = domain.AdType(typ), domain.AdPosition(pos), active == 1
	a.StartsAt, a.EndsAt = parseTimePtr(starts), parseTimePtr(ends)
	a.CreatedAt, a.UpdatedAt = parseTime(created), parseTime(updated)
	return &a, nil
}

// Create inserta un anuncio.
func (r *AdRepo) Create(ctx context.Context, a *domain.Ad) error {
	now := time.Now()
	a.CreatedAt, a.UpdatedAt = now, now
	res, err := r.db.ExecContext(ctx, `INSERT INTO ads (name, type, position, file_url, link_url, starts_at, ends_at,
		weight, skip_after_sec, active, created_at, updated_at) VALUES (?,?,?,?,?,?,?,?,?,?,?,?)`,
		a.Name, string(a.Type), string(a.Position), a.FileURL, a.LinkURL, formatTimePtr(a.StartsAt), formatTimePtr(a.EndsAt),
		a.Weight, a.SkipAfterSec, boolInt(a.Active), formatTime(now), formatTime(now))
	if err != nil {
		return fmt.Errorf("ads: create: %w", err)
	}
	a.ID, _ = res.LastInsertId()
	return nil
}

// Get busca un anuncio por ID.
func (r *AdRepo) Get(ctx context.Context, id int64) (*domain.Ad, error) {
	a, err := scanAd(r.db.QueryRowContext(ctx, `SELECT `+adCols+` FROM ads WHERE id=?`, id))
	return a, notFound(err)
}

// List devuelve los anuncios que cumplen el filtro, los más recientes primero.
func (r *AdRepo) List(ctx context.Context, f domain.AdFilter) ([]domain.Ad, error) {
	var where []string
	var args []any
	if f.Type != "" {
		where, args = append(where, "type = ?"), append(args, string(f.Type))
	}
	if f.Position != "" {
		where, args = append(where, "position = ?"), append(args, string(f.Position))
	}
	if f.OnlyActive {
		where = append(where, "active = 1")
	}
	q := `SELECT ` + adCols + ` FROM ads`
	if len(where) > 0 {
		q += " WHERE " + strings.Join(where, " AND ")
	}
	q += " ORDER BY id DESC"
	rows, err := r.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("ads: list: %w", err)
	}
	defer rows.Close()
	var out []domain.Ad
	for rows.Next() {
		a, err := scanAd(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *a)
	}
	return out, rows.Err()
}

// Update modifica un anuncio.
func (r *AdRepo) Update(ctx context.Context, a *domain.Ad) error {
	a.UpdatedAt = time.Now()
	return checkAffected(r.db.ExecContext(ctx, `UPDATE ads SET name=?, type=?, position=?, file_url=?, link_url=?,
		starts_at=?, ends_at=?, weight=?, skip_after_sec=?, active=?, updated_at=? WHERE id=?`,
		a.Name, string(a.Type), string(a.Position), a.FileURL, a.LinkURL, formatTimePtr(a.StartsAt), formatTimePtr(a.EndsAt),
		a.Weight, a.SkipAfterSec, boolInt(a.Active), formatTime(a.UpdatedAt), a.ID))
}

// Delete borra un anuncio y sus eventos.
func (r *AdRepo) Delete(ctx context.Context, id int64) error {
	return checkAffected(r.db.ExecContext(ctx, `DELETE FROM ads WHERE id=?`, id))
}

// CountActive cuenta los anuncios marcados como activos.
func (r *AdRepo) CountActive(ctx context.Context) (int, error) {
	var n int
	err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM ads WHERE active = 1`).Scan(&n)
	return n, err
}

// AdEventRepo implementa domain.AdEventRepository.
type AdEventRepo struct{ db *sql.DB }

// NewAdEventRepo crea el repositorio de eventos de anuncios.
func NewAdEventRepo(db *sql.DB) *AdEventRepo { return &AdEventRepo{db: db} }

// Record guarda un evento.
func (r *AdEventRepo) Record(ctx context.Context, e domain.AdEvent) error {
	if e.CreatedAt.IsZero() {
		e.CreatedAt = time.Now()
	}
	_, err := r.db.ExecContext(ctx, `INSERT INTO ad_events (ad_id, type, ip_hash, created_at) VALUES (?,?,?,?)`,
		e.AdID, string(e.Type), e.IPHash, formatTime(e.CreatedAt))
	if err != nil {
		return fmt.Errorf("ad_events: record: %w", err)
	}
	return nil
}

// Stats agrega los eventos por anuncio en el rango [from, to).
// Incluye todos los anuncios aunque no tengan eventos.
func (r *AdEventRepo) Stats(ctx context.Context, from, to time.Time) ([]domain.AdStats, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT a.id, a.name, a.type,
		       COALESCE(SUM(e.type = 'impression'), 0),
		       COALESCE(SUM(e.type = 'click'), 0),
		       COALESCE(SUM(e.type = 'complete'), 0),
		       COALESCE(SUM(e.type = 'skip'), 0)
		FROM ads a
		LEFT JOIN ad_events e ON e.ad_id = a.id AND e.created_at >= ? AND e.created_at < ?
		GROUP BY a.id
		ORDER BY 4 DESC, a.id DESC`, formatTime(from), formatTime(to))
	if err != nil {
		return nil, fmt.Errorf("ad_events: stats: %w", err)
	}
	defer rows.Close()
	var out []domain.AdStats
	for rows.Next() {
		var s domain.AdStats
		var typ string
		if err := rows.Scan(&s.AdID, &s.Name, &typ, &s.Impressions, &s.Clicks, &s.Completes, &s.Skips); err != nil {
			return nil, err
		}
		s.Type = domain.AdType(typ)
		out = append(out, s)
	}
	return out, rows.Err()
}
