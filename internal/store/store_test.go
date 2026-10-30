package store

import (
	"database/sql"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/jvrsantacruz/agent-status/internal/event"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var t0 = time.Date(2026, 10, 25, 12, 0, 0, 0, time.UTC)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	s, err := Open(t.TempDir() + "/test.db")
	require.NoError(t, err)
	t.Cleanup(func() { s.Close() })
	return s
}

func TestOpenAndMigrate(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)

	var tables []string
	rows, err := s.db.Query("SELECT name FROM sqlite_master WHERE type='table' ORDER BY name")
	require.NoError(t, err)
	defer rows.Close()
	for rows.Next() {
		var name string
		require.NoError(t, rows.Scan(&name))
		tables = append(tables, name)
	}
	assert.Contains(t, tables, "events")
	assert.NotContains(t, tables, "sessions")
}

func TestOpenDropsOldSchema(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "old.db")
	db, err := sql.Open("sqlite", path)
	require.NoError(t, err)
	_, err = db.Exec(`CREATE TABLE events (id INTEGER PRIMARY KEY, message TEXT);
		INSERT INTO events (message) VALUES ('secret prompt');
		CREATE TABLE sessions (session_id TEXT)`)
	require.NoError(t, err)
	require.NoError(t, db.Close())

	s, err := Open(path)
	require.NoError(t, err)
	defer s.Close()
	var n int
	require.NoError(t, s.db.QueryRow("SELECT COUNT(*) FROM events").Scan(&n))
	assert.Zero(t, n)
}

func TestOpenPermissions(t *testing.T) {
	t.Parallel()
	dir := filepath.Join(t.TempDir(), "state")
	s, err := Open(filepath.Join(dir, "state.db"))
	require.NoError(t, err)
	defer s.Close()

	info, err := os.Stat(dir)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o700), info.Mode().Perm())
	info, err = os.Stat(filepath.Join(dir, "state.db"))
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
}

func TestAppendAndRecent(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	for i, name := range []string{"working", "bogus", "idle"} {
		e := event.Event{SessionID: "s1", Harness: "pi", Event: name, PID: 100,
			Pane: "%0", TimestampMs: int64(1000 + i), Message: "not stored"}
		require.NoError(t, s.AppendEvent(e, t0, name != "bogus"))
	}
	require.NoError(t, s.AppendEvent(event.Event{SessionID: "s2", Event: "working"}, t0, true))

	rows, err := s.Recent("s1", 2)
	require.NoError(t, err)
	require.Len(t, rows, 2)
	assert.Equal(t, "idle", rows[0].Event)
	assert.Equal(t, "bogus", rows[1].Event)
	assert.False(t, rows[1].Known)
	assert.Equal(t, "%0", rows[0].Pane)
	assert.Equal(t, t0.UnixMilli(), rows[0].ReceivedMs)
}

func TestPruneEvents(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	old := event.Event{SessionID: "s1", Event: "working", TimestampMs: t0.Add(-8 * 24 * time.Hour).UnixMilli()}
	recent := event.Event{SessionID: "s1", Event: "idle", TimestampMs: t0.Add(-time.Hour).UnixMilli()}
	require.NoError(t, s.AppendEvent(old, t0, true))
	require.NoError(t, s.AppendEvent(recent, t0, true))

	n, err := s.PruneEvents(7*24*time.Hour, t0)
	require.NoError(t, err)
	assert.Equal(t, int64(1), n)
	rows, err := s.Recent("s1", 10)
	require.NoError(t, err)
	assert.Len(t, rows, 1)
}

func TestConcurrentAccess(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)

	var wg sync.WaitGroup
	for i := range 20 {
		wg.Go(func() {
			assert.NoError(t, s.AppendEvent(event.Event{SessionID: "s1", Event: "working", PID: i}, t0, true))
		})
	}
	wg.Wait()

	rows, err := s.Recent("s1", 100)
	require.NoError(t, err)
	assert.Len(t, rows, 20)
}

func TestCloseTwice(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	assert.NoError(t, s.Close())
	assert.NoError(t, s.Close())
}
