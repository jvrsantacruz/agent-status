// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 Javier Santacruz

package transcript

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/jvrsantacruz/agent-status/internal/price"
)

// Facts is what a transcript tells about its session.
type Facts struct {
	Title       string
	FirstPrompt string
	LastPrompt  string
	// Ticket is the first ticket named in a user prompt.
	Ticket       string
	Model        string
	Turns        int
	StartedAt    int64
	LastActiveAt int64
	Tokens       price.Tokens
	CostUSD      float64
	// Priced is false when a reply's model has no price: CostUSD is short.
	Priced bool
}

var ticketRe = regexp.MustCompile(`\b([A-Z][A-Z0-9]+-\d+)\b|#(\d+)`)

// Ticket returns the first ticket in s, such as LAB-7 or #12, or "".
func Ticket(s string) string {
	m := ticketRe.FindStringSubmatch(s)
	switch {
	case m == nil:
		return ""
	case m[1] != "":
		return m[1]
	}
	return "#" + m[2]
}

// Read reads the transcript of a claude or pi session.
func Read(harness, path string) (Facts, error) {
	if harness == "pi" {
		return readPi(path)
	}
	return readClaude(path)
}

type reply struct {
	model  string
	tokens price.Tokens
}

// readClaude reads the session file and its subagents' files. Claude Code
// writes one API reply over several lines with the same message id, the
// last of them with the final usage.
func readClaude(path string) (Facts, error) {
	f := Facts{Priced: true}
	subagents, _ := filepath.Glob(strings.TrimSuffix(path, ".jsonl") + "/subagents/*.jsonl")
	replies := map[string]reply{}
	var order []string
	for i, file := range append([]string{path}, subagents...) {
		main := i == 0
		err := eachLine(file, func(l line) {
			if main && l.Type == "ai-title" {
				f.Title = l.AITitle
			}
			if l.Type != "user" && l.Type != "assistant" {
				return
			}
			if main {
				f.seen(millis(l.Timestamp))
			}
			if l.Type == "user" {
				if main && !l.IsMeta {
					if t := strings.TrimSpace(text(l.Message.Content)); t != "" && !strings.HasPrefix(t, "<") {
						f.prompt(t)
					}
				}
				return
			}
			if main && l.Message.Model != "" && !strings.HasPrefix(l.Message.Model, "<") {
				f.Model = l.Message.Model
			}
			id := l.Message.ID
			if id == "" {
				id = fmt.Sprintf("%s:%d", file, len(order))
			}
			if _, ok := replies[id]; !ok {
				order = append(order, id)
			}
			replies[id] = reply{l.Message.Model, claudeTokens(l.Message.Usage)}
		})
		if err != nil && main {
			return Facts{}, err
		}
	}
	for _, id := range order {
		r := replies[id]
		f.Tokens = f.Tokens.Add(r.tokens)
		usd, ok := price.Cost(r.model, r.tokens)
		f.CostUSD += usd
		if !ok && r.model != "" && !strings.HasPrefix(r.model, "<") {
			f.Priced = false
		}
	}
	return f, nil
}

func claudeTokens(raw json.RawMessage) price.Tokens {
	var u struct {
		Input         int64 `json:"input_tokens"`
		Output        int64 `json:"output_tokens"`
		CacheRead     int64 `json:"cache_read_input_tokens"`
		CacheCreation int64 `json:"cache_creation_input_tokens"`
		Split         struct {
			OneHour int64 `json:"ephemeral_1h_input_tokens"`
		} `json:"cache_creation"`
	}
	_ = json.Unmarshal(raw, &u)
	return price.Tokens{Input: u.Input, Output: u.Output, CacheRead: u.CacheRead,
		CacheWrite5m: u.CacheCreation - u.Split.OneHour, CacheWrite1h: u.Split.OneHour}
}

// readPi reads a pi session, which prices each reply itself.
func readPi(path string) (Facts, error) {
	f := Facts{Priced: true}
	err := eachLine(path, func(l line) {
		switch l.Type {
		case "session":
			f.StartedAt = millis(l.Timestamp)
		case "session_info":
			if l.Name != "" {
				f.Title = l.Name
			}
		case "message":
			f.seen(millis(l.Timestamp))
			switch l.Message.Role {
			case "user":
				if t := strings.TrimSpace(text(l.Message.Content)); t != "" {
					f.prompt(t)
				}
			case "assistant":
				if l.Message.Model != "" {
					f.Model = l.Message.Model
				}
				var u struct {
					Input      int64 `json:"input"`
					Output     int64 `json:"output"`
					CacheRead  int64 `json:"cacheRead"`
					CacheWrite int64 `json:"cacheWrite"`
					Cost       struct {
						Total float64 `json:"total"`
					} `json:"cost"`
				}
				_ = json.Unmarshal(l.Message.Usage, &u)
				f.Tokens = f.Tokens.Add(price.Tokens{Input: u.Input, Output: u.Output,
					CacheRead: u.CacheRead, CacheWrite5m: u.CacheWrite})
				f.CostUSD += u.Cost.Total
			}
		}
	})
	return f, err
}

func (f *Facts) prompt(t string) {
	if f.Turns == 0 {
		f.FirstPrompt = t
	}
	if f.Ticket == "" {
		f.Ticket = Ticket(t)
	}
	f.Turns++
	f.LastPrompt = t
}

func (f *Facts) seen(ms int64) {
	if ms == 0 {
		return
	}
	if f.StartedAt == 0 {
		f.StartedAt = ms
	}
	f.LastActiveAt = ms
}

func eachLine(path string, fn func(line)) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	sc := scanner(file)
	for sc.Scan() {
		var l line
		if json.Unmarshal(sc.Bytes(), &l) == nil {
			fn(l)
		}
	}
	return sc.Err()
}

// text joins the text of a message's content, a string or a list of blocks.
func text(content json.RawMessage) string {
	var s string
	if json.Unmarshal(content, &s) == nil {
		return s
	}
	var blocks []contentBlock
	_ = json.Unmarshal(content, &blocks)
	var parts []string
	for _, b := range blocks {
		if b.Type == "text" {
			parts = append(parts, b.Text)
		}
	}
	return strings.Join(parts, " ")
}

// millis reads an RFC 3339 time or epoch milliseconds; 0 when neither.
func millis(raw json.RawMessage) int64 {
	var s string
	if json.Unmarshal(raw, &s) == nil {
		if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
			return t.UnixMilli()
		}
		return 0
	}
	var ms int64
	_ = json.Unmarshal(raw, &ms)
	return ms
}
