// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 Javier Santacruz

//go:build e2e

package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/jvrsantacruz/agent-status/internal/export"
	"github.com/jvrsantacruz/agent-status/internal/state"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var bin string

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "agent-status-e2e")
	if err != nil {
		panic(err)
	}
	bin = filepath.Join(dir, "agent-status")
	if out, err := exec.Command("go", "build", "-o", bin, ".").CombinedOutput(); err != nil {
		panic(string(out))
	}
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

// cleanEnv is the environment of a stranger's shell: no agent, no tmux, and
// every agent-status path inside dir.
func cleanEnv(dir string, extra ...string) []string {
	env := []string{"HOME=" + dir, "PATH=" + os.Getenv("PATH"),
		"AGENT_STATUS_STATE_DIR=" + dir, "AGENT_STATUS_RUNTIME_DIR=" + dir}
	return append(env, extra...)
}

func command(dir string, env []string, args ...string) *exec.Cmd {
	cmd := exec.Command(bin, args...)
	cmd.Env = env
	cmd.Dir = dir
	return cmd
}

// readOne reads one line from a FIFO. It opens the FIFO read-write before it
// returns, as the daemon does, so a writer always finds a reader.
func readOne(t *testing.T, fifo string) <-chan string {
	t.Helper()
	require.NoError(t, syscall.Mkfifo(fifo, 0o600))
	f, err := os.OpenFile(fifo, os.O_RDWR, 0)
	require.NoError(t, err)
	t.Cleanup(func() { f.Close() })
	ch := make(chan string, 1)
	go func() {
		line, _ := bufio.NewReader(f).ReadString('\n')
		ch <- line
	}()
	return ch
}

func emitted(t *testing.T, ch <-chan string) map[string]any {
	t.Helper()
	select {
	case line := <-ch:
		var m map[string]any
		require.NoError(t, json.Unmarshal([]byte(line), &m), line)
		return m
	case <-time.After(3 * time.Second):
		t.Fatal("timeout: nothing on the FIFO")
		return nil
	}
}

