// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 Javier Santacruz

package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"
	"syscall"
	"time"

	"github.com/jvrsantacruz/agent-status/internal/event"
)

// writeWait bounds how long emit waits on a full FIFO.
const writeWait = time.Second

var states = map[string]bool{"working": true, "question": true, "idle": true, "error": true, "done": true, "end": true}

// runEmit never exits 2: a Claude Code hook that exits 2 blocks the agent.
// A daemon that is not running is reported on stderr with exit 0, so the
// agent carries on.
func runEmit(args []string, stdin io.Reader, stderr io.Writer) int {
	fs := flag.NewFlagSet("emit", flag.ContinueOnError)
	fs.SetOutput(stderr)
	session := fs.String("session", firstEnv("AGENT_STATUS_SESSION", "CLAUDE_CODE_SESSION_ID"),
		"session id, unique per agent run; from $AGENT_STATUS_SESSION, then $CLAUDE_CODE_SESSION_ID")
	harness := fs.String("harness", defaultHarness(),
		"name of the agent, such as claude or pi; from $AGENT_STATUS_HARNESS")
	pid := fs.Int("pid", defaultPID(),
		"pid of the agent, the session is dropped when it exits; from $AGENT_STATUS_PID, then $CLAUDE_PID, then the parent process")
	pane := fs.String("pane", os.Getenv("TMUX_PANE"), "tmux pane id of the agent; from $TMUX_PANE")
	hook := fs.Bool("claude-hook", false,
		"read the JSON Claude Code gives a hook on stdin: session id, and a message from the notification text or the tool name")
	judge := fs.Bool("judge", false,
		"with --claude-hook on Stop: a small model reads the turn and sets done, question or error; see agent-status judge")
	fs.Usage = func() {
		fmt.Fprintln(stderr, "usage: agent-status emit [flags] <working|question|idle|error|done|end> [message]")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 1
	}
	if fs.NArg() < 1 || !states[fs.Arg(0)] {
		fs.Usage()
		return 1
	}
	msg := fs.Arg(1)
	var h claudeHook
	if *hook {
		// A hook must never fail the agent: bad input is reported and the
		// event goes out with what the flags and environment give.
		var err error
		h, err = readClaudeHook(stdin)
		if err != nil {
			fmt.Fprintf(stderr, "agent-status emit: %v\n", err)
		}
		if h.SessionID != "" && !flagSet(fs, "session") {
			*session = h.SessionID
		}
		if msg == "" {
			msg = h.message()
		}
		// Only Claude Code runs this hook; a restore resumes by harness name.
		if *harness == "unknown" && !flagSet(fs, "harness") {
			*harness = "claude"
		}
	}
	if *session == "" {
		fmt.Fprintln(stderr, "agent-status emit: no session id: pass --session or set AGENT_STATUS_SESSION")
		return 1
	}
	e := event.Event{
		SessionID:   *session,
		Harness:     *harness,
		Event:       fs.Arg(0),
		PID:         *pid,
		Pane:        *pane,
		TimestampMs: clock().UnixMilli(),
		Message:     msg,
		// An agent states its own outcome; a hook reports what the harness saw.
		Declared: !*hook && (fs.Arg(0) == "done" || fs.Arg(0) == "error"),
	}
	if err := send(e); err != nil {
		fmt.Fprintf(stderr, "agent-status emit: %v\n", err)
		if errors.Is(err, event.ErrTooLong) {
			return 1
		}
		return 0
	}
	if *judge && h.Event == "Stop" && h.Transcript != "" {
		if err := startJudge(e, h.Transcript); err != nil {
			fmt.Fprintf(stderr, "agent-status emit: judge: %v\n", err)
		}
	}
	return 0
}

// send writes one event to the daemon's FIFO without blocking.
func send(e event.Event) error {
	data, err := e.MarshalLine()
	if err != nil {
		return err
	}
	f, err := os.OpenFile(fifoPath(), os.O_WRONLY|syscall.O_NONBLOCK, 0)
	if err != nil {
		if errors.Is(err, syscall.ENXIO) || errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("daemon not running, no reader on %s; run agent-status doctor", fifoPath())
		}
		return fmt.Errorf("open %s: %w", fifoPath(), err)
	}
	defer f.Close()
	// On Linux Go polls the FIFO, and a write to a full FIFO would wait
	// for ever inside an agent hook. On macOS Go cannot poll a FIFO, the
	// write fails at once with EAGAIN and the deadline is not supported.
	_ = f.SetWriteDeadline(clock().Add(writeWait))
	if _, err := f.Write(data); err != nil {
		if errors.Is(err, syscall.EAGAIN) || errors.Is(err, os.ErrDeadlineExceeded) {
			return fmt.Errorf("FIFO %s full, the daemon is not reading; run agent-status doctor", fifoPath())
		}
		return fmt.Errorf("write %s: %w", fifoPath(), err)
	}
	return nil
}

func flagSet(fs *flag.FlagSet, name string) bool {
	set := false
	fs.Visit(func(f *flag.Flag) { set = set || f.Name == name })
	return set
}

func firstEnv(keys ...string) string {
	for _, k := range keys {
		if v := os.Getenv(k); v != "" {
			return v
		}
	}
	return ""
}

func defaultHarness() string {
	if h := os.Getenv("AGENT_STATUS_HARNESS"); h != "" {
		return h
	}
	if os.Getenv("AGENT_STATUS_SESSION") == "" && os.Getenv("CLAUDE_CODE_SESSION_ID") != "" {
		return "claude"
	}
	return "unknown"
}

func defaultPID() int {
	for _, k := range []string{"AGENT_STATUS_PID", "CLAUDE_PID"} {
		if n, err := strconv.Atoi(os.Getenv(k)); err == nil && n > 0 {
			return n
		}
	}
	return os.Getppid()
}
