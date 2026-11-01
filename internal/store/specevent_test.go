package store

import (
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

// TestSRS1l_TheHookAppendIsAccepted (card SPEC-RULES-STOP1, Q4): a rule_hit
// AppendRecord takes is not ErrUnknownKind, lands in the writer's form with
// critical true and a seq, and reads back as the run's one spec event, with
// nothing skipped or ignored.
func TestSRS1l_TheHookAppendIsAccepted(t *testing.T) {
	st, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	hit := NewRuleHit("s1", "PostToolUseFailure", "spec:no-force", "sha256:"+strings.Repeat("11", 32), 7, 1000)
	if err := st.AppendRecord("s1", hit); err != nil {
		t.Fatalf("AppendRecord(rule_hit) = %v, want nil", err)
	}
	lines := recordLines(t, st, "s1")
	if len(lines) != 1 || !strings.HasPrefix(lines[0], `{"type":"spec_event","schema_version":5,"critical":true,`) {
		t.Fatalf("records.ndjson %q, want one spec_event line, critical true, in the writer's form", lines)
	}
	run, err := st.ReadRunConsistent("s1", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if run.Skipped != 0 || run.Ignored != 0 || len(run.SpecEvents) != 1 {
		t.Fatalf("read %+v, want one spec event and nothing passed over", run)
	}
	got := run.SpecEvents[0]
	if got.Seq == nil || got.Reason != SpecEventRuleHit || got.Event != "PostToolUseFailure" || got.RuleID != "spec:no-force" ||
		got.SourceDigest != hit.SourceDigest || got.LandedSeq != 7 || !got.Critical {
		t.Errorf("read %+v, want the hit as written with its seq", got)
	}
}

// TestSRS1g_AReaderPassesOverWhatItCannotRead (D-e, as ruled): a kind this
// reader refuses, or a spec_event of a reason it does not know, is passed
// over by the record's own critical key. True, or no key (must-understand),
// is Skipped, which unverifies the session; false is Ignored. A known reason
// is read. g: an unknown kind; g2: an unknown reason, critical true or
// absent; g3: an unknown reason, critical false.
func TestSRS1g_AReaderPassesOverWhatItCannotRead(t *testing.T) {
	const head = `{"type":"spec_event","schema_version":5,`
	for _, c := range []struct {
		name, line string
		want       Run
	}{
		{"g unknown kind, critical", `{"type":"spec_ruling","schema_version":5,"critical":true,"seq":3}`, Run{Skipped: 1}},
		{"g2 unknown reason, critical", head + `"critical":true,"seq":3,"recorded_at_unix_ms":1,"session_id":"s1","reason":"spec_edit"}`, Run{Skipped: 1}},
		{"g2 unknown reason, no critical key", head + `"seq":3,"recorded_at_unix_ms":1,"session_id":"s1","reason":"spec_edit"}`, Run{Skipped: 1}},
		{"g3 unknown reason, not critical", head + `"critical":false,"seq":3,"recorded_at_unix_ms":1,"session_id":"s1","reason":"spec_note"}`, Run{Ignored: 1}},
		{"known reason", head + `"critical":true,"seq":3,"recorded_at_unix_ms":1,"session_id":"s1","reason":"rule_hit","event":"PostToolUse","rule_id":"spec:r","source_digest":"d","landed_seq":2}`,
			Run{SpecEvents: []SpecEvent{{EventHeader: EventHeader{Type: TypeSpecEvent, SchemaVersion: 5, Critical: true, Seq: ptr(int64(3)), RecordedAtMS: 1, SessionID: "s1"},
				Reason: SpecEventRuleHit, Event: "PostToolUse", RuleID: "spec:r", SourceDigest: "d", LandedSeq: 2}}}},
		{"spec_event at another version", `{"type":"spec_event","schema_version":6,"critical":true,"reason":"rule_hit"}`, Run{Skipped: 1}},
	} {
		var got Run
		classifyRunLine(&got, []byte(c.line))
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s: read %+v, want %+v", c.name, got, c.want)
		}
	}
	if !AcceptsKind(TypeSpecEvent, EventSchemaVersion) || AcceptsKind(TypeSpecEvent, EventSchemaVersion-1) {
		t.Error("AcceptsKind: spec_event is read at EventSchemaVersion only")
	}
}

// TestSRS1j_ATornHitIsAGapAndTheNextAppendHeals: a rule_hit killed
// mid-write leaves a fragment with no newline. The next hit starts a line of
// its own, and the reader counts the fragment Skipped, which the Stop reads
// as an incomplete recording (TestVER31_SkippedLinesAreIncomplete), so a lost
// hit never reads green.
func TestSRS1j_ATornHitIsAGapAndTheNextAppendHeals(t *testing.T) {
	st, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	digest := "sha256:" + strings.Repeat("11", 32)
	if err := st.AppendRecord("s1", NewRuleHit("s1", "PostToolUse", "spec:no-force", digest, 1, 1000)); err != nil {
		t.Fatal(err)
	}
	appendRaw(t, filepath.Join(st.RunDir("s1"), FileRecords), []byte(`{"type":"spec_event","schema_version":5,"critical":true,"seq":2`))
	if err := st.AppendRecord("s1", NewRuleHit("s1", "PostToolUseFailure", "spec:no-force", digest, 3, 1001)); err != nil {
		t.Fatal(err)
	}
	if lines := recordLines(t, st, "s1"); len(lines) != 3 || !strings.HasPrefix(lines[2], `{"type":"spec_event",`) {
		t.Fatalf("records.ndjson %q, want the fragment on a line of its own and the second hit after it", lines)
	}
	run, err := st.ReadRunConsistent("s1", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if run.Skipped != 1 || len(run.SpecEvents) != 2 || run.SpecEvents[1].LandedSeq != 3 {
		t.Errorf("read %d skipped and spec events %+v, want 1 and both hits", run.Skipped, run.SpecEvents)
	}
}

func ptr[T any](v T) *T { return &v }
