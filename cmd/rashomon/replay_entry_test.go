package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/install"
	"github.com/altrace-dev-role/rashomon/internal/replay"
)

// The replay's field table (replay.Fields) says, for every key of every hook
// payload, whether the recorder reads it. These tests hold that to the entry
// points themselves, in process: cmdHook, cmdPost, cmdRecap and cmdProbe,
// with everything they call (the host guard, the pause and stand-down checks,
// the handler and the post, the steer step, the digest and the Stop view)
// against a pinned store, on the payloads Claude Code captured. A grep of one
// file cannot show a key unread: a helper two calls down can read it.

const replayFixtures = "../../test/fixtures/claude-code/"

// step is one hook invocation.
type step struct {
	event string
	raw   []byte
}

func readCaptured(t *testing.T, version string) []step {
	t.Helper()
	raw, err := os.ReadFile(replayFixtures + version + "/payloads.ndjson")
	if err != nil {
		t.Fatal(err)
	}
	var out []step
	for _, l := range strings.Split(strings.TrimSuffix(string(raw), "\n"), "\n") {
		var m struct {
			Event string `json:"hook_event_name"`
		}
		if err := json.Unmarshal([]byte(l), &m); err != nil {
			t.Fatal(err)
		}
		out = append(out, step{m.Event, []byte(l)})
	}
	return out
}

// without removes top-level keys from a payload. It always re-encodes, so a
// payload with no key removed has been through the same re-encoding as one
// with: the comparison is never between a payload and its re-encoding.
func without(t *testing.T, raw []byte, keys ...string) []byte {
	t.Helper()
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	for _, k := range keys {
		delete(m, k)
	}
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(m); err != nil {
		t.Fatal(err)
	}
	return bytes.TrimSuffix(b.Bytes(), []byte("\n"))
}

// outcome is everything one run left behind that anyone could read: each
// entry point's exit code, stdout and stderr, and the whole store, with the
// clock's readings taken out.
type outcome struct {
	outputs []string
	store   []string
}

func (o outcome) String() string {
	return strings.Join(o.outputs, "\n") + "\n--\n" + strings.Join(o.store, "\n")
}

// runEntryPoints feeds steps through the entry points, in order, against a
// fresh store with a pinned key and install id. entrypoint is
// CLAUDE_CODE_ENTRYPOINT: set, the host guard recognises Claude Code from it;
// empty, it recognises it from the payload's transcript_path, which then
// matters.
func runEntryPoints(t *testing.T, steps []step, entrypoint string) outcome {
	t.Helper()
	root, cfg := t.TempDir(), t.TempDir()
	benchSetenv(t, root, cfg)
	t.Setenv("CLAUDE_CODE_ENTRYPOINT", entrypoint)
	id := benchInstall(t, root, cfg)
	var out outcome
	for i, s := range steps {
		var stdout, stderr bytes.Buffer
		in := bytes.NewReader(s.raw)
		var code int
		switch s.event {
		case "PreToolUse":
			code = cmdHook([]string{install.Marker, id}, in, &stderr)
		case "PostToolUse", "PostToolUseFailure":
			code = cmdPost([]string{install.Marker, id}, in, &stdout, &stderr)
		case "Stop":
			code = cmdRecap([]string{install.Marker, id}, in, &stdout, &stderr)
		case "UserPromptSubmit":
			code = cmdRecap([]string{"pending", install.Marker, id}, in, &stdout, &stderr)
		case "SessionStart":
			code = cmdProbe([]string{"start", install.Marker, id}, in, &stderr)
		case "SessionEnd":
			code = cmdProbe([]string{"end", install.Marker, id}, in, &stderr)
		default:
			t.Fatalf("step %d: event %q", i, s.event)
		}
		out.outputs = append(out.outputs, fmt.Sprintf("%d %s exit %d stdout %q stderr %q", i, s.event, code, stdout.String(), stderr.String()))
	}
	out.store = snapshot(t, root)
	return out
}

