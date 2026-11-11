// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 Javier Santacruz

package main

import (
	"context"
	"flag"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/jvrsantacruz/agent-status/internal/event"
	"github.com/jvrsantacruz/agent-status/internal/export"
	"github.com/jvrsantacruz/agent-status/internal/state"
	"github.com/jvrsantacruz/agent-status/internal/transcript"
)

const (
	modelWait = time.Minute
	// catchUpWait bounds how long judge waits for state.json to hold its stop.
	catchUpWait = 5 * time.Second
)

const judgePrompt = `You read the end of a coding agent's turn and say how it ended, from its
last message. Answer with exactly one word:
done      the last message says the whole task is complete, with nothing pending
question  the last message asks the user something or waits for the user's decision
error     the last message reports a failure the agent could not get past, an API
          error, or that it gave up
idle      anything else: work still running or promised later, a partial step, a
          status update, a reply to small talk
When unsure, answer idle.

The turn:
`

// startJudge runs judge detached from the hook, so the hook returns at once
// and the agent never waits on the model.
func startJudge(e event.Event, transcript string) error {
	self, err := os.Executable()
	if err != nil {
		return err
	}
	cmd := exec.Command(self, "judge", "--session", e.SessionID, "--harness", e.Harness,
		"--pid", strconv.Itoa(e.PID), "--pane", e.Pane,
		"--since", strconv.FormatInt(e.TimestampMs, 10), "--transcript", transcript)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
}

// runJudge prints nothing and exits 0 on any failure: it runs detached, and
// a turn it cannot judge stays idle.
func runJudge(args []string, stderr io.Writer) int {
	fs := flag.NewFlagSet("judge", flag.ContinueOnError)
	fs.SetOutput(stderr)
	session := fs.String("session", "", "session id")
	harness := fs.String("harness", "claude", "agent name")
	pid := fs.Int("pid", 0, "agent pid")
	pane := fs.String("pane", "", "tmux pane id")
	since := fs.Int64("since", 0, "timestamp of the idle event that ended the turn, in ms")
	transcriptPath := fs.String("transcript", "", "the session's JSONL transcript")
	if err := fs.Parse(args); err != nil || *session == "" || *transcriptPath == "" {
		return 0
	}
	turn, err := transcript.LastTurn(*transcriptPath)
	if err != nil || turn == "" {
		return 0
	}
	verdict := ask(turn)
	if verdict == "" || verdict == state.Idle || !stillEndedBy(*session, *since) {
		return 0
	}
	_ = send(event.Event{SessionID: *session, Harness: *harness, Event: string(verdict),
		PID: *pid, Pane: *pane, TimestampMs: clock().UnixMilli(), Message: "judged"})
	return 0
}

// ask returns the model's verdict, or "" when it gives none of the four.
func ask(turn string) state.SessionState {
	out, err := askModel(context.Background(), "AGENT_STATUS_JUDGE_COMMAND", judgePrompt+turn)
	if err != nil {
		return ""
	}
	return verdict(out)
}

// askModel sends prompt to the model and returns its answer. envVar names a
// shell command that replaces the model call and reads the prompt on stdin.
func askModel(ctx context.Context, envVar, prompt string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, modelWait)
	defer cancel()
	var cmd *exec.Cmd
	if c := os.Getenv(envVar); c != "" {
		cmd = exec.CommandContext(ctx, "sh", "-c", c)
	} else {
		// Hooks off, so the call is not a session of its own; no transcript kept.
		cmd = exec.CommandContext(ctx, "claude", "-p", "--model", "haiku",
			"--settings", `{"disableAllHooks":true}`, "--no-session-persistence")
	}
	cmd.Stdin = strings.NewReader(prompt)
	// A Claude session refuses to start inside another one.
	cmd.Env = withoutEnv(os.Environ(), "CLAUDECODE", "CLAUDE_CODE_SESSION_ID", "CLAUDE_PID")
	cmd.WaitDelay = time.Second
	out, err := cmd.Output()
	return string(out), err
}

func verdict(out string) state.SessionState {
	word := strings.ToLower(strings.Trim(strings.TrimSpace(out), ".*`'\" "))
	if f := strings.Fields(word); len(f) > 0 {
		word = f[0]
	}
	switch v := state.SessionState(word); v {
	case state.Done, state.Question, state.Error, state.Idle:
		return v
	}
	return ""
}

// stillEndedBy tells whether the session's state is still the idle event
// that started this judge: no prompt since, and nothing the agent declared.
// It first waits for state.json to hold that event, which the daemon may
// not have written yet.
func stillEndedBy(session string, since int64) bool {
	deadline := clock().Add(catchUpWait)
	for {
		sf, err := export.Read(statePath())
		if err == nil {
			if s, ok := sf.Sessions[session]; ok && s.UpdatedAt >= since {
				return s.State == state.Idle && s.UpdatedAt == since
			}
		}
		if clock().After(deadline) {
			return false
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func withoutEnv(env []string, keys ...string) []string {
	out := env[:0:0]
	for _, kv := range env {
		k, _, _ := strings.Cut(kv, "=")
		drop := false
		for _, d := range keys {
			drop = drop || k == d
		}
		if !drop {
			out = append(out, kv)
		}
	}
	return out
}
