package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/altrace-dev-role/rashomon/internal/baseline"
	"github.com/altrace-dev-role/rashomon/internal/hookio"
	"github.com/altrace-dev-role/rashomon/internal/install"
	"github.com/altrace-dev-role/rashomon/internal/store"
)

// seedPrevious writes a run of an earlier session holding one checkpoint for
// project, cut at at with the given gates, into the store at root.
func seedPrevious(t *testing.T, root, session, project string, at time.Time, green bool, gates string) {
	t.Helper()
	st, err := store.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	dir := st.RunDir(session)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	line := fmt.Sprintf(`{"type":"verify","schema_version":%d,"recorded_at_unix_ms":%d,"session_id":%q,"project_root":%q,`+
		`"point_seq":7,"record_green":%t,"gates":[%s],"gates_omitted":0,"integrity":{"unresolved":[]},"integrity_omitted":0}`+"\n",
		store.EventSchemaVersion, at.UnixMilli(), session, project, green, gates)
	if err := os.WriteFile(filepath.Join(dir, store.FileRecords), []byte(line), 0o600); err != nil {
		t.Fatal(err)
	}
}

// TestRP2t_StartupLineReplacesTheVerifyLine (card REPORT1b R1, TestRP2i's
// printBrief half): with an earlier session of the project in the store, probe
// start's brief ends in "last session: <time>, <reason>; <pointer>" in place
// of the verify line, the brief is no longer than without it, and it is still
// one systemMessage and no additional context. With no earlier session the
// verify line stays. Break: append the line instead of replacing, or
// replace it when nothing was found.
func TestRP2t_StartupLineReplacesTheVerifyLine(t *testing.T) {
	s := newBriefSession(t)
	root, err := store.DefaultRoot()
	if err != nil {
		t.Fatal(err)
	}
	first := message(t, s.start("startup"))
	if !strings.Contains(first, "\n  verify: ") || strings.Contains(first, "last session") {
		t.Fatalf("with no earlier session the verify line stays: %q", first)
	}
	at := time.Date(2026, 10, 29, 16, 20, 0, 0, time.Local)
	seedPrevious(t, root, "earlier", baseline.Key(s.dir), at, false, `{"gate":"go","required":true,"state":"stale"}`)
	got := message(t, s.start("startup"))
	want := "\n  last session: 10-29 16:20, 1 gate not green; rashomon report --session last"
	if !strings.HasSuffix(got, want) || strings.Contains(got, "verify:") {
		t.Errorf("startup brief %q; want it to end with %q and hold no verify line", got, want)
	}
	if n, m := strings.Count(got, "\n"), strings.Count(first, "\n"); n != m || n > 8 {
		t.Errorf("the brief has %d lines after the first, %d before; want equal and at most 8", n, m)
	}
	if strings.Contains(got, "additionalContext") {
		t.Errorf("the line reached the model: %q", got)
	}
}

// TestRP2u_AnErrorLeavesTheVerifyLine (card REPORT1b K5): when the scan
// cannot read the store's runs, the brief keeps the verify line and nothing
// else changes. Break: print a none line on error.
func TestRP2u_AnErrorLeavesTheVerifyLine(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("root reads a directory with no permissions")
	}
	root, cfg := t.TempDir(), t.TempDir()
	benchSetenv(t, root, cfg)
	benchInstall(t, root, cfg)
	st, err := store.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	runs := filepath.Join(root, store.DirRuns)
	if err := os.MkdirAll(runs, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(runs, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(runs, 0o700) })
	head := newBenchHead("SessionStart")
	head.CWD = dir
	payload, err := hookio.IntakeBytes(benchJSON(t, struct {
		benchHead
		Source string `json:"source"`
	}{head, "startup"}))
	if err != nil {
		t.Fatal(err)
	}
	var out strings.Builder
	printBrief([]string{store.PhaseStart, install.Marker, "x"}, payload, st, &out)
	msg := message(t, out.String())
	if !strings.Contains(msg, "\n  verify: rashomon report --session "+benchSessionID) || strings.Contains(msg, "last session") {
		t.Errorf("an unreadable runs directory must leave the verify line: %q", msg)
	}
}
