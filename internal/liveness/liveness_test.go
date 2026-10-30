// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 Javier Santacruz

package liveness

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAliveSelf(t *testing.T) {
	t.Parallel()
	assert.True(t, Alive(os.Getpid()))
}

func TestAliveDeadPID(t *testing.T) {
	t.Parallel()
	// PID 0 sends to the process group — skip that edge case
	assert.False(t, Alive(999999999))
}

func TestAliveNonExistent(t *testing.T) {
	t.Parallel()
	assert.False(t, Alive(999999999))
}
