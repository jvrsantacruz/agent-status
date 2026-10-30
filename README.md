# agent-status

Shows in your tmux window list which AI coding agent is working, waiting for you, or failed.

```
 1 api ▶  2 docs ?  3 infra ✗  4 notes
       |        |         |
       |        |         +-- the agent stopped on an error
       |        +------------ the agent asks you something
       +--------------------- the agent is working
```

With several agents in several windows, you see at a glance which one needs you, without
visiting each window.

It works with any agent that can run a command on a state change: Claude Code through its
hooks, pi through an extension, any other program through `agent-status emit`.

## Install

GNU/Linux, from the [latest release](https://github.com/jvrsantacruz/agent-status/releases/latest):

```sh
sudo apt install ./agent-status_*_amd64.deb      # Debian, Ubuntu
sudo dnf install ./agent-status-*.x86_64.rpm     # Fedora
systemctl --user enable --now agent-status
```

macOS, with Homebrew:

```sh
brew install jvrsantacruz/tap/agent-status
brew services start agent-status
```

With Go 1.26 or later, on either system:

```sh
go install github.com/jvrsantacruz/agent-status/cmd/agent-status@latest
agent-status daemon &        # or a service of your own
```

Each release lists SHA-256 checksums and a signed provenance record, which says which
workflow built each file:

```sh
sha256sum --check --ignore-missing checksums.txt
gh attestation verify --owner jvrsantacruz agent-status_*.tar.gz
```

## Quickstart

1. Start the daemon, as above.
2. Add the tmux plugin with [TPM](https://github.com/tmux-plugins/tpm) in `~/.tmux.conf`, then
   press `prefix I`:

   ```tmux
   set -g @plugin 'jvrsantacruz/agent-status'
   ```

   Without TPM: `run-shell /usr/share/agent-status/agent-status.tmux` (Linux packages), or the
   path of `agent-status.tmux` in a release archive.
3. Tell your agent to emit its state. For Claude Code, merge
   [examples/claude-hooks.json](examples/claude-hooks.json) into `~/.claude/settings.json`.
4. Check the whole chain:

   ```sh
   agent-status doctor
   ```

## How it works

```
agent hook ---- agent-status emit ----+
                                      |  one JSON line per event
other program ------------------------+--> FIFO --> agent-status daemon
                                                         |
                                                         +--> state.json --> agent-status render --> tmux
                                                         +--> state.db (last 7 days of events)
```

- `emit` writes one event to a FIFO and returns at once. If the daemon is not running, it says so
  on stderr and exits 0, so it never stops the agent.
- The daemon keeps the state of each session and writes `state.json`. Every 60 seconds it checks
  that each agent process still runs; a session whose process is gone shows as dead for 5 minutes,
  then disappears.
- tmux runs `agent-status render <pane>` for each window. It prints a reference to the tmux option
  `@agent_mark_<state>`, so the plugin or your theme decides glyphs and colours.

## Event format

Any program can write events, with `agent-status emit` or by writing JSON lines to the FIFO
itself (`$XDG_RUNTIME_DIR/agent-status/in.fifo`, or `in.fifo` in the state directory).

```sh
agent-status emit [--session ID] [--harness NAME] [--pid PID] [--pane PANE] <state> [message]
```

| Field | Flag | Default | Meaning |
|---|---|---|---|
| `session_id` | `--session` | `$AGENT_STATUS_SESSION`, `$CLAUDE_CODE_SESSION_ID` | One per agent run. Required |
| `harness` | `--harness` | `$AGENT_STATUS_HARNESS`; `claude` inside Claude Code | Name of the agent |
| `event` | `<state>` | | `working`, `question`, `idle`, `error`, or `end` to remove the session |
| `pid` | `--pid` | `$AGENT_STATUS_PID`, `$CLAUDE_PID`, then the parent process | Agent process; the session dies with it |
| `tmux_pane` | `--pane` | `$TMUX_PANE` | Pane of the agent, such as `%3` |
| `ts` | | now | Milliseconds since the epoch |
| `message` | `[message]` | | Free text, shown in `state.json`, at most 1024 bytes |

`emit` exits 1 on a usage error and never exits 2, because a Claude Code hook that exits 2 blocks
the agent.

## Agents

| Agent | How |
|---|---|
| Claude Code | [examples/claude-hooks.json](examples/claude-hooks.json): hooks set working, question, idle, end |
| pi | an extension that runs `agent-status emit --harness pi --session <id> <state>` on its events |
| Any command | [examples/agent-status-wrap](examples/agent-status-wrap): `agent-status-wrap aider ...` shows working while it runs and error if it fails |

## Configuration

| Variable | Default | Used for |
|---|---|---|
| `AGENT_STATUS_STATE_DIR` | `$XDG_STATE_HOME/agent-status`, else `~/.local/state/agent-status` | `state.json`, `state.db` |
| `AGENT_STATUS_RUNTIME_DIR` | `$XDG_RUNTIME_DIR/agent-status`, else the state directory | `in.fifo` |
| `AGENT_STATUS_LOG` | `info` | Daemon log level: `debug`, `info`, `warn`, `error` |
| `AGENT_STATUS_BIN` | `agent-status` in `PATH` | Binary the tmux plugin calls |

tmux options, set before TPM runs:

| Option | Default |
|---|---|
| `@agent_mark_working` | `#[fg=colour6]▶ #[default]` |
| `@agent_mark_question` | `#[fg=colour3,bold]? #[default]` |
| `@agent_mark_error` | `#[fg=colour1]✗ #[default]` |
| `@agent_mark_dead` | `#[fg=colour8,dim]x #[default]` |
| `@agent-status-auto-format` | `on`: the plugin puts the marker in front of `window-status-format` and `window-status-current-format`. `off`: add `#(agent-status render '#{pane_id}')` yourself |

## Troubleshooting

`agent-status doctor` checks each step and prints a report to paste in a bug report:

```
ok    daemon        listening on /run/user/1000/agent-status/in.fifo
ok    version       daemon v0.2.0 pid 4184570, this binary v0.2.0
ok    round trip    event reached state.json in 50 ms
ok    render        marker "#{E:@agent_mark_question}"
ok    tmux plugin   @agent_mark_working is "#[fg=colour6]▶ #[default]"
ok    tmux format   window-status-format calls agent-status render
warn  pid           no pid: this session is never cleared when the agent exits; pass emit --pid
```

The daemon logs one JSON line per event and per liveness check: `journalctl --user -u
agent-status` on GNU/Linux, `$(brew --prefix)/var/log/agent-status.log` on macOS.

## Data

Everything stays on your machine, readable by you alone (directories 0700, files 0600):

| File | Content | Kept |
|---|---|---|
| `state.json` | Current sessions: state, pid, pane, last message | While the session lives, plus 5 minutes |
| `state.db` | Events: session, agent, state, pid, pane, times. No message text | 7 days |

No telemetry, no update check, no network access. To wipe everything: stop the daemon and
`rm -r ~/.local/state/agent-status`. Homebrew itself collects analytics unless
`HOMEBREW_NO_ANALYTICS=1` is set.

## Contributing

Bug reports and patches are welcome in
[issues](https://github.com/jvrsantacruz/agent-status/issues). [AGENTS.md](AGENTS.md) has the
build, test and lint commands, for people and coding agents alike. Sign off each commit
(`git commit -s`) to certify the [Developer Certificate of Origin](https://developercertificate.org/);
there is no contributor agreement. Security problems: [SECURITY.md](SECURITY.md).

## Licence

GPL-3.0-or-later, see [LICENSE](LICENSE). Notices of bundled modules: [THIRD_PARTY_LICENSES](THIRD_PARTY_LICENSES).
