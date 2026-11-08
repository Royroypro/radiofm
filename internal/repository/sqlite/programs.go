package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"radioclub25/internal/domain"
)

// ProgramRepo implementa domain.ProgramRepository.
type ProgramRepo struct{ db *sql.DB }

// NewProgramRepo crea el repositorio de programación.
func NewProgramRepo(db *sql.DB) *ProgramRepo { return &ProgramRepo{db: db} }

const programCols = `id, title, host, description, image_url, weekday, start_time, end_time, active`

func scanProgram(r rowScanner) (*domain.Program, error) {
	var p domain.Program
	var wd, active int
	if err := r.Scan(&p.ID, &p.Title, &p.Host, &p.Description, &p.ImageURL, &wd, &p.StartTime, &p.EndTime, &active); err != nil {
		return nil, err
	}
	p.Weekday, p.Active = time.Weekday(wd), active == 1
	return &p, nil
}

// Create inserta un programa.
func (r *ProgramRepo) Create(ctx context.Context, p *domain.Program) error {
	res, err := r.db.ExecContext(ctx,
		`INSERT INTO programs (title, host, description, image_url, weekday, start_time, end_time, active) VALUES (?,?,?,?,?,?,?,?)`,
		p.Title, p.Host, p.Description, p.ImageURL, int(p.Weekday), p.StartTime, p.EndTime, boolInt(p.Active))
	if err != nil {
		return fmt.Errorf("programs: create: %w", err)
	}
	p.ID, _ = res.LastInsertId()
	return nil
}

// Get busca un programa por ID.
func (r *ProgramRepo) Get(ctx context.Context, id int64) (*domain.Program, error) {
	p, err := scanProgram(r.db.QueryRowContext(ctx, `SELECT `+programCols+` FROM programs WHERE id=?`, id))
	return p, notFound(err)
}

// List devuelve todos los programas ordenados por día (lunes primero) y hora.
func (r *ProgramRepo) List(ctx context.Context) ([]domain.Program, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT `+programCols+` FROM programs ORDER BY (weekday+6)%7, start_time`)
	if err != nil {
		return nil, fmt.Errorf("programs: list: %w", err)
	}
	defer rows.Close()
	var out []domain.Program
	for rows.Next() {
		p, err := scanProgram(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *p)
	}
	return out, rows.Err()
}

// Update modifica un programa.
func (r *ProgramRepo) Update(ctx context.Context, p *domain.Program) error {
	return checkAffected(r.db.ExecContext(ctx,
		`UPDATE programs SET title=?, host=?, description=?, image_url=?, weekday=?, start_time=?, end_time=?, active=? WHERE id=?`,
		p.Title, p.Host, p.Description, p.ImageURL, int(p.Weekday), p.StartTime, p.EndTime, boolInt(p.Active), p.ID))
}

// Delete borra un programa.
func (r *ProgramRepo) Delete(ctx context.Context, id int64) error {
	return checkAffected(r.db.ExecContext(ctx, `DELETE FROM programs WHERE id=?`, id))
}
