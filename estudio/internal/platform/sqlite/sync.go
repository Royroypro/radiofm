package sqlite

import (
	"context"
	"database/sql"
	"strings"
	"time"
)

// QueuedAirplay es una emisión de cuña pendiente de enviar al panel web.
type QueuedAirplay struct {
	PlayUUID string
	SpotID   int64 // remote_id
	AiredAt  time.Time
	Duration time.Duration
	Complete bool
	Attempts int
}

// AirplayQueue es la cola local de emisiones.
type AirplayQueue struct{ db *sql.DB }

// NewAirplayQueue crea la cola.
func NewAirplayQueue(db *sql.DB) *AirplayQueue { return &AirplayQueue{db: db} }

// Enqueue agrega una emisión (si ya estaba, no hace nada).
func (q *AirplayQueue) Enqueue(ctx context.Context, a QueuedAirplay) error {
	_, err := q.db.ExecContext(ctx, `INSERT INTO airplay_queue (play_uuid, spot_id, aired_at, duration_ms, complete, created_at)
		VALUES (?,?,?,?,?,?) ON CONFLICT(play_uuid) DO NOTHING`,
		a.PlayUUID, a.SpotID, fmtTime(a.AiredAt), ms(a.Duration), b2i(a.Complete), fmtTime(time.Now()))
	return err
}

// Batch devuelve hasta limit emisiones, las más antiguas primero.
func (q *AirplayQueue) Batch(ctx context.Context, limit int) ([]QueuedAirplay, error) {
	rows, err := q.db.QueryContext(ctx, `SELECT play_uuid, spot_id, aired_at, duration_ms, complete, attempts
		FROM airplay_queue ORDER BY aired_at LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []QueuedAirplay
	for rows.Next() {
		var a QueuedAirplay
		var at string
		var d int64
		var complete int
		if err := rows.Scan(&a.PlayUUID, &a.SpotID, &at, &d, &complete, &a.Attempts); err != nil {
			return nil, err
		}
		a.AiredAt, a.Duration, a.Complete = parseTime(at), dur(d), complete == 1
		out = append(out, a)
	}
	return out, rows.Err()
}

// Remove quita emisiones ya enviadas (o rechazadas definitivamente).
func (q *AirplayQueue) Remove(ctx context.Context, uuids []string) error {
	if len(uuids) == 0 {
		return nil
	}
	args := make([]any, len(uuids))
	for i, u := range uuids {
		args[i] = u
	}
	_, err := q.db.ExecContext(ctx, `DELETE FROM airplay_queue WHERE play_uuid IN (?`+strings.Repeat(",?", len(uuids)-1)+`)`, args...)
	return err
}

// Count devuelve cuántas emisiones esperan envío.
func (q *AirplayQueue) Count(ctx context.Context) (int, error) {
	var n int
	err := q.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM airplay_queue`).Scan(&n)
	return n, err
}

// KV guarda valores pequeños de estado.
type KV struct{ db *sql.DB }

// NewKV crea el almacén.
func NewKV(db *sql.DB) *KV { return &KV{db: db} }

// Get devuelve un valor ("" si no existe).
func (k *KV) Get(ctx context.Context, key string) string {
	var v string
	_ = k.db.QueryRowContext(ctx, `SELECT value FROM kv WHERE key=?`, key).Scan(&v)
	return v
}

// Set guarda un valor.
func (k *KV) Set(ctx context.Context, key, value string) error {
	_, err := k.db.ExecContext(ctx, `INSERT INTO kv (key, value) VALUES (?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`, key, value)
	return err
}
