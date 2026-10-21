// Package codex holds hook payloads captured from the Codex CLI; see
// README.md for where they came from.
//
// The test here asks one question before any Codex support is written:
// do Codex's PreToolUse and PostToolUse payloads decode into the types the
// recorder already uses, and which of the fields those types expect from
// Claude Code does Codex leave out? A field that is absent decodes to its zero
// value without an error, so a clean decode says nothing about whether the
// record built from it would be right. The log lines are that second answer.
package codex

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/hook"
)

func TestCodexPayloadsDecode(t *testing.T) {
	paths, err := filepath.Glob(filepath.Join("payloads", "*.json"))
	if err != nil {
		t.Fatal(err)
	}
	// An empty directory would pass every check below without decoding
	// anything, so the two events under test have to have been seen.
	seen := map[string]int{}

	for _, path := range paths {
		name := filepath.Base(path)
		event, _, _ := strings.Cut(name, "-")

		t.Run(strings.TrimSuffix(name, ".json"), func(t *testing.T) {
			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			var keys map[string]json.RawMessage
			if err := json.Unmarshal(raw, &keys); err != nil {
				t.Fatalf("not a JSON object: %v", err)
			}
			var named string
			if err := json.Unmarshal(keys["hook_event_name"], &named); err != nil || named != event {
				t.Fatalf("hook_event_name is %q, filename says %q", named, event)
			}
			seen[event]++

			switch event {
			case "PreToolUse":
				var p hook.Payload
				if err := json.Unmarshal(raw, &p); err != nil {
					t.Fatalf("does not decode into hook.Payload: %v", err)
				}
				// Proves the decode landed in the struct rather than
				// succeeding on a shape that shares no keys with it.
				if p.HookEventName != event {
					t.Fatalf("decoded hook_event_name %q", p.HookEventName)
				}
				logMissing(t, "hook.Payload", hook.Payload{}, keys)
			case "PostToolUse":
				var p hook.PostPayload
				if err := json.Unmarshal(raw, &p); err != nil {
					t.Fatalf("does not decode into hook.PostPayload: %v", err)
				}
				if p.HookEventName != event {
					t.Fatalf("decoded hook_event_name %q", p.HookEventName)
				}
				logMissing(t, "hook.PostPayload", hook.PostPayload{}, keys)
			}
		})
	}

	for _, event := range []string{"PreToolUse", "PostToolUse"} {
		if seen[event] == 0 {
			t.Errorf("no %s fixture was checked", event)
		}
	}
}

// logMissing logs the keys typ declares that the payload does not carry.
//
// The expected keys are read from typ's json tags rather than listed here, so
// a field added to the payload type is checked against Codex without anyone
// remembering to add it. Some are conditional even in Claude Code -- agent_id
// and agent_type only inside a subagent, error and is_interrupt only on a
// failure event -- and are logged anyway: whether Codex ever sends them is the
// thing being found out.
func logMissing(t *testing.T, label string, typ any, keys map[string]json.RawMessage) {
	t.Helper()
	var missing []string
	rt := reflect.TypeOf(typ)
	for i := range rt.NumField() {
		tag, _, _ := strings.Cut(rt.Field(i).Tag.Get("json"), ",")
		if tag == "" || tag == "-" {
			continue
		}
		if _, ok := keys[tag]; !ok {
			missing = append(missing, tag)
		}
	}
	slices.Sort(missing)
	if len(missing) == 0 {
		t.Logf("%s: no field missing", label)
		return
	}
	t.Logf("%s: missing %s", label, strings.Join(missing, ", "))
}
