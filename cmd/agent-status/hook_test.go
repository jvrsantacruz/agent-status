// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 Javier Santacruz

package main

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Inputs as Claude Code sent them in a real session, trimmed.
func TestClaudeHookMessage(t *testing.T) {
	t.Parallel()
	cases := map[string]struct{ input, message string }{
		"notification": {`{"session_id":"s","hook_event_name":"Notification","message":"Claude needs your permission","notification_type":"permission_prompt"}`, "Claude needs your permission"},
		"permission":   {`{"session_id":"s","hook_event_name":"PermissionRequest","tool_name":"Bash","tool_input":{"command":"touch x"}}`, "permission: Bash"},
		"pre tool":     {`{"session_id":"s","hook_event_name":"PreToolUse","tool_name":"Bash","tool_input":{"command":"export TOKEN=secret"}}`, "Bash"},
		"post tool":    {`{"session_id":"s","hook_event_name":"PostToolUse","tool_name":"Edit"}`, "Edit"},
		"prompt":       {`{"session_id":"s","hook_event_name":"UserPromptSubmit","prompt":"my private question"}`, ""},
		"stop":         {`{"session_id":"s","hook_event_name":"Stop"}`, ""},
		"tool failure": {`{"session_id":"s","hook_event_name":"PostToolUseFailure","tool_name":"Bash","tool_input":{"command":"cat secret"},"error":"exit 1"}`, "Bash"},
		"stop failure": {`{"session_id":"s","hook_event_name":"StopFailure","error":"rate_limit","error_details":"429 Too Many Requests","last_assistant_message":"API Error: Rate limit reached"}`, "error: rate_limit"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			h, err := readClaudeHook(strings.NewReader(c.input))
			require.NoError(t, err)
			assert.Equal(t, "s", h.SessionID)
			assert.Equal(t, c.message, h.message())
		})
	}
}

func TestClaudeHookBadInput(t *testing.T) {
	t.Parallel()
	_, err := readClaudeHook(strings.NewReader("not json"))
	assert.ErrorContains(t, err, "parse hook input")
}
