package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jvrsantacruz/agent-status/internal/event"
	"github.com/jvrsantacruz/agent-status/internal/export"
	"github.com/jvrsantacruz/agent-status/internal/liveness"
	"github.com/jvrsantacruz/agent-status/internal/state"
	"github.com/jvrsantacruz/agent-status/internal/store"
)

var clock = time.Now

func main() {
	if len(os.Args) > 1 && os.Args[1] == "emit" {
		runEmit(os.Args[2:])
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "render" {
		runRender(os.Args[2:])
		return
	}
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})))

	stateDir := envOrDefault("AGENT_STATUS_STATE_DIR", defaultStateDir())
	runtimeDir := envOrDefault("AGENT_STATUS_RUNTIME_DIR", defaultRuntimeDir())

	fifoPath := runtimeDir + "/in.fifo"
	dbPath := stateDir + "/state.db"
	stateJSONPath := stateDir + "/state.json"

	slog.Info("starting", "fifo", fifoPath, "db", dbPath, "state", stateJSONPath)

	// Open store
	st, err := store.Open(dbPath)
	if err != nil {
		slog.Error("store open", "error", err)
		os.Exit(1)
	}
	defer st.Close()

	// Initialise state machine
	sm := state.NewMachine()

	// Ensure FIFO exists
	rd := event.NewReader(fifoPath)
	if err := rd.Ready(); err != nil {
		slog.Error("fifo ready", "error", err)
		os.Exit(1)
	}

	// Signal handling for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	// Channel for state change notifications
	changed := make(chan struct{}, 1)
	notify := func() {
		select {
		case changed <- struct{}{}:
		default:
		}
	}

	// Event reader goroutine
	eventCh := rd.Events(ctx.Done())
	go func() {
		for e := range eventCh {
			if err := st.AppendEvent(e); err != nil {
				slog.Error("append event", "session_id", e.SessionID, "error", err)
			}
			sm.Apply(e)
			if err := st.UpsertSession(sm.Snapshot()[e.SessionID], e.SessionID); err != nil {
				slog.Error("upsert session", "session_id", e.SessionID, "error", err)
			}
			notify()
			slog.Debug("event", "session_id", e.SessionID, "event", e.Event)
		}
	}()

	// Liveness loop (60s)
	go func() {
		ticker := time.NewTicker(60 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case now := <-ticker.C:
				for id, pid := range sm.PIDs() {
					if !liveness.Alive(pid) {
						sm.MarkDead(id, now)
						// Update store
						snapshot := sm.Snapshot()
						if s, ok := snapshot[id]; ok {
							if err := st.UpsertSession(s, id); err != nil {
								slog.Error("store dead session", "session_id", id, "error", err)
							}
						}
						_ = st.PruneEvents(7*24*time.Hour, now)
						slog.Info("dead", "session_id", id, "pid", pid)
						notify()
					}
				}
				sm.PruneDead(5*time.Minute, now)
			}
		}
	}()

	// Export loop — writes state.json when notified, also every 10s as fallback
	go func() {
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-changed:
			case <-ticker.C:
			}
			if err := export.Write(stateJSONPath, sm.Snapshot()); err != nil {
				slog.Error("export", "error", err)
			}
		}
	}()

	// Wait for signal
	sig := <-sigCh
	slog.Info("shutting down", "signal", sig)
	cancel()
	// Final export
	if err := export.Write(stateJSONPath, sm.Snapshot()); err != nil {
		slog.Error("final export", "error", err)
	}
}

func envOrDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func defaultStateDir() string {
	if d := os.Getenv("XDG_STATE_HOME"); d != "" {
		return d + "/agent-status"
	}
	return os.Getenv("HOME") + "/.local/state/agent-status"
}

func defaultRuntimeDir() string {
	if d := os.Getenv("XDG_RUNTIME_DIR"); d != "" {
		return d + "/agent-status"
	}
	return "/tmp/agent-status"
}
