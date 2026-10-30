package main

import (
	"os"
	"path/filepath"
)

// Every command finds the FIFO and state.json through these functions, so
// the daemon and its clients cannot disagree on a path.

func stateDir() string {
	if d := os.Getenv("AGENT_STATUS_STATE_DIR"); d != "" {
		return d
	}
	if d := os.Getenv("XDG_STATE_HOME"); d != "" {
		return filepath.Join(d, "agent-status")
	}
	return filepath.Join(os.Getenv("HOME"), ".local", "state", "agent-status")
}

// runtimeDir holds the FIFO. Without XDG_RUNTIME_DIR, as on macOS, it is the
// state directory: private to the user, unlike /tmp.
func runtimeDir() string {
	if d := os.Getenv("AGENT_STATUS_RUNTIME_DIR"); d != "" {
		return d
	}
	if d := os.Getenv("XDG_RUNTIME_DIR"); d != "" {
		return filepath.Join(d, "agent-status")
	}
	return stateDir()
}

func fifoPath() string  { return filepath.Join(runtimeDir(), "in.fifo") }
func statePath() string { return filepath.Join(stateDir(), "state.json") }
func dbPath() string    { return filepath.Join(stateDir(), "state.db") }
