#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# agent-status-resurrect.sh: a tmux-resurrect save command strategy. A pane
# running an agent is saved with its session id, so a restore resumes that
# session; every other pane is saved as resurrect's default strategy saves it.
# The tmux plugin links it into resurrect when @agent-status-resurrect is on.
#
#   agent-status-resurrect.sh <pane pid>

# Linked into resurrect's strategy directory, next to its default strategy.
pane_pid="$1"
command="$("$(dirname "$0")/ps.sh" "$pane_pid")"
pane="$(tmux list-panes -a -F '#{pane_pid} #{pane_id}' | awk -v p="$pane_pid" '$1 == p { print $2 }')"
read -r harness id <<<"$("${AGENT_STATUS_BIN:-agent-status}" session "$pane" 2>/dev/null)"

program="$(basename "${command%% *}")"
case "$harness" in
claude) flag=--resume ;;
pi) flag=--session ;;
*) flag= ;;
esac
# ponytail: the agent's other arguments are dropped; keep them if a restored session needs them.
if [ -n "$id" ] && [ -n "$flag" ] && [ "$program" = "$harness" ]; then
	command="$harness $flag $id"
fi
echo "$command"
