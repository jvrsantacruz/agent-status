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
		// idle shows nothing — no marker, no option reference
		if s.State == "idle" {
			return
		}
		// Output a tmux variable reference — the plugin or theme defines
		// @agent_mark_<state> with the actual colours and marker glyph.
		fmt.Printf("#{E:@agent_mark_%s}", s.State)
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

func stateDir() string {
	if d := os.Getenv("AGENT_STATUS_STATE_DIR"); d != "" {
		return d
	}
	if d := os.Getenv("XDG_STATE_HOME"); d != "" {
		return d + "/agent-status"
	}
	return os.Getenv("HOME") + "/.local/state/agent-status"
}
