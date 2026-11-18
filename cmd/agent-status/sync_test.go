// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 Javier Santacruz

package main

import (
	"testing"

	"github.com/jvrsantacruz/agent-status/internal/export"
	"github.com/jvrsantacruz/agent-status/internal/state"
	"github.com/stretchr/testify/assert"
)

func TestSyncArgsSetsOnlyWhatChanged(t *testing.T) {
	t.Parallel()
	sf := export.StateFile{Sessions: map[string]state.Session{
		"a": {State: state.Working, Pane: "%1"},
		"b": {State: state.Question, Pane: "%2"},
		"c": {State: state.Idle, Pane: "%3"},
	}}
	current := "@1 %1 #{E:@agent_mark_working}\n@2 %2 \n@3 %3 #{E:@agent_mark_error}\n@4 %4 \n"
	assert.Equal(t, []string{
		"set-option", "-p", "-t", "%2", "@agent_mark", "#{E:@agent_mark_question}", ";",
		"set-option", "-p", "-u", "-t", "%3", "@agent_mark",
	}, syncArgs(sf, current))
}

func TestSyncArgsNothingWhenUnchanged(t *testing.T) {
	t.Parallel()
	sf := export.StateFile{Sessions: map[string]state.Session{"a": {State: state.Working, Pane: "%1"}}}
	assert.Empty(t, syncArgs(sf, "@1 %1 #{E:@agent_mark_working}\n@2 %2 \n"))
	assert.Empty(t, syncArgs(export.StateFile{}, ""))
}

// req: window-marks
// A window shows the pane that most needs me, whichever pane is active:
// error, then question, then working, then done, then dead; idle shows none.
func TestSyncArgsWindowShowsItsMostUrgentPane(t *testing.T) {
	t.Parallel()
	cases := map[string]struct {
		states []state.SessionState
		want   string
	}{
		"error over idle":       {[]state.SessionState{state.Idle, state.Error}, "#{E:@agent_mark_error}"},
		"error over working":    {[]state.SessionState{state.Working, state.Error}, "#{E:@agent_mark_error}"},
		"question over working": {[]state.SessionState{state.Working, state.Question}, "#{E:@agent_mark_question}"},
		"working over done":     {[]state.SessionState{state.Done, state.Working}, "#{E:@agent_mark_working}"},
		"done over dead":        {[]state.SessionState{state.Dead, state.Done}, "#{E:@agent_mark_done}"},
		"dead over idle":        {[]state.SessionState{state.Idle, state.Dead}, "#{E:@agent_mark_dead}"},
		"all idle":              {[]state.SessionState{state.Idle, state.Idle}, ""},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			sf := export.StateFile{Sessions: map[string]state.Session{
				"a": {State: c.states[0], Pane: "%1"},
				"b": {State: c.states[1], Pane: "%2"},
			}}
			// Both panes start with a stale mark, so every wanted mark is set.
			args := syncArgs(sf, "@7 %1 stale\n@7 %2 stale\n")
			for _, pane := range []string{"%1", "%2"} {
				assert.Equal(t, c.want, setTo(args, pane), pane)
			}
		})
	}
}

// setTo reads what syncArgs sets a pane's mark to; "" for an unset.
func setTo(args []string, pane string) string {
	for i := 0; i+4 < len(args); i++ {
		if args[i] == "set-option" && args[i+2] == "-t" && args[i+3] == pane {
			return args[i+5]
		}
		if args[i] == "set-option" && args[i+2] == "-u" && args[i+4] == pane {
			return ""
		}
	}
	return "not set"
}

// Every state but idle shows in a window, so a new state needs a rank.
func TestEveryStateButIdleHasARank(t *testing.T) {
	t.Parallel()
	for _, s := range []state.SessionState{state.Working, state.Question, state.Idle, state.Error, state.Done, state.Dead} {
		rank := markRank("#{E:@agent_mark_" + string(s) + "}")
		if s == state.Idle {
			assert.Zero(t, rank, s)
		} else {
			assert.Positive(t, rank, s)
		}
	}
}
