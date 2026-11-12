package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"radioclub25/internal/domain"
)

// StudioRepo implementa domain.StudioRepository sobre studio_stations (una
// fila por frecuencia que tiene o tuvo PC del estudio).
type StudioRepo struct{ db *sql.DB }

// NewStudioRepo crea el repositorio del estado del estudio.
func NewStudioRepo(db *sql.DB) *StudioRepo { return &StudioRepo{db: db} }

const studioCols = `frequency_id, token_hash, token_created_at, last_seen, version, mode, encoder_json,
	now_playing_json, now_playing_at, status_json`

func scanStudio(r rowScanner) (*domain.StudioState, error) {
	var s domain.StudioState
	var tokenAt, lastSeen, npAt sql.NullString
	var mode, np string
	if err := r.Scan(&s.FrequencyID, &s.TokenHash, &tokenAt, &lastSeen, &s.Version, &mode, &s.EncoderJSON, &np,
		&npAt, &s.StatusJSON); err != nil {
		return nil, err
	}
	s.TokenCreatedAt, s.LastSeen, s.NowPlayingAt = parseTimePtr(tokenAt), parseTimePtr(lastSeen), parseTimePtr(npAt)
	s.Mode = domain.StudioMode(mode)
	if np != "" {
		var n domain.NowPlaying
		if json.Unmarshal([]byte(np), &n) == nil {
			s.NowPlaying = &n
		}
	}
	return &s, nil
}

// Get devuelve el estado de la PC de una frecuencia (vacío si nunca tuvo).
func (r *StudioRepo) Get(ctx context.Context, frequencyID int64) (*domain.StudioState, error) {
	s, err := scanStudio(r.db.QueryRowContext(ctx, `SELECT `+studioCols+` FROM studio_stations WHERE frequency_id = ?`,
		frequencyID))
	if err == sql.ErrNoRows {
		return &domain.StudioState{FrequencyID: frequencyID, EncoderJSON: "{}", StatusJSON: "{}"}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("studio_stations: get: %w", err)
	}
	return s, nil
}

