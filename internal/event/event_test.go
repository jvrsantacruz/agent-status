// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 Javier Santacruz

package event

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEventMarshalRoundTrip(t *testing.T) {
	t.Parallel()
	in := Event{
		SessionID:   "s1",
		Harness:     "claude",
		Event:       "question",
		PID:         1234,
		Pane:        "%12",
		TimestampMs: 1700000000000,
		Message:     "what next?",
	}
	data, err := json.Marshal(in)
	require.NoError(t, err)

	var out Event
	err = json.Unmarshal(data, &out)
	require.NoError(t, err)
	assert.Equal(t, in, out)
}

func TestEventTime(t *testing.T) {
	t.Parallel()
	e := Event{TimestampMs: 1700000000000}
	assert.Equal(t, int64(1700000000), e.Time().Unix())
}

func TestEventZeroTimestamp(t *testing.T) {
	t.Parallel()
	e := Event{}
	assert.Equal(t, int64(0), e.Time().Unix())
}

func TestEventLargePID(t *testing.T) {
	t.Parallel()
	e := Event{PID: 999999, Event: "working"}
	data, _ := json.Marshal(e)
	assert.Contains(t, string(data), `"pid":999999`)
}

func TestEventExtendedMessage(t *testing.T) {
	t.Parallel()
	e := Event{SessionID: "s1", Message: `msg with "quotes" and \backslash`}
	data, err := json.Marshal(e)
	require.NoError(t, err)
	var out Event
	require.NoError(t, json.Unmarshal(data, &out))
	assert.Equal(t, e.Message, out.Message)
}

func TestEventMinimal(t *testing.T) {
	t.Parallel()
	e := Event{SessionID: "s1", Event: "working", TimestampMs: time.Now().UnixMilli()}
	data, err := json.Marshal(e)
	require.NoError(t, err)
	assert.Contains(t, string(data), `"session_id":"s1"`)
}

func TestEventHarnessValues(t *testing.T) {
	t.Parallel()
	for _, h := range []string{"pi", "claude", "unknown"} {
		e := Event{SessionID: "s1", Harness: h, Event: "idle"}
		data, _ := json.Marshal(e)
		assert.Contains(t, string(data), `"harness":"`+h+`"`)
	}
}

func TestMarshalLineFitsPipeBuf(t *testing.T) {
	t.Parallel()
	base := Event{SessionID: "0b9d6f2e-3c51-4a8e-9f0a-7d2c1e5b8a43", Harness: "claude",
		Event: "question", PID: 4242, Pane: "%17", TimestampMs: 1793359277515}
	cases := map[string]string{
		"short":      "what next?",
		"ascii":      strings.Repeat("a", 2000),
		"multibyte":  strings.Repeat("ñé€😀", 300),
		"escaped":    strings.Repeat(`<"\>`, 300), // each byte grows to 2 or 6 in JSON
		"cut inside": strings.Repeat("a", 400) + strings.Repeat("😀", 50),
	}
	for name, msg := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			e := base
			e.Message = msg
			line, err := e.MarshalLine()
			require.NoError(t, err)
			assert.LessOrEqual(t, len(line), MaxLine)
			assert.Equal(t, byte('\n'), line[len(line)-1])

			var out Event
			require.NoError(t, json.Unmarshal(line, &out))
			assert.True(t, utf8.ValidString(out.Message), "cut inside a character")
			if len(msg) < 100 {
				assert.Equal(t, msg, out.Message)
			} else {
				assert.True(t, strings.HasSuffix(out.Message, "..."))
				assert.True(t, strings.HasPrefix(msg, strings.TrimSuffix(out.Message, "...")))
				assert.Greater(t, len(line), MaxLine-40, "cut far more than needed")
			}
			out.Message = e.Message
			assert.Equal(t, e, out, "only the message changes")
		})
	}
}

func TestMarshalLineTooLongWithoutMessage(t *testing.T) {
	t.Parallel()
	_, err := Event{SessionID: strings.Repeat("s", MaxLine), Message: "m"}.MarshalLine()
	assert.ErrorIs(t, err, ErrTooLong)
}

// A line too long loses the message first, then the transcript path, the
// working directory and the tmux socket, in that order.
func TestMarshalLineDropsPathsInOrder(t *testing.T) {
	t.Parallel()
	base := Event{SessionID: "s1", Harness: "claude", Event: "working", PID: 1, Pane: "%1", TimestampMs: 1793359277515}
	long := "/" + strings.Repeat("p", 400)
	short := "/home/u/src/api"
	cases := map[string]struct {
		in, want     Event
		cutMessage   bool
		keepsMessage bool
	}{
		"fits as is": {
			in:   Event{Message: "Bash", Cwd: short, Transcript: short + "/t.jsonl", TmuxSocket: "/tmp/tmux-1000/default"},
			want: Event{Message: "Bash", Cwd: short, Transcript: short + "/t.jsonl", TmuxSocket: "/tmp/tmux-1000/default"},
		},
		"message cut before any path": {
			in:         Event{Message: strings.Repeat("m", 600), Cwd: short, Transcript: short, TmuxSocket: short},
			want:       Event{Cwd: short, Transcript: short, TmuxSocket: short},
			cutMessage: true,
		},
		"transcript dropped first": {
			in:   Event{Cwd: short, Transcript: long, TmuxSocket: short},
			want: Event{Cwd: short, TmuxSocket: short},
		},
		"then the working directory": {
			in:   Event{Cwd: long, Transcript: long, TmuxSocket: short},
			want: Event{TmuxSocket: short},
		},
		"then the socket": {
			in:   Event{Cwd: long, Transcript: long, TmuxSocket: long + long},
			want: Event{},
		},
		"the message comes back once the paths are gone": {
			in:           Event{Message: "Bash", Transcript: long + long},
			want:         Event{Message: "Bash"},
			keepsMessage: true,
		},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			e := c.in
			e.SessionID, e.Harness, e.Event, e.PID, e.Pane, e.TimestampMs = base.SessionID, base.Harness, base.Event, base.PID, base.Pane, base.TimestampMs
			line, err := e.MarshalLine()
			require.NoError(t, err)
			assert.LessOrEqual(t, len(line), MaxLine)
			var out Event
			require.NoError(t, json.Unmarshal(line, &out))
			assert.Equal(t, c.want.Cwd, out.Cwd)
			assert.Equal(t, c.want.Transcript, out.Transcript)
			assert.Equal(t, c.want.TmuxSocket, out.TmuxSocket)
			if c.cutMessage {
				assert.True(t, strings.HasSuffix(out.Message, "..."))
				assert.Greater(t, len(out.Message), 3, "the message keeps what fits")
			} else {
				assert.Equal(t, c.want.Message, out.Message)
			}
		})
	}
}

func TestMarshalLineTooLongWithoutPaths(t *testing.T) {
	t.Parallel()
	_, err := Event{SessionID: strings.Repeat("s", MaxLine), Cwd: "/x", Transcript: "/y", TmuxSocket: "/z"}.MarshalLine()
	assert.ErrorIs(t, err, ErrTooLong)
}
