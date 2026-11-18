package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"radioclub25/internal/domain"
)

// EmisorReleaseRepo implementa domain.EmisorReleaseRepository.
type EmisorReleaseRepo struct{ db *sql.DB }

// NewEmisorReleaseRepo crea el repositorio de versiones del emisor.
func NewEmisorReleaseRepo(db *sql.DB) *EmisorReleaseRepo { return &EmisorReleaseRepo{db: db} }

const emisorCols = `id, version, file_name, sha256, size, notes, created_at, created_by`

func scanEmisorRelease(r rowScanner) (*domain.EmisorRelease, error) {
	var e domain.EmisorRelease
	var created string
	if err := r.Scan(&e.ID, &e.Version, &e.FileName, &e.SHA256, &e.Size, &e.Notes, &created, &e.CreatedBy); err != nil {
		return nil, err
	}
	e.CreatedAt = parseTime(created)
	return &e, nil
}

// Create inserta una versión. Una versión repetida es un error de validación.
func (r *EmisorReleaseRepo) Create(ctx context.Context, e *domain.EmisorRelease) error {
	res, err := r.db.ExecContext(ctx, `INSERT INTO emisor_releases (version, file_name, sha256, size, notes, created_at, created_by)
		VALUES (?,?,?,?,?,?,?)`, e.Version, e.FileName, e.SHA256, e.Size, e.Notes, formatTime(e.CreatedAt), e.CreatedBy)
	if err != nil {
		if isUnique(err) {
			return domain.Invalid("version", "Esa versión ya está publicada.")
		}
		return fmt.Errorf("emisor_releases: insertar: %w", err)
	}
	e.ID, _ = res.LastInsertId()
	return nil
}

// List devuelve las versiones, las más nuevas primero.
func (r *EmisorReleaseRepo) List(ctx context.Context) ([]domain.EmisorRelease, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT `+emisorCols+` FROM emisor_releases ORDER BY id DESC`)
	if err != nil {
		return nil, fmt.Errorf("emisor_releases: listar: %w", err)
	}
	defer rows.Close()
	var out []domain.EmisorRelease
	for rows.Next() {
		e, err := scanEmisorRelease(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *e)
	}
	return out, rows.Err()
}

// Get devuelve una versión por id.
func (r *EmisorReleaseRepo) Get(ctx context.Context, id int64) (*domain.EmisorRelease, error) {
	e, err := scanEmisorRelease(r.db.QueryRowContext(ctx, `SELECT `+emisorCols+` FROM emisor_releases WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	return e, err
}

// Delete borra una versión.
func (r *EmisorReleaseRepo) Delete(ctx context.Context, id int64) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM emisor_releases WHERE id = ?`, id)
	return err
}
