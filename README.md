# agent-status

AI session state tracker for tmux. Shows what each pi and Claude session is doing
in the window list: working, waiting for you, or failed.

```
 1 lab   ▶ 3 ops ✗  5 notes ?  7 build    ← pi running, claude error, question
```

## Components

| What | Where | Install |
|---|---|---|
| `watcher` binary | `cmd/watcher/` | apt (Linux) or brew (macOS) |
| tmux plugin | `tmux/agent-status.tmux` | `set -g @plugin 'jvrsantacruz/agent-status-tmux'` |
| pi extension | `<agent-dir>/extensions/agent-status.ts` | stowed from Skills |
| Claude hooks | inline in `settings.json` | stowed from Skills |

## Architecture

```
emitters → FIFO → watcher (daemon) → sqlite + state.json → tmux plugin (render)
```

## Subcommands

| Command | What |
|---|---|
| `watcher` | Daemon: reads FIFO, writes state.json. Runs as systemd/launchd service |
| `watcher emit <state> [msg]` | Write one event to FIFO (called by Claude hooks) |
| `watcher render <tmux_target>` | Output tmux marker for a pane (called by window-status-format) |

## Install

**macOS:**
```bash
brew install jvrsantacruz/tap/agent-status
brew services start agent-status
```

**Linux:**
```bash
sudo apt install ./agent-status_0.1.0_amd64.deb
systemctl --user enable --now agent-status
```

See [Formula/agent-status.rb](Formula/agent-status.rb) for the Homebrew recipe.

## Build

```bash
go build -o watcher ./cmd/watcher/
```

## Test

```bash
go test ./internal/...
go test -tags=e2e ./cmd/watcher/
```
