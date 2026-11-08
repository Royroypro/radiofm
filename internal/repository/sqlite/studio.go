package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"radioclub25/internal/domain"
)

// StudioRepo implementa domain.StudioRepository sobre la fila única de studio_state.
type StudioRepo struct{ db *sql.DB }

// NewStudioRepo crea el repositorio del estado del estudio.
func NewStudioRepo(db *sql.DB) *StudioRepo { return &StudioRepo{db: db} }

// Get devuelve el estado actual.
func (r *StudioRepo) Get(ctx context.Context) (*domain.StudioState, error) {
	var s domain.StudioState
	var tokenAt, lastSeen, npAt sql.NullString
	var mode, np string
	err := r.db.QueryRowContext(ctx, `SELECT token_hash, token_created_at, last_seen, version, mode, encoder_json,
		now_playing_json, now_playing_at FROM studio_state WHERE id = 1`).
		Scan(&s.TokenHash, &tokenAt, &lastSeen, &s.Version, &mode, &s.EncoderJSON, &np, &npAt)
	if err != nil {
		return nil, fmt.Errorf("studio_state: get: %w", notFound(err))
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

// SetToken guarda el hash del token (o lo borra si hash es "").
func (r *StudioRepo) SetToken(ctx context.Context, hash string, at time.Time) error {
	var created any
	if hash != "" {
		created = formatTime(at)
	}
	_, err := r.db.ExecContext(ctx, `UPDATE studio_state SET token_hash = ?, token_created_at = ? WHERE id = 1`, hash, created)
	return err
}

// Heartbeat registra que la PC sigue conectada.
func (r *StudioRepo) Heartbeat(ctx context.Context, at time.Time, version string, mode domain.StudioMode, encoderJSON string) error {
	_, err := r.db.ExecContext(ctx, `UPDATE studio_state SET last_seen = ?, version = ?, mode = ?, encoder_json = ?
		WHERE id = 1`, formatTime(at), version, string(mode), encoderJSON)
	return err
}

// SetNowPlaying guarda lo que suena. También cuenta como señal de vida.
func (r *StudioRepo) SetNowPlaying(ctx context.Context, np domain.NowPlaying, at time.Time) error {
	b, err := json.Marshal(np)
	if err != nil {
		return err
	}
	_, err = r.db.ExecContext(ctx, `UPDATE studio_state SET now_playing_json = ?, now_playing_at = ?, last_seen = ?
		WHERE id = 1`, string(b), formatTime(at), formatTime(at))
	return err
}
