package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"radioclub25/internal/domain"
)

// FMAdvertiserRepo implementa domain.FMAdvertiserRepository.
type FMAdvertiserRepo struct{ db *sql.DB }

// NewFMAdvertiserRepo crea el repositorio de anunciantes.
func NewFMAdvertiserRepo(db *sql.DB) *FMAdvertiserRepo { return &FMAdvertiserRepo{db: db} }

func scanAdvertiser(r rowScanner) (*domain.FMAdvertiser, error) {
	var a domain.FMAdvertiser
	var created string
	if err := r.Scan(&a.ID, &a.Name, &a.Sector, &created); err != nil {
		return nil, err
	}
	a.CreatedAt = parseTime(created)
	return &a, nil
}

// List devuelve los anunciantes por nombre.
func (r *FMAdvertiserRepo) List(ctx context.Context) ([]domain.FMAdvertiser, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT id, name, sector, created_at FROM fm_advertisers ORDER BY name COLLATE NOCASE`)
	if err != nil {
		return nil, fmt.Errorf("fm_advertisers: list: %w", err)
	}
	defer rows.Close()
	var out []domain.FMAdvertiser
	for rows.Next() {
		a, err := scanAdvertiser(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *a)
	}
	return out, rows.Err()
}

// Get busca un anunciante.
func (r *FMAdvertiserRepo) Get(ctx context.Context, id int64) (*domain.FMAdvertiser, error) {
	a, err := scanAdvertiser(r.db.QueryRowContext(ctx, `SELECT id, name, sector, created_at FROM fm_advertisers WHERE id=?`, id))
	return a, notFound(err)
}

// ByName busca un anunciante por nombre (sin distinguir mayúsculas).
func (r *FMAdvertiserRepo) ByName(ctx context.Context, name string) (*domain.FMAdvertiser, error) {
	a, err := scanAdvertiser(r.db.QueryRowContext(ctx,
		`SELECT id, name, sector, created_at FROM fm_advertisers WHERE name=? COLLATE NOCASE`, name))
	return a, notFound(err)
}

// Save crea o modifica un anunciante. Al cambiar nombre o rubro, se copian a sus cuñas.
func (r *FMAdvertiserRepo) Save(ctx context.Context, a *domain.FMAdvertiser) error {
	if a.ID == 0 {
		a.CreatedAt = time.Now()
		res, err := r.db.ExecContext(ctx, `INSERT INTO fm_advertisers (name, sector, created_at) VALUES (?,?,?)`,
			a.Name, a.Sector, formatTime(a.CreatedAt))
		if err != nil {
			return uniqueName(fmt.Errorf("fm_advertisers: create: %w", err))
		}
		a.ID, _ = res.LastInsertId()
		return nil
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if err := checkAffected(tx.ExecContext(ctx, `UPDATE fm_advertisers SET name=?, sector=? WHERE id=?`, a.Name, a.Sector, a.ID)); err != nil {
		return uniqueName(err)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE fm_spots SET advertiser=?, sector=?
		WHERE campaign_id IN (SELECT id FROM fm_campaigns WHERE advertiser_id=?)`, a.Name, a.Sector, a.ID); err != nil {
		return err
	}
	return tx.Commit()
}

// Delete borra un anunciante sin campañas.
func (r *FMAdvertiserRepo) Delete(ctx context.Context, id int64) error {
	var n int
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM fm_campaigns WHERE advertiser_id=?`, id).Scan(&n); err != nil {
		return err
	}
	if n > 0 {
		return domain.ErrConflict
	}
	return checkAffected(r.db.ExecContext(ctx, `DELETE FROM fm_advertisers WHERE id=?`, id))
}

func uniqueName(err error) error {
	if err != nil && strings.Contains(err.Error(), "UNIQUE") {
		return domain.Invalid("name", "Ya existe un anunciante con ese nombre")
	}
	return err
}

// FMCampaignRepo implementa domain.FMCampaignRepository.
type FMCampaignRepo struct{ db *sql.DB }

// NewFMCampaignRepo crea el repositorio de campañas.
func NewFMCampaignRepo(db *sql.DB) *FMCampaignRepo { return &FMCampaignRepo{db: db} }

const campaignCols = `c.id, c.advertiser_id, a.name, a.sector, c.name, c.starts_at, c.ends_at, c.plays_per_day,
	c.hour_from, c.hour_to, c.days_mask, c.position, c.priority, c.frequency_id, c.active, c.created_at, c.updated_at`

const campaignFrom = ` FROM fm_campaigns c JOIN fm_advertisers a ON a.id = c.advertiser_id`

func scanCampaign(r rowScanner) (*domain.FMCampaign, error) {
	var c domain.FMCampaign
	var starts, ends sql.NullString
	var fid sql.NullInt64
	var active int
	var position, created, updated string
	if err := r.Scan(&c.ID, &c.AdvertiserID, &c.Advertiser, &c.Sector, &c.Name, &starts, &ends, &c.PlaysPerDay,
		&c.HourFrom, &c.HourTo, &c.DaysMask, &position, &c.Priority, &fid, &active, &created, &updated); err != nil {
		return nil, err
	}
	c.StartsAt, c.EndsAt, c.FrequencyID = parseTimePtr(starts), parseTimePtr(ends), fid.Int64
	c.Position, c.Active = domain.SpotPosition(position), active == 1
	c.CreatedAt, c.UpdatedAt = parseTime(created), parseTime(updated)
	return &c, nil
}

// List devuelve las campañas (sin versiones) por anunciante y nombre.
func (r *FMCampaignRepo) List(ctx context.Context) ([]domain.FMCampaign, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT `+campaignCols+campaignFrom+
		` ORDER BY a.name COLLATE NOCASE, c.name COLLATE NOCASE, c.id`)
	if err != nil {
		return nil, fmt.Errorf("fm_campaigns: list: %w", err)
	}
	defer rows.Close()
	var out []domain.FMCampaign
	for rows.Next() {
		c, err := scanCampaign(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *c)
	}
	return out, rows.Err()
}

