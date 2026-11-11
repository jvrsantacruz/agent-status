// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 Javier Santacruz

// Package transcript reads what agent harnesses write about a session.
package transcript

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// turnBytes bounds the end of the turn LastTurn returns.
const turnBytes = 6000

// line is the part of a transcript line the readers use: Claude Code's
// user and assistant lines, and pi's message lines, whose role is inside.
type line struct {
	Type      string          `json:"type"`
	Timestamp json.RawMessage `json:"timestamp"`
	IsMeta    bool            `json:"isMeta"`
	AITitle   string          `json:"aiTitle"`
	Name      string          `json:"name"`
	Message   struct {
		ID      string          `json:"id"`
		Role    string          `json:"role"`
		Model   string          `json:"model"`
		Content json.RawMessage `json:"content"`
		Usage   json.RawMessage `json:"usage"`
	} `json:"message"`
}

// kind is user or assistant for both harnesses.
func (l line) kind() string {
	if l.Type == "message" {
		return l.Message.Role
	}
	return l.Type
}

type contentBlock struct {
	Type    string `json:"type"`
	Text    string `json:"text"`
	Name    string `json:"name"`
	IsError bool   `json:"is_error"`
}

// LastTurn renders the last user prompt and what the agent did after it:
// its text, the tools it called and which of them failed. Tool input and
// output are left out; the end is kept when it is long.
func LastTurn(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	var turn []string
	sc := scanner(f)
	for sc.Scan() {
		var l line
		if json.Unmarshal(sc.Bytes(), &l) != nil {
			continue
		}
		k := l.kind()
		if k != "user" && k != "assistant" {
			continue
		}
		var text string
		if json.Unmarshal(l.Message.Content, &text) == nil {
			if k == "user" {
				turn = []string{"USER: " + text}
			}
			continue
		}
		var blocks []contentBlock
		if json.Unmarshal(l.Message.Content, &blocks) != nil {
			continue
		}
		for _, b := range blocks {
			switch {
			case b.Type == "text" && k == "user":
				turn = []string{"USER: " + b.Text}
			case b.Type == "text":
				turn = append(turn, "AGENT: "+b.Text)
			case b.Type == "tool_use" || b.Type == "toolCall":
				turn = append(turn, "TOOL: "+b.Name)
			case b.Type == "tool_result" && b.IsError:
				turn = append(turn, "TOOL FAILED")
			}
		}
	}
	if err := sc.Err(); err != nil {
		return "", err
	}
	// A turn ends with the agent's message; anything else is still running.
	if len(turn) == 0 || !strings.HasPrefix(turn[len(turn)-1], "AGENT: ") {
		return "", nil
	}
	out := strings.Join(turn, "\n")
	if len(out) > turnBytes {
		out = fmt.Sprintf("...%s", out[len(out)-turnBytes:])
	}
	return out, nil
}

func scanner(f *os.File) *bufio.Scanner {
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 16<<20)
	return sc
}
