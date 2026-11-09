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
	now_playing_json, now_playing_at`

func scanStudio(r rowScanner) (*domain.StudioState, error) {
	var s domain.StudioState
	var tokenAt, lastSeen, npAt sql.NullString
	var mode, np string
	if err := r.Scan(&s.FrequencyID, &s.TokenHash, &tokenAt, &lastSeen, &s.Version, &mode, &s.EncoderJSON, &np,
		&npAt); err != nil {
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
		return &domain.StudioState{FrequencyID: frequencyID, EncoderJSON: "{}"}, nil
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
func (r *StudioRepo) Heartbeat(ctx context.Context, frequencyID int64, at time.Time, version string, mode domain.StudioMode, encoderJSON string) error {
	if err := r.ensure(ctx, frequencyID); err != nil {
		return err
	}
	_, err := r.db.ExecContext(ctx, `UPDATE studio_stations SET last_seen = ?, version = ?, mode = ?, encoder_json = ?
		WHERE frequency_id = ?`, formatTime(at), version, string(mode), encoderJSON, frequencyID)
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
