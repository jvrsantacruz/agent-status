package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"
	"syscall"

	"github.com/jvrsantacruz/agent-status/internal/event"
)

// maxMessage keeps one event line under PIPE_BUF (4096), so concurrent
// writers never interleave on the FIFO.
const maxMessage = 1024

var states = map[string]bool{"working": true, "question": true, "idle": true, "error": true, "end": true}

// runEmit never exits 2: a Claude Code hook that exits 2 blocks the agent.
// A daemon that is not running is reported on stderr with exit 0, so the
// agent carries on.
func runEmit(args []string, stderr io.Writer) int {
	fs := flag.NewFlagSet("emit", flag.ContinueOnError)
	fs.SetOutput(stderr)
	session := fs.String("session", firstEnv("AGENT_STATUS_SESSION", "CLAUDE_CODE_SESSION_ID"),
		"session id, unique per agent run; from $AGENT_STATUS_SESSION, then $CLAUDE_CODE_SESSION_ID")
	harness := fs.String("harness", defaultHarness(),
		"name of the agent, such as claude or pi; from $AGENT_STATUS_HARNESS")
	pid := fs.Int("pid", defaultPID(),
		"pid of the agent, the session is dropped when it exits; from $AGENT_STATUS_PID, then $CLAUDE_PID, then the parent process")
	pane := fs.String("pane", os.Getenv("TMUX_PANE"), "tmux pane id of the agent; from $TMUX_PANE")
	fs.Usage = func() {
		fmt.Fprintln(stderr, "usage: agent-status emit [flags] <working|question|idle|error|end> [message]")
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
	if *session == "" {
		fmt.Fprintln(stderr, "agent-status emit: no session id: pass --session or set AGENT_STATUS_SESSION")
		return 1
	}
	msg := fs.Arg(1)
	if len(msg) > maxMessage {
		msg = msg[:maxMessage]
	}
	e := event.Event{
		SessionID:   *session,
		Harness:     *harness,
		Event:       fs.Arg(0),
		PID:         *pid,
		Pane:        *pane,
		TimestampMs: clock().UnixMilli(),
		Message:     msg,
	}
	if err := send(e); err != nil {
		fmt.Fprintf(stderr, "agent-status emit: %v\n", err)
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
	if _, err := f.Write(data); err != nil {
		return fmt.Errorf("write %s: %w", fifoPath(), err)
	}
	return nil
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
