package store

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/jvrsantacruz/agent-status/internal/event"

	_ "modernc.org/sqlite"
)

// schemaVersion is stored in PRAGMA user_version. The events table is a
// short history, so an older schema is dropped rather than migrated.
const schemaVersion = 2

type Store struct {
	db *sql.DB
}

// Row is one stored event with what the daemon did with it.
type Row struct {
	SessionID   string
	Harness     string
	Event       string
	PID         int
	Pane        string
	TimestampMs int64
	ReceivedMs  int64
	Known       bool
}

func Open(path string) (*Store, error) {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("store mkdir %s: %w", dir, err)
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("store open: %w", err)
	}
	db.SetMaxOpenConns(1)
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
		db.Close()
		return nil, fmt.Errorf("store migrate: %w", err)
	}
	if err := os.Chmod(path, 0o600); err != nil {
		db.Close()
		return nil, fmt.Errorf("store chmod: %w", err)
	}
	return &Store{db: db}, nil
}

func migrate(db *sql.DB) error {
	var v int
	if err := db.QueryRow("PRAGMA user_version").Scan(&v); err != nil {
		return err
	}
	if v == schemaVersion {
		return nil
	}
	// No message column: the text of a notification is not kept on disk.
	_, err := db.Exec(fmt.Sprintf(`
		DROP TABLE IF EXISTS events;
		DROP TABLE IF EXISTS sessions;
		CREATE TABLE events (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			session_id TEXT NOT NULL,
			harness TEXT NOT NULL,
			event TEXT NOT NULL,
			pid INTEGER NOT NULL,
			tmux_pane TEXT NOT NULL DEFAULT '',
			timestamp_ms INTEGER NOT NULL,
			received_ms INTEGER NOT NULL,
			known INTEGER NOT NULL
		);
		CREATE INDEX idx_events_session ON events(session_id, timestamp_ms);
		PRAGMA user_version = %d;
	`, schemaVersion))
	return err
}

// AppendEvent stores an event, when it arrived and whether it was understood.
func (s *Store) AppendEvent(e event.Event, received time.Time, known bool) error {
	_, err := s.db.Exec(
		`INSERT INTO events (session_id, harness, event, pid, tmux_pane, timestamp_ms, received_ms, known)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		e.SessionID, e.Harness, e.Event, e.PID, e.Pane, e.TimestampMs, received.UnixMilli(), known,
	)
	if err != nil {
		return fmt.Errorf("append event: %w", err)
	}
	return nil
}

// Recent returns the last n events of a session, newest first.
func (s *Store) Recent(sessionID string, n int) ([]Row, error) {
	rows, err := s.db.Query(
		`SELECT session_id, harness, event, pid, tmux_pane, timestamp_ms, received_ms, known
		 FROM events WHERE session_id = ? ORDER BY id DESC LIMIT ?`, sessionID, n)
	if err != nil {
		return nil, fmt.Errorf("recent events %s: %w", sessionID, err)
	}
	defer rows.Close()
	var out []Row
	for rows.Next() {
		var r Row
		if err := rows.Scan(&r.SessionID, &r.Harness, &r.Event, &r.PID, &r.Pane,
			&r.TimestampMs, &r.ReceivedMs, &r.Known); err != nil {
			return nil, fmt.Errorf("recent events %s: %w", sessionID, err)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// PruneEvents removes events older than ttl and returns how many.
func (s *Store) PruneEvents(ttl time.Duration, now time.Time) (int64, error) {
	cutoff := now.Add(-ttl).UnixMilli()
	res, err := s.db.Exec(`DELETE FROM events WHERE timestamp_ms < ?`, cutoff)
	if err != nil {
		return 0, fmt.Errorf("prune events: %w", err)
	}
	return res.RowsAffected()
}

// Close closes the database.
func (s *Store) Close() error {
	return s.db.Close()
}
