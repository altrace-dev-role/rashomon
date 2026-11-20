package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/altrace-dev-role/rashomon/internal/backtest"
	"github.com/altrace-dev-role/rashomon/internal/baseline"
	"github.com/altrace-dev-role/rashomon/internal/steer/pattern"
	"github.com/altrace-dev-role/rashomon/internal/store"
)

// Spec rules and the gate the replay fixtures declare, as the hook's own
// tests declare them (internal/hook steer_rules_test.go, specdiscover_test.go).
const (
	rh7cNoForce   = `{"id": "no-force", "kind": "forbid_command", "command": {"argv": ["git", "push"], "flags": ["--force", "-f"]}, "why": "history is shared", "hard_control": "branch protection"}`
	rh7cFrozen    = `{"id": "frozen-migrations", "kind": "protect_path", "paths": ["db/migrations/**"], "why": "applied migrations are immutable", "hard_control": "CODEOWNERS"}`
	rh7cTestFirst = `{"id": "test-first", "kind": "order", "then": {"argv": ["git", "push"]}, "after": {"argv": ["go", "test"]}, "invalidated_by": ["**/*.go"], "why": "push only tested code", "hard_control": "CI"}`
	rh7cOneGate   = `{"rashomon_spec": 1, "gates": [{"id": "unit", "kind": "test", "argv": ["go", "test", "./..."]}]}`
)

// rh7cRules is a spec of rules, one JSON object each.
func rh7cRules(rules ...string) string {
	return `{"rashomon_spec": 1, "rules": [` + strings.Join(rules, ", ") + `]}`
}

// rh7cCall is one tool call of a transcript copy: the assistant's tool_use
// line, then the user's tool_result line with the toolUseResult Claude Code
// keeps beside it.
type rh7cCall struct {
	id, tool string
	input    map[string]any
	result   any
	text     string
	isError  bool
}

func rh7cBash(id, command, stdout string) rh7cCall {
	return rh7cCall{id: id, tool: "Bash", input: map[string]any{"command": command}, text: stdout,
		result: map[string]any{"stdout": stdout, "stderr": "", "interrupted": false, "isImage": false}}
}

func rh7cRed(id, command string) rh7cCall {
	const out = "Exit code 1\nFAIL\texample"
	return rh7cCall{id: id, tool: "Bash", input: map[string]any{"command": command}, text: out, result: "Error: " + out, isError: true}
}

func rh7cEdit(id, path string) rh7cCall {
	return rh7cCall{id: id, tool: "Edit", text: "The file has been updated.",
		input:  map[string]any{"file_path": path, "old_string": "a", "new_string": "b"},
		result: map[string]any{"filePath": path, "oldString": "a", "newString": "b"}}
}

// rh7cTranscript is a main-thread transcript of one prompt in cwd: the
// prompt, then each call a minute after the last, its result 5 s later.
func rh7cTranscript(t *testing.T, session, cwd string, calls []rh7cCall) []byte {
	t.Helper()
	at := time.Date(2026, 10, 29, 10, 0, 0, 0, time.UTC)
	stamp := func(d time.Duration) string { return at.Add(d).Format("2006-01-02T15:04:05.000Z") }
	var b bytes.Buffer
	line := func(v map[string]any) {
		j, err := json.Marshal(v)
		if err != nil {
			t.Fatal(err)
		}
		b.Write(j)
		b.WriteByte('\n')
	}
	line(map[string]any{"type": "user", "isSidechain": false, "sessionId": session, "cwd": cwd, "timestamp": stamp(0),
		"promptId": "p1", "permissionMode": "default", "message": map[string]any{"role": "user", "content": "do the task"}})
	for i, c := range calls {
		start := time.Duration(i+1) * time.Minute
		line(map[string]any{"type": "assistant", "isSidechain": false, "sessionId": session, "cwd": cwd, "timestamp": stamp(start),
			"message": map[string]any{"role": "assistant", "content": []any{
				map[string]any{"type": "tool_use", "id": c.id, "name": c.tool, "input": c.input}}}})
		line(map[string]any{"type": "user", "isSidechain": false, "sessionId": session, "cwd": cwd, "timestamp": stamp(start + 5*time.Second),
			"promptId": "p1", "message": map[string]any{"role": "user", "content": []any{
				map[string]any{"type": "tool_result", "tool_use_id": c.id, "content": c.text, "is_error": c.isError}}},
			"toolUseResult": c.result})
	}
	return b.Bytes()
}

// rh7cProject is bodiesHere's project, the test's working directory, made a
// git tree whose repository spec is doc ("" for none), with the store at
// record scope v5. It returns the store, the project's resolved path, which
// the transcript names as its cwd, and the project's bodies directory.
func rh7cProject(t *testing.T, doc string) (st *store.Store, project, dir string) {
	t.Helper()
	t.Setenv("CLAUDE_CONFIG_DIR", t.TempDir())
	st, project, dir = bodiesHere(t)
	if err := store.WriteRecordScopeMarker(st.Root(), store.RecordScopeV5); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(project, ".git"), 0o700); err != nil {
		t.Fatal(err)
	}
	if doc != "" {
		p := filepath.Join(project, ".claude", "rashomon", "spec.json")
		if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(doc), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	// The key bodiesHere named the bodies directory by must still be the
	// project's once it is a git tree, or the backtest reads another one.
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if key := baseline.Key(cwd); key != project {
		t.Fatalf("the project's key is %q after .git, %q before", key, project)
	}
	return st, project, dir
}

