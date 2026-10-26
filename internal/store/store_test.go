package store

import (
	"sync"
	"testing"
	"time"

	"github.com/jvrsantacruz/agent-status-watcher/internal/event"
	"github.com/jvrsantacruz/agent-status-watcher/internal/state"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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

	// Tables should exist — query them
	rows, err := s.db.Query("SELECT name FROM sqlite_master WHERE type='table' ORDER BY name")
	require.NoError(t, err)
	defer rows.Close()

	var tables []string
	for rows.Next() {
		var name string
		rows.Scan(&name)
		tables = append(tables, name)
	}
	assert.Contains(t, tables, "events")
	assert.Contains(t, tables, "sessions")
}

func TestAppendEvent(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	e := event.Event{SessionID: "s1", Harness: "pi", Event: "working", PID: 100,
		TmuxTarget: "m:%0", TimestampMs: 1000, Message: "starting"}
	err := s.AppendEvent(e)
	require.NoError(t, err)
}

func TestAppendMultipleEvents(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	for i := range 10 {
		err := s.AppendEvent(event.Event{
			SessionID: "s1", Harness: "pi", Event: "working",
			PID: 100 + i, TimestampMs: int64(1000 + i),
		})
		require.NoError(t, err)
	}
}

func TestUpsertSessionInsert(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	sess := state.Session{Harness: "claude", State: state.Question, PID: 42,
		TmuxTarget: "main:%7", UpdatedAt: 500}
	err := s.UpsertSession(sess, "s1")
	require.NoError(t, err)
}

func TestUpsertSessionUpdate(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	sess1 := state.Session{Harness: "claude", State: state.Working, PID: 42,
		TmuxTarget: "main:%7", UpdatedAt: 500}
	require.NoError(t, s.UpsertSession(sess1, "s1"))

	sess2 := state.Session{Harness: "claude", State: state.Idle, PID: 42,
		TmuxTarget: "main:%7", UpdatedAt: 600}
	err := s.UpsertSession(sess2, "s1")
	require.NoError(t, err)
}

func TestUpsertSessionDeadSince(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	ms := int64(1700000000000)
	sess := state.Session{State: state.Dead, DeadSince: &ms, UpdatedAt: ms}
	require.NoError(t, s.UpsertSession(sess, "s1"))

	// Read back via sqlite directly
	var deadSince *int64
	err := s.db.QueryRow("SELECT dead_since FROM sessions WHERE session_id=?", "s1").Scan(&deadSince)
	require.NoError(t, err)
	require.NotNil(t, deadSince)
	assert.Equal(t, ms, *deadSince)
}

func TestUpsertSessionDeadSinceNull(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	sess := state.Session{State: state.Working, DeadSince: nil, UpdatedAt: 1}
	require.NoError(t, s.UpsertSession(sess, "s1"))

	var deadSince *int64
	err := s.db.QueryRow("SELECT dead_since FROM sessions WHERE session_id=?", "s1").Scan(&deadSince)
	require.NoError(t, err)
	assert.Nil(t, deadSince)
}

func TestPruneEvents(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)

	now := time.Date(2026, 10, 25, 12, 0, 0, 0, time.UTC)
	// Old event (outside 7-day window)
	require.NoError(t, s.AppendEvent(event.Event{SessionID: "s1", Event: "working", TimestampMs: now.Add(-8 * 24 * time.Hour).UnixMilli()}))
	// Recent event (within window)
	require.NoError(t, s.AppendEvent(event.Event{SessionID: "s1", Event: "idle", TimestampMs: now.Add(-1 * time.Hour).UnixMilli()}))

	err := s.PruneEvents(7*24*time.Hour, now)
	require.NoError(t, err)

	var count int
	s.db.QueryRow("SELECT COUNT(*) FROM events").Scan(&count)
	assert.Equal(t, 1, count)
}

func TestConcurrentAccess(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)

	var wg sync.WaitGroup
	for i := range 20 {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			s.AppendEvent(event.Event{
				SessionID: "s1", Harness: "pi", Event: "working",
				PID: n, TimestampMs: int64(n),
			})
			s.UpsertSession(state.Session{
				State: state.Working, PID: n, UpdatedAt: int64(n),
			}, "s1")
		}(i)
	}
	wg.Wait()

	var count int
	s.db.QueryRow("SELECT COUNT(*) FROM events").Scan(&count)
	assert.Equal(t, 20, count)
}

func TestCloseTwice(t *testing.T) {
	t.Parallel()
	s := newTestStore(t)
	assert.NoError(t, s.Close())
	assert.NoError(t, s.Close()) // second close should not panic
}
