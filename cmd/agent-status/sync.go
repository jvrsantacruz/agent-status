// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 Javier Santacruz

package main

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/jvrsantacruz/agent-status/internal/export"
)

const syncEvery = time.Second

// runSync loads each pane's marker into the tmux pane option @agent_mark, so
// the window list reads it and runs no job. It reads state.json once a pass
// and sets only what changed, so an unchanged state never redraws the status
// line. One runs per tmux server; it exits when the server is gone.
func runSync(args []string, stderr io.Writer) int {
	tmuxEnv := os.Getenv("TMUX")
	if tmuxEnv == "" {
		fmt.Fprintln(stderr, "agent-status sync: not inside tmux; the plugin starts it")
		return 2
	}
	once := len(args) > 0 && args[0] == "--once"
	if !once {
		lock, err := lockServer(tmuxEnv)
		if err != nil {
			// Another sync already serves this server.
			return 0
		}
		defer lock.Close()
	}
	for {
		if err := syncPass(); err != nil {
			// The server is gone, or tmux cannot be run: nothing left to serve.
			return 0
		}
		if once {
			return 0
		}
		time.Sleep(syncEvery)
	}
}

// lockServer holds a lock per tmux server, named from its socket, for as
// long as this process lives.
func lockServer(tmuxEnv string) (*os.File, error) {
	socket, _, _ := strings.Cut(tmuxEnv, ",")
	sum := sha256.Sum256([]byte(socket))
	path := filepath.Join(runtimeDir(), "sync-"+hex.EncodeToString(sum[:8])+".lock")
	if err := os.MkdirAll(runtimeDir(), 0o700); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		f.Close()
		return nil, err
	}
	return f, nil
}

// syncPass returns an error only when tmux cannot be asked for its panes. A
// state.json that cannot be read skips the pass: the next one repairs it.
func syncPass() error {
	out, err := exec.Command("tmux", "list-panes", "-a", "-F", "#{pane_id} #{@agent_mark}").Output()
	if err != nil {
		return err
	}
	sf, err := export.Read(statePath())
	if errors.Is(err, os.ErrNotExist) {
		sf = export.StateFile{}
	} else if err != nil {
		return nil
	}
	args := syncArgs(sf, string(out))
	if len(args) == 0 {
		return nil
	}
	// A pane closed between the two calls fails its set-option; the rest apply.
	_ = exec.Command("tmux", args...).Run()
	return nil
}

// syncArgs returns one tmux command line that brings every pane's
// @agent_mark to its marker, or nothing when every pane already has it.
// current is list-panes output: a pane id and its option on each line.
func syncArgs(sf export.StateFile, current string) []string {
	var args []string
	for _, line := range strings.Split(strings.TrimSpace(current), "\n") {
		pane, have, _ := strings.Cut(line, " ")
		if pane == "" {
			continue
		}
		want := marker(sf, pane)
		if want == have {
			continue
		}
		if len(args) > 0 {
			args = append(args, ";")
		}
		if want == "" {
			args = append(args, "set-option", "-p", "-u", "-t", pane, "@agent_mark")
		} else {
			args = append(args, "set-option", "-p", "-t", pane, "@agent_mark", want)
		}
	}
	return args
}
