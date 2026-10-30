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
	var newest *state.Session
	for _, s := range sf.Sessions {
		if s.Pane == pane && (newest == nil || s.UpdatedAt > newest.UpdatedAt) {
			newest = &s
		}
	}
	if newest == nil || newest.State == state.Idle {
		return ""
	}
	return fmt.Sprintf("#{E:@agent_mark_%s}", newest.State)
}
