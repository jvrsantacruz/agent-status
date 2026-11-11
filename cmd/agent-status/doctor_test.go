// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 Javier Santacruz

package main

import (
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMetaAge(t *testing.T) {
	t.Setenv("AGENT_STATUS_STATE_DIR", t.TempDir())
	status := func() string {
		var got string
		metaAge(func(s, _, _ string) { got = s })
		return got
	}
	assert.Equal(t, "warn", status(), "no meta.json")
	require.NoError(t, os.WriteFile(metaPath(), []byte(`{}`), 0o600))
	assert.Equal(t, "ok", status())
	old := time.Now().Add(-metaStale - time.Second)
	require.NoError(t, os.Chtimes(metaPath(), old, old))
	assert.Equal(t, "warn", status(), "older than 3 minutes")
}
