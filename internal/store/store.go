package store

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/jvrsantacruz/agent-status/internal/event"
	"github.com/jvrsantacruz/agent-status/internal/state"

	_ "modernc.org/sqlite"
)

type Store struct {
	db *sql.DB
}

func Open(path string) (*Store, error) {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("store mkdir %s: %w", dir, err)
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("store open: %w", err)
	}
	// Enable WAL and busy timeout via PRAGMAs
	for _, pragma := range []string{
		"PRAGMA journal_mode=WAL",
		"PRAGMA busy_timeout=5000",
	} {
		if _, err := db.Exec(pragma); err != nil {
			db.Close()
			return nil, fmt.Errorf("store pragma: %w", err)
		}
	}
	if err := migrate(db); err != nil {
		return nil, fmt.Errorf("store migrate: %w", err)
	}
	db.SetMaxOpenConns(1)
	return &Store{db: db}, nil
}

func migrate(db *sql.DB) error {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS events (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			session_id TEXT NOT NULL,
			harness TEXT NOT NULL,
			event TEXT NOT NULL,
			pid INTEGER NOT NULL,
			tmux_target TEXT NOT NULL DEFAULT '',
			timestamp_ms INTEGER NOT NULL,
			message TEXT NOT NULL DEFAULT ''
		);
		CREATE INDEX IF NOT EXISTS idx_events_session ON events(session_id, timestamp_ms);

		CREATE TABLE IF NOT EXISTS sessions (
			session_id TEXT PRIMARY KEY,
			harness TEXT NOT NULL,
			state TEXT NOT NULL,
			pid INTEGER NOT NULL DEFAULT 0,
			tmux_target TEXT NOT NULL DEFAULT '',
			dead_since INTEGER,
			updated_at INTEGER NOT NULL,
			message TEXT NOT NULL DEFAULT ''
		);
	`)
	return err
}

// AppendEvent stores a raw event.
func (s *Store) AppendEvent(e event.Event) error {
	_, err := s.db.Exec(
		`INSERT INTO events (session_id, harness, event, pid, tmux_target, timestamp_ms, message)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		e.SessionID, e.Harness, e.Event, e.PID, e.TmuxTarget, e.TimestampMs, e.Message,
	)
	if err != nil {
		return fmt.Errorf("append event: %w", err)
	}
	return nil
}

// UpsertSession writes the materialised session state.
func (s *Store) UpsertSession(sess state.Session, id string) error {
	_, err := s.db.Exec(
		`INSERT INTO sessions (session_id, harness, state, pid, tmux_target, dead_since, updated_at, message)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(session_id) DO UPDATE SET
			harness=excluded.harness, state=excluded.state, pid=excluded.pid,
			tmux_target=excluded.tmux_target, dead_since=excluded.dead_since,
			updated_at=excluded.updated_at, message=excluded.message`,
		id, sess.Harness, string(sess.State), sess.PID, sess.TmuxTarget,
		sess.DeadSince, sess.UpdatedAt, sess.Message,
	)
	if err != nil {
		return fmt.Errorf("upsert session %s: %w", id, err)
	}
	return nil
}

// PruneEvents removes events older than ttl.
func (s *Store) PruneEvents(ttl time.Duration, now time.Time) error {
	cutoff := now.Add(-ttl).UnixMilli()
	_, err := s.db.Exec(`DELETE FROM events WHERE timestamp_ms < ?`, cutoff)
	if err != nil {
		return fmt.Errorf("prune events: %w", err)
	}
	return nil
}

// Close closes the database.
func (s *Store) Close() error {
	return s.db.Close()
}
