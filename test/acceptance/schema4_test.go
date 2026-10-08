package acceptance

import (
	"testing"
)

// TestSchema4_AnOlderReaderKeepsTheSession: a released reader accepts schema
// 1 to 3 and skips anything newer. Schema 4 changed only the call records, so
// only they may move to 4: every coverage and gap record stays readable to
// that reader, and each terminal carries its own declaration's version, so
// that a reader never holds a terminal whose declaration it skipped.
//
// Measured on the released binary before this rule: a session recorded
// through the hooks read back as `start recorded: no`, `end recorded: no`,
// `probe_absent`, `run_not_closed` and `records_unreadable`. Break: stamp
// coverage or gaps with the call records' version, or write a terminal at
// another version than its declaration's.
func TestSchema4_AnOlderReaderKeepsTheSession(t *testing.T) {
	s := newTBSession(t)
	s.e.probe("start", testSession)
	s.shell("make test 2>&1 | tail -40", true, "")
	s.shell("go test ./...", false, "")
	if res := s.e.pause(); res.exitCode != 0 {
		t.Fatalf("pause: exit %d, stderr %q", res.exitCode, res.stderr)
	}
	if res := s.e.resume(); res.exitCode != 0 {
		t.Fatalf("resume: exit %d, stderr %q", res.exitCode, res.stderr)
	}
	s.e.probe("end", testSession)

	olderReads := func(r record) bool {
		v, ok := r.fields["schema_version"].(float64)
		return ok && v >= 1 && v <= 3
	}

	phases := map[string]bool{}
	cov := s.e.read(testSession, "coverage.ndjson")
	for i, r := range cov {
		if !olderReads(r) {
			t.Errorf("coverage record %d is at schema_version %v, which a reader of 1 to 3 skips", i, r.fields["schema_version"])
			continue
		}
		if p, _ := r.fields["phase"].(string); p != "" {
			phases[p] = true
		}
	}
	for _, p := range []string{"start", "call", "end"} {
		if !phases[p] {
			t.Errorf("no %s coverage record an older reader can read; it prints the session as never %sed", p, p)
		}
	}
	gaps := s.e.gaps()
	if len(gaps) == 0 {
		t.Fatal("pause and resume wrote no gap")
	}
	for i, r := range gaps {
		if !olderReads(r) {
			t.Errorf("gap %d is at schema_version %v, which a reader of 1 to 3 skips", i, r.fields["schema_version"])
		}
	}

	declared := map[string]any{}
	for _, r := range s.e.declarations(testSession) {
		id, _ := r.fields["tool_use_id"].(string)
		declared[id] = r.fields["schema_version"]
	}
	terminals := recordsOfType(s.e.records(testSession), "terminal")
	if len(terminals) == 0 {
		t.Fatal("no terminal record was written")
	}
	for i, r := range terminals {
		id, _ := r.fields["tool_use_id"].(string)
		want, ok := declared[id]
		if !ok {
			t.Errorf("terminal %d closes %q, which has no declaration", i, id)
			continue
		}
		if r.fields["schema_version"] != want {
			t.Errorf("terminal %d is at schema_version %v and its declaration at %v: a reader that skips the one keeps the other",
				i, r.fields["schema_version"], want)
		}
	}
}
