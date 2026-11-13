// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 Javier Santacruz

package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// launchd gives a service this PATH: tmux, gh and claude are on none of it.
func TestServicePath(t *testing.T) {
	t.Parallel()
	got := servicePath("/usr/bin:/bin:/opt/homebrew/bin", "/home/u")
	assert.Equal(t, "/usr/bin:/bin:/opt/homebrew/bin:/home/u/.local/bin:/home/linuxbrew/.linuxbrew/bin:/usr/local/bin", got)
}

// No empty or relative entry: sh would read it as the current directory.
func TestServicePathWithoutPathOrHome(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "/opt/homebrew/bin:/home/linuxbrew/.linuxbrew/bin:/usr/local/bin", servicePath("", ""))
}

func TestInFlightClaimsOnce(t *testing.T) {
	t.Parallel()
	f := inFlight{m: map[string]bool{}}
	assert.True(t, f.claim("/tmp/s"))
	assert.False(t, f.claim("/tmp/s"), "a server being asked is not asked again")
	f.release("/tmp/s")
	assert.True(t, f.claim("/tmp/s"))
}
