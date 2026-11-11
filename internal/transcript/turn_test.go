// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 Javier Santacruz

package transcript

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Lines shaped as Claude Code writes them, trimmed.
func TestLastTurnKeepsTheLastPromptOnward(t *testing.T) {
	t.Parallel()
	lines := []string{
		`{"type":"user","message":{"role":"user","content":"an earlier prompt"}}`,
		`{"type":"assistant","message":{"content":[{"type":"text","text":"earlier answer"}]}}`,
		`{"type":"mode","mode":"auto"}`,
		`{"type":"user","message":{"role":"user","content":[{"type":"text","text":"fix the build"}]}}`,
		`{"type":"assistant","message":{"content":[{"type":"thinking","thinking":"secret"},{"type":"tool_use","name":"Bash","input":{"command":"make"}}]}}`,
		`{"type":"user","message":{"content":[{"type":"tool_result","is_error":true,"content":"private output"}]}}`,
		`{"type":"assistant","message":{"content":[{"type":"text","text":"The build passes now."}]}}`,
		`not json`,
	}
	path := filepath.Join(t.TempDir(), "t.jsonl")
	require.NoError(t, os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0o600))
	turn, err := LastTurn(path)
	require.NoError(t, err)
	assert.Equal(t, "USER: fix the build\nTOOL: Bash\nTOOL FAILED\nAGENT: The build passes now.", turn)
}

func TestLastTurnStillRunningIsEmpty(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "t.jsonl")
	require.NoError(t, os.WriteFile(path, []byte(`{"type":"user","message":{"content":"fix it"}}
{"type":"assistant","message":{"content":[{"type":"tool_use","name":"Bash"}]}}
`), 0o600))
	turn, err := LastTurn(path)
	require.NoError(t, err)
	assert.Empty(t, turn)
}
