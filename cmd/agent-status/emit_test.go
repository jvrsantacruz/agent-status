//go:build e2e

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func buildBinary(t *testing.T) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "watcher")
	cmd := exec.Command("go", "build", "-o", bin, ".")
	cmd.Dir = "."
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "build failed: %s", string(out))
	return bin
}

func TestEmitWritesToFifo(t *testing.T) {
	dir := t.TempDir()
	fifo := dir + "/in.fifo"
	require.NoError(t, syscall.Mkfifo(fifo, 0666))

	// Start FIFO reader
	ch := make(chan string)
	go func() {
		f, err := os.OpenFile(fifo, os.O_RDONLY, 0)
		if err != nil {
			return
		}
		defer f.Close()
		buf := make([]byte, 4096)
		n, _ := f.Read(buf)
		ch <- string(buf[:n])
	}()

	time.Sleep(100 * time.Millisecond)

	bin := buildBinary(t)
	cmd := exec.Command(bin, "emit", "question", "test-msg")
	cmd.Env = append(os.Environ(), "AGENT_STATUS_RUNTIME_DIR="+dir,
		"CLAUDE_CODE_SESSION_ID=e2e", "TMUX_SESSION_NAME=test", "TMUX_PANE=%1")
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "emit failed: %s", string(out))

	select {
	case line := <-ch:
		assert.Contains(t, string(line), `"session_id":"e2e"`)
		assert.Contains(t, string(line), `"event":"question"`)
		assert.Contains(t, string(line), `"harness":"claude"`)
	case <-time.After(3 * time.Second):
		t.Fatal("timeout: no data from FIFO")
	}
}

func TestEmitSilentDrop(t *testing.T) {
	dir := t.TempDir()
	bin := buildBinary(t)
	// No FIFO exists — emit should exit 0 silently
	cmd := exec.Command(bin, "emit", "working")
	cmd.Env = append(os.Environ(), "AGENT_STATUS_RUNTIME_DIR="+dir)
	out, err := cmd.CombinedOutput()
	assert.NoError(t, err, "should exit 0, got: %s", string(out))
}

func TestEmitMissingState(t *testing.T) {
	bin := buildBinary(t)
	cmd := exec.Command(bin, "emit")
	out, err := cmd.CombinedOutput()
	assert.Error(t, err)
	assert.Contains(t, string(out), "usage:")
}

func TestEmitStateRoundTrip(t *testing.T) {
	for _, state := range []string{"working", "question", "idle", "error"} {
		dir := t.TempDir()
		bin := buildBinary(t)
		fifo := dir + "/in.fifo"
		require.NoError(t, syscall.Mkfifo(fifo, 0666))

		ch := make(chan string)
		go func() {
			f, _ := os.OpenFile(fifo, os.O_RDONLY, 0)
			buf := make([]byte, 4096)
			n, _ := f.Read(buf)
			ch <- string(buf[:n])
			f.Close()
		}()
		time.Sleep(100 * time.Millisecond)

		cmd := exec.Command(bin, "emit", state)
		cmd.Env = append(os.Environ(), "AGENT_STATUS_RUNTIME_DIR="+dir,
			"CLAUDE_CODE_SESSION_ID=test", "TMUX_SESSION_NAME=m", "TMUX_PANE=%0")
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "state=%s: %s", state, string(out))

		select {
		case line := <-ch:
			assert.Contains(t, line, `"event":"`+state+`"`, "state=%s", state)
		case <-time.After(3 * time.Second):
			t.Fatal("timeout for state=" + state)
		}
	}
}
