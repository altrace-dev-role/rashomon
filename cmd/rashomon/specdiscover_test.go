package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/altrace-dev-role/rashomon/internal/digest"
	"github.com/altrace-dev-role/rashomon/internal/harness"
	"github.com/altrace-dev-role/rashomon/internal/hook"
	"github.com/altrace-dev-role/rashomon/internal/install"
	"github.com/altrace-dev-role/rashomon/internal/store"
	"github.com/altrace-dev-role/rashomon/test/perf/payload"
)

// TestSP15b_SteerEnvCarriesTheSpecDirs: steerEnv hands the steer step
// CLAUDE_PROJECT_DIR and the managed-settings directory, the one under
// RASHOMON_MANAGED_SETTINGS_PATH when it is set (card 3.1 (c), Gate A 4 and
// 8).
func TestSP15b_SteerEnvCarriesTheSpecDirs(t *testing.T) {
	t.Setenv("CLAUDE_PROJECT_DIR", "/work/project")
	t.Setenv("RASHOMON_MANAGED_SETTINGS_PATH", "/etc/managed/managed-settings.json")
	env := steerEnv(harness.Identity{}, time.Unix(0, 0))
	if env.ProjectDir != "/work/project" || env.ManagedDir != "/etc/managed" {
		t.Errorf("steerEnv ProjectDir %q, ManagedDir %q; want /work/project and /etc/managed", env.ProjectDir, env.ManagedDir)
	}
}

// TestVG1m_SteerEnvReadsBothHoldoutSwitches: RASHOMON_HOLDOUT reaches the
// steer step as the hook reads it, "on" and "off" exactly and nothing for
// any other value (card VERSION-GATE1).
func TestVG1m_SteerEnvReadsBothHoldoutSwitches(t *testing.T) {
	for _, tc := range []struct {
		value   string
		on, off bool
	}{{"on", true, false}, {"off", false, true}, {"", false, false}, {"ON", false, false}} {
		t.Setenv("RASHOMON_HOLDOUT", tc.value)
		env := steerEnv(harness.Identity{}, time.Unix(0, 0))
		if env.HoldoutOn != tc.on || env.HoldoutOff != tc.off {
			t.Errorf("RASHOMON_HOLDOUT=%q: HoldoutOn %v, HoldoutOff %v; want %v, %v", tc.value, env.HoldoutOn, env.HoldoutOff, tc.on, tc.off)
		}
	}
}

// TestSP15c_StopJudgesTheDiscoveredSpec: the Stop view discovers the spec
// from the Stop payload's cwd as the steer step does, and the checkpoint the
// Stop appends carries its digest (card 3.1 (c), Gate A 1 and 2).
func TestSP15c_StopJudgesTheDiscoveredSpec(t *testing.T) { stopJudgesSpec(t, false) }

// TestSR1ad_StopNeverReadsTheNoSpecNote (D210): a no-spec note for the turn
// and the directory, which the turn's posts honour, is not the Stop's to
// read: the Stop discovers fresh and judges the spec that is there now.
// Break: let the Stop take the note for no spec.
func TestSR1ad_StopNeverReadsTheNoSpecNote(t *testing.T) { stopJudgesSpec(t, true) }

func stopJudgesSpec(t *testing.T, noted bool) {
	t.Helper()
	t.Setenv("CLAUDE_PROJECT_DIR", "")
	t.Setenv("RASHOMON_MANAGED_SETTINGS_PATH", filepath.Join(t.TempDir(), "managed-settings.json"))
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	cwd := filepath.Join(base, "a", "b")
	path := filepath.Join(base, ".claude", "rashomon", "spec.json")
	for _, dir := range []string{cwd, filepath.Join(base, ".git"), filepath.Dir(path)} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	doc := `{"rashomon_spec": 1, "gates": [{"id": "unit", "kind": "test", "argv": ["go", "test", "./..."]}]}`
	if err := os.WriteFile(path, []byte(doc), 0o644); err != nil {
		t.Fatal(err)
	}

	root := t.TempDir()
	if _, err := payload.WriteSession(root, 100, payload.Prompts); err != nil {
		t.Fatal(err)
	}
	st, err := store.OpenExisting(root)
	if err != nil {
		t.Fatal(err)
	}
	run, err := digest.ReadRun(st, payload.Session)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 10, 13, 18, 0, 0, 0, time.UTC)
	d, err := digest.BuildFrom(run, st, payload.Session, "", payload.FinalMessage, now)
	if err != nil {
		t.Fatal(err)
	}
	p := recapPayload{SessionID: payload.Session, LastAssistantMessage: payload.FinalMessage,
		Harness: harnessSignal{HookEventName: install.EventStop}, OK: true, CWD: cwd}
	if noted {
		projectDir, managedDir := specDirs()
		hook.NoteSpecFound(st, payload.Session, d.PromptID, cwd, projectDir, managedDir, nil)
		if _, err := os.Stat(filepath.Join(st.RunDir(payload.Session), store.FileSpecNone)); err != nil {
			t.Fatalf("the note was not written: %v", err)
		}
	}
	turn, err := stopView(st, run, d, p, false, now)
	if err != nil {
		t.Fatal(err)
	}
	info := turn.view.Snap.Spec
	if info == nil || info.Path != path || len(info.Errors) != 0 {
		t.Fatalf("the Stop snapshot's spec %+v; want %s with no errors", info, path)
	}
	data, err := os.ReadFile(filepath.Join(st.RunDir(payload.Session), store.FileRecords))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"spec_digest":"`+info.Digest+`"`) {
		t.Errorf("no checkpoint carries spec_digest %s", info.Digest)
	}
}
