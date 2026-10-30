// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 Javier Santacruz

package event

import (
	"encoding/json"
	"fmt"
	"time"
)

// Event is one line on the FIFO. Any program may write it; see README "Event format".
type Event struct {
	SessionID   string `json:"session_id"`
	Harness     string `json:"harness"`
	Event       string `json:"event"`
	PID         int    `json:"pid"`
	Pane        string `json:"tmux_pane"`
	TimestampMs int64  `json:"ts"`
	Message     string `json:"message"`
}

func (e *Event) Time() time.Time {
	return time.UnixMilli(e.TimestampMs)
}

// MarshalLine encodes the event as one JSON line, as the FIFO carries it.
func (e Event) MarshalLine() ([]byte, error) {
	data, err := json.Marshal(e)
	if err != nil {
		return nil, fmt.Errorf("encode event: %w", err)
	}
	return append(data, '\n'), nil
}
