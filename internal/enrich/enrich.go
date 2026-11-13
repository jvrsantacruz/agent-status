// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 Javier Santacruz

// Package enrich makes a session's metadata from its transcript, its
// directory's git branch and the branch's pull request. It only reads.
package enrich

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/jvrsantacruz/agent-status/internal/transcript"
)

// SchemaVersion is the version of schema/meta.schema.json that WriteFile produces.
const SchemaVersion = 1

const (
	commandWait   = 5 * time.Second
	lastPromptMax = 200
	nameWords     = 4
	// firstPromptMax bounds the first prompt the namer reads, in characters.
	firstPromptMax = 2000
)

// Session is the part of a state.json session enrich reads.
type Session struct {
	Harness        string `json:"harness"`
	State          string `json:"state"`
	Cwd            string `json:"cwd"`
	TranscriptPath string `json:"transcript_path"`
}

// State is the part of state.json enrich reads.
type State struct {
	Sessions map[string]Session `json:"sessions"`
}

// File is meta.json.
type File struct {
	SchemaVersion int             `json:"schema_version"`
	Sessions      map[string]Meta `json:"sessions"`
}

// Meta is one session's metadata, as schema/meta.schema.json defines it.
type Meta struct {
	Title        string   `json:"title,omitempty"`
	Name         string   `json:"name,omitempty"`
	NameTurns    int      `json:"name_turns,omitempty"`
	Repo         string   `json:"repo,omitempty"`
	Branch       string   `json:"branch,omitempty"`
	Ticket       string   `json:"ticket,omitempty"`
	PR           *PR      `json:"pr,omitempty"`
	Model        string   `json:"model,omitempty"`
	Turns        int      `json:"turns,omitempty"`
	LastPrompt   string   `json:"last_prompt,omitempty"`
	Tokens       *Tokens  `json:"tokens,omitempty"`
	CostUSD      *float64 `json:"cost_usd,omitempty"`
	StartedAt    int64    `json:"started_at,omitempty"`
	LastActiveAt int64    `json:"last_active_at,omitempty"`
}

type PR struct {
	Number int    `json:"number"`
	State  string `json:"state"`
}

type Tokens struct {
	Input      int64 `json:"input"`
	Output     int64 `json:"output"`
	CacheRead  int64 `json:"cache_read"`
	CacheWrite int64 `json:"cache_write"`
}

// Namer asks a model for a short name; it gets the prompt and returns the answer.
// A nil Namer, or one that returns ErrNotNamed, leaves naming to a later run.
type Namer func(ctx context.Context, prompt string) (string, error)

// ErrNotNamed is a namer's answer when it had no time to ask.
var ErrNotNamed = errors.New("not named in this run")

const namePrompt = `Name this coding agent session in at most four words, from its first
prompt, its title and its last turn. Answer with the name only.

`

// Enrich makes the metadata of one session. old is what the last run made:
// its name is kept until a turn ends after it was made.
func Enrich(ctx context.Context, s Session, old Meta, name Namer) Meta {
	var m Meta
	var f transcript.Facts
	if s.TranscriptPath != "" {
		f, _ = transcript.Read(s.Harness, s.TranscriptPath)
	}
	m.Title, m.Model, m.Turns = f.Title, f.Model, f.Turns
	m.LastPrompt = cut(f.LastPrompt, lastPromptMax)
	m.StartedAt, m.LastActiveAt = f.StartedAt, f.LastActiveAt
	if t := f.Tokens; t.Input+t.Output+t.CacheRead+t.CacheWrite5m+t.CacheWrite1h > 0 {
		m.Tokens = &Tokens{t.Input, t.Output, t.CacheRead, t.CacheWrite5m + t.CacheWrite1h}
		if f.Priced {
			usd := math.Round(f.CostUSD*1e4) / 1e4
			m.CostUSD = &usd
		}
	}

	m.Repo, m.Branch = gitPlace(ctx, s.Cwd)
	m.PR = pullRequest(ctx, m.Repo, m.Branch)
	for _, from := range []string{m.Branch, m.Title} {
		if m.Ticket = transcript.Ticket(from); m.Ticket != "" {
			break
		}
	}
	if m.Ticket == "" {
		m.Ticket = f.Ticket
	}

	m.Name, m.NameTurns = old.Name, old.NameTurns
	switch {
	case f.Turns > old.NameTurns && s.State != "working" && name != nil:
		if n, ok := makeName(ctx, s, f, name); ok {
			m.Name, m.NameTurns = n, f.Turns
		} else if m.Name == "" {
			m.Name = titleName(f)
		}
	case m.Name == "":
		// A session busy since it started has a name from its title until the
		// namer may run; name_turns stays, so the namer runs when the turn ends.
		m.Name = titleName(f)
	}
	return m
}

