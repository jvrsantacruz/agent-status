// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 Javier Santacruz

package event

import (
	"encoding/json"
	"testing"
	"time"

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
