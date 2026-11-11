// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 Javier Santacruz

package main

import (
	"testing"

	"github.com/jvrsantacruz/agent-status/internal/state"
	"github.com/stretchr/testify/assert"
)

func TestVerdict(t *testing.T) {
	t.Parallel()
	cases := map[string]state.SessionState{
		"done":                   state.Done,
		"Question.\n":            state.Question,
		"**error**":              state.Error,
		"idle":                   state.Idle,
		"done - the tests pass":  state.Done,
		"working":                "",
		"I think it is finished": "",
		"":                       "",
	}
	for out, want := range cases {
		assert.Equal(t, want, verdict(out), out)
	}
}
