// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 Javier Santacruz

package export

import (
	"encoding/json"
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
	return compile(t, "state.schema.json")
}

// compile loads a schema from schema/, with meta.schema.json registered under
// its $id, so state.schema.json's $ref resolves without the network.
func compile(t *testing.T, name string) *jsonschema.Schema {
	t.Helper()
	c := jsonschema.NewCompiler()
	f, err := os.Open("../../schema/meta.schema.json")
	require.NoError(t, err)
	defer f.Close()
	doc, err := jsonschema.UnmarshalJSON(f)
	require.NoError(t, err)
	require.NoError(t, c.AddResource("https://raw.githubusercontent.com/jvrsantacruz/agent-status/main/schema/meta.schema.json", doc))
	s, err := c.Compile("../../schema/" + name)
	require.NoError(t, err)
	return s
}

// The contract fixtures other programs build against match the schemas.
func TestContractFixturesMatchSchema(t *testing.T) {
	t.Parallel()
	files := map[string]string{
		"state-with-meta.json":    "state.schema.json",
		"state-without-meta.json": "state.schema.json",
		"meta.json":               "meta.schema.json",
	}
	for file, schema := range files {
		t.Run(file, func(t *testing.T) {
			t.Parallel()
			data, err := os.ReadFile(filepath.Join("../../testdata/contract", file))
			require.NoError(t, err)
			assert.NoError(t, validate(t, compile(t, schema), string(data)))
		})
	}
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
				"m": {State: state.Idle, Reason: "event:idle", Meta: json.RawMessage(`{"name":"Fix login","turns":3,"pr":{"number":1,"state":"open"}}`)},
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
