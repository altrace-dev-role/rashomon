package main

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/altrace-dev-role/rashomon/internal/brief"
	"github.com/altrace-dev-role/rashomon/internal/hook"
	"github.com/altrace-dev-role/rashomon/internal/spec"
	"github.com/altrace-dev-role/rashomon/internal/store"
)

// bindSpec binds the project's spec in the store p holds, as the verb does.
func (p specProject) bindSpec() {
	p.t.Helper()
	bound := spec.Binding{ProjectRoot: hook.SpecProject(p.path), SpecPath: p.path, SpecDigest: digestOf(p.t, specUnit), BoundAtMS: 1}
	if err := hook.WriteSpecBindings(p.root, &spec.Bindings{Bindings: []spec.Binding{bound}}); err != nil {
		p.t.Fatal(err)
	}
}

// TestST2_LastStopReadsTrustPastAStoreWrite (SPEC-TRUST2): the replay of a
// session's last Stop reads trust as the Stop did (D173): a spec bound in the
// store reads trusted until the session wrote under the store root, and
// untrusted after, where the raw root it once passed kept it trusted.
func TestST2_LastStopReadsTrustPastAStoreWrite(t *testing.T) {
	p := newSpecProject(t, specUnit)
	p.bindSpec()
	st, err := store.Open(p.root)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.WriteRecordScopeMarker(p.root, store.RecordScopeV5); err != nil {
		t.Fatal(err)
	}
	const session = "sess-st2"
	now := time.Date(2026, 10, 30, 18, 0, 0, 0, time.UTC)
	clock := func() time.Time { return now }
	if err := st.AppendCoverage(store.Coverage{Type: store.TypeCoverage, SchemaVersion: store.SessionSchemaVersion,
		RecordedAtMS: now.UnixMilli(), SessionID: session, Phase: store.PhasePost, State: store.StateVerified,
		HookEntry: store.EntryPresentSettings, Probe: store.ProbeFresh}); err != nil {
		t.Fatal(err)
	}
	land := func(id, tool string, input map[string]any) {
		t.Helper()
		for _, event := range []string{"PreToolUse", "PostToolUse"} {
			b, err := json.Marshal(map[string]any{"session_id": session, "tool_use_id": id, "prompt_id": "p1",
				"transcript_path": "/tmp/t.jsonl", "permission_mode": "default", "tool_name": tool, "cwd": p.dir,
				"hook_event_name": event, "tool_input": input, "tool_response": map[string]any{"stdout": ""}})
			if err != nil {
				t.Fatal(err)
			}
			if event == "PreToolUse" {
				h := hook.New(st, clock)
				h.Close(&hook.Signals{}, h.Capture(bytes.NewReader(b)))
			} else if err := hook.NewPost(st, clock).Capture(bytes.NewReader(b)); err != nil {
				t.Fatalf("post %s: %v", id, err)
			}
		}
	}
	replayTrust := func() spec.Trust {
		t.Helper()
		r, err := replayLastStop(st, session)
		if err != nil {
			t.Fatal(err)
		}
		if r.Spec == nil {
			t.Fatal("the replay discovered no spec")
		}
		return r.Spec.Trust
	}
	land("toolu_r", "Read", map[string]any{"file_path": p.path})
	if got := replayTrust(); got != spec.Trusted {
		t.Fatalf("the replay before the write: trust %q, want trusted", got)
	}
	land("toolu_w", "Write", map[string]any{"file_path": filepath.Join(p.root, spec.TrustFile), "content": "{}"})
	if got := replayTrust(); got != spec.Untrusted {
		t.Errorf("the replay after a store write: trust %q, want untrusted", got)
	}
}

// TestST2b_BriefAtSessionStartReadsBindings (SPEC-TRUST2): the brief holds no
// run, so it reads the bindings as hook.NoRun, the root as it is: an unbound
// spec reads untrusted and a bound one trusted.
func TestST2b_BriefAtSessionStartReadsBindings(t *testing.T) {
	p := newSpecProject(t, specUnit)
	text := func() string {
		t.Helper()
		return brief.Render(buildBrief(p.dir, p.dir, p.root, brief.Pointer{}))
	}
	if got := text(); !strings.Contains(got, "spec.json untrusted") {
		t.Errorf("an unbound spec in the brief:\n%s\nwant it untrusted", got)
	}
	p.bindSpec()
	if got := text(); !strings.Contains(got, "spec.json trusted") {
		t.Errorf("a bound spec in the brief:\n%s\nwant it trusted", got)
	}
}
