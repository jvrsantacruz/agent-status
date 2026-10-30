# agent-status

A Go daemon and CLI that shows in the tmux window list what each AI coding agent is doing.
Agents emit events to a FIFO; the daemon keeps state and writes `state.json`; tmux calls
`render` per window. README.md "How it works" has the diagram.

## Commands

```sh
go build ./cmd/agent-status                         # build
go test -race ./...                                 # unit tests
go test -race -tags e2e ./...                       # plus end to end: real binary, daemon, FIFO
golangci-lint run --build-tags e2e ./...            # lint, config in .golangci.yml
pre-commit run --all-files                          # every check CI runs
goreleaser release --snapshot --clean --skip=sbom   # build release archives and packages locally
tools/third-party-licenses > THIRD_PARTY_LICENSES   # after any change to go.mod
```

## Layout

| Path | What |
|---|---|
| `cmd/agent-status/` | Commands: `daemon`, `emit`, `render`, `doctor`, `version`; `paths.go` is the one place paths are decided |
| `internal/event` | Event type and FIFO reader |
| `internal/state` | State machine, one session per `session_id` |
| `internal/store` | SQLite event history, 7 days |
| `internal/export` | `state.json` read and write |
| `agent-status.tmux` | TPM plugin |
| `examples/` | Claude Code hooks, a generic wrapper emitter |

## Rules

- The event JSON (README "Event format") and the `state.json` fields are a public contract.
  Adding a field is fine; renaming or removing one is a breaking change and needs a CHANGELOG entry.
- `emit` must never block and never exit 2: it runs inside agent hooks.
- `render` prints nothing on any error: its output is the tmux status line. Errors belong in
  `doctor` and the daemon log.
- The daemon logs one wide event per unit of work, not scattered lines.
- No message text on disk outside `state.json`; files 0600, directories 0700; no network access.
- Tests: testify; e2e tests build the binary and run it with a clean environment, see `cleanEnv`.
- Every Go and shell file starts with `SPDX-License-Identifier: GPL-3.0-or-later`.
- Commits: `<area>: <what changed>`, signed off (`git commit -s`). User-visible changes go in
  CHANGELOG.md under the next version.
