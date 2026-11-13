// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 Javier Santacruz

package main

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/jvrsantacruz/agent-status/internal/export"
	"github.com/jvrsantacruz/agent-status/internal/liveness"
	"github.com/jvrsantacruz/agent-status/internal/state"
)

// runRender prints nothing on any error: its output lands in the tmux status
// line on every refresh. doctor is where errors are reported.
func runRender(args []string, stdout io.Writer) int {
	if len(args) < 1 {
		return 0
	}
	sf, err := export.Read(statePath())
	if err != nil {
		return 0
	}
	fmt.Fprint(stdout, marker(sf, args[0]))
	return 0
}

// marker returns the tmux format for the newest session on a pane. It prints
// a reference to @agent_mark_<state>; the plugin or the theme defines the
// glyph and colours. Idle shows nothing.
func marker(sf export.StateFile, pane string) string {
	// Older themes pass "session:%3"; the pane id alone identifies the pane.
	pane = pane[strings.LastIndex(pane, ":")+1:]
	// A live session wins over one that died later in the same pane, such
	// as the agent quit before this one started.
	_, newest := newestOnPane(sf, pane, func(s state.Session) bool { return s.State != state.Dead })
	if newest == nil {
		_, newest = newestOnPane(sf, pane, func(state.Session) bool { return true })
	}
	if newest == nil || newest.State == state.Idle {
		return ""
	}
	return fmt.Sprintf("#{E:@agent_mark_%s}", newest.State)
}

// runSession prints the harness and id of the newest live session on a pane,
// such as "claude 37f1...", so other tools can resume it. Nothing when the
// pane has none.
func runSession(args []string, stdout io.Writer) int {
	if len(args) < 1 || args[0] == "" {
		return 0
	}
	sf, err := export.Read(statePath())
	if err != nil {
		return 0
	}
	// A process that exited is skipped before the daemon marks it dead, so a
	// new agent in the same pane is never saved with the old session's id.
	id, s := newestOnPane(sf, args[0], func(s state.Session) bool {
		return s.State != state.Dead && (s.PID <= 0 || liveness.Alive(s.PID))
	})
	if s != nil {
		fmt.Fprintln(stdout, s.Harness, id)
	}
	return 0
}

func newestOnPane(sf export.StateFile, pane string, keep func(state.Session) bool) (string, *state.Session) {
	pane = pane[strings.LastIndex(pane, ":")+1:]
	// Pane ids repeat across tmux servers: a session from another server is
	// not this pane's.
	server := tmuxSocket(os.Getenv("TMUX"))
	var id string
	var newest *state.Session
	for sid, s := range sf.Sessions {
		other := server != "" && s.TmuxSocket != "" && s.TmuxSocket != server
		if s.Pane == pane && !other && keep(s) && (newest == nil || s.UpdatedAt > newest.UpdatedAt) {
			id, newest = sid, &s
		}
	}
	return id, newest
}
