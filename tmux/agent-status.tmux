#!/usr/bin/env bash
# agent-status.tmux — tpm plugin that shows ai session state in the window list.
# Relies on the `watcher` binary (agent-status-watcher) being in PATH.
# Install:
#   set -g @plugin 'jvrsantacruz/agent-status-tmux'
# Requires watcher 0.1.0+ installed separately.
#
# The marker is a prefix before each window name.
# Add the placeholder yourself, or let this plugin set the default format:
#   set -g @agent-status-auto-format 'on'   (default)
#   set -g @agent-status-auto-format 'off'  (you manage window-status-format)

CURRENT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

if ! command -v watcher &>/dev/null && ! test -f "$CURRENT_DIR/../watcher"; then
	exit 0
fi

RENDER_CMD="${AGENT_STATUS_BIN:-$(command -v watcher || echo "$CURRENT_DIR/../watcher")}"

# Style options — the theme may override these to palette slots.
# Defaults use the brand-terminal palette slots:
#   slot 6 cyan, slot 3 yellow (activity), slot 1 red (error ink),
#   slot 8 bright black (secondary text, idle windows)
tmux set-option -g @agent_mark_working  '#[fg=colour6]▶ #[default]'
tmux set-option -g @agent_mark_question '#[fg=colour3,bold]? #[default]'
tmux set-option -g @agent_mark_error   '#[fg=colour1]✗ #[default]'
tmux set-option -g @agent_mark_dead    '#[fg=colour8,dim]x #[default]'

# Window-status-format: render call + index + name + flags
DEFAULT_FMT="#($RENDER_CMD render '#{session_name}:#{pane_id}')#I #{window_name}#{?window_flags, #{window_flags},}"

tmux set-option -g window-status-format "$DEFAULT_FMT"
tmux set-option -g window-status-current-format "$DEFAULT_FMT"
