// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 Javier Santacruz

package event

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"
	"unicode/utf8"
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
	// Declared marks a done or error the agent states about its own work.
	// It holds until the turn ends, past the hooks that fire after it.
	Declared bool `json:"declared,omitempty"`
	// Prompt marks the event that starts a turn: nothing declared in an
	// earlier turn holds past it, even when that turn never ended.
	Prompt bool `json:"prompt,omitempty"`
}

func (e *Event) Time() time.Time {
	return time.UnixMilli(e.TimestampMs)
}

// MaxLine is the longest line a writer sends: PIPE_BUF on macOS, and the
// minimum POSIX allows. A FIFO write of at most PIPE_BUF bytes is atomic, so
// lines from concurrent writers never interleave. Linux allows 4096; macOS
// only 512.
const MaxLine = 512

// ErrTooLong is returned when an event does not fit in MaxLine even with
// no message.
var ErrTooLong = errors.New("event longer than 512 bytes without its message: session id, harness or pane too long")

// MarshalLine encodes the event as one JSON line of at most MaxLine bytes,
// as the FIFO carries it. A message that does not fit is cut to its longest
// prefix that fits, on a character boundary, ending in "...".
func (e Event) MarshalLine() ([]byte, error) {
	line, err := e.line()
	if err != nil || len(line) <= MaxLine {
		return line, err
	}
	full := e.Message
	e.Message = ""
	if line, err = e.line(); err != nil || len(line) > MaxLine {
		if err == nil {
			err = ErrTooLong
		}
		return nil, err
	}
	// Escaping makes JSON longer than the text by a varying amount, so
	// search for the longest prefix that fits: lo always fits, hi never.
	lo, hi := 0, len(full)
	for hi-lo > 1 {
		mid := (lo + hi) / 2
		e.Message = full[:mid] + "..."
		if l, err := e.line(); err == nil && len(l) <= MaxLine {
			lo, line = mid, l
		} else {
			hi = mid
		}
	}
	// The search may stop inside a character; step back to its start.
	keep := lo
	for keep > 0 && keep < len(full) && !utf8.RuneStart(full[keep]) {
		keep--
	}
	if keep != lo {
		e.Message = full[:keep] + "..."
		line, err = e.line()
	}
	return line, err
}

func (e Event) line() ([]byte, error) {
	data, err := json.Marshal(e)
	if err != nil {
		return nil, fmt.Errorf("encode event: %w", err)
	}
	return append(data, '\n'), nil
}
