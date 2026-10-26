//go:build e2e

package main

import (
	"os"
	"os/exec"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInstallCopiesBinary(t *testing.T) {
	dir := t.TempDir()
	bin := buildBinary(t)

	cmd := exec.Command(bin, "install")
	cmd.Env = append(os.Environ(),
		"HOME="+dir,
		"XDG_CONFIG_HOME="+dir+"/.config",
		"XDG_STATE_HOME="+dir+"/.local/state",
		"XDG_RUNTIME_DIR="+dir+"/runtime",
	)
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "install failed: %s", string(out))

	_, err = os.Stat(dir + "/.local/bin/watcher")
	assert.NoError(t, err, "binary should be installed")

	_, err = os.Stat(dir + "/runtime/agent-status")
	assert.NoError(t, err, "runtime dir should exist")

	_, err = os.Stat(dir + "/.local/state/agent-status")
	assert.NoError(t, err, "state dir should exist")
}

func TestInstallWritesSystemdUnit(t *testing.T) {
	dir := t.TempDir()
	bin := buildBinary(t)

	cmd := exec.Command(bin, "install")
	cmd.Env = append(os.Environ(),
		"HOME="+dir,
		"XDG_CONFIG_HOME="+dir+"/.config",
		"XDG_STATE_HOME="+dir+"/.local/state",
		"XDG_RUNTIME_DIR="+dir+"/runtime",
	)
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "install failed: %s", string(out))

	unitPath := dir + "/.config/systemd/user/agent-status.service"
	data, err := os.ReadFile(unitPath)
	require.NoError(t, err)

	content := string(data)
	assert.Contains(t, content, "[Unit]")
	assert.Contains(t, content, "agent-status")
	assert.Contains(t, content, "ExecStart="+dir+"/.local/bin/watcher")
	assert.Contains(t, content, "AGENT_STATUS_STATE_DIR="+dir+"/.local/state/agent-status")
}

func TestInstallCopySkipsSymlinks(t *testing.T) {
	// Ensure we install the real binary, not a symlink
	dir := t.TempDir()
	bin := buildBinary(t)

	link := dir + "/watcher-link"
	require.NoError(t, os.Symlink(bin, link))

	cmd := exec.Command(link, "install")
	cmd.Env = append(os.Environ(),
		"HOME="+dir,
		"XDG_CONFIG_HOME="+dir+"/.config",
		"XDG_STATE_HOME="+dir+"/.local/state",
		"XDG_RUNTIME_DIR="+dir+"/runtime",
	)
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "install via symlink failed: %s", string(out))

	// Should have resolved the symlink and installed the real binary
	installed, _ := os.ReadFile(dir + "/.local/bin/watcher")
	original, _ := os.ReadFile(bin)
	assert.Equal(t, original, installed, "installed binary should match original, not symlink")
}

func TestInstallIdempotent(t *testing.T) {
	dir := t.TempDir()
	bin := buildBinary(t)

	env := append(os.Environ(),
		"HOME="+dir,
		"XDG_CONFIG_HOME="+dir+"/.config",
		"XDG_STATE_HOME="+dir+"/.local/state",
		"XDG_RUNTIME_DIR="+dir+"/runtime",
	)
	// Run install twice
	cmd1 := exec.Command(bin, "install")
	cmd1.Env = env
	require.NoError(t, cmd1.Run())

	cmd2 := exec.Command(bin, "install")
	cmd2.Env = env
	out, err := cmd2.CombinedOutput()
	assert.NoError(t, err, "second install should not fail: %s", string(out))
}

