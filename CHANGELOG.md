# Changelog

## 0.2.1

The first published release. Everything listed under 0.2.0 is in it: 0.2.0 was tagged, but its
release stopped before publishing.

- Homebrew: this repository is its own tap. Install with
  `brew tap jvrsantacruz/agent-status https://github.com/jvrsantacruz/agent-status`, then
  `brew install jvrsantacruz/agent-status/agent-status`.
- `doctor` lists sessions as they are after its round trip, not from before it.

## 0.2.0

The first release with installable files. 0.1.0 was tagged, but never built.

- Claude Code sessions show their marker. The pane now comes from `$TMUX_PANE`, which tmux sets;
  before, the target needed `TMUX_SESSION_NAME`, which tmux never sets, so no window matched.
- A session disappears when its agent exits. `emit` takes the agent pid from `--pid`,
  `$AGENT_STATUS_PID` or `$CLAUDE_PID`; before, every session had pid 0 and lived forever.
- On macOS, `emit` and the daemon use the same FIFO. Before, `emit` wrote to `/tmp`.
- No event is lost when two arrive close together. Before, after each writer closed, the FIFO had
  no reader until the daemon reopened it, and an event sent then was dropped.
- On macOS, events from several agents at once no longer corrupt each other. Each line now fits in
  512 bytes, the size macOS writes to a FIFO in one piece; a longer message is cut and ends in
  `...`. Before, lines up to about 1200 bytes could be split and mixed.
- `emit` gives up after 1 second if the daemon stops reading, instead of waiting for ever inside
  an agent hook.
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
- `emit --claude-hook` reads Claude Code's hook input: the session id, and a message such as
  "Claude needs your permission" or the tool name. The example hooks use it, and add
  `PermissionRequest`, so question shows as the permission prompt appears instead of 6 seconds
  later, and `PostToolUse`, so the state goes back to working after you approve a tool.
