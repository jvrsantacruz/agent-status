# Changelog

## 0.4.0

- A `done` state, marked `✓`: an agent runs `agent-status emit done "<summary>"` when it finishes
  its task. A `done` or `error` an agent emits itself is declared: it holds until the turn ends, so
  the hooks after it do not hide it. `emit --prompt`, or Claude's prompt hook, marks the event that starts a turn, so a turn interrupted
  after a declaration does not carry it into the next.
- `emit --claude-hook --judge` on the `Stop` hook: a small model (`claude -p --model haiku`, hooks
  off) reads how the turn ended and sets `done`, `question` or `error` when the agent declared
  nothing. It runs detached and never delays the agent. Opt-in.
- Claude's idle reminder, a minute after a turn, is ignored: it no longer turns a finished turn into
  `question`.
- `state.json`: the `done` state, a `declared` field while a turn has one, and the reason
  `declared:<state>`. `schema_version` stays 1.
- tmux: markers show again after tmux restarts. tmux loads the plugin before any session exists,
  and `sync` took that empty server for a gone one and exited, so no window had a marker until
  the plugin was reloaded.

## 0.3.1

- tmux-resurrect: with `set -g @agent-status-resurrect 'on'`, a pane running claude or pi is saved
  with its session id, so a restore resumes that session instead of starting a new one.
- `agent-status session <pane>` prints the harness and id of the live session on a pane.
- tmux: a pane where an agent quit and another started shows the new agent's state, not the old
  one as dead.
- emit `--claude-hook` names the harness `claude` even without Claude's environment.
- A restarted daemon reloads the sessions in `state.json`, so an upgrade forgets no running
  session; the ones whose process exited while it was down show as dead.

## 0.3.0

- tmux: the window list no longer blinks. The plugin ran `agent-status render` once per window on
  every `status-interval`, and tmux redrew the status line each time one returned. Now one
  `agent-status sync` per tmux server reads `state.json` once a second and sets the pane option
  `@agent_mark` only where the marker changed; the format reads `#{E:@agent_mark}` and runs no
  command. Reloading the plugin replaces an old `render` call in the format. `render` stays for
  themes that call it.
- `doctor` checks that `window-status-format` reads `@agent_mark`.

## 0.2.3

- `state.json` has a schema, [schema/state.schema.json](schema/state.schema.json), and a
  `schema_version` field, 1. A test checks every file the daemon writes against it. Packages
  and archives install it next to the tmux plugin.

## 0.2.2

- tmux: the marker goes after the window format's leading spaces, one space from the window name:
  ` ▶ 1:api `. Before, it went in front of them, against the previous window and two spaces
  from its name ([#2](https://github.com/jvrsantacruz/agent-status/issues/2)).
- Claude Code: the example has two more hooks. `StopFailure` shows error when an API error ends
  the turn, with its type such as `error: rate_limit`; before, the window showed working until
  Claude exited. `PostToolUseFailure` shows working after a tool fails; before, a failed tool
  that needed approval left question until the next tool ran.

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
