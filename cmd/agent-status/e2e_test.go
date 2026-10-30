//go:build e2e

package main

import (
	"bufio"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/jvrsantacruz/agent-status/internal/export"
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

// readOne reads one line from a FIFO opened before the writer runs.
func readOne(t *testing.T, fifo string) <-chan string {
	t.Helper()
	require.NoError(t, syscall.Mkfifo(fifo, 0o600))
	ch := make(chan string, 1)
	go func() {
		// A writer that closes without writing, as waitReader does, gives
		// an empty read: reopen and wait for a real line.
		for {
			f, err := os.OpenFile(fifo, os.O_RDONLY, 0)
			if err != nil {
				return
			}
			line, _ := bufio.NewReader(f).ReadString('\n')
			f.Close()
			if line != "" {
				ch <- line
				return
			}
		}
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
	waitReader(filepath.Join(dir, "in.fifo"))

	// What a Claude Code hook sees: tmux sets TMUX_PANE, Claude sets the
	// session and its own pid. tmux does not set a session name.
	env := cleanEnv(dir, "CLAUDE_CODE_SESSION_ID=abc", "CLAUDE_PID=4242", "TMUX_PANE=%7")
	out, err := command(dir, env, "emit", "question", `say "hi"`).CombinedOutput()
	require.NoError(t, err, string(out))

	m := emitted(t, ch)
	assert.Equal(t, "abc", m["session_id"])
	assert.Equal(t, "claude", m["harness"])
	assert.Equal(t, "question", m["event"])
	assert.Equal(t, float64(4242), m["pid"])
	assert.Equal(t, "%7", m["tmux_pane"])
	assert.Equal(t, `say "hi"`, m["message"])
}

func TestEmitFlagsForAnyAgent(t *testing.T) {
	dir := t.TempDir()
	ch := readOne(t, filepath.Join(dir, "in.fifo"))
	waitReader(filepath.Join(dir, "in.fifo"))

	out, err := command(dir, cleanEnv(dir), "emit", "--session", "s9", "--harness", "aider",
		"--pid", "77", "--pane", "%2", "working").CombinedOutput()
	require.NoError(t, err, string(out))

	m := emitted(t, ch)
	assert.Equal(t, "s9", m["session_id"])
	assert.Equal(t, "aider", m["harness"])
	assert.Equal(t, float64(77), m["pid"])
}

func TestEmitWithoutDaemonExitsZero(t *testing.T) {
	dir := t.TempDir()
	out, err := command(dir, cleanEnv(dir, "AGENT_STATUS_SESSION=s"), "emit", "working").CombinedOutput()
	assert.NoError(t, err)
	assert.Contains(t, string(out), "daemon not running")
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
	assert.Regexp(t, `^agent-status \S+ \(\S+, \S+\) go1\.\d+`, string(out))
}

func TestRender(t *testing.T) {
	dir := t.TempDir()
	sf := `{"sessions":{
		"old":{"state":"error","tmux_pane":"%0","updated_at":100},
		"new":{"state":"working","tmux_pane":"%0","updated_at":200},
		"idle":{"state":"idle","tmux_pane":"%1","updated_at":100}}}`
	require.NoError(t, os.WriteFile(filepath.Join(dir, "state.json"), []byte(sf), 0o600))

	cases := map[string]string{
		"%0":      "#{E:@agent_mark_working}", // newest session on the pane wins
		"main:%0": "#{E:@agent_mark_working}", // older theme format
		"%1":      "",
		"%9":      "",
	}
	for pane, want := range cases {
		out, err := command(dir, cleanEnv(dir), "render", pane).Output()
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

func TestDoctorWithoutDaemonFails(t *testing.T) {
	dir := t.TempDir()
	cmd := command(dir, cleanEnv(dir), "doctor")
	out, _ := cmd.CombinedOutput()
	assert.Equal(t, 1, cmd.ProcessState.ExitCode())
	assert.Contains(t, string(out), "FAIL  daemon")
}