// makeName asks the namer; on failure it takes the first words of the title.
// It reports false when the namer had no time, so a later run asks again.
func makeName(ctx context.Context, s Session, f transcript.Facts, name Namer) (string, bool) {
	turn, _ := transcript.LastTurn(s.TranscriptPath)
	prompt := fmt.Sprintf("%sFirst prompt: %s\nTitle: %s\nLast turn:\n%s\n",
		namePrompt, cut(f.FirstPrompt, firstPromptMax), f.Title, turn)
	out, err := name(ctx, prompt)
	if errors.Is(err, ErrNotNamed) {
		return "", false
	}
	if err == nil {
		if n := words(strings.Trim(firstLine(out), "\"'`*. ")); n != "" {
			return n, true
		}
	}
	return titleName(f), true
}

// titleName is the first words of the title, else of the first prompt.
func titleName(f transcript.Facts) string {
	if n := words(f.Title); n != "" {
		return n
	}
	return words(f.FirstPrompt)
}

// gitPlace returns the repository toplevel and branch of dir, or "".
func gitPlace(ctx context.Context, dir string) (repo, branch string) {
	if dir == "" {
		return "", ""
	}
	out, err := run(ctx, "", "git", "-C", dir, "rev-parse", "--show-toplevel", "--abbrev-ref", "HEAD")
	if err != nil {
		return "", ""
	}
	repo, branch, _ = strings.Cut(strings.TrimSpace(out), "\n")
	return repo, strings.TrimSpace(branch)
}

// pullRequest asks gh for the branch's pull request; nil when there is
// none, gh fails or it takes too long.
func pullRequest(ctx context.Context, repo, branch string) *PR {
	switch branch {
	case "", "HEAD", "main", "master":
		return nil
	}
	out, err := run(ctx, repo, "gh", "pr", "view", branch, "--json", "number,state")
	if err != nil {
		return nil
	}
	var pr PR
	if json.Unmarshal([]byte(out), &pr) != nil || pr.Number < 1 {
		return nil
	}
	switch pr.State = strings.ToLower(pr.State); pr.State {
	case "open", "closed", "merged":
		return &pr
	}
	return nil
}

func run(ctx context.Context, dir, name string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, commandWait)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	cmd.WaitDelay = time.Second
	out, err := cmd.Output()
	return string(out), err
}

// cut keeps the first n characters of s.
func cut(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}

func words(s string) string {
	f := strings.Fields(s)
	return strings.Join(f[:min(len(f), nameWords)], " ")
}

func firstLine(s string) string {
	l, _, _ := strings.Cut(strings.TrimSpace(s), "\n")
	return l
}

// ReadState reads the sessions of state.json.
func ReadState(path string) (State, error) {
	var st State
	data, err := os.ReadFile(path)
	if err != nil {
		return st, fmt.Errorf("read state: %w", err)
	}
	if err := json.Unmarshal(data, &st); err != nil {
		return st, fmt.Errorf("parse state %s: %w", path, err)
	}
	return st, nil
}

// ReadFile reads meta.json.
func ReadFile(path string) (File, error) {
	var mf File
	data, err := os.ReadFile(path)
	if err != nil {
		return mf, fmt.Errorf("read meta: %w", err)
	}
	if err := json.Unmarshal(data, &mf); err != nil {
		return mf, fmt.Errorf("parse meta %s: %w", path, err)
	}
	return mf, nil
}

// WriteFile replaces meta.json whole, readable by its owner only.
func WriteFile(path string, mf File) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("meta mkdir: %w", err)
	}
	mf.SchemaVersion = SchemaVersion
	if mf.Sessions == nil {
		mf.Sessions = map[string]Meta{}
	}
	data, err := json.MarshalIndent(mf, "", "  ")
	if err != nil {
		return fmt.Errorf("meta marshal: %w", err)
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("meta write tmp: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("meta rename: %w", err)
	}
	return nil
}
