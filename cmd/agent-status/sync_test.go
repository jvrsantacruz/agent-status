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
	current := "%1 #{E:@agent_mark_working}\n%2 \n%3 #{E:@agent_mark_error}\n%4 \n"
	assert.Equal(t, []string{
		"set-option", "-p", "-t", "%2", "@agent_mark", "#{E:@agent_mark_question}", ";",
		"set-option", "-p", "-u", "-t", "%3", "@agent_mark",
	}, syncArgs(sf, current))
}

func TestSyncArgsNothingWhenUnchanged(t *testing.T) {
	t.Parallel()
	sf := export.StateFile{Sessions: map[string]state.Session{"a": {State: state.Working, Pane: "%1"}}}
	assert.Empty(t, syncArgs(sf, "%1 #{E:@agent_mark_working}\n%2 \n"))
	assert.Empty(t, syncArgs(export.StateFile{}, ""))
}
