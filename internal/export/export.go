// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 Javier Santacruz

package export

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/jvrsantacruz/agent-status/internal/state"
)

// Daemon tells a reader of state.json which daemon wrote it and whether
// events are arriving.
type Daemon struct {
	Version        string `json:"version"`
	PID            int    `json:"pid"`
	StartedAt      int64  `json:"started_at"`
	LastEventAt    int64  `json:"last_event_at"`
	EventsTotal    int64  `json:"events_total"`
	EventsRejected int64  `json:"events_rejected"`
}

type StateFile struct {
	Daemon   Daemon                   `json:"daemon"`
	Sessions map[string]state.Session `json:"sessions"`
}

// Read loads a state file.
func Read(path string) (StateFile, error) {
	var sf StateFile
	data, err := os.ReadFile(path)
	if err != nil {
		return sf, fmt.Errorf("read state: %w", err)
	}
	if err := json.Unmarshal(data, &sf); err != nil {
		return sf, fmt.Errorf("parse state %s: %w", path, err)
	}
	return sf, nil
}

// Write atomically writes the state file to path, readable by its owner only.
// Uses a temp file + rename to avoid partial reads.
func Write(path string, sf StateFile) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("export mkdir: %w", err)
	}

	data, err := json.MarshalIndent(sf, "", "  ")
	if err != nil {
		return fmt.Errorf("export marshal: %w", err)
	}

	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("export write tmp: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("export rename: %w", err)
	}
	return nil
}
