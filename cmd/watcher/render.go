package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

func runRender(args []string) {
	if len(args) < 1 {
		return
	}
	tmuxTarget := args[0]

	data, err := os.ReadFile(filepath.Join(stateDir(), "state.json"))
	if err != nil {
		return
	}

	var sf struct {
		Sessions map[string]sessionEntry `json:"sessions"`
	}
	if err := json.Unmarshal(data, &sf); err != nil {
		return
	}

	for _, s := range sf.Sessions {
		if s.TmuxTarget != tmuxTarget {
			continue
		}
		fmt.Print(markerFor(s))
		return
	}
}

type sessionEntry struct {
	Harness    string `json:"harness"`
	State      string `json:"state"`
	PID        int    `json:"pid"`
	TmuxTarget string `json:"tmux_target"`
	DeadSince  *int64 `json:"dead_since"`
	UpdatedAt  int64  `json:"updated_at"`
	Message    string `json:"message"`
}

func markerFor(s sessionEntry) string {
	switch s.State {
	case "working":
		return "#[fg=cyan]▶ #[default]"
	case "question":
		return "#[fg=yellow,bold]? #[default]"
	case "error":
		return "#[fg=red]✗ #[default]"
	case "dead":
		return "#[fg=brightblack,dim]x #[default]"
	default:
		return ""
	}
}

func stateDir() string {
	if d := os.Getenv("AGENT_STATUS_STATE_DIR"); d != "" {
		return d
	}
	if d := os.Getenv("XDG_STATE_HOME"); d != "" {
		return d + "/agent-status"
	}
	return os.Getenv("HOME") + "/.local/state/agent-status"
}
