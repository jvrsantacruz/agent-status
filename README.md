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

### GNU/Linux: .deb or .rpm

Download the package for your machine from the
[latest release](https://github.com/jvrsantacruz/agent-status/releases/latest), check it, install
it, and start the daemon as a user service:

```sh
v=$(basename "$(curl -sLo /dev/null -w '%{url_effective}' https://github.com/jvrsantacruz/agent-status/releases/latest)")
arch=$(uname -m | sed 's/x86_64/amd64/; s/aarch64/arm64/')
base=https://github.com/jvrsantacruz/agent-status/releases/download/$v
curl -LO "$base/agent-status_${v#v}_linux_$arch.deb" -LO "$base/checksums.txt"   # or .rpm
sha256sum --check --ignore-missing checksums.txt

sudo apt install "./agent-status_${v#v}_linux_$arch.deb"     # Debian, Ubuntu
sudo dnf install "./agent-status_${v#v}_linux_$arch.rpm"     # Fedora, if you downloaded the .rpm
systemctl --user enable --now agent-status
```

The package installs `/usr/bin/agent-status`, the user unit
`/usr/lib/systemd/user/agent-status.service`, the tmux plugin
`/usr/share/agent-status/agent-status.tmux`, the [state.json](#data) schema
`/usr/share/agent-status/state.schema.json` and the examples in `/usr/share/doc/agent-status/`.

### macOS: Homebrew

This repository is its own tap:

```sh
brew tap jvrsantacruz/agent-status https://github.com/jvrsantacruz/agent-status
brew install jvrsantacruz/agent-status/agent-status
brew services start agent-status
```

The tmux plugin is in `$(brew --prefix)/share/agent-status/agent-status.tmux`.

### Any system: archive or Go

The release also has a `.tar.gz` per system (`linux` or `darwin`) and architecture (`amd64` or
`arm64`), with the binary, the plugin, the user unit and the examples. Put `agent-status` in your
`PATH` and run `agent-status daemon` from your own service manager.

With Go 1.26 or later:

```sh
go install github.com/jvrsantacruz/agent-status/cmd/agent-status@latest
```

### Check a download

Each release lists SHA-256 checksums, as above, and a signed provenance record that says which
GitHub workflow built each file:

```sh
gh attestation verify --owner jvrsantacruz agent-status_*.tar.gz
```

### Upgrade and remove

| Installed with | Upgrade | Remove |
|---|---|---|
| .deb, .rpm | install the new package the same way, then `systemctl --user restart agent-status` | `systemctl --user disable --now agent-status`, then `sudo apt remove agent-status` or `sudo dnf remove agent-status` |
| Homebrew | `brew upgrade agent-status`, then `brew services restart agent-status` | `brew services stop agent-status`, then `brew uninstall agent-status` and `brew untap jvrsantacruz/agent-status` |
| Go | `go install` again | delete the binary from `$(go env GOPATH)/bin` |

Removing the program keeps your data; the [Data](#data) section says where it is.

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
                                                         +--> state.json --> agent-status sync --> tmux @agent_mark
                                                         +--> state.db (last 7 days of events)
```

- `emit` writes one event to a FIFO and returns at once. If the daemon is not running, it says so
  on stderr and exits 0, so it never stops the agent.
- The daemon keeps the state of each session and writes `state.json`. Every 60 seconds it checks
  that each agent process still runs; a session whose process is gone shows as dead for 5 minutes,
  then disappears.
- The plugin starts one `agent-status sync` per tmux server. Once a second it reads `state.json`
  and sets the pane option `@agent_mark` on each pane whose marker changed, to a reference to
  `@agent_mark_<state>`, so the plugin or your theme decides glyphs and colours. The window list
  only reads the option: a redraw runs no command, and an unchanged state redraws nothing.
  `agent-status render <pane>` prints the same reference, for themes that call it.

## Event format

Any program can write events, with `agent-status emit` or by writing JSON lines to the FIFO
itself (`$XDG_RUNTIME_DIR/agent-status/in.fifo`, or `in.fifo` in the state directory). Keep each
line within 512 bytes, newline included: a FIFO write is atomic only up to `PIPE_BUF`, which is 512
on macOS, and a longer line can interleave with another agent's line.

```sh
agent-status emit [--session ID] [--harness NAME] [--pid PID] [--pane PANE] [--claude-hook] <state> [message]
```

Inside a Claude Code hook, `--claude-hook` reads the JSON Claude gives the hook on stdin: the
session id, and a message from the notification text ("Claude needs your permission") or the tool
name ("Bash", "permission: Bash"), or the type of an API error ("error: rate_limit"). It never
takes the prompt, a tool's input or an error's text, which can hold private text. Bad input is reported on stderr and the event is still sent.

| Field | Flag | Default | Meaning |
|---|---|---|---|
| `session_id` | `--session` | `$AGENT_STATUS_SESSION`, `$CLAUDE_CODE_SESSION_ID` | One per agent run. Required |
| `harness` | `--harness` | `$AGENT_STATUS_HARNESS`; `claude` inside Claude Code | Name of the agent |
| `event` | `<state>` | | `working`, `question`, `idle`, `error`, or `end` to remove the session |
| `pid` | `--pid` | `$AGENT_STATUS_PID`, `$CLAUDE_PID`, then the parent process | Agent process; the session dies with it |
| `tmux_pane` | `--pane` | `$TMUX_PANE` | Pane of the agent, such as `%3` |
| `ts` | | now | Milliseconds since the epoch |
| `message` | `[message]` | | Free text, shown in `state.json`; cut, ending in `...`, so the line fits 512 bytes |

`emit` exits 1 on a usage error and never exits 2, because a Claude Code hook that exits 2 blocks
the agent.

## Agents

| Agent | How |
|---|---|
| Claude Code | [examples/claude-hooks.json](examples/claude-hooks.json): ten hooks, below |
| pi | an extension that runs `agent-status emit --harness pi --session <id> <state>` on its events |
| Any command | [examples/agent-status-wrap](examples/agent-status-wrap): `agent-status-wrap aider ...` shows working while it runs and error if it fails |

Claude Code hooks in the example, and what each shows:

| Hook | State | Message |
|---|---|---|
| `SessionStart` | idle | |
| `UserPromptSubmit` | working | |
| `PreToolUse` | working | tool name |
| `PermissionRequest` | question, as the prompt appears | `permission: <tool>` |
| `PostToolUse` | working, after an approved tool | tool name |
| `PostToolUseFailure` | working, after a tool fails; Claude goes on | tool name |
| `Notification` | question, about 6 s after Claude starts waiting | Claude's text |
| `Stop` | idle | |
| `StopFailure` | error, when an API error ends the turn | `error: <type>`, such as `error: rate_limit` |
| `SessionEnd` | session removed | |

Claude has no hook for the moment you approve a tool, so a long tool you approved shows question
until it finishes.

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
| `@agent-status-auto-format` | `on`: the plugin puts the marker at the start of `window-status-format` and `window-status-current-format`, after their leading spaces. `off`: add `#{E:@agent_mark}` yourself |
| `@agent-status-resurrect` | `off`. `on`: with [tmux-resurrect](https://github.com/tmux-plugins/tmux-resurrect) installed by TPM, a pane running claude or pi is saved as `claude --resume <id>` or `pi --session <id>`, so a restore resumes the same session. The plugin links `agent-status-resurrect.sh` into resurrect's strategy directory and sets `@resurrect-save-command-strategy`. Add `claude` and `pi` to `@resurrect-processes` |

## Troubleshooting

`agent-status doctor` checks each step and prints a report to paste in a bug report:

```
ok    daemon        listening on /run/user/1000/agent-status/in.fifo
ok    version       daemon v0.2.0 pid 4184570, this binary v0.2.0
ok    round trip    event reached state.json in 50 ms
ok    render        marker "#{E:@agent_mark_question}"
ok    tmux plugin   @agent_mark_working is "#[fg=colour6]▶ #[default]"
ok    tmux format   window-status-format reads @agent_mark
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

`state.json` is the read side for other programs, such as a status bar or a script. Its shape is
[schema/state.schema.json](schema/state.schema.json) (JSON Schema 2020-12), also installed next
to the tmux plugin. Check `schema_version`: a new field keeps it, a removed or changed field
raises it. The file is replaced whole, never written in place, so a reader never sees half of it.

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
