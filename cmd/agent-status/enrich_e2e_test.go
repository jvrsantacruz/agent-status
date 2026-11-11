// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 Javier Santacruz

//go:build e2e

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jvrsantacruz/agent-status/internal/export"
)

// enrichRig is a state dir whose state.json holds a live session and a dead
// one, a git repository on a ticket branch, the session's transcript, and
// fake gh and git on PATH that log their argv and working directory.
type enrichRig struct {
	t                *testing.T
	dir, repo, calls string
	env              []string
}

func newEnrichRig(t *testing.T, gh string) enrichRig {
	dir := t.TempDir()
	repo := filepath.Join(dir, "api")
	realGit, err := exec.LookPath("git")
	require.NoError(t, err)
	for _, args := range [][]string{{"init", "-q", "-b", "feat/LAB-7-x", repo},
		{"-C", repo, "-c", "user.name=t", "-c", "user.email=t@example.com", "commit", "-q", "--allow-empty", "-m", "x"}} {
		out, err := exec.Command(realGit, args...).CombinedOutput()
		require.NoError(t, err, string(out))
	}

	tr := filepath.Join(dir, "projects", "s1.jsonl")
	copyFile(t, "../../internal/transcript/testdata/s1.jsonl", tr)
	copyFile(t, "../../internal/transcript/testdata/s1/subagents/agent-a1.jsonl", filepath.Join(dir, "projects", "s1", "subagents", "agent-a1.jsonl"))

	calls := filepath.Join(dir, "calls.log")
	bin := filepath.Join(dir, "bin")
	require.NoError(t, os.Mkdir(bin, 0o700))
	for name, body := range map[string]string{
		"git": fmt.Sprintf(`echo "$PWD|git $*" >> %s; exec %s "$@"`, calls, realGit),
		"gh":  fmt.Sprintf(`echo "$PWD|gh $*" >> %s; %s`, calls, gh),
	} {
		require.NoError(t, os.WriteFile(filepath.Join(bin, name), []byte("#!/bin/sh\n"+body+"\n"), 0o700))
	}

	sf := fmt.Sprintf(`{"schema_version":1,"daemon":{},"sessions":{
		"s1":{"harness":"claude","state":"idle","pid":%d,"tmux_pane":"%%1","dead_since":null,"updated_at":1,"message":"","reason":"event:idle","cwd":%q,"transcript_path":%q},
		"gone":{"harness":"claude","state":"dead","pid":0,"tmux_pane":"%%2","dead_since":1,"updated_at":1,"message":"","reason":"liveness:pid_gone","cwd":%q,"transcript_path":%q}}}`,
		os.Getpid(), repo, tr, repo, tr)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "state.json"), []byte(sf), 0o600))

	env := cleanEnv(dir, "AGENT_STATUS_NAMER_COMMAND=cat >/dev/null; echo 'Fix login retry bug'")
	env[1] = "PATH=" + bin + string(os.PathListSeparator) + os.Getenv("PATH")
	return enrichRig{t: t, dir: dir, repo: repo, calls: calls, env: env}
}

func copyFile(t *testing.T, from, to string) {
	data, err := os.ReadFile(from)
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(filepath.Dir(to), 0o700))
	require.NoError(t, os.WriteFile(to, data, 0o600))
}

func (r enrichRig) enrich() map[string]any {
	out, err := command(r.dir, r.env, "enrich").CombinedOutput()
	require.NoError(r.t, err, string(out))
	data, err := os.ReadFile(filepath.Join(r.dir, "meta.json"))
	require.NoError(r.t, err)
	var mf map[string]any
	require.NoError(r.t, json.Unmarshal(data, &mf))
	return mf
}

