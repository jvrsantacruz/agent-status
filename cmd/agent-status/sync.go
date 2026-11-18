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
	"github.com/jvrsantacruz/agent-status/internal/state"
)

const syncEvery = time.Second

// runSync loads each window's most urgent marker into the pane option @agent_mark, so
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
		if err := syncPass(); err != nil && !serverAlive() {
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
	sum := sha256.Sum256([]byte(tmuxSocket(tmuxEnv)))
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

// serverAlive tells a server with no session yet, as while it reads its
// config at start, from one that is gone.
func serverAlive() bool {
	return exec.Command("tmux", "show-options", "-gqv", "status").Run() == nil
}

// syncPass returns an error only when tmux cannot be asked for its panes. A
// state.json that cannot be read skips the pass: the next one repairs it.
func syncPass() error {
	out, err := exec.Command("tmux", "list-panes", "-a", "-F", "#{window_id} #{pane_id} #{@agent_mark}").Output()
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

// syncArgs returns one tmux command line that gives every pane its window's
// marker, or nothing when every pane already has it. A window shows the pane
// that most needs me, so the mark does not depend on which pane is active.
// current is list-panes output: window id, pane id and option on each line.
func syncArgs(sf export.StateFile, current string) []string {
	type pane struct{ window, id, have string }
	var panes []pane
	top := map[string]string{}
	for _, line := range strings.Split(strings.TrimSpace(current), "\n") {
		f := strings.SplitN(line, " ", 3)
		if len(f) < 2 || f[1] == "" {
			continue
		}
		p := pane{window: f[0], id: f[1]}
		if len(f) == 3 {
			p.have = f[2]
		}
		panes = append(panes, p)
		if m := marker(sf, p.id); markRank(m) > markRank(top[p.window]) {
			top[p.window] = m
		}
	}
	var args []string
	for _, p := range panes {
		want := top[p.window]
		if want == p.have {
			continue
		}
		if len(args) > 0 {
			args = append(args, ";")
		}
		if want == "" {
			args = append(args, "set-option", "-p", "-u", "-t", p.id, "@agent_mark")
		} else {
			args = append(args, "set-option", "-p", "-t", p.id, "@agent_mark", want)
		}
	}
	return args
}

// markOrder ranks the marks from the one that most needs me; idle has none.
var markOrder = []state.SessionState{state.Error, state.Question, state.Working, state.Done, state.Dead}

func markRank(mark string) int {
	for i, s := range markOrder {
		if mark == fmt.Sprintf("#{E:@agent_mark_%s}", s) {
			return len(markOrder) - i
		}
	}
	return 0
}
