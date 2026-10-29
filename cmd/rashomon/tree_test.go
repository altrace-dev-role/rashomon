package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/altrace-dev-role/rashomon/internal/digest"
	"github.com/altrace-dev-role/rashomon/internal/hook"
	"github.com/altrace-dev-role/rashomon/internal/install"
	"github.com/altrace-dev-role/rashomon/internal/store"
	"github.com/altrace-dev-role/rashomon/internal/verify"
)

// TestPG6e_StopBindsTheTree: the Stop view reads the content now of each
// file the session's tree names (hook.TreeShas over verify.TreeTargets) and
// judges the gates against it (card P5A, D140). An Edit's post hashed a.go,
// a green go test ran after it, and the Stop reads the gate Green; once a.go
// changes on disk with no write in the session's record, it reads Stale with
// stale_foreign, and the checkpoint the Stop appends says so.
func TestPG6e_StopBindsTheTree(t *testing.T) {
	t.Setenv("CLAUDE_CONFIG_DIR", t.TempDir())
	t.Setenv("CLAUDE_PROJECT_DIR", "")
	t.Setenv("RASHOMON_MANAGED_SETTINGS_PATH", filepath.Join(t.TempDir(), "managed-settings.json"))
	cwd, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(cwd, "a.go")
	if err := os.WriteFile(file, []byte("package a\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := store.WriteRecordScopeMarker(st.Root(), store.RecordScopeV5); err != nil {
		t.Fatal(err)
	}
	now := time.UnixMilli(1_800_000_000_000)
	clock := func() time.Time { return now }
	if err := st.AppendCoverage(store.Coverage{Type: store.TypeCoverage, SchemaVersion: store.SessionSchemaVersion,
		RecordedAtMS: now.UnixMilli(), SessionID: "s", Phase: store.PhasePost, State: store.StateVerified,
		HookEntry: store.EntryPresentSettings, Probe: store.ProbeFresh}); err != nil {
		t.Fatal(err)
	}
	land := func(id, tool string, input map[string]any) {
		t.Helper()
		for _, event := range []string{"PreToolUse", "PostToolUse"} {
			b, err := json.Marshal(map[string]any{"session_id": "s", "tool_use_id": id, "prompt_id": "p1",
				"transcript_path": "/tmp/t.jsonl", "permission_mode": "default", "tool_name": tool, "cwd": cwd,
				"hook_event_name": event, "tool_input": input, "tool_response": map[string]any{"stdout": ""}})
			if err != nil {
				t.Fatal(err)
			}
			if event == "PreToolUse" {
				h := hook.New(st, clock)
				h.Close(&hook.Signals{}, h.Capture(strings.NewReader(string(b))))
			} else if err := hook.NewPost(st, clock).Capture(strings.NewReader(string(b))); err != nil {
				t.Fatalf("post %s: %v", id, err)
			}
			now = now.Add(time.Minute)
		}
	}
	land("toolu_e", "Edit", map[string]any{"file_path": file, "old_string": "a", "new_string": "a"})
	land("toolu_g", "Bash", map[string]any{"command": "go test ./..."})

	stop := func() verify.GateState {
		t.Helper()
		run, err := digest.ReadRun(st, "s")
		if err != nil {
			t.Fatal(err)
		}
		d, err := digest.BuildFrom(run, st, "s", "", "", now)
		if err != nil {
			t.Fatal(err)
		}
		p := recapPayload{SessionID: "s", Harness: harnessSignal{HookEventName: install.EventStop}, OK: true, CWD: cwd}
		turn, err := stopView(st, run, d, p, false, now)
		if err != nil {
			t.Fatal(err)
		}
		if len(turn.view.Snap.Gates) != 1 {
			t.Fatalf("the Stop read %d gates, want one: %+v", len(turn.view.Snap.Gates), turn.view.Snap)
		}
		return turn.view.Snap.Gates[0]
	}
	if g := stop(); g.State != verify.Green {
		t.Fatalf("the tree as the run left it: %s %s, want green", g.State, g.Reason)
	}
	if err := os.WriteFile(file, []byte("package b\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if g := stop(); g.State != verify.Stale || g.Reason != verify.StaleForeign {
		t.Fatalf("a.go changed with no write recorded: %s %s, want stale stale_foreign", g.State, g.Reason)
	}
	data, err := os.ReadFile(filepath.Join(st.RunDir("s"), store.FileRecords))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"reason":"stale_foreign"`) {
		t.Error("no checkpoint carries stale_foreign")
	}
}