// Get busca una campaña (sin versiones).
func (r *FMCampaignRepo) Get(ctx context.Context, id int64) (*domain.FMCampaign, error) {
	c, err := scanCampaign(r.db.QueryRowContext(ctx, `SELECT `+campaignCols+campaignFrom+` WHERE c.id=?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	return c, err
}

// Save crea o modifica una campaña.
func (r *FMCampaignRepo) Save(ctx context.Context, c *domain.FMCampaign) error {
	c.UpdatedAt = time.Now()
	args := []any{c.AdvertiserID, c.Name, formatTimePtr(c.StartsAt), formatTimePtr(c.EndsAt), c.PlaysPerDay, c.HourFrom,
		c.HourTo, daysOr(c.DaysMask), positionOr(c.Position), priorityOr(c.Priority), nullID(c.FrequencyID),
		boolInt(c.Active), formatTime(c.UpdatedAt)}
	if c.ID == 0 {
		c.CreatedAt = c.UpdatedAt
		res, err := r.db.ExecContext(ctx, `INSERT INTO fm_campaigns (advertiser_id, name, starts_at, ends_at, plays_per_day,
			hour_from, hour_to, days_mask, position, priority, frequency_id, active, updated_at, created_at)
			VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, append(args, formatTime(c.CreatedAt))...)
		if err != nil {
			return fmt.Errorf("fm_campaigns: create: %w", err)
		}
		c.ID, _ = res.LastInsertId()
		return nil
	}
	return checkAffected(r.db.ExecContext(ctx, `UPDATE fm_campaigns SET advertiser_id=?, name=?, starts_at=?, ends_at=?,
		plays_per_day=?, hour_from=?, hour_to=?, days_mask=?, position=?, priority=?, frequency_id=?, active=?, updated_at=?
		WHERE id=?`, append(args, c.ID)...))
}

// Delete borra una campaña y sus versiones (las emisiones se conservan).
func (r *FMCampaignRepo) Delete(ctx context.Context, id int64) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `DELETE FROM fm_spots WHERE campaign_id=?`, id); err != nil {
		return err
	}
	if err := checkAffected(tx.ExecContext(ctx, `DELETE FROM fm_campaigns WHERE id=?`, id)); err != nil {
		return err
	}
	return tx.Commit()
}

// SyncVersions copia las reglas de la campaña a todas sus versiones. Una
// campaña pausada pausa sus versiones; al reactivarla vuelven las que tienen
// archivo (el operador puede pausar una versión concreta desde la campaña).
func (r *FMCampaignRepo) SyncVersions(ctx context.Context, c domain.FMCampaign) error {
	_, err := r.db.ExecContext(ctx, `UPDATE fm_spots SET advertiser=?, sector=?, starts_at=?, ends_at=?, plays_per_day=?,
		hour_from=?, hour_to=?, days_mask=?, position=?, priority=?, frequency_id=?,
		active = CASE WHEN ? = 0 THEN 0 ELSE active END, updated_at=? WHERE campaign_id=?`,
		c.Advertiser, c.Sector, formatTimePtr(c.StartsAt), formatTimePtr(c.EndsAt), c.PlaysPerDay, c.HourFrom, c.HourTo,
		daysOr(c.DaysMask), positionOr(c.Position), priorityOr(c.Priority), nullID(c.FrequencyID), boolInt(c.Active),
		formatTime(time.Now()), c.ID)
	return err
}