// snapshot lists the store's files with their contents, JSON parsed and the
// clock's readings (every key naming unix_ms) removed.
func snapshot(t *testing.T, root string) []string {
	t.Helper()
	var out []string
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, _ := filepath.Rel(root, p)
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		if filepath.Base(p) == "probe" { // a timestamp and nothing else
			out = append(out, rel+": (probe)")
			return nil
		}
		var lines []string
		for _, l := range strings.Split(strings.TrimSpace(string(b)), "\n") {
			var v any
			if json.Unmarshal([]byte(l), &v) == nil {
				l = canonical(v)
			}
			lines = append(lines, l)
		}
		out = append(out, rel+": "+strings.Join(lines, "\n    "))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(out)
	return out
}

func canonical(v any) string {
	var strip func(any) any
	strip = func(v any) any {
		switch v := v.(type) {
		case map[string]any:
			if v["type"] == "steer" {
				// A steer line's id is minted per delivery, not derived from
				// what was delivered.
				delete(v, "id")
				delete(v, "steer_id")
			}
			for k, x := range v {
				if strings.Contains(k, "unix_ms") {
					delete(v, k)
				} else {
					v[k] = strip(x)
				}
			}
		case []any:
			for i := range v {
				v[i] = strip(v[i])
			}
		}
		return v
	}
	b, _ := json.Marshal(strip(v))
	return string(b)
}

func diff(a, b outcome) string {
	x, y := a.String(), b.String()
	if x == y {
		return ""
	}
	xs, ys := strings.Split(x, "\n"), strings.Split(y, "\n")
	for i := 0; i < len(xs) && i < len(ys); i++ {
		if xs[i] != ys[i] {
			at := 0
			for at < len(xs[i]) && at < len(ys[i]) && xs[i][at] == ys[i][at] {
				at++
			}
			from := max(at-60, 0)
			return fmt.Sprintf("line %d, first difference at byte %d:\n  ...%.200s\n  ...%.200s", i, at, xs[i][from:], ys[i][from:])
		}
	}
	return fmt.Sprintf("lengths %d and %d", len(xs), len(ys))
}

// scenarios are the sequences a row is tried in. The captured session as it
// ran; and the same with its Stop and SessionEnd taken out and a second
// prompt after it, so that the catch-up (`recap pending`) has a turn no Stop
// reached to speak about, which it does only then.
func scenarios(t *testing.T, version string) map[string][]step {
	t.Helper()
	cap := readCaptured(t, version)
	var open []step
	for _, s := range cap {
		if s.event == "Stop" || s.event == "SessionEnd" {
			continue
		}
		open = append(open, s)
	}
	var next []byte
	for _, s := range cap {
		if s.event == "UserPromptSubmit" {
			var m map[string]json.RawMessage
			_ = json.Unmarshal(s.raw, &m)
			m["prompt_id"] = json.RawMessage(`"00000000-0000-4000-8000-0000000000ff"`)
			next, _ = json.Marshal(m)
		}
	}
	open = append(open, step{"UserPromptSubmit", next})
	return map[string][]step{"as captured": cap, "catch-up": open}
}

// applies reports whether a field-table row concerns a step.
func applies(kind replay.Kind, tools string, s step) bool {
	if s.event != kind.String() {
		return false
	}
	if tools == "" {
		return true
	}
	var m struct {
		Tool string `json:"tool_name"`
	}
	_ = json.Unmarshal(s.raw, &m)
	return (tools == "bash") == (m.Tool == "Bash")
}

