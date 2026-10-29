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
	"github.com/altrace-dev-role/rashomon/internal/hook"
	"github.com/altrace-dev-role/rashomon/internal/hookio"
	"github.com/altrace-dev-role/rashomon/internal/store"
)

// liveCall is one call as Claude Code sends it to the hooks: the payloads a
// live session's hooks read, written by hand, apart from any transcript.
type liveCall struct {
	id, tool string
	input    map[string]any
	at, done string
	response map[string]any
	failure  string
	denied   bool
}

func clock(t *testing.T, s string) time.Time {
	t.Helper()
	v, err := time.Parse(time.RFC3339, "2026-10-29T10:"+s+"Z")
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func livePayload(t *testing.T, session, event string, c liveCall, more map[string]any) *hookio.Payload {
	t.Helper()
	p := map[string]any{"hook_event_name": event, "session_id": session, "prompt_id": "p1", "cwd": "/tmp/project",
		"permission_mode": "default", "transcript_path": "/tmp/t.jsonl", "tool_name": c.tool, "tool_input": c.input,
		"tool_use_id": c.id}
	for k, v := range more {
		p[k] = v
	}
	b, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	pl, err := hookio.IntakeBytes(b)
	if err != nil {
		t.Fatal(err)
	}
	return pl
}

// liveSession runs calls through the hooks as cmdHook and cmdPost do after
// their guards, with consent read from the store and the holdout drawn, so
// the live log owes nothing to the replay's settings. A hook's own coverage
// record is unverified without an install, so the session's verified post
// coverage is appended by hand, as the hook's steer-step tests append it.
func liveSession(t *testing.T, st *store.Store, session string, calls []liveCall) {
	t.Helper()
	if err := st.AppendCoverage(store.Coverage{Type: store.TypeCoverage, SchemaVersion: store.SessionSchemaVersion,
		RecordedAtMS: clock(t, calls[0].at).UnixMilli(), SessionID: session, Phase: store.PhasePost,
		State: store.StateVerified, HookEntry: store.EntryPresentSettings, Probe: store.ProbeFresh}); err != nil {
		t.Fatal(err)
	}
	sig := hook.WatchSignals()
	defer sig.Stop()
	for _, c := range calls {
		pre, done := clock(t, c.at), clock(t, c.done)
		h := hook.New(st, func() time.Time { return pre })
		h.Close(sig, h.CapturePayload(livePayload(t, session, "PreToolUse", c, nil)))
		if err := h.Flush(); err != nil {
			t.Fatal(err)
		}
		if c.denied {
			continue
		}
		event, more := "PostToolUse", map[string]any{"tool_response": c.response}
		if c.failure != "" {
			event, more = "PostToolUseFailure", map[string]any{"error": c.failure, "is_interrupt": false,
				"duration_ms": done.Sub(pre).Milliseconds()}
		}
		p := hook.NewPost(st, func() time.Time { return done })
		p.Close(sig, p.CapturePayload(livePayload(t, session, event, c, more)))
		if err := p.Flush(); err != nil {
			t.Fatal(err)
		}
		p.Steer(hook.SteerEnv{Consent: store.SteerDisclosureMarker, Now: done})
	}
}

var (
	btGoTest = map[string]any{"command": "go test ./..."}
	btOK     = map[string]any{"stdout": "ok  \texample\t0.1s", "stderr": "", "interrupted": false, "isImage": false}
	btGreen  = liveCall{id: "toolu_g", tool: "Bash", input: btGoTest, at: "00:01", done: "00:06", response: btOK}
	btEdit   = liveCall{id: "toolu_e", tool: "Edit", at: "01:00", done: "01:01",
		input:    map[string]any{"file_path": "/tmp/project/internal/a.go", "old_string": "a", "new_string": "b"},
		response: map[string]any{"filePath": "/tmp/project/internal/a.go", "oldString": "a", "newString": "b"}}
	btRed    = liveCall{id: "toolu_r", tool: "Bash", input: btGoTest, at: "03:00", done: "03:06", failure: "Exit code 1\nFAIL\texample"}
	btDenied = liveCall{id: "toolu_r", tool: "Bash", input: btGoTest, at: "03:00", done: "03:09", denied: true}
)

// TestP6_BacktestReproducesTheLiveFires (PRD PF-RC14's done-when): three
// transcript copies under testdata, replayed by `rashomon backtest`, give
// each session the regression-location fires its live hooks logged, matched
// on (pattern, call): a regression fires once, a denied red does not run and
// so does not fire, and a session whose run is gone fires in the replay with
// no live run to match. A subagent's red in the main transcript is not
// replayed.
func TestP6_BacktestReproducesTheLiveFires(t *testing.T) {
	t.Setenv("CLAUDE_CONFIG_DIR", t.TempDir())
	src, err := filepath.Abs(filepath.Join("testdata", "backtest")) // before bodiesHere's chdir
	if err != nil {
		t.Fatal(err)
	}
	st, _, dir := bodiesHere(t)
	if err := store.WriteRecordScopeMarker(st.Root(), store.RecordScopeV5); err != nil {
		t.Fatal(err)
	}
	liveSession(t, st, "sess-bt-1", []liveCall{btGreen, btEdit, btRed})
	liveSession(t, st, "sess-bt-2", []liveCall{btGreen, btEdit, btDenied})
	for i, name := range []string{"sess-bt-1", "sess-bt-2", "sess-bt-3"} {
		b, err := os.ReadFile(filepath.Join(src, name+".jsonl"))
		if err != nil {
			t.Fatal(err)
		}
		writeBody(t, st, dir, name+".jsonl", 0)
		p := filepath.Join(st.Root(), store.DirBodies, dir, name+".jsonl")
		if err := os.WriteFile(p, b, 0o600); err != nil {
			t.Fatal(err)
		}
		mod := clock(t, "00:00").Add(time.Duration(i) * time.Hour)
		if err := os.Chtimes(p, mod, mod); err != nil {
			t.Fatal(err)
		}
	}
	var out, errOut bytes.Buffer
	if code := run([]string{"backtest", "--sensor", "regression-location"}, nil, &out, &errOut); code != 0 {
		t.Fatalf("backtest: exit %d, stderr %q", code, errOut.String())
	}
	lines := strings.Split(strings.TrimSuffix(out.String(), "\n"), "\n")
	var head backtestHeader
	if len(lines) != 4 || json.Unmarshal([]byte(lines[0]), &head) != nil || head.Schema != "backtest/1" ||
		head.Sensor != "regression-location" || head.Sessions != 3 || head.Fidelity != backtest.Fidelity {
		t.Fatalf("backtest printed %q", out.String())
	}
	want := []string{
		`{"session_id":"sess-bt-1","fires":1,"live_run":true,"live_fires":1,"matched":1}`,
		`{"session_id":"sess-bt-2","fires":0,"live_run":true,"live_fires":0,"matched":0}`,
		`{"session_id":"sess-bt-3","fires":1,"live_run":false,"live_fires":0,"matched":0}`,
	}
	for i, w := range want {
		if lines[i+1] != w {
			t.Errorf("row %d = %s, want %s", i+1, lines[i+1], w)
		}
	}
	if _, err := os.Stat(st.RunDir("sess-bt-3")); err == nil {
		t.Error("the backtest wrote a run into the store")
	}
}

// TestP6_BacktestArguments: an id outside the frozen pattern set, a missing
// --sensor, and --help are refused.
func TestP6_BacktestArguments(t *testing.T) {
	bodiesHere(t)
	for _, args := range [][]string{{"--help"}, {"--sensor"}, {"--sensor", "no-such-pattern"}, {"--sensor", "a", "b"}} {
		var out, errOut bytes.Buffer
		if code := run(append([]string{"backtest"}, args...), nil, &out, &errOut); code != 1 || out.Len() != 0 {
			t.Errorf("backtest %v: exit %d, stdout %q; want refused", args, code, out.String())
		}
	}
}

// TestP6_MatchedIsAMultisetIntersection: each live fire answers one replay
// fire at most, and only on the same pattern and call.
func TestP6_MatchedIsAMultisetIntersection(t *testing.T) {
	a, b := backtest.Fire{Pattern: "regression-location", Call: "k"}, backtest.Fire{Pattern: "regression-location", Call: "j"}
	for _, tc := range []struct {
		replay, live []backtest.Fire
		want         int
	}{
		{[]backtest.Fire{a, a}, []backtest.Fire{a}, 1},
		{[]backtest.Fire{a}, []backtest.Fire{a, a}, 1},
		{[]backtest.Fire{a, b}, []backtest.Fire{b, a}, 2},
		{[]backtest.Fire{a}, []backtest.Fire{b}, 0},
	} {
		if got := backtest.Matched(tc.replay, tc.live); got != tc.want {
			t.Errorf("Matched(%v, %v) = %d, want %d", tc.replay, tc.live, got, tc.want)
		}
	}
	if got := ofPattern([]backtest.Fire{a, {Pattern: "integrity", Call: "k"}}, "integrity"); len(got) != 1 {
		t.Errorf("ofPattern kept %v", got)
	}
}