// req: session-metadata
// req: short-name
// req: read-only-enrich
func TestEnrichWritesMetaAndTheDaemonMergesIt(t *testing.T) {
	r := newEnrichRig(t, `echo '{"number":7,"state":"OPEN"}'`)
	r.enrich()

	data, err := os.ReadFile(filepath.Join(r.dir, "meta.json"))
	require.NoError(t, err)
	golden, err := os.ReadFile("testdata/meta.golden.json")
	require.NoError(t, err)
	assert.JSONEq(t, strings.ReplaceAll(string(golden), "REPO", r.repo), string(data), "the dead session is left out")
	info, err := os.Stat(filepath.Join(r.dir, "meta.json"))
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())

	// Every git and gh call is one of the two read commands; gh runs in the repo.
	logged, err := os.ReadFile(r.calls)
	require.NoError(t, err)
	allowed := map[string]bool{
		"git -C " + r.repo + " rev-parse --show-toplevel --abbrev-ref HEAD": true,
		"gh pr view feat/LAB-7-x --json number,state":                       true,
	}
	lines := strings.Split(strings.TrimSpace(string(logged)), "\n")
	assert.Len(t, lines, 2)
	for _, l := range lines {
		dir, call, _ := strings.Cut(l, "|")
		assert.True(t, allowed[call], "unexpected call %q", call)
		if strings.HasPrefix(call, "gh ") {
			assert.Equal(t, r.repo, dir, "gh runs in the repository")
		}
	}

	daemon := command(r.dir, r.env, "daemon")
	require.NoError(t, daemon.Start())
	t.Cleanup(func() { _ = daemon.Process.Kill() })
	waitReader(filepath.Join(r.dir, "in.fifo"))
	var mf struct {
		Sessions map[string]json.RawMessage `json:"sessions"`
	}
	require.NoError(t, json.Unmarshal(data, &mf))
	require.Eventually(t, func() bool {
		sf, err := export.Read(filepath.Join(r.dir, "state.json"))
		return err == nil && sf.Daemon.PID == daemon.Process.Pid && len(sf.Sessions["s1"].Meta) > 0
	}, exportEvery+5*time.Second, 100*time.Millisecond)
	sf, err := export.Read(filepath.Join(r.dir, "state.json"))
	require.NoError(t, err)
	assert.JSONEq(t, string(mf.Sessions["s1"]), string(sf.Sessions["s1"].Meta))

	// The event history holds no prompt, title or name.
	require.NoError(t, daemon.Process.Signal(syscall.SIGTERM))
	require.NoError(t, daemon.Wait())
	dbs, err := filepath.Glob(filepath.Join(r.dir, "state.db*"))
	require.NoError(t, err)
	require.NotEmpty(t, dbs)
	for _, db := range dbs {
		raw, err := os.ReadFile(db)
		require.NoError(t, err)
		for _, text := range []string{"neutral prompt", "Neutral session title", "Fix login retry"} {
			assert.NotContains(t, string(raw), text, db)
		}
	}
}

// req: read-only-enrich
// A gh that hangs costs the pull request only.
func TestEnrichSlowGhLeavesPROut(t *testing.T) {
	r := newEnrichRig(t, `sleep 30`)
	start := time.Now()
	s1 := r.enrich()["sessions"].(map[string]any)["s1"].(map[string]any)
	assert.Less(t, time.Since(start), 10*time.Second)
	assert.NotContains(t, s1, "pr")
	assert.Equal(t, "feat/LAB-7-x", s1["branch"])
	assert.Equal(t, "Fix login retry bug", s1["name"])
	assert.Equal(t, 1.3483, s1["cost_usd"])
}

// A run while another holds the lock does nothing and exits 0.
func TestEnrichSkipsWhileOneRuns(t *testing.T) {
	r := newEnrichRig(t, `exit 1`)
	lock, err := os.OpenFile(filepath.Join(r.dir, "enrich.lock"), os.O_CREATE|os.O_RDWR, 0o600)
	require.NoError(t, err)
	defer lock.Close()
	require.NoError(t, syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB))
	out, err := command(r.dir, r.env, "enrich").CombinedOutput()
	require.NoError(t, err, string(out))
	assert.NoFileExists(t, filepath.Join(r.dir, "meta.json"))
}