// TestREPLAY5_FieldTableHoldsAtTheEntryPoints removes each key of the field
// table from every captured payload it concerns, runs the real entry points,
// and compares what they leave behind with the unaltered run. A key the
// table calls Unread must change nothing; a key it calls read (Equal,
// Normalised, Omitted) must change something, in at least one of the two
// ways the host guard can be satisfied (CLAUDE_CODE_ENTRYPOINT set, or not,
// when the guard reads transcript_path) and the two scenarios (the session as
// captured, and one left open for the catch-up to speak about), on at least
// one fixture.
//
// The entry points and what they call are listed on replay.Fields.
func TestREPLAY5_FieldTableHoldsAtTheEntryPoints(t *testing.T) {
	kinds := []replay.Kind{replay.UserPromptSubmit, replay.PreToolUse, replay.PostToolUse, replay.PostToolUseFailure, replay.Stop}
	type env struct{ name, entrypoint string }
	envs := []env{{"entrypoint set", "cli"}, {"entrypoint unset", ""}}

	// Baselines, and that a baseline is repeatable at all: a run that differs
	// from itself says nothing about a removed key.
	type run struct {
		version, scenario string
		env               env
	}
	base := map[run]outcome{}
	versions := []string{"2.1.280", "2.1.312"}
	for _, v := range versions {
		for name, steps := range scenarios(t, v) {
			var re []step
			for _, s := range steps {
				re = append(re, step{s.event, without(t, s.raw)})
			}
			for _, e := range envs {
				a, b := runEntryPoints(t, re, e.entrypoint), runEntryPoints(t, re, e.entrypoint)
				if d := diff(a, b); d != "" {
					t.Fatalf("%s %s %s: two identical runs differ: %s", v, name, e.name, d)
				}
				base[run{v, name, e}] = a
			}
		}
	}
	// The baseline must have done something worth comparing.
	if b := base[run{"2.1.312", "as captured", envs[0]}]; !strings.Contains(b.String(), "verified - python3 passed") ||
		!strings.Contains(b.String(), "regression-location") {
		t.Fatalf("the 2.1.312 run recorded, steered and recapped nothing recognisable:\n%.2000s", b)
	}
	if b := base[run{"2.1.312", "catch-up", envs[0]}]; !strings.Contains(b.String(), "previous turn") {
		t.Fatalf("the catch-up scenario's last prompt spoke of no previous turn:\n%.1500s", b.outputs)
	}

	var table []string
	for _, kind := range kinds {
		for _, f := range replay.Fields(kind) {
			if f.Class == replay.Unrecoverable {
				continue
			}
			// try runs the row's step with the key removed or, if how is
			// "changed", set to a value of its own, over every fixture,
			// scenario and host-guard setting, and returns where the outcome
			// moved.
			present := false
			try := func(how string) map[string]string {
				effects := map[string]string{}
				for _, v := range versions {
					for name, steps := range scenarios(t, v) {
						var altered []step
						hit := false
						for _, s := range steps {
							var m map[string]json.RawMessage
							_ = json.Unmarshal(s.raw, &m)
							if old, ok := m[f.Key]; ok && applies(kind, f.Tools, s) {
								if how == "removed" {
									hit = true
									altered = append(altered, step{s.event, without(t, s.raw, f.Key)})
									continue
								}
								if nv, ok := perturbed(kind, f.Key, old); ok {
									hit = true
									m[f.Key] = nv
									b, err := json.Marshal(m)
									if err != nil {
										t.Fatal(err)
									}
									altered = append(altered, step{s.event, without(t, b)})
									continue
								}
							}
							altered = append(altered, step{s.event, without(t, s.raw)})
						}
						if !hit {
							continue
						}
						present = true
						for _, e := range envs {
							if d := diff(base[run{v, name, e}], runEntryPoints(t, altered, e.entrypoint)); d != "" {
								effects[v+", "+name+", "+e.name+", "+how] = d
							}
						}
					}
				}
				return effects
			}
			effects := try("removed")
			if len(effects) == 0 {
				for k, d := range try("changed") {
					effects[k] = d
				}
			} else if f.Class == replay.Unread {
				for k, d := range try("changed") {
					effects[k] = d
				}
			}
			if !present {
				t.Errorf("%s/%s (%s): no captured payload carries it", kind, f.Key, f.Tools)
				continue
			}
			read := len(effects) > 0
			var ways []string
			for w, d := range effects {
				ways = append(ways, w)
				if f.Class == replay.Unread {
					t.Errorf("%s/%s (%s) is Unread and the outcome moves (%s): %s", kind, f.Key, f.Tools, w, d)
				}
			}
			switch {
			case f.Class != replay.Unread && !read && f.NotExercised == "":
				t.Errorf("%s/%s (%s) is class %s (read) and neither removing nor changing it moves the outcome on any fixture, scenario or host-guard setting; if the code reads it and the fixtures cannot show it, say why in NotExercised", kind, f.Key, f.Tools, f.Class)
			case read && f.NotExercised != "":
				t.Errorf("%s/%s (%s) has NotExercised %q and the outcome moves (%v): the note is stale", kind, f.Key, f.Tools, f.NotExercised, ways)
			}
			sort.Strings(ways)
			seen := fmt.Sprintf("%d ways", len(ways))
			if !read {
				seen = "no effect seen"
				if f.NotExercised != "" {
					seen += " (not exercised: " + f.NotExercised + ")"
				}
			}
			table = append(table, fmt.Sprintf("%-18s %-24s %-5s class %s  %s", kind, f.Key, f.Tools, f.Class, seen))
		}
	}
	t.Log("\n" + strings.Join(table, "\n"))
}

