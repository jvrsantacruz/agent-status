// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 Javier Santacruz

package main

import (
	"encoding/json"
	"fmt"
	"io"
)

// maxHookInput bounds what emit reads from a hook's stdin.
const maxHookInput = 1 << 20

// claudeHook is the part of the JSON Claude Code writes to a hook's stdin
// that emit uses. https://code.claude.com/docs/en/hooks
type claudeHook struct {
	SessionID string `json:"session_id"`
	Event     string `json:"hook_event_name"`
	Message   string `json:"message"`
	ToolName  string `json:"tool_name"`
	Error     string `json:"error"`
}

func readClaudeHook(r io.Reader) (claudeHook, error) {
	var h claudeHook
	data, err := io.ReadAll(io.LimitReader(r, maxHookInput))
	if err != nil {
		return h, fmt.Errorf("read hook input: %w", err)
	}
	if err := json.Unmarshal(data, &h); err != nil {
		return h, fmt.Errorf("parse hook input: %w", err)
	}
	return h, nil
}

// message says what the agent is doing or asking, from the fields Claude
// sends for that hook. It never takes the prompt, a tool's input or an
// error's text, which can hold private text; state.json shows only the
// tool's name or the error's type, such as rate_limit.
func (h claudeHook) message() string {
	switch h.Event {
	case "Notification":
		return h.Message
	case "PermissionRequest":
		if h.ToolName != "" {
			return "permission: " + h.ToolName
		}
	case "PreToolUse", "PostToolUse", "PostToolUseFailure":
		return h.ToolName
	case "StopFailure":
		if h.Error != "" {
			return "error: " + h.Error
		}
	}
	return ""
}