// TestRH7c_ReplayFixtures (send 1011 item 3): each pattern a backtest
// measures beside regression-location, which TestP6_ covers, fires in the
// replay of a transcript copy built to raise it once, through `rashomon
// backtest --sensor ID` as a person runs it: a forced push under a
// forbid_command rule (platform-rule), an edit under a protect_path rule
// (integrity), a push with no go test before it under an order rule
// (ordering-rule), and a piped pytest whose output holds no summary after a
// green and a write (masked-status). The fifth fixture is the pattern a
// backtest refuses (TestRH7b_): a declared required gate, green, then an
// edit, which fires stale-evidence live under a trusted spec (internal/hook
// TestDG2d_), replayed: the replay judges the gate and the write, so the red
// after them fires regression-location, but its scratch store binds no
// repository spec, so the spec reads as untrusted and stale-evidence fires 0
// times (TestDG2e_'s untrusted case).
func TestRH7c_ReplayFixtures(t *testing.T) {
	for _, c := range []struct {
		sensor pattern.ID
		doc    string
		calls  func(project string) []rh7cCall
	}{
		{pattern.PlatformRule, rh7cRules(rh7cNoForce), func(string) []rh7cCall {
			return []rh7cCall{rh7cBash("toolu_p", "git push --force origin main", "")}
		}},
		{pattern.Integrity, rh7cRules(rh7cFrozen), func(project string) []rh7cCall {
			return []rh7cCall{rh7cEdit("toolu_i", filepath.Join(project, "db", "migrations", "001.sql"))}
		}},
		{pattern.OrderingRule, rh7cRules(rh7cTestFirst), func(string) []rh7cCall {
			return []rh7cCall{rh7cBash("toolu_o", "git push origin main", "")}
		}},
		{pattern.MaskedStatus, "", func(project string) []rh7cCall {
			return []rh7cCall{
				rh7cBash("toolu_g", "pytest -q", "3 passed in 0.12s\n"),
				rh7cEdit("toolu_e", filepath.Join(project, "a.py")),
				rh7cBash("toolu_m", "pytest -q | tail -5", "collecting ...\n"),
			}
		}},
	} {
		t.Run(string(c.sensor), func(t *testing.T) {
			st, project, dir := rh7cProject(t, c.doc)
			session := "sess-rh7c-" + string(c.sensor)
			writeBody(t, st, dir, session+".jsonl", 0)
			p := filepath.Join(st.Root(), store.DirBodies, dir, session+".jsonl")
			if err := os.WriteFile(p, rh7cTranscript(t, session, project, c.calls(project)), 0o600); err != nil {
				t.Fatal(err)
			}
			var out, errOut bytes.Buffer
			if code := run([]string{"backtest", "--sensor", string(c.sensor)}, nil, &out, &errOut); code != 0 {
				t.Fatalf("backtest --sensor %s: exit %d, stderr %q", c.sensor, code, errOut.String())
			}
			lines := strings.Split(strings.TrimSuffix(out.String(), "\n"), "\n")
			var head backtestHeader
			var row backtestRow
			if len(lines) != 2 || json.Unmarshal([]byte(lines[0]), &head) != nil || head.Sensor != string(c.sensor) ||
				head.Sessions != 1 || json.Unmarshal([]byte(lines[1]), &row) != nil {
				t.Fatalf("backtest --sensor %s printed %q", c.sensor, out.String())
			}
			if row.SessionID != session || row.Fires != 1 || row.LiveRun {
				t.Errorf("backtest --sensor %s: row %+v, want %s fired once in the replay, with no live run", c.sensor, row, session)
			}
		})
	}
	t.Run(string(pattern.StaleEvidence), func(t *testing.T) {
		_, project, _ := rh7cProject(t, rh7cOneGate)
		b := rh7cTranscript(t, "sess-rh7c-stale", project, []rh7cCall{
			rh7cBash("toolu_g", "go test ./...", "ok  \texample\t0.1s"),
			rh7cEdit("toolu_e", filepath.Join(project, "internal", "a.go")),
			rh7cRed("toolu_r", "go test ./..."),
		})
		session, calls, err := backtest.Calls(bytes.NewReader(b))
		if err != nil || session != "sess-rh7c-stale" || len(calls) != 3 {
			t.Fatalf("Calls: session %q, %d calls, err %v; want sess-rh7c-stale and 3", session, len(calls), err)
		}
		fires, err := backtest.Replay(session, store.RecordScopeV5, calls)
		if err != nil {
			t.Fatal(err)
		}
		got := map[string]int{}
		for _, f := range fires {
			got[f.Pattern]++
		}
		if got[string(pattern.RegressionLocation)] != 1 || got[string(pattern.StaleEvidence)] != 0 {
			t.Errorf("the replay fired %v; want regression-location once, so the gate and the write were judged, and stale-evidence 0 times", got)
		}
	})
}