// perturbed is a value for a key that is not the captured one, to see whether
// the entry points consult it. A key that is read only for one value gets
// that value.
func perturbed(kind replay.Kind, key string, old json.RawMessage) (json.RawMessage, bool) {
	switch key {
	case "hook_event_name":
		return nil, false // a different name is a different event
	case "permission_mode":
		return json.RawMessage(`"dontAsk"`), true // the one mode the steer step reads
	case "last_assistant_message":
		return json.RawMessage(`"Everything works. All tests pass and there were no errors."`), true
	case "tool_response":
		return json.RawMessage(`{"backgroundTaskId":"bg-1"}`), true
	}
	switch strings.TrimSpace(string(old))[0] {
	case '"':
		return json.RawMessage(`"perturbed-value"`), true
	case 't':
		return json.RawMessage(`false`), true
	case 'f':
		return json.RawMessage(`true`), true
	case '{':
		return json.RawMessage(`{"perturbed":1}`), true
	case '[':
		return json.RawMessage(`["perturbed"]`), true
	case 'n':
		return nil, false
	}
	return json.RawMessage(`987654321`), true
}

// TestREPLAY6_RebuiltPayloadsRecordWhatTheCapturedOnesDid feeds the entry
// points the payloads the replay rebuilds from each fixture's transcript, and
// the payloads Claude Code captured, and compares everything they leave
// behind. SessionStart and SessionEnd are the captured ones in both runs: the
// replay does not rebuild them. The captured payloads lose duration_ms, which
// the replay leaves out by design (replay.Omitted), and the replay is given
// Claude Code's own transcript_path, which the caller supplies in
// production.
func TestREPLAY6_RebuiltPayloadsRecordWhatTheCapturedOnesDid(t *testing.T) {
	for _, version := range []string{"2.1.280", "2.1.312"} {
		cap := readCaptured(t, version)
		var path string
		for _, s := range cap {
			var m struct {
				Path string `json:"transcript_path"`
			}
			_ = json.Unmarshal(s.raw, &m)
			path = m.Path
		}
		f, err := os.Open(replayFixtures + version + "/transcript.jsonl")
		if err != nil {
			t.Fatal(err)
		}
		tl, err := replay.Build(f, replay.Options{TranscriptPath: path})
		f.Close()
		if err != nil {
			t.Fatal(err)
		}
		var want, got []step
		got = append(got, cap[0])
		for _, s := range cap {
			if s.event == "SessionStart" || s.event == "SessionEnd" {
				continue
			}
			want = append(want, step{s.event, without(t, s.raw, "duration_ms")})
		}
		for _, e := range tl.Events {
			if !e.Supported() {
				t.Fatalf("%s: %s %s unsupported: %v", version, e.Kind, e.ToolUseID, e.Unsupported)
			}
			got = append(got, step{e.Kind.String(), without(t, e.Payload)})
		}
		got = append(got, cap[len(cap)-1])
		want = append([]step{cap[0]}, append(want, cap[len(cap)-1])...)
		for i := range want {
			want[i].raw = without(t, want[i].raw)
		}
		for _, entrypoint := range []string{"cli", ""} {
			if d := diff(runEntryPoints(t, want, entrypoint), runEntryPoints(t, got, entrypoint)); d != "" {
				t.Errorf("%s (entrypoint %q): the rebuilt payloads leave a different store or output than the captured ones:\n%s", version, entrypoint, d)
			}
		}
	}
}
