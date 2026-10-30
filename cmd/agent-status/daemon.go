// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 Javier Santacruz

package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/jvrsantacruz/agent-status/internal/event"
	"github.com/jvrsantacruz/agent-status/internal/export"
	"github.com/jvrsantacruz/agent-status/internal/liveness"
	"github.com/jvrsantacruz/agent-status/internal/state"
	"github.com/jvrsantacruz/agent-status/internal/store"
)

const (
	livenessEvery = 60 * time.Second
	exportEvery   = 10 * time.Second
	eventsKeep    = 7 * 24 * time.Hour
	deadKeep      = 5 * time.Minute
)

// counters feed the daemon block of state.json.
type counters struct {
	total, rejected, lastEventAt atomic.Int64
}

// runDaemon logs one wide JSON event per unit of work to stderr: one per
// received event, one per liveness tick. AGENT_STATUS_LOG sets the level.
func runDaemon() int {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{
		Level: logLevel(os.Getenv("AGENT_STATUS_LOG")),
	})))

	fifo, db, stateJSON := fifoPath(), dbPath(), statePath()
	started := clock()
	slog.Info("starting", "version", buildVersion(), "fifo", fifo, "db", db, "state", stateJSON)

	st, err := store.Open(db)
	if err != nil {
		slog.Error("store open", "error", err)
		return 1
	}
	defer st.Close()

	sm := state.NewMachine()
	var c counters

	rd := event.NewReader(fifo)
	rd.Rejected = func(line []byte, err error) {
		c.rejected.Add(1)
		slog.Warn("event_rejected", "reason", err.Error(), "bytes", len(line))
	}
	if err := rd.Ready(); err != nil {
		slog.Error("fifo ready", "error", err)
		return 1
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	changed := make(chan struct{}, 1)
	notify := func() {
		select {
		case changed <- struct{}{}:
		default:
		}
	}

	snapshot := func() export.StateFile {
		return export.StateFile{
			Daemon: export.Daemon{
				Version:        buildVersion(),
				PID:            os.Getpid(),
				StartedAt:      started.UnixMilli(),
				LastEventAt:    c.lastEventAt.Load(),
				EventsTotal:    c.total.Load(),
				EventsRejected: c.rejected.Load(),
			},
			Sessions: sm.Snapshot(),
		}
	}

	go func() {
		for e := range rd.Events(ctx.Done()) {
			now := clock()
			out := sm.Apply(e)
			storeErr := st.AppendEvent(e, now, out.Known)
			c.total.Add(1)
			c.lastEventAt.Store(now.UnixMilli())
			notify()
			slog.Info("event",
				"session_id", e.SessionID, "harness", e.Harness, "event", e.Event,
				"known_event", out.Known, "prev_state", out.Prev, "new_state", out.Next,
				"pid", e.PID, "pid_alive", e.PID > 0 && liveness.Alive(e.PID),
				"tmux_pane", e.Pane, "lag_ms", now.UnixMilli()-e.TimestampMs,
				"store_ok", storeErr == nil, "error", errString(storeErr),
				"dur_ms", clock().Sub(now).Milliseconds())
		}
	}()

	go func() {
		ticker := time.NewTicker(livenessEvery)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case now := <-ticker.C:
				if tick(sm, st, now) {
					notify()
				}
			}
		}
	}()

	go func() {
		ticker := time.NewTicker(exportEvery)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-changed:
			case <-ticker.C:
			}
			if err := export.Write(stateJSON, snapshot()); err != nil {
				slog.Error("export", "error", err)
			}
		}
	}()

	// Write state.json at once, so doctor sees a live daemon before any event.
	if err := export.Write(stateJSON, snapshot()); err != nil {
		slog.Error("export", "error", err)
	}

	<-ctx.Done()
	slog.Info("shutting down")
	if err := export.Write(stateJSON, snapshot()); err != nil {
		slog.Error("final export", "error", err)
		return 1
	}
	return 0
}

// tick marks sessions whose agent has exited as dead, forgets old ones and
// prunes old events. It reports whether any session changed.
func tick(sm *state.Machine, st *store.Store, now time.Time) bool {
	var checked, noPID, died int
	for id, s := range sm.Snapshot() {
		if s.State == state.Dead {
			continue
		}
		if s.PID <= 0 {
			// A session without a pid is never cleared; doctor reports it.
			noPID++
			continue
		}
		checked++
		if !liveness.Alive(s.PID) {
			sm.MarkDead(id, now)
			died++
		}
	}
	before := len(sm.Snapshot())
	sm.PruneDead(deadKeep, now)
	forgotten := before - len(sm.Snapshot())
	pruned, err := st.PruneEvents(eventsKeep, now)
	slog.Info("liveness", "checked", checked, "no_pid", noPID, "died", died,
		"forgotten", forgotten, "pruned_events", pruned, "error", errString(err))
	return died > 0 || forgotten > 0
}

func logLevel(s string) slog.Level {
	var l slog.Level
	if err := l.UnmarshalText([]byte(strings.ToUpper(s))); err != nil {
		return slog.LevelInfo
	}
	return l
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
