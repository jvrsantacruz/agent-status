// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 Javier Santacruz

package export

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/jvrsantacruz/agent-status/internal/state"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWrite(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")
	sessions := map[string]state.Session{
		"s1": {Harness: "claude", State: state.Working, PID: 123, Pane: "%0", UpdatedAt: 100},
	}
	err := Write(path, StateFile{Sessions: sessions})
	require.NoError(t, err)

	data, err := os.ReadFile(path)
	require.NoError(t, err)

	var out struct {
		Sessions map[string]state.Session `json:"sessions"`
	}
	err = json.Unmarshal(data, &out)
	require.NoError(t, err)
	assert.Equal(t, "claude", out.Sessions["s1"].Harness)
	assert.Equal(t, state.Working, out.Sessions["s1"].State)
}

func TestWriteEmptySessions(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "empty.json")
	err := Write(path, StateFile{Sessions: make(map[string]state.Session)})
	require.NoError(t, err)

	data, _ := os.ReadFile(path)
	var out struct {
		Sessions map[string]state.Session `json:"sessions"`
	}
	err = json.Unmarshal(data, &out)
	require.NoError(t, err)
	assert.Empty(t, out.Sessions)
}

func TestWriteDeadSince(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "dead.json")
	ms := int64(1700000000000)
	sessions := map[string]state.Session{
		"s1": {State: state.Dead, DeadSince: &ms, UpdatedAt: ms},
	}
	err := Write(path, StateFile{Sessions: sessions})
	require.NoError(t, err)

	data, _ := os.ReadFile(path)
	var out struct {
		Sessions map[string]struct {
			DeadSince *int64 `json:"dead_since"`
		} `json:"sessions"`
	}
	json.Unmarshal(data, &out)
	require.NotNil(t, out.Sessions["s1"].DeadSince)
	assert.Equal(t, ms, *out.Sessions["s1"].DeadSince)
}

func TestWriteDeadSinceNull(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "no-dead.json")
	sessions := map[string]state.Session{
		"s1": {State: state.Working, DeadSince: nil, UpdatedAt: 1},
	}
	err := Write(path, StateFile{Sessions: sessions})
	require.NoError(t, err)

	data, _ := os.ReadFile(path)
	var out struct {
		Sessions map[string]struct {
			DeadSince *int64 `json:"dead_since"`
		} `json:"sessions"`
	}
	json.Unmarshal(data, &out)
	assert.Nil(t, out.Sessions["s1"].DeadSince)
}

func TestWritePermissionDenied(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	err := os.Chmod(dir, 0444)
	require.NoError(t, err)
	path := filepath.Join(dir, "state.json")
	err = Write(path, StateFile{Sessions: map[string]state.Session{}})
	assert.Error(t, err)
}

func TestWriteAtomic(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")

	err := Write(path, StateFile{Sessions: map[string]state.Session{
		"s1": {State: state.Working, UpdatedAt: 1},
	}})
	require.NoError(t, err)

	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		assert.NotContains(t, e.Name(), ".tmp")
	}
}

func TestWriteOverwrites(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")

	require.NoError(t, Write(path, StateFile{Sessions: map[string]state.Session{
		"s1": {State: state.Working, UpdatedAt: 1},
	}}))
	require.NoError(t, Write(path, StateFile{Sessions: map[string]state.Session{
		"s2": {State: state.Idle, UpdatedAt: 2},
	}}))

	data, _ := os.ReadFile(path)
	var out struct {
		Sessions map[string]state.Session `json:"sessions"`
	}
	json.Unmarshal(data, &out)
	_, hasOld := out.Sessions["s1"]
	_, hasNew := out.Sessions["s2"]
	assert.False(t, hasOld, "old session should be gone")
	assert.True(t, hasNew, "new session should be present")
}

func TestWriteNilSessions(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "nil.json")
	err := Write(path, StateFile{Sessions: nil})
	require.NoError(t, err)

	data, _ := os.ReadFile(path)
	var out struct {
		Sessions map[string]state.Session `json:"sessions"`
	}
	err = json.Unmarshal(data, &out)
	require.NoError(t, err)
	assert.Empty(t, out.Sessions)
}

func TestWriteReadDaemonAndPermissions(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "state.json")
	in := StateFile{
		Daemon:   Daemon{Version: "v1.2.3", PID: 9, EventsTotal: 4, EventsRejected: 1},
		Sessions: map[string]state.Session{"s1": {State: state.Error, Reason: "event:error"}},
	}
	require.NoError(t, Write(path, in))

	out, err := Read(path)
	require.NoError(t, err)
	assert.Equal(t, in, out)
	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
}