func waitReader(fifo string) {
	for range 100 {
		if f, err := os.OpenFile(fifo, os.O_WRONLY|syscall.O_NONBLOCK, 0); err == nil {
			f.Close()
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestEmitFromClaudeHookEnv(t *testing.T) {
	dir := t.TempDir()
	ch := readOne(t, filepath.Join(dir, "in.fifo"))

	// What a Claude Code hook sees: tmux sets TMUX_PANE, Claude sets the
	// session and its own pid. tmux does not set a session name.
	env := cleanEnv(dir, "CLAUDE_CODE_SESSION_ID=abc", "CLAUDE_PID=4242", "TMUX_PANE=%7")
	out, err := command(dir, env, "emit", "question", `say "hi"`).CombinedOutput()
	require.NoError(t, err, string(out))
	require.Empty(t, string(out))

	m := emitted(t, ch)
	assert.Equal(t, "abc", m["session_id"])
	assert.Equal(t, "claude", m["harness"])
	assert.Equal(t, "question", m["event"])
	assert.Equal(t, float64(4242), m["pid"])
	assert.Equal(t, "%7", m["tmux_pane"])
	assert.Equal(t, `say "hi"`, m["message"])
}

// req: restore-resumes
// A hook without Claude's environment still names its harness, so a restore
// can resume the session.
func TestEmitClaudeHookNamesHarness(t *testing.T) {
	dir := t.TempDir()
	ch := readOne(t, filepath.Join(dir, "in.fifo"))
	cmd := command(dir, cleanEnv(dir), "emit", "--claude-hook", "working")
	cmd.Stdin = strings.NewReader(`{"session_id":"s1","hook_event_name":"PreToolUse","tool_name":"Bash"}`)
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, string(out))

	m := emitted(t, ch)
	assert.Equal(t, "s1", m["session_id"])
	assert.Equal(t, "claude", m["harness"])
}

func TestEmitFlagsForAnyAgent(t *testing.T) {
	dir := t.TempDir()
	ch := readOne(t, filepath.Join(dir, "in.fifo"))

	out, err := command(dir, cleanEnv(dir), "emit", "--session", "s9", "--harness", "aider",
		"--pid", "77", "--pane", "%2", "working").CombinedOutput()
	require.NoError(t, err, string(out))
	require.Empty(t, string(out))

	m := emitted(t, ch)
	assert.Equal(t, "s9", m["session_id"])
	assert.Equal(t, "aider", m["harness"])
	assert.Equal(t, float64(77), m["pid"])
}

// req: never-blocks
func TestEmitWithoutDaemonExitsZero(t *testing.T) {
	dir := t.TempDir()
	out, err := command(dir, cleanEnv(dir, "AGENT_STATUS_SESSION=s"), "emit", "working").CombinedOutput()
	assert.NoError(t, err)
	assert.Contains(t, string(out), "daemon not running")
}

// req: never-blocks
func TestEmitWithFullFIFOReturns(t *testing.T) {
	dir := t.TempDir()
	fifo := filepath.Join(dir, "in.fifo")
	require.NoError(t, syscall.Mkfifo(fifo, 0o600))
	// A daemon that holds the FIFO open and never reads it. Raw calls, so
	// the fill stops at EAGAIN; Go's poller would wait for room on Linux.
	fd, err := syscall.Open(fifo, syscall.O_RDWR|syscall.O_NONBLOCK, 0)
	require.NoError(t, err)
	t.Cleanup(func() { _ = syscall.Close(fd) })
	for {
		if _, err := syscall.Write(fd, make([]byte, 512)); err != nil {
			require.ErrorIs(t, err, syscall.EAGAIN)
			break
		}
	}

	start := time.Now()
	out, err := command(dir, cleanEnv(dir, "AGENT_STATUS_SESSION=s"), "emit", "working").CombinedOutput()
	assert.NoError(t, err)
	assert.Contains(t, string(out), "full")
	assert.Less(t, time.Since(start), 2*time.Second)
}

func TestEmitUsageErrorsExitOne(t *testing.T) {
	dir := t.TempDir()
	for _, args := range [][]string{{"emit"}, {"emit", "bogus"}, {"emit", "working"}} {
		cmd := command(dir, cleanEnv(dir), args...)
		out, err := cmd.CombinedOutput()
		require.Error(t, err, "%v", args)
		assert.Equal(t, 1, cmd.ProcessState.ExitCode(), "%v: never 2, which blocks a Claude hook", args)
		assert.NotEmpty(t, string(out))
	}
}

func TestUnknownCommandIsUsageError(t *testing.T) {
	dir := t.TempDir()
	for _, args := range [][]string{{}, {"--frobnicate"}} {
		cmd := command(dir, cleanEnv(dir), args...)
		out, _ := cmd.CombinedOutput()
		assert.Equal(t, 2, cmd.ProcessState.ExitCode(), "%v", args)
		assert.Contains(t, string(out), "usage: agent-status")
	}
}

func TestVersion(t *testing.T) {
	out, err := exec.Command(bin, "--version").Output()
	require.NoError(t, err)
	assert.Regexp(t, `^agent-status \S+ (\([0-9a-f]+(\+dirty)?, \S+\) )?go1\.\d+`, string(out))
	assert.NotContains(t, string(out), "unknown")
}

func TestRender(t *testing.T) {
	dir := t.TempDir()
	sf := `{"sessions":{
		"old":{"state":"error","tmux_pane":"%0","updated_at":100},
		"new":{"state":"working","tmux_pane":"%0","updated_at":200},
		"idle":{"state":"idle","tmux_pane":"%1","updated_at":100},
		"exited":{"state":"dead","tmux_pane":"%2","updated_at":300},
		"next":{"state":"question","tmux_pane":"%2","updated_at":200},
		"gone":{"state":"dead","tmux_pane":"%3","updated_at":100}}}`
	require.NoError(t, os.WriteFile(filepath.Join(dir, "state.json"), []byte(sf), 0o600))

	cases := map[string]string{
		"%0":      "#{E:@agent_mark_working}", // newest session on the pane wins
		"main:%0": "#{E:@agent_mark_working}", // older theme format
		"%1":      "",
		"%2":      "#{E:@agent_mark_question}", // req: window-marks, a live session wins over one that died later
		"%3":      "#{E:@agent_mark_dead}",     // req: exit-clears
		"%9":      "",
	}
	for pane, want := range cases {
		out, err := command(dir, cleanEnv(dir), "render", pane).Output()
		require.NoError(t, err)
		assert.Equal(t, want, string(out), pane)
	}
}

// req: restore-resumes
func TestSession(t *testing.T) {
	dir := t.TempDir()
	sf := `{"sessions":{
		"old":{"harness":"claude","state":"idle","tmux_pane":"%0","updated_at":100},
		"new":{"harness":"claude","state":"working","tmux_pane":"%0","updated_at":200},
		"p1":{"harness":"pi","state":"idle","tmux_pane":"%1","updated_at":100},
		"exited":{"harness":"pi","state":"idle","pid":2147483646,"tmux_pane":"%1","updated_at":200},
		"gone":{"harness":"claude","state":"dead","tmux_pane":"%2","updated_at":100},
		"outside":{"harness":"claude","state":"idle","tmux_pane":"","updated_at":300}}}`
	require.NoError(t, os.WriteFile(filepath.Join(dir, "state.json"), []byte(sf), 0o600))

	cases := map[string]string{
		"%0": "claude new\n", // newest session on the pane wins
		"%1": "pi p1\n",      // a newer session whose process exited is skipped
		"%2": "",             // a dead session is not resumed
		"%9": "",
		"":   "", // no pane: never a session sent from outside tmux
	}
	for pane, want := range cases {
		out, err := command(dir, cleanEnv(dir), "session", pane).Output()
		require.NoError(t, err)
		assert.Equal(t, want, string(out), pane)
	}
}

func TestRenderWithoutStateIsSilent(t *testing.T) {
	dir := t.TempDir()
	out, err := command(dir, cleanEnv(dir), "render", "%0").CombinedOutput()
	assert.NoError(t, err)
	assert.Empty(t, string(out))
}

func TestDaemonDoctorRoundTrip(t *testing.T) {
	dir := t.TempDir()
	env := cleanEnv(dir, "AGENT_STATUS_LOG=debug")
	daemon := command(dir, env, "daemon")
	var log strings.Builder
	daemon.Stderr = &log
	require.NoError(t, daemon.Start())
	t.Cleanup(func() { daemon.Process.Kill() })
	waitReader(filepath.Join(dir, "in.fifo"))

	// An agent event without a pid, as the old emit sent: doctor flags it.
	out, err := command(dir, cleanEnv(dir, "TMUX_PANE=%5"), "emit", "--session", "nopid", "--pid", "0", "working").CombinedOutput()
	require.NoError(t, err, string(out))

	doctor := command(dir, env, "doctor")
	report, err := doctor.CombinedOutput()
	require.NoError(t, err, string(report))
	assert.Contains(t, string(report), "ok    round trip")
	assert.Contains(t, string(report), `ok    render        marker "#{E:@agent_mark_question}"`)
	assert.Contains(t, string(report), "nopid  harness=unknown state=working pid=0 pane=%5 reason=event:working")
	assert.Contains(t, string(report), "warn  pid")

	sf, err := export.Read(filepath.Join(dir, "state.json"))
	require.NoError(t, err)
	assert.NotContains(t, sf.Sessions, "doctor-"+strconv.Itoa(doctor.Process.Pid), "doctor removes its test session")
	assert.Positive(t, sf.Daemon.EventsTotal)

	info, err := os.Stat(filepath.Join(dir, "state.json"))
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())

	require.NoError(t, daemon.Process.Signal(syscall.SIGTERM))
	require.NoError(t, daemon.Wait(), "daemon exits 0 on SIGTERM")
	assert.Contains(t, log.String(), `"msg":"event"`)
	assert.Contains(t, log.String(), `"known_event":true`)
}

// req: exit-clears
// A restarted daemon keeps the sessions still running and marks dead the ones
// whose process exited while it was down.
func TestDaemonRestartKeepsLiveSessions(t *testing.T) {
	dir := t.TempDir()
	prev := fmt.Sprintf(`{"schema_version":1,"sessions":{
		"alive":{"harness":"claude","state":"idle","pid":%d,"tmux_pane":"%%1","updated_at":1},
		"exited":{"harness":"claude","state":"working","pid":2147483646,"tmux_pane":"%%2","updated_at":1}}}`, os.Getpid())
	require.NoError(t, os.WriteFile(filepath.Join(dir, "state.json"), []byte(prev), 0o600))

	daemon := command(dir, cleanEnv(dir), "daemon")
	require.NoError(t, daemon.Start())
	t.Cleanup(func() { daemon.Process.Kill() })
	waitReader(filepath.Join(dir, "in.fifo"))

	require.Eventually(t, func() bool {
		sf, err := export.Read(filepath.Join(dir, "state.json"))
		return err == nil && sf.Daemon.PID == daemon.Process.Pid
	}, 15*time.Second, 100*time.Millisecond)
	sf, err := export.Read(filepath.Join(dir, "state.json"))
	require.NoError(t, err)
	assert.Equal(t, state.Idle, sf.Sessions["alive"].State)
	assert.Equal(t, state.Dead, sf.Sessions["exited"].State)
	out, err := command(dir, cleanEnv(dir), "session", "%2").Output()
	require.NoError(t, err)
	assert.Empty(t, string(out))
}

func TestDoctorWithoutDaemonFails(t *testing.T) {
	dir := t.TempDir()
	cmd := command(dir, cleanEnv(dir), "doctor")
	out, _ := cmd.CombinedOutput()
	assert.Equal(t, 1, cmd.ProcessState.ExitCode())
	assert.Contains(t, string(out), "FAIL  daemon")
}

// Lines longer than PIPE_BUF (512 on macOS) can be split and interleave with
// another writer's line, and both events are lost. emit keeps every line
// within 512 bytes, so concurrent agents never corrupt each other.
func TestConcurrentEmitsWithLongMessages(t *testing.T) {
	dir := t.TempDir()
	env := cleanEnv(dir)
	daemon := command(dir, env, "daemon")
	require.NoError(t, daemon.Start())
	t.Cleanup(func() { daemon.Process.Kill() })
	waitReader(filepath.Join(dir, "in.fifo"))

	const writers = 40
	msg := strings.Repeat(`long "quoted" <text> ñ😀 `, 100)
	var wg sync.WaitGroup
	for i := range writers {
		wg.Go(func() {
			out, err := command(dir, env, "emit", "--session", "s"+strconv.Itoa(i),
				"--pid", strconv.Itoa(os.Getpid()), "--pane", "%1", "working", msg).CombinedOutput()
			assert.NoError(t, err, string(out))
			assert.Empty(t, string(out))
		})
	}
	wg.Wait()

	var sf export.StateFile
	assert.Eventually(t, func() bool {
		var err error
		sf, err = export.Read(filepath.Join(dir, "state.json"))
		return err == nil && sf.Daemon.EventsTotal == writers
	}, 3*time.Second, 50*time.Millisecond)
	assert.Zero(t, sf.Daemon.EventsRejected)
	assert.Len(t, sf.Sessions, writers)
	for _, s := range sf.Sessions {
		assert.True(t, strings.HasSuffix(s.Message, "..."))
	}
}

func TestEmitTooLongSessionExitsOne(t *testing.T) {
	dir := t.TempDir()
	cmd := command(dir, cleanEnv(dir), "emit", "--session", strings.Repeat("s", 600), "working")
	out, _ := cmd.CombinedOutput()
	assert.Equal(t, 1, cmd.ProcessState.ExitCode())
	assert.Contains(t, string(out), "longer than 512 bytes")
}

func TestEmitClaudeHook(t *testing.T) {
	cases := []struct {
		name, stdin, wantSession, wantMessage string
		args                                  []string
		wantStderr                            string
	}{
		{"session and message from stdin",
			`{"session_id":"from-stdin","hook_event_name":"Notification","message":"Claude needs your permission"}`,
			"from-stdin", "Claude needs your permission", []string{"question"}, ""},
		{"--session and the message argument win",
			`{"session_id":"from-stdin","hook_event_name":"PreToolUse","tool_name":"Bash"}`,
			"from-flag", "my text", []string{"--session", "from-flag", "working", "my text"}, ""},
		{"bad input still emits, from the environment",
			`not json`, "from-env", "", []string{"working"}, "parse hook input"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			dir := t.TempDir()
			ch := readOne(t, filepath.Join(dir, "in.fifo"))
			args := append([]string{"emit", "--claude-hook"}, c.args...)
			cmd := command(dir, cleanEnv(dir, "CLAUDE_CODE_SESSION_ID=from-env", "CLAUDE_PID=7"), args...)
			cmd.Stdin = strings.NewReader(c.stdin)
			out, err := cmd.CombinedOutput()
			require.NoError(t, err, string(out))
			if c.wantStderr == "" {
				assert.Empty(t, string(out))
			} else {
				assert.Contains(t, string(out), c.wantStderr)
			}
			m := emitted(t, ch)
			assert.Equal(t, c.wantSession, m["session_id"])
			assert.Equal(t, c.wantMessage, m["message"])
			assert.Equal(t, "claude", m["harness"])
		})
	}
}

