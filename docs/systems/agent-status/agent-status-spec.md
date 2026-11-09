# agent-status spec

## overview

The **agent-status binary** holds every command. A harness reports each state change with
`emit`, which writes one line to a FIFO. The **daemon** reads it, keeps each session's state
and event history, marks a session dead when its process exits, and writes **state.json**.
In each tmux server, **sync** loads each pane's marker from state.json once a second. The
**tmux plugin** starts sync and, with resurrect integration on, gives resurrect a save
strategy that asks **session** for a pane's session id. **doctor** reads the history; it is left out of
the diagram.

unit: one agent session; every part above keeps, shows or resumes sessions.

```mermaid
flowchart LR
  agent[harness] -- state change --> fifo
  subgraph agent-status
    fifo@{ shape: h-cyl, label: "FIFO queue" } -- events --> daemon[agent-status daemon]
    daemon -- events --> db[(event history)]
    daemon -- sessions --> state[(state.json)]
    state -- sessions --> sync[agent-status sync]
    state -- sessions --> session[agent-status session]
    session -- session id --> plugin[tmux plugin]
  end
  sync -- pane marker --> tmux[tmux window list]
  resurrect[tmux-resurrect] -- pane pid --> plugin
  plugin -- resume command --> resurrect
```

_The containers of one machine; an arrow is data from producer to consumer._

## building blocks

**agent-status binary** (service, lives in `cmd/agent-status`): one binary. `daemon` runs as a
systemd user unit or a launchd agent and is the only writer of
state.json and the event history. `sync` is a loop, one per tmux server, that ends with the
server. session and render print nothing on any error and always exit 0.

**tmux plugin** (library, lives in `.`): `agent-status.tmux` defines the markers, puts the
marker in the window format and starts sync. With `@agent-status-resurrect` on, it links
`agent-status-resurrect.sh` into resurrect's strategy directory, names it as the save command
strategy, and adds claude and pi to the programs resurrect replays.

## contracts

**harness -> agent-status emit** (file, FIFO, auth file permissions)
One event per state change, from Claude Code hooks, a pi extension or any program. The
harness needs emit to exit 0 within its write timeout on any failure but a usage error: no
state, no session id, or fields too long for one FIFO line. agent-status needs the session
id, the process id to tell when it ends, the tmux pane, and the harness name, which must
match the program for a restore to resume it. Format:
[event format](https://github.com/jvrsantacruz/agent-status#event-format).
Requirements: `never-blocks`, `private-text`, `window-marks`, `restore-resumes`.

**daemon -> the user's programs** (file, state.json, auth file permissions)
Replaced whole after each change, so a reader never sees half a file. Fields are only added;
a removed or changed one raises the version of its
[schema](https://github.com/jvrsantacruz/agent-status/blob/main/schema/state.schema.json).
Requirements: `readable-record`.

**sync -> tmux** (ui, tmux pane options, auth tmux server socket)
sync sets the pane option `@agent_mark` where a marker changed; the window format reads it,
so a redraw runs no command.
Requirements: `window-marks`.

**tmux-resurrect -> tmux plugin** (file, save command strategy, auth none: runs as the user)
On save, resurrect records what the strategy prints for a pane's process id: the resume
command when the pane holds a live session of its program. On restore it types that command
when its restore list matches the program; a user's inline strategy for claude or pi
replaces it.
The plugin needs tmux-resurrect installed with TPM, since resurrect runs strategies only from
its own directory, and each harness to reopen a session from its id: `claude --resume`,
`pi --session`.
Requirements: `restore-resumes`.

## data model

| entity | stored in | key | shape | written by | kept for |
| --- | --- | --- | --- | --- | --- |
| session | state.json | session id, joined to a tmux pane by pane id | [state.schema.json](https://github.com/jvrsantacruz/agent-status/blob/main/schema/state.schema.json) | daemon | while live, then the dead keep time in `cmd/agent-status/daemon.go`, across restarts |
| event | state.db | row id, joined to a session by session id; no message text | session id, harness, event, process id, pane, sent and received time, whether understood | daemon | the retention in `cmd/agent-status/daemon.go` |

## states

```mermaid
stateDiagram-v2
  live: live (working, question, idle or error)
  [*] --> live: harness event
  live --> live: harness event
  live --> dead: process exited
  dead --> live: harness event
  live --> [*]: end event
  dead --> [*]: end event
  dead --> [*]: dead keep time passed
```

_Session states. A harness event moves to the state it names. Dead and removal serve `exit-clears`._

## failure modes

- **Event lost**: with no daemon or a full FIFO, emit says so on stderr and exits 0; the next event brings the session up to date.
- **Plain command saved**: when state.json is unreadable, the pane has no live session, or its agent sent no event, resurrect saves the plain command and the restore starts a new session.
- **Markers kept**: when state.json is unreadable, sync leaves every marker as it was; when it is missing, sync clears them.
- **Resurrect left alone**: when agent-status is too old or another save strategy is set, the plugin shows a message and changes nothing.
- **Session never cleared**: one without a process id is never marked dead and stays until its end event; doctor reports it.

## validation

fixtures: [e2e tests](https://github.com/jvrsantacruz/agent-status/blob/main/cmd/agent-status/e2e_test.go): the real binary runs in a clean environment, fed emit calls and saved Claude Code hook inputs, or hand-written state.json files, for every state, a dead session, a restart and a missing daemon; expected outputs from hand-checked state.json, render and session values; compared exactly.

- `restore-resumes` (test, [resurrect-e2e](https://github.com/jvrsantacruz/agent-status/blob/main/tools/resurrect-e2e)): a real save and restore on a scratch tmux server types the resume command for a live session and the plain command for an exited one.
- `never-blocks` (test, [e2e tests](https://github.com/jvrsantacruz/agent-status/blob/main/cmd/agent-status/e2e_test.go)): with no daemon, and with a daemon that never reads a full FIFO, emit returns within its write timeout and exits 0.
- `private-text` (test, [hook tests](https://github.com/jvrsantacruz/agent-status/blob/main/cmd/agent-status/hook_test.go)): the message taken from hook input never carries the prompt, a tool input or error text.
- `local-only` (inspection, [AGENTS.md](https://github.com/jvrsantacruz/agent-status/blob/main/AGENTS.md)): no package opens a network connection.
