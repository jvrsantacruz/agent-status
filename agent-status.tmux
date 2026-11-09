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
#   @agent-status-resurrect 'on'     tmux-resurrect saves each agent pane
#                                    with its session, so a restore resumes it

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

# With tmux-resurrect, an agent pane is saved with its session id, so a
# restore resumes it. Resurrect only runs strategies from its own directory.
if [ "$(tmux show-options -gqv @agent-status-resurrect)" = "on" ]; then
	plugin_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
	tpm_dir="$(tmux show-environment -g TMUX_PLUGIN_MANAGER_PATH 2>/dev/null | cut -d= -f2-)"
	strategies="${tpm_dir%/}/tmux-resurrect/save_command_strategies"
	if [ -n "$tpm_dir" ] && [ -d "$strategies" ]; then
		ln -sf "$plugin_dir/agent-status-resurrect.sh" "$strategies/agent-status.sh"
		tmux set-option -g @resurrect-save-command-strategy agent-status
		# Resurrect replays only the programs on its list, which it adds to
		# its defaults; false and :all: are the user's choice.
		processes="$(tmux show-options -gqv @resurrect-processes)"
		case "$processes" in
		false | :all:) ;;
		*)
			for program in claude pi; do
				case " $processes " in
				*" $program "*) ;;
				*) processes="$processes $program" ;;
				esac
			done
			tmux set-option -g @resurrect-processes "${processes# }"
			;;
		esac
	else
		tmux display-message "agent-status: @agent-status-resurrect is on, but tmux-resurrect is not installed with TPM"
	fi
fi

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