// A real tmux server on a private socket: sync loads each pane's marker into
// @agent_mark, so the window list reads it and runs no job.
func TestSyncLoadsMarkersIntoTmux(t *testing.T) {
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux not installed")
	}
	dir := t.TempDir()
	// A socket path must fit in 104 bytes; a temp dir on macOS may not.
	sockDir, err := os.MkdirTemp("/tmp", "as")
	require.NoError(t, err)
	t.Cleanup(func() { os.RemoveAll(sockDir) })
	sock := filepath.Join(sockDir, "s")
	tmux := func(args ...string) string {
		out, err := exec.Command("tmux", append([]string{"-S", sock, "-f", "/dev/null"}, args...)...).CombinedOutput()
		require.NoError(t, err, string(out))
		return strings.TrimSpace(string(out))
	}
	tmux("new-session", "-d", "-s", "t")
	t.Cleanup(func() { _ = exec.Command("tmux", "-S", sock, "kill-server").Run() })
	pane := tmux("list-panes", "-F", "#{pane_id}")
	env := cleanEnv(dir, "TMUX="+sock+",1,0")

	write := func(st state.SessionState) {
		require.NoError(t, export.Write(filepath.Join(dir, "state.json"), export.StateFile{
			SchemaVersion: export.SchemaVersion,
			Sessions:      map[string]state.Session{"s": {Harness: "claude", State: st, Pane: pane}},
		}))
	}
	mark := func() string { return tmux("display-message", "-p", "-t", pane, "#{@agent_mark}") }

	write(state.Working)
	out, err := command(dir, env, "sync", "--once").CombinedOutput()
	require.NoError(t, err, string(out))
	assert.Equal(t, "#{E:@agent_mark_working}", mark())

	write(state.Idle)
	require.NoError(t, command(dir, env, "sync", "--once").Run())
	assert.Empty(t, mark(), "idle unsets the marker")

	// The loop: one per server; it follows changes and exits with the server.
	loop := command(dir, env, "sync")
	require.NoError(t, loop.Start())
	t.Cleanup(func() { loop.Process.Kill() })
	write(state.Question)
	assert.Eventually(t, func() bool { return mark() == "#{E:@agent_mark_question}" }, 3*time.Second, 50*time.Millisecond)
	second := command(dir, env, "sync")
	require.NoError(t, second.Run(), "a second sync for the same server exits 0 at once")

	tmux("kill-server")
	done := make(chan error, 1)
	go func() { done <- loop.Wait() }()
	select {
	case err := <-done:
		assert.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("sync did not exit after the tmux server was gone")
	}
}
