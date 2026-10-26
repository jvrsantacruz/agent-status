#!/bin/sh
# agent-status-emit — write one state event to the agent-status FIFO.
# Called by Claude Code hooks and optionally by the pi extension.
# Usage: agent-status-emit <state> [message]
# State: working | question | idle | error
# Reads env: CLAUDE_CODE_SESSION_ID, PPID, TMUX_PANE, TMUX_SESSION_NAME
set -u

state="${1:?usage: agent-status-emit <state> [message]}"
msg="${2:-}"

fifo="${AGENT_STATUS_RUNTIME_DIR:-${XDG_RUNTIME_DIR:-/tmp}/agent-status/in.fifo}"

# Construct the line with what Claude's hooks give us
session_id="${CLAUDE_CODE_SESSION_ID:-}"
pid="${PPID:-}"
pane="${TMUX_PANE:-}"
session_name="${TMUX_SESSION_NAME:-}"
ts=$(date +%s%3N 2>/dev/null || date +%s000)

# Write JSON to FIFO (non-blocking best-effort)
printf '{"session_id":"%s","harness":"claude","event":"%s","pid":%s,"tmux_target":"%s:%s","ts":%s,"message":"%s"}\n' \
  "$session_id" "$state" "$pid" "$session_name" "$pane" "$ts" "$msg" \
  > "$fifo" 2>/dev/null || :
