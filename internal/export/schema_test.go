// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 Javier Santacruz

package export

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jvrsantacruz/agent-status/internal/state"
)

func compileSchema(t *testing.T) *jsonschema.Schema {
	t.Helper()
	s, err := jsonschema.NewCompiler().Compile("../../schema/state.schema.json")
	require.NoError(t, err)
	return s
}

func validate(t *testing.T, s *jsonschema.Schema, data string) error {
	t.Helper()
	v, err := jsonschema.UnmarshalJSON(strings.NewReader(data))
	require.NoError(t, err)
	return s.Validate(v)
}

// What Write produces, in every state a session can be in, matches the schema.
func TestWriteMatchesSchema(t *testing.T) {
	t.Parallel()
	dead := int64(1700000000000)
	files := map[string]StateFile{
		"nil sessions": {},
		"every state": {
			Daemon: Daemon{Version: "v0.2.2", PID: 10, StartedAt: 1, LastEventAt: 2, EventsTotal: 3, EventsRejected: 1},
			Sessions: map[string]state.Session{
				"w": {Harness: "claude", State: state.Working, PID: 1, Pane: "%1", UpdatedAt: 5, Message: "Bash", Reason: "event:working"},
				"q": {State: state.Question, Reason: "event:question"},
				"i": {State: state.Idle, Reason: "event:idle"},
				"e": {State: state.Error, Message: "error: rate_limit", Reason: "event:error"},
				"u": {State: state.Idle, Reason: "event:unknown(paused)"},
				"d": {State: state.Dead, DeadSince: &dead, Reason: "liveness:pid_gone"},
			},
		},
	}
	s := compileSchema(t)
	for name, sf := range files {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			path := filepath.Join(t.TempDir(), "state.json")
			require.NoError(t, Write(path, sf))
			data, err := os.ReadFile(path)
			require.NoError(t, err)
			assert.NoError(t, validate(t, s, string(data)))
		})
	}
}

// The schema is strict enough to catch a field Write would not produce.
func TestSchemaRejects(t *testing.T) {
	t.Parallel()
	daemon := `"daemon":{"version":"v1","pid":1,"started_at":1,"last_event_at":0,"events_total":0,"events_rejected":0}`
	session := func(fields string) string {
		return `{"schema_version":1,` + daemon + `,"sessions":{"s":{` + fields + `}}}`
	}
	ok := `"harness":"","pid":0,"tmux_pane":"","dead_since":null,"updated_at":0,"message":"","reason":"event:idle"`
	s := compileSchema(t)
	require.NoError(t, validate(t, s, session(`"state":"idle",`+ok)), "the base case is valid")

	cases := map[string]string{
		"other version":   `{"schema_version":2,` + daemon + `,"sessions":{}}`,
		"no sessions":     `{"schema_version":1,` + daemon + `}`,
		"unknown state":   session(`"state":"paused",` + ok),
		"unknown field":   session(`"state":"idle","extra":1,` + ok),
		"missing field":   session(`"state":"idle"`),
		"bad reason":      session(`"state":"idle",` + strings.Replace(ok, "event:idle", "because", 1)),
		"negative pid":    session(`"state":"idle",` + strings.Replace(ok, `"pid":0`, `"pid":-1`, 1)),
		"string ms times": session(`"state":"idle",` + strings.Replace(ok, `"updated_at":0`, `"updated_at":"0"`, 1)),
	}
	for name, data := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			assert.Error(t, validate(t, s, data))
		})
	}
}
