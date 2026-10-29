package state

import (
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
	Dead     SessionState = "dead"
)

type Session struct {
	Harness    string       `json:"harness"`
	State      SessionState `json:"state"`
	PID        int          `json:"pid"`
	TmuxTarget string       `json:"tmux_target"`
	DeadSince  *int64       `json:"dead_since"`
	UpdatedAt  int64        `json:"updated_at"`
	Message    string       `json:"message"`
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
func (m *Machine) Apply(e event.Event) {
	m.mu.Lock()
	defer m.mu.Unlock()

	s, ok := m.sessions[e.SessionID]
	if !ok {
		s = &Session{State: Idle}
		m.sessions[e.SessionID] = s
	}
	s.Harness = e.Harness
	switch SessionState(e.Event) {
	case Working, Question, Idle, Error:
		s.State = SessionState(e.Event)
	}
	s.PID = e.PID
	s.TmuxTarget = e.TmuxTarget
	s.UpdatedAt = e.TimestampMs
	s.Message = e.Message
	// Clear dead_since on any new event — the session came back.
	s.DeadSince = nil
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

// PIDs returns all non-zero PIDs indexed by session id.
func (m *Machine) PIDs() map[string]int {
	m.mu.RLock()
	defer m.mu.RUnlock()

	out := make(map[string]int, len(m.sessions))
	for id, s := range m.sessions {
		if s.PID > 0 {
			out[id] = s.PID
		}
	}
	return out
}
