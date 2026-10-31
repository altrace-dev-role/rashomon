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
	"github.com/altrace-dev-role/rashomon/internal/verify"
)

// seedPrevious writes a run of an earlier session holding one checkpoint for
// project, cut at at with the given gates, into the store at root.
func seedPrevious(t *testing.T, root, session, project string, at time.Time, green bool, gates string) {
	t.Helper()
	seedCheckpoint(t, root, session, project, at, green, gates)
	pointTo(t, root, project, session, 7, at)
}

// pointTo writes the project index with its Last naming session at seq, as
// the Stop that cut the checkpoint does.
func pointTo(t *testing.T, root, project, session string, seq int64, at time.Time) {
	t.Helper()
	idx := &verify.ProjectIndex{ProjectKey: project, Last: &verify.LastCheckpoint{SessionID: session, PointSeq: seq, AtMS: at.UnixMilli()}}
	if _, err := verify.WriteIndex(verify.IndexPath(root, project), idx); err != nil {
		t.Fatal(err)
	}
}

// seedCheckpoint writes the run and its checkpoint, and no pointer.
func seedCheckpoint(t *testing.T, root, session, project string, at time.Time, green bool, gates string) {
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
	at := time.Date(2026, 10, 29, 16, 20, 0, 0, time.UTC)
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

// TestRP2zb_APointerThatCannotBeTrustedLeavesTheLine (card REPORT1b): the
// startup line comes from the project index's pointer and the one checkpoint
// it names, and nothing is guessed. A checkpoint in the store with no pointer
// to it, a pointer whose checkpoint is another project's or another point's,
// and an index that names only the current session all leave the verify line;
// a pointer to a run that was removed says so. Break: walk the store when
// the pointer is missing, skip the project or seq check, or skip the
// existence check.
func TestRP2zb_APointerThatCannotBeTrustedLeavesTheLine(t *testing.T) {
	s := newBriefSession(t)
	root, err := store.DefaultRoot()
	if err != nil {
		t.Fatal(err)
	}
	project := baseline.Key(s.dir)
	at := time.Date(2026, 10, 29, 16, 20, 0, 0, time.UTC)
	green := `{"gate":"go","required":true,"state":"green"}`
	verifyLine := "\n  verify: rashomon report --session " + benchSessionID
	keeps := func(name string) {
		t.Helper()
		if msg := message(t, s.start("startup")); !strings.HasSuffix(msg, verifyLine) {
			t.Errorf("%s: %q; want the verify line kept", name, msg)
		}
	}
	seedCheckpoint(t, root, "earlier", project, at, true, green)
	keeps("a checkpoint with no pointer to it")
	pointTo(t, root, project, "earlier", 8, at)
	keeps("a pointer to another point of the run")
	seedCheckpoint(t, root, "other", "/some/other/project", at, true, green)
	pointTo(t, root, project, "other", 7, at)
	keeps("a pointer to another project's checkpoint")
	pointTo(t, root, project, benchSessionID, 7, at)
	keeps("an index that names only the current session")
	pointTo(t, root, project, "earlier", 7, at)
	if msg := message(t, s.start("startup")); !strings.HasSuffix(msg, "\n  last session: 10-29 16:20, record green; rashomon report --session last") {
		t.Fatalf("the control, a good pointer: %q", msg)
	}
	if err := os.RemoveAll(filepath.Join(root, store.DirRuns, "earlier")); err != nil {
		t.Fatal(err)
	}
	if msg := message(t, s.start("startup")); !strings.HasSuffix(msg, "\n  last session: none; its record is no longer in the store") || strings.Contains(msg, "verify:") {
		t.Errorf("a removed run: %q; want it said", msg)
	}
}

// TestRP2zg_APointedCheckpointBehindMoreThanTheTailIsStillFound (card
// REPORT1b review): a run that cut its checkpoint and then wrote 100 KB of
// other records still has it, and SessionStart prints its line, not that the
// record is gone. A run longer than the widened read says it was not found
// within the bound. Break: never widen the read, or call every tail miss gone.
func TestRP2zg_APointedCheckpointBehindMoreThanTheTailIsStillFound(t *testing.T) {
	s := newBriefSession(t)
	root, err := store.DefaultRoot()
	if err != nil {
		t.Fatal(err)
	}
	project := baseline.Key(s.dir)
	at := time.Date(2026, 10, 29, 16, 20, 0, 0, time.UTC)
	seedPrevious(t, root, "earlier", project, at, true, `{"gate":"go","required":true,"state":"green"}`)
	pad := []byte(`{"type":"declaration","schema_version":2,"pad":"` + strings.Repeat("x", 1000) + "\"}\n")
	appendPad := func(n int) {
		f, err := os.OpenFile(filepath.Join(root, store.DirRuns, "earlier", store.FileRecords), os.O_APPEND|os.O_WRONLY, 0o600)
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		for range n {
			if _, err := f.Write(pad); err != nil {
				t.Fatal(err)
			}
		}
	}
	appendPad(100)
	msg := message(t, s.start("startup"))
	if !strings.HasSuffix(msg, "\n  last session: 10-29 16:20, record green; rashomon report --session last") || strings.Contains(msg, "no longer") {
		t.Errorf("a checkpoint behind 100 KB of records: %q", msg)
	}
	appendPad(1100)
	msg = message(t, s.start("startup"))
	if !strings.HasSuffix(msg, "\n  last session: its record was not found within the 1 MiB read bound") || strings.Contains(msg, "no longer") {
		t.Errorf("a checkpoint behind more than the widened read: %q", msg)
	}
}
