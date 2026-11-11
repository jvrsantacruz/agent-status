// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 Javier Santacruz

package main

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
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
	tmuxWait      = 5 * time.Second
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
	if prev, err := export.Read(stateJSON); err == nil {
		sm.Restore(prev.Sessions)
	}
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

	tick(sm, st, clock())

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
			// A new session gets its place now, not at the next liveness tick.
			if socket, ok := needsPlace(sm, e.SessionID); ok {
				go func() {
					if c, err := setPlacesOn(sm, socket); err == nil && c {
						notify()
					}
				}()
			}
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
				if err := startEnrich(); err != nil {
					slog.Error("enrich start", "error", err)
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
			mergeMeta(sm)
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

// mergeMeta copies each meta.json entry into its session's meta. A missing
// or bad file leaves every meta as it was.
func mergeMeta(sm *state.Machine) {
	data, err := os.ReadFile(metaPath())
	var mf struct {
		Sessions map[string]json.RawMessage `json:"sessions"`
	}
	if err == nil {
		err = json.Unmarshal(data, &mf)
	}
	if err != nil {
		slog.Warn("meta", "error", err)
		return
	}
	for id, m := range mf.Sessions {
		sm.SetMeta(id, m)
	}
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
	sockets, placeErrs, placed := setPlaces(sm)
	slog.Info("liveness", "checked", checked, "no_pid", noPID, "died", died,
		"forgotten", forgotten, "pruned_events", pruned, "error", errString(err),
		"tmux_servers", sockets, "tmux_errors", placeErrs, "places_changed", placed)
	return died > 0 || forgotten > 0 || placed
}

// placeFormat is what list-panes prints per pane, parsed by parsePlaces.
const placeFormat = "#{pane_id}\t#{session_name}\t#{window_index}\t#{window_name}"

// setPlaces asks each tmux server a session runs on where its panes sit, one
// call per server. A server that does not answer leaves its sessions as they
// were. It returns the servers asked, the ones that failed, and whether any
// session changed.
func setPlaces(sm *state.Machine) (sockets, failed int, changed bool) {
	seen := map[string]bool{}
	for _, s := range sm.Snapshot() {
		if s.TmuxSocket == "" || s.Pane == "" || seen[s.TmuxSocket] {
			continue
		}
		seen[s.TmuxSocket] = true
		c, err := setPlacesOn(sm, s.TmuxSocket)
		if err != nil {
			failed++
			continue
		}
		changed = c || changed
	}
	return len(seen), failed, changed
}

// setPlacesOn asks one tmux server where its panes sit.
func setPlacesOn(sm *state.Machine, socket string) (bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), tmuxWait)
	defer cancel()
	// -u: without a UTF-8 locale, as under systemd, tmux prints a tab as
	// "_" and escapes a name that is not ASCII.
	out, err := exec.CommandContext(ctx, "tmux", "-u", "-S", socket, "list-panes", "-a", "-F", placeFormat).Output()
	if err != nil {
		return false, err
	}
	return sm.SetPlaces(socket, parsePlaces(string(out))), nil
}

// needsPlace reports the socket of a session in tmux that has no place yet:
// a new session, or one that moved pane.
func needsPlace(sm *state.Machine, id string) (string, bool) {
	s, ok := sm.Snapshot()[id]
	if !ok || s.TmuxSocket == "" || s.Pane == "" || s.TmuxSession != "" {
		return "", false
	}
	return s.TmuxSocket, true
}

// parsePlaces reads list-panes output in placeFormat into places by pane id.
// A line it cannot read is skipped.
func parsePlaces(out string) map[string]state.Place {
	places := map[string]state.Place{}
	for _, line := range strings.Split(out, "\n") {
		f := strings.SplitN(line, "\t", 4)
		if len(f) != 4 || f[0] == "" {
			continue
		}
		index, err := strconv.Atoi(f[2])
		if err != nil || index < 0 {
			continue
		}
		places[f[0]] = state.Place{Session: f[1], WindowIndex: index, WindowName: f[3]}
	}
	return places
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
