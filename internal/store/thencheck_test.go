package store

import (
	"strings"
	"testing"
	"time"
)

// TestOL1w_AThenCheckAppendIsAcceptedAndReadsBack (card ORDER-LIVE1): a
// then_check AppendRecord takes is not ErrUnknownKind, lands in the writer's
// form at v5 with critical true and a seq, every key present, and reads back
// as the run's then_checks with nothing skipped or ignored.
func TestOL1w_AThenCheckAppendIsAcceptedAndReadsBack(t *testing.T) {
	st, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	digest := "sha256:" + strings.Repeat("11", 32)
	for i, then := range []string{ThenNo, ThenYes, ThenMaybe} {
		if err := st.AppendRecord("s1", NewThenCheck("s1", "spec:test-first", digest, int64(7+i), then, 1000)); err != nil {
			t.Fatalf("AppendRecord(then_check %s) = %v, want nil", then, err)
		}
	}
	lines := recordLines(t, st, "s1")
	if len(lines) != 3 || !strings.HasPrefix(lines[0], `{"type":"then_check","schema_version":5,"critical":true,`) || !strings.Contains(lines[0], `"then":"no"`) {
		t.Fatalf("records.ndjson %q, want three then_check lines, critical true, the first then no", lines)
	}
	run, err := st.ReadRunConsistent("s1", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if run.Skipped != 0 || run.Ignored != 0 || len(run.ThenChecks) != 3 || len(run.AfterHits) != 0 {
		t.Fatalf("read %+v, want three then checks and nothing passed over", run)
	}
	c := run.ThenChecks[1]
	if c.Seq == nil || c.RuleID != "spec:test-first" || c.SourceDigest != digest || c.LandedSeq != 8 || c.Then != ThenYes || !c.Critical {
		t.Errorf("second check %+v, want it as written with its seq", c)
	}
}

// TestOL1x_AThenCheckAtAnotherVersionIsSkipped: the kind is read at v5 only,
// and a line of it at another version is counted Skipped by its critical key,
// as every refused kind is.
func TestOL1x_AThenCheckAtAnotherVersionIsSkipped(t *testing.T) {
	var got Run
	classifyRunLine(&got, []byte(`{"type":"then_check","schema_version":6,"critical":true,"seq":3}`))
	if got.Skipped != 1 || len(got.ThenChecks) != 0 {
		t.Errorf("read %+v, want one skipped", got)
	}
}
