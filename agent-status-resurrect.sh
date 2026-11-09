#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# agent-status-resurrect.sh: a tmux-resurrect save command strategy. A pane
# running an agent is saved with its session id, so a restore resumes that
# session; every other pane is saved with its command, as resurrect saves it.
# The tmux plugin links it into resurrect when @agent-status-resurrect is on.
#
#   agent-status-resurrect.sh <pane pid>

# req: restore-resumes
pane_pid="$1"
# The pane's child processes, matched on the exact parent pid.
command="$(ps -ao "ppid=,args=" | sed -n "s/^ *$pane_pid //p")"

# A node agent shows as "node /path/bin/pi ..."; the harness is then the second word.
read -r first second _ <<<"$command"
program="$(basename "$first")"
[ "$program" = node ] && program="$(basename "$second")"
case "$program" in
claude) flag=--resume ;;
pi) flag=--session ;;
*) echo "$command" && exit 0 ;;
esac

pane="$(tmux list-panes -a -F '#{pane_pid} #{pane_id}' | awk -v p="$pane_pid" '$1 == p { print $2 }')"
[ -n "$pane" ] && read -r harness id <<<"$("${AGENT_STATUS_BIN:-agent-status}" session "$pane" 2>/dev/null)"
# ponytail: the agent's other arguments are dropped; keep them if a restored session needs them.
if [ -n "${id:-}" ] && [ "${harness:-}" = "$program" ]; then
	command="$program $flag $id"
fi
echo "$command"
