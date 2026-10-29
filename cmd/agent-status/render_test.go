//go:build e2e

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRenderNoArgs(t *testing.T) {
	bin := buildBinary(t)
	cmd := exec.Command(bin, "render")
	out, err := cmd.CombinedOutput()
	assert.NoError(t, err)
	assert.Empty(t, string(out))
}

func TestRenderNoStateFile(t *testing.T) {
	bin := buildBinary(t)
	dir := t.TempDir()
	cmd := exec.Command(bin, "render", "main:%0")
	cmd.Env = append(os.Environ(), "AGENT_STATUS_STATE_DIR="+dir)
	out, err := cmd.CombinedOutput()
	assert.NoError(t, err)
	assert.Empty(t, string(out))
}

func TestRenderStates(t *testing.T) {
	bin := buildBinary(t)

	cases := []string{"working", "question", "error", "dead", "idle"}
	for _, state := range cases {
		t.Run(state, func(t *testing.T) {
			dir := t.TempDir()
			stateFile := filepath.Join(dir, "state.json")
			json := `{"sessions":{"s1":{"harness":"pi","state":"` + state + `","pid":1,"tmux_target":"s:%0","updated_at":100}}}`
			require.NoError(t, os.WriteFile(stateFile, []byte(json), 0644))

			cmd := exec.Command(bin, "render", "s:%0")
			cmd.Env = append(os.Environ(), "AGENT_STATUS_STATE_DIR="+dir)
			out, err := cmd.CombinedOutput()
			require.NoError(t, err)

			if state == "idle" {
				assert.Empty(t, string(out))
			} else {
				assert.Equal(t, "#{E:@agent_mark_"+state+"}", string(out))
			}
		})
	}
}

func TestRenderNoMatch(t *testing.T) {
	bin := buildBinary(t)
	dir := t.TempDir()
	stateFile := filepath.Join(dir, "state.json")
	json := `{"sessions":{"s1":{"harness":"pi","state":"working","pid":1,"tmux_target":"other:%0","updated_at":100}}}`
	require.NoError(t, os.WriteFile(stateFile, []byte(json), 0644))

	cmd := exec.Command(bin, "render", "main:%0")
	cmd.Env = append(os.Environ(), "AGENT_STATUS_STATE_DIR="+dir)
	out, err := cmd.CombinedOutput()
	assert.NoError(t, err)
	assert.Empty(t, string(out))
}
