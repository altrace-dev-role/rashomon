package acceptance

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"
)

// seqs is every seq the run's ordered stream holds, in file order.
func (e *env) seqs(sessionID string) []int {
	e.t.Helper()
	var out []int
	for _, r := range e.records(sessionID) {
		if v, ok := r.fields["seq"].(float64); ok {
			out = append(out, int(v))
		}
	}
	return out
}

// strictlyIncreasing reports whether seqs never repeats or goes back.
func strictlyIncreasing(seqs []int) bool {
	return sort.SliceIsSorted(seqs, func(i, j int) bool { return seqs[i] < seqs[j] }) && func() bool {
		for i := 1; i < len(seqs); i++ {
			if seqs[i] == seqs[i-1] {
				return false
			}
		}
		return true
	}()
}

// call records one more PreToolUse invocation under a fresh tool_use_id.
func (e *env) call(id string, extraEnv ...string) {
	e.t.Helper()
	p := defaultPayload()
	p.ToolUseID = id
	if res := e.hook(p.build(e.t), extraEnv...); res.exitCode != 0 {
		e.t.Fatalf("hook: exit %d", res.exitCode)
	}
}

// TestRubricTEST5_ALostOrGarbledSeqFileNeverRepeatsASeq: the seq counter is
// a file beside the records, and when it is missing or does not parse the
// records themselves are the authority. No test exercised that recovery, and
// a counter that restarted would hand seq 1 out a second time -- two records
// claiming one place in the total order.
func TestRubricTEST5_ALostOrGarbledSeqFileNeverRepeatsASeq(t *testing.T) {
	e := newEnv(t)
	e.watched(testSession)
	e.call("toolu_a")
	seqFile := filepath.Join(e.home, "runs", testSession, "seq")
	if err := os.Remove(seqFile); err != nil {
		t.Fatal(err)
	}
	e.call("toolu_b")
	if err := os.WriteFile(seqFile, []byte("not a number\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	e.call("toolu_c")

	got := e.seqs(testSession)
	if len(got) != 6 || !strictlyIncreasing(got) || got[0] != 1 {
		t.Errorf("seqs %v, want six strictly increasing from 1 across a lost and a garbled counter", got)
	}
}

// TestRubricTEST5_APanicAfterTheSeqAdvancesSkipsOneNumber: the counter is
// advanced and synced before the record that uses it is written, so a crash
// between the two costs one number and never hands one out twice.
func TestRubricTEST5_APanicAfterTheSeqAdvancesSkipsOneNumber(t *testing.T) {
	e := newEnv(t)
	e.watched(testSession)
	e.call("toolu_a")
	e.call("toolu_crashed", "RASHOMON_FAULT=store.seq.advanced:plain_panic")
	e.call("toolu_b")

	got := e.seqs(testSession)
	want := []int{1, 2, 4, 5}
	if len(got) != len(want) {
		t.Fatalf("seqs %v, want %v: exactly one number skipped", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("seqs %v, want %v: exactly one number skipped", got, want)
		}
	}
}

// TestRubricTEST5_ACrashInForgetsRewriteLosesNoKeptRecord: forget rewrites a
// run's records in place. It used to truncate the file first and write the
// kept lines after, so a crash in between lost every kept record while the
// gap claimed only the forgotten ones had gone. It now writes the kept lines
// from the start of the file and truncates last: a crash part-way leaves
// every kept record readable, beside duplicates or one damaged line that the
// report surfaces.
func TestRubricTEST5_ACrashInForgetsRewriteLosesNoKeptRecord(t *testing.T) {
	e := newEnv(t)
	seedHostSession(t, e)
	forgotten := e.declarations(testSession)[0].str("tool_use_id")
	before, err := os.ReadFile(e.recordsPath(testSession))
	if err != nil {
		t.Fatal(err)
	}
	var kept [][]byte
	for _, line := range bytes.Split(before, []byte("\n")) {
		var r map[string]any
		if json.Unmarshal(line, &r) == nil && r["tool_use_id"] != forgotten {
			kept = append(kept, line)
		}
	}
	if len(kept) < 2 {
		t.Fatalf("premise: %d kept records, want the other call's declaration and terminal", len(kept))
	}

	res := e.run("", []string{"RASHOMON_FAULT=store.rewrite.partial:plain_panic"}, "forget", "--host", forgettable)
	if res.exitCode == 0 {
		t.Fatalf("premise: the fault did not fire; forget exited 0")
	}
	after, err := os.ReadFile(e.recordsPath(testSession))
	if err != nil {
		t.Fatal(err)
	}
	lines := map[string]bool{}
	for _, line := range bytes.Split(after, []byte("\n")) {
		lines[string(line)] = true
	}
	for _, line := range kept {
		if !lines[string(line)] {
			t.Errorf("a crash part-way through forget's rewrite lost a kept record:\n%s\nfile now:\n%s", line, after)
		}
	}
	if rep := e.report(testSession); rep.Declarations.Recorded == 0 {
		t.Errorf("the report reads no declaration after the crash")
	}
}

// TestRubricTEST5_ACrashBeforeTheBaselineRenameKeepsTheOldOne: a project
// baseline is replaced by write-to-temporary and rename, so a crash before
// the rename leaves the previous baseline exactly as it was.
func TestRubricTEST5_ACrashBeforeTheBaselineRenameKeepsTheOldOne(t *testing.T) {
	e := newEnv(t)
	e.watched("sess-first")
	e.declare("sess-first", "curl https://keep-me.example/y")
	e.probe("end", "sess-first")
	e.sessionJSON(t, "sess-first")
	path := baselineFile(t, e)
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	e.probe("start", testSession)
	e.declare(testSession, "curl https://new-host.example/z")
	e.probe("end", testSession)
	e.run("", []string{"RASHOMON_FAULT=baseline.before_rename:plain_panic"}, "report", "--json", "--session", testSession)

	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Errorf("a crash before the rename changed the baseline:\nbefore %s\nafter  %s", before, after)
	}
}

// TestRubricTEST5_ACrashBeforeTheRecapRenameKeepsTheOldState: recap's state
// file is replaced the same way, and a crash before its rename leaves the
// previous state in place.
func TestRubricTEST5_ACrashBeforeTheRecapRenameKeepsTheOldState(t *testing.T) {
	e := newEnv(t)
	e.watched(testSession)
	e.mustHook(defaultPayload().build(t))
	e.recapLine(stopPayload(testSession, "Ran it.", false))
	path := filepath.Join(e.home, "recap.json")
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	time.Sleep(5 * time.Millisecond)
	e.recapLine(stopPayload(testSession, "Ran it again.", false), "RASHOMON_FAULT=recap.before_rename:plain_panic")

	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Errorf("a crash before the rename changed recap's state:\nbefore %s\nafter  %s", before, after)
	}
}
