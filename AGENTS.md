# agent-status

A Go daemon and CLI that shows in the tmux window list what each AI coding agent is doing.
Agents emit events to a FIFO; the daemon keeps state and writes `state.json`; one `sync` per
tmux server loads each pane's marker into `@agent_mark`, which the window list reads. README.md "How it works" has the diagram.

## Commands

```sh
go build ./cmd/agent-status                         # build
go test -race ./...                                 # unit tests
go test -race -tags e2e ./...                       # plus end to end: real binary, daemon, FIFO
tools/resurrect-e2e                                 # real tmux-resurrect save and restore on a scratch server
golangci-lint run --build-tags e2e ./...            # lint, config in .golangci.yml
pre-commit run --all-files                          # every check CI runs
goreleaser release --snapshot --clean --skip=sbom   # build release archives and packages locally
tools/third-party-licenses > THIRD_PARTY_LICENSES   # after any change to go.mod
```

## Install

To run what you changed, build it: `go build -o /tmp/agent-status ./cmd/agent-status`, then
`AGENT_STATUS_STATE_DIR=/tmp/as AGENT_STATUS_RUNTIME_DIR=/tmp/as /tmp/agent-status daemon`, so
it does not touch an installed daemon's state. Check it with `agent-status doctor`, using the same
two variables.

Users install a release: README "Install" has the commands per system (.deb or .rpm with a
systemd user unit, Homebrew from this repository's own tap, an archive, or `go install`). When you
change what a package installs or where, change that section too.

## Release

1. Move the changes in CHANGELOG.md under a new `## X.Y.Z` heading at the top. No `Unreleased`
   section may remain; the release workflow refuses the tag otherwise.
2. Commit, push, and wait for CI to pass on `main`.
3. `git tag -s vX.Y.Z -m "agent-status X.Y.Z"` and `git push origin vX.Y.Z`. The workflow builds
   every file, attests it, publishes the release, and commits `Formula/agent-status.rb` to `main`.
4. `git pull`: the formula commit lands after your tag.

Never edit `Formula/` by hand; the next release overwrites it. A published version is never
rebuilt: fix forward with a new patch version.

## Layout

| Path | What |
|---|---|
| `cmd/agent-status/` | Commands: `daemon`, `emit`, `enrich`, `sync`, `render`, `doctor`, `version`; `paths.go` is the one place paths are decided |
| `internal/event` | Event type and FIFO reader |
| `internal/state` | State machine, one session per `session_id` |
| `internal/store` | SQLite event history, 7 days |
| `internal/export` | `state.json` read and write |
| `internal/transcript` | Claude Code and pi transcript readers, shared by judge and enrich |
| `internal/price` | dollars per reply, a price table per model |
| `internal/enrich` | one session's metadata from its transcript, git, gh and the namer |
| `agent-status.tmux` | TPM plugin |
| `examples/` | Claude Code hooks, a generic wrapper emitter |
| `packaging/` | systemd user unit for the .deb and .rpm |
| `schema/` | JSON Schema of `state.json`, the read side for other programs. Change it with any field of `export.StateFile` or `state.Session`; raise `export.SchemaVersion` when a field is removed or changes meaning. `internal/export/schema_test.go` holds the two together |
| `Formula/` | Homebrew formula, written by the release workflow |
| `.goreleaser.yaml`, `.github/workflows/release.yml` | What a tag builds and publishes |

## Rules

- The event JSON (README "Event format") and the `state.json` fields are a public contract.
  Adding a field is fine; renaming or removing one is a breaking change and needs a CHANGELOG entry.
- `emit` must never block and never exit 2: it runs inside agent hooks.
- An event line is at most 512 bytes (`event.MaxLine`), the `PIPE_BUF` of macOS: longer writes to a
  FIFO are not atomic and interleave. Test FIFO changes on macOS too; CI does.
- `render` and `sync` print nothing on any error: their output is the tmux status line. Errors belong in
  `doctor` and the daemon log.
- The daemon logs one wide event per unit of work, not scattered lines.
- No message text on disk outside `state.json` and `meta.json`; files 0600, directories 0700; no network access except the opt-in judge and the enrich run's namer and `gh` read calls, each a separate process, never the daemon.
- Tests: testify; e2e tests build the binary and run it with a clean environment, see `cleanEnv`.
- Every Go and shell file starts with `SPDX-License-Identifier: GPL-3.0-or-later`.
- Commits: `<area>: <what changed>`, signed off (`git commit -s`). User-visible changes go in
  CHANGELOG.md under the next version.
- System definitions live in docs/systems/<name>/. Build or update them using the system-def skill, and lint them with its check.py before committing.
