# agent-status intent

## domain

agent-status shows in the tmux window list what each AI coding agent session is doing, so
with several agents in several windows the one that needs me is visible without visiting
each window. It also knows which pane each session runs in, so a
[tmux-resurrect](https://github.com/tmux-plugins/tmux-resurrect) restore reopens the same
Claude Code or pi conversation. It is not a notifier and sends nothing off the machine.

- **session**: one agent conversation, named by the id the agent gives it, running in one
  pane; it ends with an end event, or when its process exits and the dead keep time passes.
- **harness**: the agent program that runs a session: claude, pi, or any program that
  reports its state.
- **state**: working, question, idle, error, or dead; dead means the session's process is
  gone.

```mermaid
flowchart LR
  me([me]) -- sees which window needs me --> sys[agent-status]
  agent([harness]) -- reports each state change --> sys
  resurrect([tmux-resurrect]) -- saves each pane with its session --> sys
  reader([user's program]) -- reads every session --> sys
```

_The use cases: each arrow is an actor and the goal it has with agent-status._

## requirements

- `window-marks`: The tmux window list MUST mark each window whose active pane holds a session that is working, asking a question, in error or dead.
- `exit-clears`: When a session's process exits, the window list MUST show it dead and then stop showing it.
- `restore-resumes`: When tmux-resurrect restores a pane that held a live claude or pi session, the pane MUST reopen that same session.
- `readable-record`: The system MUST let any program its user runs read the harness, state and pane of every current session.

### non-functional

- `never-blocks`: If the system is down or slow, then reporting a state change MUST NOT block the harness.
- `private-text`: When reading a Claude Code hook's input, the system MUST NOT keep its prompt, tool input or error text.
- `local-only`: The system MUST NOT use the network.

### assumptions

- `id-resume`: The harness MUST reopen a session from its id given on the command line.
- `resurrect-installed`: tmux-resurrect MUST be installed with TPM, saving and restoring panes, and running a save command strategy installed in its plugin directory.
