package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"syscall"
)

func runEmit(args []string) {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "usage: watcher emit <state> [message]")
		os.Exit(1)
	}
	state := args[0]
	message := ""
	if len(args) > 1 {
		message = args[1]
	}

	e := eventLine{
		SessionID:   os.Getenv("CLAUDE_CODE_SESSION_ID"),
		Harness:     "claude",
		Event:       state,
		PID:         atoiOr(os.Getenv("PPID")),
		TmuxTarget:  os.Getenv("TMUX_SESSION_NAME") + ":" + os.Getenv("TMUX_PANE"),
		TimestampMs: clock().UnixMilli(),
		Message:     message,
	}

	fifo := emitRuntimeDir() + "/in.fifo"
	data, _ := json.Marshal(e)
	data = append(data, '\n')

	f, err := os.OpenFile(fifo, os.O_WRONLY|syscall.O_NONBLOCK, 0)
	if err != nil {
		fmt.Fprintf(os.Stderr, "emit fifo open err: %v\n", err)
		return
	}
	f.Write(data)
	f.Close()
}

type eventLine struct {
	SessionID   string `json:"session_id"`
	Harness     string `json:"harness"`
	Event       string `json:"event"`
	PID         int    `json:"pid"`
	TmuxTarget  string `json:"tmux_target"`
	TimestampMs int64  `json:"ts"`
	Message     string `json:"message"`
}

func emitRuntimeDir() string {
	if d := os.Getenv("AGENT_STATUS_RUNTIME_DIR"); d != "" {
		return d
	}
	return defaultRuntimeDir()
}

func atoiOr(s string) int {
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0
	}
	return n
}
