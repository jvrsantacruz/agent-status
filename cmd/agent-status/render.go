// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 Javier Santacruz

package main

import (
	"fmt"
	"io"
	"strings"

	"github.com/jvrsantacruz/agent-status/internal/export"
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
	_, newest := newestOnPane(sf, pane)
	if newest == nil || newest.State == state.Idle {
		return ""
	}
	return fmt.Sprintf("#{E:@agent_mark_%s}", newest.State)
}

// runSession prints the harness and id of the newest live session on a pane,
// such as "claude 37f1...", so other tools can resume it. Nothing when the
// pane has none.
func runSession(args []string, stdout io.Writer) int {
	if len(args) < 1 {
		return 0
	}
	sf, err := export.Read(statePath())
	if err != nil {
		return 0
	}
	id, s := newestOnPane(sf, args[0])
	if s != nil && s.State != state.Dead {
		fmt.Fprintln(stdout, s.Harness, id)
	}
	return 0
}

func newestOnPane(sf export.StateFile, pane string) (string, *state.Session) {
	pane = pane[strings.LastIndex(pane, ":")+1:]
	var id string
	var newest *state.Session
	for sid, s := range sf.Sessions {
		if s.Pane == pane && (newest == nil || s.UpdatedAt > newest.UpdatedAt) {
			id, newest = sid, &s
		}
	}
	return id, newest
}
