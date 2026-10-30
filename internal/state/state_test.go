// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 Javier Santacruz

package state

import (
	"testing"
	"time"

	"github.com/jvrsantacruz/agent-status/internal/event"
	"github.com/stretchr/testify/assert"
)

func e(sid, state string) event.Event {
	return event.Event{SessionID: sid, Event: state}
}

func TestApplyFourStates(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		events []event.Event
		want   SessionState
	}{
		{"working", []event.Event{e("s1", "working")}, Working},
		{"question", []event.Event{e("s1", "question")}, Question},
		{"idle", []event.Event{e("s1", "idle")}, Idle},
		{"error", []event.Event{e("s1", "error")}, Error},
		{"working→question", []event.Event{e("s1", "working"), e("s1", "question")}, Question},
		{"working→idle", []event.Event{e("s1", "working"), e("s1", "idle")}, Idle},
		{"working→error", []event.Event{e("s1", "working"), e("s1", "error")}, Error},
		{"error→working", []event.Event{e("s1", "error"), e("s1", "working")}, Working},
		{"question→working", []event.Event{e("s1", "question"), e("s1", "working")}, Working},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			sm := NewMachine()
			for _, e := range tc.events {
				sm.Apply(e)
			}
			assert.Equal(t, tc.want, sm.Snapshot()["s1"].State)
		})
	}
}

func TestApplyRemovesDeadOnAnyEvent(t *testing.T) {
	t.Parallel()
	sm := NewMachine()
	sm.Apply(e("s1", "working"))
	now := time.Now()
	sm.MarkDead("s1", now)
	s := sm.Snapshot()["s1"]
	assert.Equal(t, Dead, s.State)
	assert.NotNil(t, s.DeadSince)

	// A new event clears dead_since and restores working
	sm.Apply(e("s1", "working"))
	s2 := sm.Snapshot()["s1"]
	assert.Equal(t, Working, s2.State)
	assert.Nil(t, s2.DeadSince)
	assert.Equal(t, 0, s2.PID)
}

func TestApplyUnknownEventIgnored(t *testing.T) {
	t.Parallel()
	sm := NewMachine()
	out := sm.Apply(e("s1", "unknown"))
	assert.False(t, out.Known)
	assert.Equal(t, Idle, sm.Snapshot()["s1"].State)
	assert.Equal(t, "event:unknown(unknown)", sm.Snapshot()["s1"].Reason)
}

func TestApplyOutcomeAndReason(t *testing.T) {
	t.Parallel()
	sm := NewMachine()
	sm.Apply(e("s1", "working"))
	out := sm.Apply(e("s1", "question"))
	assert.Equal(t, Outcome{Prev: Working, Next: Question, Known: true}, out)
	assert.Equal(t, "event:question", sm.Snapshot()["s1"].Reason)
}

func TestApplyEndRemovesSession(t *testing.T) {
	t.Parallel()
	sm := NewMachine()
	sm.Apply(e("s1", "working"))
	sm.Apply(e("s1", End))
	assert.NotContains(t, sm.Snapshot(), "s1")
}

func TestMarkDeadReason(t *testing.T) {
	t.Parallel()
	sm := NewMachine()
	sm.Apply(event.Event{SessionID: "s1", Event: "working", PID: 7})
	sm.MarkDead("s1", time.Now())
	assert.Equal(t, "liveness:pid_gone", sm.Snapshot()["s1"].Reason)
}

func TestApplyMultipleSessions(t *testing.T) {
	t.Parallel()
	sm := NewMachine()
	sm.Apply(e("s1", "working"))
	sm.Apply(e("s2", "question"))
	sm.Apply(e("s1", "idle"))

	snap := sm.Snapshot()
	assert.Equal(t, Idle, snap["s1"].State)
	assert.Equal(t, Question, snap["s2"].State)
}

func TestMarkDead(t *testing.T) {
	t.Parallel()
	sm := NewMachine()
	sm.Apply(event.Event{SessionID: "s1", Event: "working", PID: 123})
	now := time.Now()
	sm.MarkDead("s1", now)

	s := sm.Snapshot()["s1"]
	assert.Equal(t, Dead, s.State)
	assert.Equal(t, 0, s.PID)
	assert.NotNil(t, s.DeadSince)
	assert.Equal(t, now.UnixMilli(), *s.DeadSince)
}

func TestMarkDeadMissingSession(t *testing.T) {
	t.Parallel()
	sm := NewMachine()
	sm.MarkDead("never-seen", time.Now())
	assert.Empty(t, sm.Snapshot())
}

func TestMarkDeadTwice(t *testing.T) {
	t.Parallel()
	sm := NewMachine()
	sm.Apply(e("s1", "working"))
	t1 := time.Now()
	sm.MarkDead("s1", t1)
	t2 := t1.Add(time.Minute)
	sm.MarkDead("s1", t2)
	assert.Equal(t, t2.UnixMilli(), *sm.Snapshot()["s1"].DeadSince)
}

func TestPruneDead(t *testing.T) {
	t.Parallel()
	sm := NewMachine()
	sm.Apply(e("s1", "working"))
	sm.Apply(e("s2", "idle"))
	now := time.Date(2026, 10, 25, 12, 0, 0, 0, time.UTC)

	sm.MarkDead("s1", now.Add(-5*time.Minute))
	sm.MarkDead("s2", now.Add(-1*time.Minute))
	sm.PruneDead(2*time.Minute, now)

	snap := sm.Snapshot()
	assert.NotContains(t, snap, "s1") // older than 2 min → pruned
	assert.Contains(t, snap, "s2")    // newer → kept
}

func TestPruneDeadMissingSession(t *testing.T) {
	t.Parallel()
	sm := NewMachine()
	sm.PruneDead(5*time.Minute, time.Now()) // no dead sessions
	assert.Empty(t, sm.Snapshot())
}

func TestSnapshotIsolation(t *testing.T) {
	t.Parallel()
	sm := NewMachine()
	sm.Apply(e("s1", "working"))

	snap := sm.Snapshot()
	snap["s1"] = Session{State: Idle} // mutate copy

	original := sm.Snapshot()["s1"]
	assert.Equal(t, Working, original.State)
}

func TestApplySetsFields(t *testing.T) {
	t.Parallel()
	sm := NewMachine()
	ev := event.Event{SessionID: "s1", Event: "question", PID: 42,
		Pane: "%0", Message: "hello", TimestampMs: 100}
	sm.Apply(ev)

	s := sm.Snapshot()["s1"]
	assert.Equal(t, Question, s.State)
	assert.Equal(t, 42, s.PID)
	assert.Equal(t, "%0", s.Pane)
	assert.Equal(t, "hello", s.Message)
}

func TestApplyOrderInvariant(t *testing.T) {
	t.Parallel()
	// For any two events, final state = second event's state regardless of first
	states := []string{"working", "question", "idle", "error"}
	for _, first := range states {
		for _, second := range states {
			sm := NewMachine()
			sm.Apply(event.Event{SessionID: "s1", Event: first})
			sm.Apply(event.Event{SessionID: "s1", Event: second})
			assert.Equal(t, SessionState(second), sm.Snapshot()["s1"].State,
				"first=%s second=%s", first, second)
		}
	}
}

func TestPruneDeadOnlyDead(t *testing.T) {
	t.Parallel()
	sm := NewMachine()
	sm.Apply(e("s1", "working"))
	sm.Apply(e("s2", "idle"))
	now := time.Now()
	sm.PruneDead(1*time.Minute, now)
	assert.Len(t, sm.Snapshot(), 2) // alive sessions not affected
}
