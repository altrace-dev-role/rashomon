package store

import (
	"strings"
	"testing"
	"time"
)

// TestSPR1w_AnAfterHitAppendIsAcceptedAndReadsBack (card SPEC-RULES1 1b, #216
// ruling 1): an after_hit AppendRecord takes is not ErrUnknownKind, lands in
// the writer's form at v5 with critical true and a seq, every key present
// (the exit status null where none was stated), and reads back as the run's
// one after_hit with nothing skipped or ignored.
func TestSPR1w_AnAfterHitAppendIsAcceptedAndReadsBack(t *testing.T) {
	st, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	zero := 0
	digest := "sha256:" + strings.Repeat("11", 32)
	if err := st.AppendRecord("s1", NewAfterHit("s1", "PostToolUse", "spec:test-first", digest, 7, &zero, true, "cwd:a", false, 1000)); err != nil {
		t.Fatalf("AppendRecord(after_hit) = %v, want nil", err)
	}
	if err := st.AppendRecord("s1", NewAfterHit("s1", "PostToolUseFailure", "spec:test-first", digest, 9, nil, true, "", true, 1001)); err != nil {
		t.Fatalf("AppendRecord(after_hit, no exit) = %v, want nil", err)
	}
	lines := recordLines(t, st, "s1")
	if len(lines) != 2 || !strings.HasPrefix(lines[0], `{"type":"after_hit","schema_version":5,"critical":true,`) || !strings.Contains(lines[1], `"exit_code":null`) {
		t.Fatalf("records.ndjson %q, want two after_hit lines, critical true, the second with exit_code null", lines)
	}
	run, err := st.ReadRunConsistent("s1", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if run.Skipped != 0 || run.Ignored != 0 || len(run.AfterHits) != 2 || len(run.SpecEvents) != 0 {
		t.Fatalf("read %+v, want two after hits and nothing passed over", run)
	}
	a, b := run.AfterHits[0], run.AfterHits[1]
	if a.Seq == nil || a.RuleID != "spec:test-first" || a.SourceDigest != digest || a.LandedSeq != 7 || a.ExitCode == nil || *a.ExitCode != 0 ||
		!a.Foreground || a.CWDDigest != "cwd:a" || a.Maybe || !a.Critical || a.Event != "PostToolUse" {
		t.Errorf("first hit %+v, want it as written with its seq", a)
	}
	if b.ExitCode != nil || !b.Maybe || b.CWDDigest != "" || b.Event != "PostToolUseFailure" || b.LandedSeq != 9 {
		t.Errorf("second hit %+v, want no exit, maybe, no directory", b)
	}
}

// TestSPR1x_AnAfterHitAtAnotherVersionIsSkipped: the kind is read at v5 only,
// and a line of it at another version is counted Skipped by its critical key,
// as every refused kind is.
func TestSPR1x_AnAfterHitAtAnotherVersionIsSkipped(t *testing.T) {
	var got Run
	classifyRunLine(&got, []byte(`{"type":"after_hit","schema_version":6,"critical":true,"seq":3}`))
	if got.Skipped != 1 || len(got.AfterHits) != 0 {
		t.Errorf("read %+v, want one skipped", got)
	}
}
