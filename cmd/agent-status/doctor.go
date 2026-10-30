// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 Javier Santacruz

package main

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/jvrsantacruz/agent-status/internal/event"
	"github.com/jvrsantacruz/agent-status/internal/export"
	"github.com/jvrsantacruz/agent-status/internal/state"
	"github.com/jvrsantacruz/agent-status/internal/store"
)

const roundTripWait = 2 * time.Second

// runDoctor checks the real installation, from emit to render, and prints a
// report to paste in a bug report. Exit 1 when a check fails.
func runDoctor(w io.Writer) int {
	failed := false
	report := func(status, name, detail string) {
		if status == "FAIL" {
			failed = true
		}
		fmt.Fprintf(w, "%-4s  %-12s  %s\n", status, name, detail)
	}
	check := func(ok bool, name, detail string) {
		report(map[bool]string{true: "ok", false: "FAIL"}[ok], name, detail)
	}

	fmt.Fprintln(w, versionLine())

	f, err := os.OpenFile(fifoPath(), os.O_WRONLY|syscall.O_NONBLOCK, 0)
	if err != nil {
		check(false, "daemon", fmt.Sprintf("no reader on %s (%v). Start it: systemctl --user start agent-status, or brew services start agent-status", fifoPath(), err))
		return 1
	}
	f.Close()
	check(true, "daemon", "listening on "+fifoPath())

	sf, err := export.Read(statePath())
	if err != nil {
		check(false, "state", err.Error())
		return 1
	}
	check(sf.Daemon.Version == buildVersion(), "version",
		fmt.Sprintf("daemon %s pid %d, this binary %s", sf.Daemon.Version, sf.Daemon.PID, buildVersion()))

	roundTrip(check)
	wiring(report, check)
	sessions(w, report, sf)

	if failed {
		return 1
	}
	return 0
}

// roundTrip sends a test session for this process, waits for it in
// state.json, renders its marker, then removes it.
func roundTrip(check func(bool, string, string)) {
	id := "doctor-" + strconv.Itoa(os.Getpid())
	pane := "doctor-" + strconv.Itoa(os.Getpid())
	e := event.Event{SessionID: id, Harness: "doctor", Event: "question",
		PID: os.Getpid(), Pane: pane, TimestampMs: clock().UnixMilli()}
	if err := send(e); err != nil {
		check(false, "round trip", err.Error())
		return
	}
	defer func() {
		e.Event = state.End
		_ = send(e)
	}()

	start := clock()
	for clock().Sub(start) < roundTripWait {
		sf, err := export.Read(statePath())
		if err == nil {
			if s, ok := sf.Sessions[id]; ok && s.State == state.Question {
				check(true, "round trip", fmt.Sprintf("event reached state.json in %d ms", clock().Sub(start).Milliseconds()))
				got := marker(sf, pane)
				check(got == "#{E:@agent_mark_question}", "render", fmt.Sprintf("marker %q", got))
				return
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	check(false, "round trip", fmt.Sprintf("event not in %s after %s; see the daemon log", statePath(), roundTripWait))
}

func wiring(report func(string, string, string), check func(bool, string, string)) {
	if os.Getenv("TMUX") == "" {
		report("skip", "tmux", "not inside tmux")
	} else {
		mark, _ := exec.Command("tmux", "show-options", "-gqv", "@agent_mark_working").Output()
		check(len(strings.TrimSpace(string(mark))) > 0, "tmux plugin", "@agent_mark_working is "+strconv.Quote(strings.TrimSpace(string(mark))))
		format, _ := exec.Command("tmux", "show-options", "-gqv", "window-status-format").Output()
		check(strings.Contains(string(format), "agent-status render"), "tmux format", "window-status-format calls agent-status render")
	}
	settings := filepath.Join(os.Getenv("HOME"), ".claude", "settings.json")
	if data, err := os.ReadFile(settings); err == nil {
		if strings.Contains(string(data), "agent-status emit") {
			report("ok", "claude hooks", settings+" calls agent-status emit")
		} else {
			report("warn", "claude hooks", "no agent-status emit in "+settings+"; see examples/claude-hooks.json")
		}
	}
}

// sessions lists every session with what doctor finds wrong in it, and its
// last events from the store.
func sessions(w io.Writer, report func(string, string, string), sf export.StateFile) {
	panes := tmuxPanes()
	st, err := store.Open(dbPath())
	if err != nil {
		report("warn", "events", err.Error())
	} else {
		defer st.Close()
	}
	ids := make([]string, 0, len(sf.Sessions))
	for id := range sf.Sessions {
		if !strings.HasPrefix(id, "doctor-") {
			ids = append(ids, id)
		}
	}
	slices.Sort(ids)
	fmt.Fprintf(w, "\nsessions: %d, events received %d, rejected %d\n", len(ids), sf.Daemon.EventsTotal, sf.Daemon.EventsRejected)
	for _, id := range ids {
		s := sf.Sessions[id]
		fmt.Fprintf(w, "\n%s  harness=%s state=%s pid=%d pane=%s reason=%s updated=%s\n", id, s.Harness, s.State,
			s.PID, s.Pane, s.Reason, time.UnixMilli(s.UpdatedAt).Format(time.RFC3339))
		if s.PID <= 0 && s.State != state.Dead {
			report("warn", "pid", "no pid: this session is never cleared when the agent exits; pass emit --pid")
		}
		if panes != nil && !slices.Contains(panes, s.Pane) {
			report("warn", "pane", fmt.Sprintf("pane %q is not a tmux pane: no window shows this session; check $TMUX_PANE in the agent", s.Pane))
		}
		if st == nil {
			continue
		}
		rows, err := st.Recent(id, 5)
		if err != nil {
			report("warn", "events", err.Error())
			continue
		}
		for _, r := range rows {
			fmt.Fprintf(w, "  %s  %-8s known=%t pid=%d pane=%s lag=%dms\n", time.UnixMilli(r.ReceivedMs).Format(time.TimeOnly),
				r.Event, r.Known, r.PID, r.Pane, r.ReceivedMs-r.TimestampMs)
		}
	}
}

// tmuxPanes lists every pane id of the tmux server, or nil without tmux.
func tmuxPanes() []string {
	if os.Getenv("TMUX") == "" {
		return nil
	}
	out, err := exec.Command("tmux", "list-panes", "-a", "-F", "#{pane_id}").Output()
	if err != nil {
		return nil
	}
	return strings.Fields(string(out))
}
