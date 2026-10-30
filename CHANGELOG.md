# Changelog

## 0.2.0

The first release with installable files. 0.1.0 was tagged, but never built.

- Claude Code sessions show their marker. The pane now comes from `$TMUX_PANE`, which tmux sets;
  before, the target needed `TMUX_SESSION_NAME`, which tmux never sets, so no window matched.
- A session disappears when its agent exits. `emit` takes the agent pid from `--pid`,
  `$AGENT_STATUS_PID` or `$CLAUDE_PID`; before, every session had pid 0 and lived forever.
- On macOS, `emit` and the daemon use the same FIFO. Before, `emit` wrote to `/tmp`.
- Any agent can emit: `--session`, `--harness`, `--pid`, `--pane`. The `end` state removes a
  session at once. `emit` never exits 2, which would block a Claude Code hook.
- The binary is `agent-status`, not `watcher`. The daemon starts with `agent-status daemon`; no
  command, or an unknown one, prints usage instead of starting it.
- `agent-status version` and `agent-status doctor`: doctor sends a test event through the daemon
  to render, checks the tmux and Claude Code wiring, and lists each session with what is wrong.
- The daemon logs one JSON line per event and per liveness check, on stderr; `AGENT_STATUS_LOG`
  sets the level. `state.json` says which daemon wrote it, and why each session has its state.
- Privacy: no message text is stored in `state.db`, files are readable by you alone, and nothing
  goes to `/tmp`. An older `state.db` is reset on start.
- The repository is the TPM plugin: `set -g @plugin 'jvrsantacruz/agent-status'`. It puts the
  marker in front of your window format instead of replacing it.
- Licence: GPL-3.0-or-later, before MIT.
