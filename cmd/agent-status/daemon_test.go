// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 Javier Santacruz

package main

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/jvrsantacruz/agent-status/internal/state"
)

// Output of list-panes -a -F placeFormat on a real server, with one line
// for each way a line can go wrong.
func TestParsePlaces(t *testing.T) {
	t.Parallel()
	out := "%0\twork\t0\tapi\n" +
		"%3\twork\t2\tmy docs\n" +
		"%7\tother\t11\tname\twith tab\n" +
		"%8\tnoindex\tx\tbad\n" +
		"short line\n" +
		"\n"
	assert.Equal(t, map[string]state.Place{
		"%0": {Session: "work", WindowIndex: 0, WindowName: "api"},
		"%3": {Session: "work", WindowIndex: 2, WindowName: "my docs"},
		"%7": {Session: "other", WindowIndex: 11, WindowName: "name\twith tab"},
	}, parsePlaces(out))
	assert.Empty(t, parsePlaces(""))
}
