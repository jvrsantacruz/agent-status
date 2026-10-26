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

	cases := []struct {
		state string
		want  string
	}{
		{"working",  "#[fg=cyan]▶ #[default]"},
		{"question", "#[fg=yellow,bold]? #[default]"},
		{"error",    "#[fg=red]✗ #[default]"},
		{"dead",     "#[fg=brightblack,dim]x #[default]"},
		{"idle",     ""},
	}

	for _, tc := range cases {
		t.Run(tc.state, func(t *testing.T) {
			dir := t.TempDir()
			stateFile := filepath.Join(dir, "state.json")
			json := `{"sessions":{"s1":{"harness":"pi","state":"` + tc.state + `","pid":1,"tmux_target":"s:%0","updated_at":100}}}`
			require.NoError(t, os.WriteFile(stateFile, []byte(json), 0644))

			cmd := exec.Command(bin, "render", "s:%0")
			cmd.Env = append(os.Environ(), "AGENT_STATUS_STATE_DIR="+dir)
			out, err := cmd.CombinedOutput()
			assert.NoError(t, err)
			assert.Equal(t, tc.want, string(out))
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
