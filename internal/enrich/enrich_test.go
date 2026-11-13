// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 Javier Santacruz

package enrich

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const fixture = "../transcript/testdata/s1.jsonl"

// fakeTools puts git and gh scripts first on PATH. Tests using it cannot
// run in parallel.
func fakeTools(t *testing.T, git, gh string) {
	dir := t.TempDir()
	for name, body := range map[string]string{"git": git, "gh": gh} {
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte("#!/bin/sh\n"+body+"\n"), 0o700))
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func noName(context.Context, string) (string, error) { return "", errors.New("not called") }

func TestEnrichFromTranscriptGitAndGh(t *testing.T) {
	repo := t.TempDir() // gh runs in it
	fakeTools(t, `printf '`+repo+`\nfeat/LAB-7-login\n'`, `echo '{"number":12,"state":"MERGED"}'`)
	m := Enrich(context.Background(), Session{Harness: "claude", State: "idle", Cwd: repo + "/cmd",
		TranscriptPath: fixture}, Meta{}, func(context.Context, string) (string, error) {
		return "  \"Fix login retry.\"\nmore", nil
	})
	cost := 1.3483
	assert.Equal(t, Meta{
		Title: "Neutral session title LAB-42", Name: "Fix login retry", NameTurns: 2,
		Repo: repo, Branch: "feat/LAB-7-login", Ticket: "LAB-7",
		PR:    &PR{Number: 12, State: "merged"},
		Model: "claude-opus-5-5", Turns: 2, LastPrompt: "neutral prompt 2, see #31",
		Tokens:  &Tokens{Input: 42, Output: 8935, CacheRead: 688532, CacheWrite: 206342},
		CostUSD: &cost, StartedAt: 1791840222917, LastActiveAt: 1791842454749,
	}, m)
}

func TestTicketOrder(t *testing.T) {
	cases := []struct{ branch, want string }{
		{"feat/LAB-7-login", "LAB-7"},     // the branch first
		{"jvrsantacruz/enrich", "LAB-42"}, // then the title
	}
	for _, c := range cases {
		fakeTools(t, `printf '/r\n`+c.branch+`\n'`, `exit 1`)
		m := Enrich(context.Background(), Session{State: "working", Cwd: "/r", TranscriptPath: fixture}, Meta{}, noName)
		assert.Equal(t, c.want, m.Ticket, c.branch)
		assert.Nil(t, m.PR)
	}
	// Then the first prompt that names one; no git repository here.
	path := filepath.Join(t.TempDir(), "s.jsonl")
	require.NoError(t, os.WriteFile(path, []byte(`{"type":"user","message":{"content":"fix it"}}
{"type":"user","message":{"content":"see #8 and #9"}}
`), 0o600))
	m := Enrich(context.Background(), Session{State: "working", TranscriptPath: path}, Meta{}, noName)
	assert.Equal(t, "#8", m.Ticket)
	assert.Empty(t, m.Repo)
}

func TestNamer(t *testing.T) {
	t.Parallel()
	cases := []struct {
		desc  string
		state string
		old   Meta
		out   string
		err   error
		want  Meta
		asked bool
	}{
		{"a turn ended since the last name", "idle", Meta{Name: "Old", NameTurns: 1}, "New name", nil, Meta{Name: "New name", NameTurns: 2}, true},
		{"no turn since", "question", Meta{Name: "Old", NameTurns: 2}, "", nil, Meta{Name: "Old", NameTurns: 2}, false},
		{"the turn still runs", "working", Meta{Name: "Old", NameTurns: 1}, "", nil, Meta{Name: "Old", NameTurns: 1}, false},
		{"busy since it started: the title's words until the turn ends", "working", Meta{}, "", nil, Meta{Name: "Neutral session title LAB-42"}, false},
		{"namer fails: the title's first words", "idle", Meta{}, "", errors.New("no claude"), Meta{Name: "Neutral session title LAB-42", NameTurns: 2}, true},
		{"too long: four words", "idle", Meta{}, "one two three four five", nil, Meta{Name: "one two three four", NameTurns: 2}, true},
		{"cut off by the run's budget: the old name, asked again later", "idle", Meta{Name: "Old", NameTurns: 1}, "", ErrNotNamed, Meta{Name: "Old", NameTurns: 1}, true},
		{"cut off with no name yet: the title's words, asked again later", "idle", Meta{}, "", ErrNotNamed, Meta{Name: "Neutral session title LAB-42"}, true},
	}
	for _, c := range cases {
		t.Run(c.desc, func(t *testing.T) {
			t.Parallel()
			asked := false
			m := Enrich(context.Background(), Session{State: c.state, TranscriptPath: fixture}, c.old,
				func(_ context.Context, prompt string) (string, error) {
					asked = true
					assert.Contains(t, prompt, "First prompt: neutral prompt 1\nTitle: Neutral session title LAB-42\nLast turn:\nUSER: neutral prompt 2")
					return c.out, c.err
				})
			assert.Equal(t, c.asked, asked)
			assert.Equal(t, c.want, Meta{Name: m.Name, NameTurns: m.NameTurns})
		})
	}
}

func TestLastPromptCutAt200Characters(t *testing.T) {
	t.Parallel()
	long := strings.Repeat("é", 250)
	path := filepath.Join(t.TempDir(), "s.jsonl")
	require.NoError(t, os.WriteFile(path, []byte(`{"type":"user","message":{"content":"`+long+`"}}`), 0o600))
	m := Enrich(context.Background(), Session{State: "working", TranscriptPath: path}, Meta{}, noName)
	assert.Equal(t, strings.Repeat("é", 200), m.LastPrompt)
}

func TestUnpricedModelLeavesCostOut(t *testing.T) {
	t.Parallel()
	m := Enrich(context.Background(), Session{Harness: "claude", State: "working",
		TranscriptPath: writeLines(t, `{"type":"assistant","message":{"id":"a","model":"gpt-9","usage":{"output_tokens":5}}}`)}, Meta{}, noName)
	assert.Equal(t, &Tokens{Output: 5}, m.Tokens)
	assert.Nil(t, m.CostUSD)
}

func writeLines(t *testing.T, lines ...string) string {
	path := filepath.Join(t.TempDir(), "s.jsonl")
	require.NoError(t, os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0o600))
	return path
}

func TestWriteFileIsPrivateAndMatchesSchema(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "meta.json")
	m := Enrich(context.Background(), Session{Harness: "claude", State: "idle", TranscriptPath: fixture}, Meta{},
		func(context.Context, string) (string, error) { return "Neutral name", nil })
	pi := Enrich(context.Background(), Session{Harness: "pi", State: "idle", TranscriptPath: "../transcript/testdata/pi.jsonl"}, Meta{}, noName)
	require.NoError(t, WriteFile(path, File{Sessions: map[string]Meta{"a": m, "b": pi, "c": {}}}))
	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())

	s, err := jsonschema.NewCompiler().Compile("../../schema/meta.schema.json")
	require.NoError(t, err)
	f, err := os.Open(path)
	require.NoError(t, err)
	defer f.Close()
	doc, err := jsonschema.UnmarshalJSON(f)
	require.NoError(t, err)
	assert.NoError(t, s.Validate(doc))

	back, err := ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, m, back.Sessions["a"])
}