// List devuelve el estado de todas las PC, por frecuencia.
func (r *StudioRepo) List(ctx context.Context) (map[int64]*domain.StudioState, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT `+studioCols+` FROM studio_stations`)
	if err != nil {
		return nil, fmt.Errorf("studio_stations: list: %w", err)
	}
	defer rows.Close()
	out := map[int64]*domain.StudioState{}
	for rows.Next() {
		s, err := scanStudio(rows)
		if err != nil {
			return nil, err
		}
		out[s.FrequencyID] = s
	}
	return out, rows.Err()
}

// FindToken busca la frecuencia dueña del token (por su hash).
func (r *StudioRepo) FindToken(ctx context.Context, hash string) (int64, error) {
	var id int64
	err := r.db.QueryRowContext(ctx, `SELECT frequency_id FROM studio_stations WHERE token_hash = ? AND token_hash <> ''`,
		hash).Scan(&id)
	return id, notFound(err)
}

// ensure crea la fila de la frecuencia si no existe.
func (r *StudioRepo) ensure(ctx context.Context, frequencyID int64) error {
	_, err := r.db.ExecContext(ctx, `INSERT INTO studio_stations (frequency_id) VALUES (?) ON CONFLICT DO NOTHING`, frequencyID)
	return err
}

// SetToken guarda el hash del token (o lo borra si hash es "").
func (r *StudioRepo) SetToken(ctx context.Context, frequencyID int64, hash string, at time.Time) error {
	if err := r.ensure(ctx, frequencyID); err != nil {
		return err
	}
	var created any
	if hash != "" {
		created = formatTime(at)
	}
	_, err := r.db.ExecContext(ctx, `UPDATE studio_stations SET token_hash = ?, token_created_at = ? WHERE frequency_id = ?`,
		hash, created, frequencyID)
	return err
}

// Heartbeat registra que la PC sigue conectada.
func (r *StudioRepo) Heartbeat(ctx context.Context, frequencyID int64, at time.Time, version string, mode domain.StudioMode, encoderJSON, statusJSON string) error {
	if err := r.ensure(ctx, frequencyID); err != nil {
		return err
	}
	if statusJSON == "" {
		statusJSON = "{}"
	}
	_, err := r.db.ExecContext(ctx, `UPDATE studio_stations SET last_seen = ?, version = ?, mode = ?, encoder_json = ?,
		status_json = ? WHERE frequency_id = ?`, formatTime(at), version, string(mode), encoderJSON, statusJSON, frequencyID)
	return err
}

// SetNowPlaying guarda lo que suena. También cuenta como señal de vida.
func (r *StudioRepo) SetNowPlaying(ctx context.Context, frequencyID int64, np domain.NowPlaying, at time.Time) error {
	if err := r.ensure(ctx, frequencyID); err != nil {
		return err
	}
	b, err := json.Marshal(np)
	if err != nil {
		return err
	}
	_, err = r.db.ExecContext(ctx, `UPDATE studio_stations SET now_playing_json = ?, now_playing_at = ?, last_seen = ?
		WHERE frequency_id = ?`, string(b), formatTime(at), formatTime(at), frequencyID)
	return err
}

// ---------- Control remoto ----------

const cmdCols = `id, frequency_id, kind, arg, created_at, created_by, taken_at, done_at, ok, result`

func scanCommand(r rowScanner) (domain.StudioCommand, error) {
	var c domain.StudioCommand
	var created string
	var taken, done sql.NullString
	var ok int
	err := r.Scan(&c.ID, &c.FrequencyID, &c.Kind, &c.Arg, &created, &c.CreatedBy, &taken, &done, &ok, &c.Result)
	c.CreatedAt, c.TakenAt, c.DoneAt, c.OK = parseTime(created), parseTimePtr(taken), parseTimePtr(done), ok == 1
	return c, err
}

// AddCommand encola una orden para la PC.
func (r *StudioRepo) AddCommand(ctx context.Context, c *domain.StudioCommand) error {
	res, err := r.db.ExecContext(ctx, `INSERT INTO studio_commands (frequency_id, kind, arg, created_at, created_by)
		VALUES (?,?,?,?,?)`, c.FrequencyID, c.Kind, c.Arg, formatTime(c.CreatedAt), c.CreatedBy)
	if err != nil {
		return fmt.Errorf("studio_commands: insertar: %w", err)
	}
	c.ID, _ = res.LastInsertId()
	// limpieza: se guardan las últimas 200 por frecuencia
	_, _ = r.db.ExecContext(ctx, `DELETE FROM studio_commands WHERE frequency_id = ? AND id NOT IN
		(SELECT id FROM studio_commands WHERE frequency_id = ? ORDER BY id DESC LIMIT 200)`, c.FrequencyID, c.FrequencyID)
	return nil
}

// TakeCommands devuelve las órdenes pendientes recientes y las marca tomadas.
func (r *StudioRepo) TakeCommands(ctx context.Context, frequencyID int64, since, at time.Time) ([]domain.StudioCommand, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	rows, err := tx.QueryContext(ctx, `SELECT `+cmdCols+` FROM studio_commands
		WHERE frequency_id = ? AND taken_at IS NULL AND created_at >= ? ORDER BY id`, frequencyID, formatTime(since))
	if err != nil {
		return nil, err
	}
	var out []domain.StudioCommand
	for rows.Next() {
		c, err := scanCommand(rows)
		if err != nil {
			rows.Close()
			return nil, err
		}
		out = append(out, c)
	}
	rows.Close()
	if _, err := tx.ExecContext(ctx, `UPDATE studio_commands SET taken_at = ? WHERE frequency_id = ? AND taken_at IS NULL`,
		formatTime(at), frequencyID); err != nil { // las vencidas también: ya no se ejecutan
		return nil, err
	}
	return out, tx.Commit()
}

// FinishCommand guarda el resultado de una orden.
func (r *StudioRepo) FinishCommand(ctx context.Context, frequencyID, id int64, ok bool, result string, at time.Time) error {
	return checkAffected(r.db.ExecContext(ctx, `UPDATE studio_commands SET done_at = ?, ok = ?, result = ?
		WHERE id = ? AND frequency_id = ?`, formatTime(at), boolInt(ok), result, id, frequencyID))
}

// RecentCommands devuelve las últimas órdenes de una frecuencia (más nuevas primero).
func (r *StudioRepo) RecentCommands(ctx context.Context, frequencyID int64, limit int) ([]domain.StudioCommand, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT `+cmdCols+` FROM studio_commands WHERE frequency_id = ?
		ORDER BY id DESC LIMIT ?`, frequencyID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.StudioCommand
	for rows.Next() {
		c, err := scanCommand(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}
