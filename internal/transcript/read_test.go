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

	"github.com/jvrsantacruz/agent-status/internal/price"
)

// testdata/s1.jsonl is a real Claude Code session cut short, text replaced:
// replies split over several lines, meta and slash-command user lines, and
// one subagent whose reply's first lines hold a partial usage.
func TestReadClaude(t *testing.T) {
	t.Parallel()
	f, err := Read("claude", "testdata/s1.jsonl")
	require.NoError(t, err)
	cost := f.CostUSD
	f.CostUSD = 0
	assert.Equal(t, Facts{
		Title:        "Neutral session title LAB-42",
		FirstPrompt:  "neutral prompt 1",
		LastPrompt:   "neutral prompt 2, see #31",
		Ticket:       "#31",
		Model:        "claude-opus-5-5",
		Turns:        2,
		StartedAt:    1791840222917,
		LastActiveAt: 1791842454749,
		Tokens:       price.Tokens{Input: 42, Output: 8935, CacheRead: 688532, CacheWrite5m: 206342},
		Priced:       true,
	}, f)
	assert.InDelta(t, 1.3482844, cost, 1e-9)
}

// testdata/pi.jsonl is a real pi session, text replaced, with a
// session_info line added: pi writes one when the session is named.
func TestReadPi(t *testing.T) {
	t.Parallel()
	f, err := Read("pi", "testdata/pi.jsonl")
	require.NoError(t, err)
	cost := f.CostUSD
	f.CostUSD = 0
	assert.Equal(t, Facts{
		Title:        "Neutral pi session #12",
		FirstPrompt:  "neutral prompt 1",
		LastPrompt:   "neutral prompt 3",
		Model:        "deepseek-ai/DeepSeek-V4-Flash",
		Turns:        3,
		StartedAt:    1792781658536,
		LastActiveAt: 1792922921971,
		Tokens:       price.Tokens{Input: 50444, Output: 4542, CacheRead: 34304},
		Priced:       true,
	}, f)
	assert.InDelta(t, 0.007919328, cost, 1e-12)
}

func TestReadPricesEachReplyAtItsModel(t *testing.T) {
	t.Parallel()
	write := func(lines ...string) string {
		path := filepath.Join(t.TempDir(), "s.jsonl")
		require.NoError(t, os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0o600))
		return path
	}
	opus := `{"type":"assistant","message":{"id":"a","model":"claude-opus-5-5","usage":{"input_tokens":1000000,"cache_creation_input_tokens":2000000,"cache_creation":{"ephemeral_1h_input_tokens":1000000}}}}`
	haiku := `{"type":"assistant","message":{"id":"b","model":"claude-haiku-5-5","usage":{"output_tokens":1000000}}}`
	other := `{"type":"assistant","message":{"id":"c","model":"gpt-9","usage":{"output_tokens":1}}}`

	f, err := Read("claude", write(opus, haiku))
	require.NoError(t, err)
	assert.True(t, f.Priced)
	assert.InDelta(t, 4+5+8+0.50, f.CostUSD, 1e-9)
	assert.Equal(t, "claude-haiku-5-5", f.Model, "the last reply's model")
	assert.Equal(t, price.Tokens{Input: 1e6, Output: 1e6, CacheWrite5m: 1e6, CacheWrite1h: 1e6}, f.Tokens)

	f, err = Read("claude", write(opus, other))
	require.NoError(t, err)
	assert.False(t, f.Priced)
}

func TestTicket(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		"feat/LAB-7-login":     "LAB-7",
		"jvrsantacruz/enrich":  "",
		"fix #12 then LAB-3":   "#12",
		"AB2-99":               "AB2-99",
		"lab-7 lowercase":      "",
		"main":                 "",
		"see issue #4 and #5":  "#4",
		"XLAB-12x not a match": "",
	}
	for in, want := range cases {
		assert.Equal(t, want, Ticket(in), in)
	}
}

func TestLastTurnPi(t *testing.T) {
	t.Parallel()
	turn, err := LastTurn("testdata/pi.jsonl")
	require.NoError(t, err)
	assert.Equal(t, "USER: neutral prompt 3\nAGENT: neutral text", turn)
}
