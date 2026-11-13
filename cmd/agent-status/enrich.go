// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 Javier Santacruz

package main

import (
	"context"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"

	"github.com/jvrsantacruz/agent-status/internal/enrich"
	"github.com/jvrsantacruz/agent-status/internal/state"
)

// enrichBudget is how long a run may spend naming sessions.
const enrichBudget = 45 * time.Second

// startEnrich runs enrich detached; the daemon reaps it and never waits on it.
func startEnrich() error {
	self, err := os.Executable()
	if err != nil {
		return err
	}
	cmd := exec.Command(self, "enrich")
	cmd.Stderr = os.Stderr // its log line joins the daemon's
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return err
	}
	go func() { _ = cmd.Wait() }()
	return nil
}

// runEnrich writes meta.json for every live session in state.json and logs
// one wide event. A run already holding the lock makes it exit at once.
func runEnrich(stderr io.Writer) int {
	log := slog.New(slog.NewJSONHandler(stderr, nil))
	if err := os.MkdirAll(stateDir(), 0o700); err != nil {
		log.Error("enrich", "error", err)
		return 1
	}
	lock, err := os.OpenFile(filepath.Join(stateDir(), "enrich.lock"), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		log.Error("enrich", "error", err)
		return 1
	}
	defer lock.Close()
	if syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB) != nil {
		log.Info("enrich", "skipped", "another run holds the lock")
		return 0
	}

	start := clock()
	st, err := enrich.ReadState(statePath())
	if err != nil {
		log.Error("enrich", "error", err)
		return 1
	}
	old, _ := enrich.ReadFile(metaPath())
	namer := func(ctx context.Context, prompt string) (string, error) {
		return askModel(ctx, "AGENT_STATUS_NAMER_COMMAND", prompt)
	}
	out := enrich.File{Sessions: map[string]enrich.Meta{}}
	named := 0
	// A run ends within a minute, the next one's start: sessions past the
	// budget keep their name and are named by a later run.
	ctx, cancel := context.WithDeadline(context.Background(), start.Add(enrichBudget))
	defer cancel()
	for id, s := range st.Sessions {
		if s.State == string(state.Dead) {
			continue
		}
		n := enrich.Namer(func(_ context.Context, prompt string) (string, error) {
			out, err := namer(ctx, prompt)
			if ctx.Err() != nil {
				return "", enrich.ErrNotNamed
			}
			return out, err
		})
		if ctx.Err() != nil {
			n = nil
		}
		m := enrich.Enrich(context.Background(), s, old.Sessions[id], n)
		if m.NameTurns != old.Sessions[id].NameTurns {
			named++
		}
		out.Sessions[id] = m
	}
	err = enrich.WriteFile(metaPath(), out)
	log.Info("enrich", "sessions", len(out.Sessions), "named", named,
		"dur_ms", clock().Sub(start).Milliseconds(), "error", errString(err))
	if err != nil {
		return 1
	}
	return 0
}
