#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# agent-status.tmux: TPM plugin. Puts the state of the AI agent running in
# each window in front of the window name in the status line.
#
#   set -g @plugin 'jvrsantacruz/agent-status'
#
# Needs the agent-status binary in PATH, or its path in $AGENT_STATUS_BIN.
# Options, set before TPM runs:
#   @agent-status-auto-format 'on'   prefix window-status-format and
#                                    window-status-current-format (default)
#   @agent-status-auto-format 'off'  you add the marker yourself:
#                                    #{E:@agent_mark}
#   @agent_mark_working, @agent_mark_question, @agent_mark_error,
#   @agent_mark_dead                 the glyph and colours of each state

bin="${AGENT_STATUS_BIN:-$(command -v agent-status)}"
if [ -z "$bin" ]; then
	tmux display-message "agent-status: binary not found in PATH"
	exit 0
fi

# Defaults use the 16 ANSI colours, so any terminal theme recolours them. A
# value the user already set is kept.
set_default() {
	if [ -z "$(tmux show-options -gqv "$1")" ]; then
		tmux set-option -g "$1" "$2"
	fi
}
set_default @agent_mark_working '#[fg=colour6]▶ #[default]'
set_default @agent_mark_question '#[fg=colour3,bold]? #[default]'
set_default @agent_mark_error '#[fg=colour1]✗ #[default]'
set_default @agent_mark_dead '#[fg=colour8,dim]x #[default]'

# One sync per server reads state.json and loads each pane's marker into
# @agent_mark; the window list only reads it, so a redraw runs no job.
tmux run-shell -b "'$bin' sync"

[ "$(tmux show-options -gqv @agent-status-auto-format)" = "off" ] && exit 0

# After the format's leading spaces, so the marker is one space from the
# window name and not against the previous window: " #I:#W " becomes
# " ▶ #I:#W ". A format from an older plugin loses its render job.
mark='#{E:@agent_mark}'
for option in window-status-format window-status-current-format; do
	current="$(tmux show-options -gqv "$option")"
	current="${current//"#($bin render '#{pane_id}')"/}"
	current="${current//"#(agent-status render '#{pane_id}')"/}"
	lead="${current%%[! ]*}"
	case "$current" in
	*"$mark"*) ;;
	*) current="$lead$mark${current#"$lead"}" ;;
	esac
	tmux set-option -g "$option" "$current"
done
