// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 Javier Santacruz

package state

import (
	"encoding/json"
	"sync"
	"time"

	"github.com/jvrsantacruz/agent-status/internal/event"
)

type SessionState string

const (
	Working  SessionState = "working"
	Question SessionState = "question"
	Idle     SessionState = "idle"
	Error    SessionState = "error"
	Done     SessionState = "done"
	Dead     SessionState = "dead"
)

type Session struct {
	Harness   string       `json:"harness"`
	State     SessionState `json:"state"`
	PID       int          `json:"pid"`
	Pane      string       `json:"tmux_pane"`
	DeadSince *int64       `json:"dead_since"`
	UpdatedAt int64        `json:"updated_at"`
	Message   string       `json:"message"`
	// Reason names what set State: "event:<name>", "event:unknown(<name>)",
	// "declared:<state>" or "liveness:pid_gone".
	Reason string `json:"reason"`
	// Declared is what the agent stated about its work in this turn, shown
	// when the turn ends.
	Declared *Declaration `json:"declared,omitempty"`
	// Meta is the session's entry in meta.json, copied as the enrich run wrote it.
	Meta json.RawMessage `json:"meta,omitempty"`
}

type Declaration struct {
	State   SessionState `json:"state"`
	Message string       `json:"message"`
}

// End is the event that removes a session, sent when an agent exits.
const End = "end"

// Outcome is what one Apply did, for the daemon's log line.
type Outcome struct {
	Prev  SessionState
	Next  SessionState
	Known bool
}

type Machine struct {
	mu       sync.RWMutex
	sessions map[string]*Session
}

func NewMachine() *Machine {
	return &Machine{
		sessions: make(map[string]*Session),
	}
}

// Apply an event from the FIFO. Thread-safe.
func (m *Machine) Apply(e event.Event) Outcome {
	m.mu.Lock()
	defer m.mu.Unlock()

	s, ok := m.sessions[e.SessionID]
	if !ok {
		s = &Session{State: Idle}
		m.sessions[e.SessionID] = s
	}
	out := Outcome{Prev: s.State, Known: true}
	if e.Event == End {
		delete(m.sessions, e.SessionID)
		return out
	}
	s.Harness = e.Harness
	next := SessionState(e.Event)
	message := e.Message
	if e.Prompt {
		s.Declared = nil
	}
	switch next {
	case Working, Question, Idle, Error, Done:
		s.State = next
		s.Reason = "event:" + e.Event
		switch {
		case e.Declared && (next == Done || next == Error):
			s.Declared = &Declaration{State: next, Message: e.Message}
		case next == Idle && s.Declared != nil:
			// The turn ended: what the agent declared in it is the outcome.
			s.State, message = s.Declared.State, s.Declared.Message
			s.Reason = "declared:" + string(s.Declared.State)
			s.Declared = nil
		case next == Error:
			// A harness error ends the turn over anything declared in it.
			s.Declared = nil
		}
	default:
		out.Known = false
		s.Reason = "event:unknown(" + e.Event + ")"
	}
	s.PID = e.PID
	s.Pane = e.Pane
	s.UpdatedAt = e.TimestampMs
	s.Message = message
	// Clear dead_since on any new event — the session came back.
	s.DeadSince = nil
	out.Next = s.State
	return out
}

// MarkDead sets a session as dead. Thread-safe.
func (m *Machine) MarkDead(sessionID string, now time.Time) {
	m.mu.Lock()
	defer m.mu.Unlock()

	s, ok := m.sessions[sessionID]
	if !ok {
		return
	}
	ms := now.UnixMilli()
	s.DeadSince = &ms
	s.State = Dead
	s.Reason = "liveness:pid_gone"
	s.PID = 0
	s.UpdatedAt = ms
}

// PruneDead removes sessions that have been dead for more than ttl.
func (m *Machine) PruneDead(ttl time.Duration, now time.Time) {
	m.mu.Lock()
	defer m.mu.Unlock()

	cutoff := now.Add(-ttl).UnixMilli()
	for id, s := range m.sessions {
		if s.DeadSince != nil && *s.DeadSince < cutoff {
			delete(m.sessions, id)
		}
	}
}

// Restore loads sessions a previous daemon left, so a restart forgets no
// session that is still running; the next liveness pass clears the rest.
func (m *Machine) Restore(sessions map[string]Session) {
	m.mu.Lock()
	defer m.mu.Unlock()

	for id, s := range sessions {
		m.sessions[id] = &s
	}
}

// SetMeta sets a session's meta; it does nothing for an unknown session.
// Thread-safe.
func (m *Machine) SetMeta(sessionID string, meta json.RawMessage) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if s, ok := m.sessions[sessionID]; ok {
		s.Meta = meta
	}
}

// Snapshot returns a copy of the current state.
func (m *Machine) Snapshot() map[string]Session {
	m.mu.RLock()
	defer m.mu.RUnlock()

	out := make(map[string]Session, len(m.sessions))
	for id, s := range m.sessions {
		out[id] = *s
	}
	return out
}
