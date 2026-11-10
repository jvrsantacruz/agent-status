// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 Javier Santacruz

package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
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
)

const (
	judgeWait = time.Minute
	// catchUpWait bounds how long judge waits for state.json to hold its stop.
	catchUpWait = 5 * time.Second
	// turnBytes bounds the end of the turn the model reads.
	turnBytes = 6000
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

// judgeCommand is the model call; AGENT_STATUS_JUDGE_COMMAND replaces it,
// as a shell command that reads the prompt on stdin.
func judgeCommand(ctx context.Context) *exec.Cmd {
	if c := os.Getenv("AGENT_STATUS_JUDGE_COMMAND"); c != "" {
		return exec.CommandContext(ctx, "sh", "-c", c)
	}
	// Hooks off, so the call is not a session of its own; no transcript kept.
	return exec.CommandContext(ctx, "claude", "-p", "--model", "haiku",
		"--settings", `{"disableAllHooks":true}`, "--no-session-persistence")
}

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
	transcript := fs.String("transcript", "", "the session's JSONL transcript")
	if err := fs.Parse(args); err != nil || *session == "" || *transcript == "" {
		return 0
	}
	turn, err := lastTurn(*transcript)
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
	ctx, cancel := context.WithTimeout(context.Background(), judgeWait)
	defer cancel()
	cmd := judgeCommand(ctx)
	cmd.Stdin = strings.NewReader(judgePrompt + turn)
	// A Claude session refuses to start inside another one.
	cmd.Env = withoutEnv(os.Environ(), "CLAUDECODE", "CLAUDE_CODE_SESSION_ID", "CLAUDE_PID")
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return verdict(string(out))
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

// showsOutcome tells whether the session's window shows how its turn ended.
func showsOutcome(session string) bool {
	sf, err := export.Read(statePath())
	if err != nil {
		return false
	}
	s := sf.Sessions[session].State
	return s == state.Done || s == state.Error
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

// transcriptLine is the part of a Claude Code transcript line lastTurn reads.
type transcriptLine struct {
	Type    string `json:"type"`
	Message struct {
		Content json.RawMessage `json:"content"`
	} `json:"message"`
}

type contentBlock struct {
	Type    string `json:"type"`
	Text    string `json:"text"`
	Name    string `json:"name"`
	IsError bool   `json:"is_error"`
}

// lastTurn renders the last user prompt and what the agent did after it:
// its text, the tools it called and which of them failed. Tool input and
// output are left out; the end is kept when it is long.
func lastTurn(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	var turn []string
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 16<<20)
	for sc.Scan() {
		var l transcriptLine
		if json.Unmarshal(sc.Bytes(), &l) != nil || (l.Type != "user" && l.Type != "assistant") {
			continue
		}
		var text string
		if json.Unmarshal(l.Message.Content, &text) == nil {
			if l.Type == "user" {
				turn = []string{"USER: " + text}
			}
			continue
		}
		var blocks []contentBlock
		if json.Unmarshal(l.Message.Content, &blocks) != nil {
			continue
		}
		for _, b := range blocks {
			switch {
			case b.Type == "text" && l.Type == "user":
				turn = []string{"USER: " + b.Text}
			case b.Type == "text":
				turn = append(turn, "AGENT: "+b.Text)
			case b.Type == "tool_use":
				turn = append(turn, "TOOL: "+b.Name)
			case b.Type == "tool_result" && b.IsError:
				turn = append(turn, "TOOL FAILED")
			}
		}
	}
	if err := sc.Err(); err != nil {
		return "", err
	}
	// A turn ends with the agent's message; anything else is still running.
	if len(turn) == 0 || !strings.HasPrefix(turn[len(turn)-1], "AGENT: ") {
		return "", nil
	}
	out := strings.Join(turn, "\n")
	if len(out) > turnBytes {
		out = fmt.Sprintf("...%s", out[len(out)-turnBytes:])
	}
	return out, nil
}
