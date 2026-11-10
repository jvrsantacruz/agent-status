# agent-status intent

## domain

agent-status shows in the tmux window list what each AI coding agent session is doing, so
with several agents the one that needs me is visible without visiting each window. Where I turn resurrect integration on, a
[tmux-resurrect](https://github.com/tmux-plugins/tmux-resurrect) restore reopens the same
Claude Code or pi conversation in its pane. A session can declare its turn done or failed; where I turn the judge
on, a small model reads how a turn ended. It is not a notifier; only the judge's model call
leaves the machine.

- **session**: one agent conversation, named by the id the agent gives it, running in one
  pane; it ends with an end event, or when its process exits and the dead keep time passes.
- **harness**: the agent program that runs a session: claude, pi, or any program that
  reports its state.
- **turn**: one prompt and the agent's work on it, ending when the harness stops.

```mermaid
flowchart LR
  me([me]) -- sees which window needs me --> sys[agent-status]
  agent([harness]) -- reports each state change --> sys
  resurrect([tmux-resurrect]) -- saves each pane with its session --> sys
  reader([user's programs]) -- reads every session --> sys
  sys -- the turn from the Claude Code transcript --> claude[claude, small model]
```

_The use cases: each arrow into agent-status is an actor and its goal; the arrow out is the model the judge asks._

## requirements

- `window-marks`: The tmux window list MUST mark each window whose active pane holds a session that is working, asking a question, in error or done.
- `turn-outcome`: When a turn ends, its window MUST show until the next prompt a harness error, else the session's declared done or error, else, where the judge is on for Claude Code, a small model's reading.
- `exit-clears`: If a session's process exits without an end event, then the window whose active pane held it MUST show it dead until the dead keep time passes or a live session takes that pane.
- `restore-resumes`: Where resurrect integration is on, when tmux-resurrect restores a pane that held a live claude or pi session, the pane MUST reopen that same session.
- `readable-record`: The system MUST let any program its user runs read the harness, state and pane of every current session.

### non-functional

- `never-blocks`: If the system is down or slow, then reporting a state change MUST NOT block the harness.
- `private-text`: When reading a Claude Code hook's input or transcript, the system MUST NOT keep its prompt, tool input or error details.
- `local-only`: Except the judge's model call, the system MUST NOT use the network.

### assumptions

- `resurrect-strategy`: tmux-resurrect MUST run the save command strategy named in `@resurrect-save-command-strategy` from its own strategy directory, and save what it prints as the pane's command.
