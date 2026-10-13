package digest

import (
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"

	"github.com/altrace-dev-role/rashomon/internal/report"
	"github.com/altrace-dev-role/rashomon/internal/store"
)

const diffSession = "diff-session"

// diffRun writes one single-turn run into a fresh store: a start probe, one
// call per id under prompt-1, each with its terminal and an ok execution,
// and -- when closed -- an end probe. The transcript path names no file, so the report's
// transcript accounting reads nothing and adds nothing.
func diffRun(t *testing.T, closed bool, ids ...string) (*store.Store, time.Time) {
	t.Helper()
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t0 := time.Now().Add(-time.Minute)
	cov := func(phase string, at time.Time) {
		if err := st.AppendCoverage(store.Coverage{Type: "coverage", SchemaVersion: 2, SessionID: diffSession,
			Phase: phase, State: store.StateVerified, InstallID: "inst-1", RecordedAtMS: at.UnixMilli()}); err != nil {
			t.Fatal(err)
		}
	}
	cov(store.PhaseStart, t0)
	prompt := "prompt-1"
	for i, id := range ids {
		at := t0.Add(time.Duration(i+1) * time.Second).UnixMilli()
		if err := st.AppendDeclaration(store.Declaration{Type: store.TypeDeclaration, SchemaVersion: store.SchemaVersion,
			SessionID: diffSession, ToolUseID: id, ToolName: "Read",
			PromptID: &prompt, TranscriptPath: filepath.Join(t.TempDir(), "absent.jsonl"), RecordedAtMS: at}); err != nil {
			t.Fatal(err)
		}
		if err := st.AppendTerminal(store.Terminal{Type: store.TypeTerminal, SchemaVersion: store.SchemaVersion,
			SessionID: diffSession, ToolUseID: id, Outcome: store.OutcomeOK, RecordedAtMS: at}); err != nil {
			t.Fatal(err)
		}
		if err := st.AppendExecution(store.Execution{Type: store.TypeExecution, SchemaVersion: store.SchemaVersion,
			SessionID: diffSession, ToolUseID: id, Outcome: store.ExecOK,
			RecordedAtMS: at + 1}); err != nil {
			t.Fatal(err)
		}
	}
	if closed {
		cov(store.PhaseEnd, t0.Add(time.Hour))
	}
	return st, t0.Add(2 * time.Hour)
}

// reportBasis is report's session reasons as digest's turn reasons should
// read them: less the three report adds by design and digest never does
// (run_not_closed, which H-84 keeps off a turn read at Stop, and the two
// transcript reasons, which need the transcript digest never opens), and
// with records_unreadable in digest's word, records_skipped.
func reportBasis(reasons []string) []string {
	out := []string{}
	for _, r := range reasons {
		switch r {
		case report.ReasonRunNotClosed, report.ReasonTranscriptMismatch, report.ReasonExecutionMismatch:
			continue
		case report.ReasonRecordsUnreadable:
			r = ReasonRecordsSkipped
		}
		out = append(out, r)
	}
	sort.Strings(out)
	return out
}

func sortedReasons(reasons []string) []string {
	out := append([]string{}, reasons...)
	sort.Strings(out)
	return out
}

// bothReasons builds the turn digest and the session report from one store.
func bothReasons(t *testing.T, st *store.Store, now time.Time) (digestReasons, reportReasons []string) {
	t.Helper()
	d, err := Build(st, diffSession, "prompt-1", "", now)
	if err != nil {
		t.Fatal(err)
	}
	rep, err := report.Build(st, diffSession, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Sessions) != 1 {
		t.Fatalf("report holds %d sessions, want 1", len(rep.Sessions))
	}
	return d.Coverage.Reasons, rep.Sessions[0].Coverage.Reasons
}

// TestRubricTEST12_DigestAndReportAgreeOnAClosedTurn is the differential the
// two coverage derivations never had: on a closed single-turn run, the turn
// digest's reasons are the session report's, less the reasons only report
// adds by design. The test named for the difference re-implemented report's
// rule inline instead of building a report.
func TestRubricTEST12_DigestAndReportAgreeOnAClosedTurn(t *testing.T) {
	cases := []struct {
		name  string
		ids   []string
		setup func(t *testing.T, st *store.Store, now time.Time)
		want  string // the reason the case exists to produce; "" for none
	}{
		{name: "clean", ids: []string{"t1", "t2"}},
		{name: "gap", ids: []string{"t1"}, want: report.ReasonGap,
			setup: func(t *testing.T, st *store.Store, now time.Time) {
				g := store.Gap{Type: "gap", SessionID: diffSession,
					FromUnixMS: now.Add(-2 * time.Hour).UnixMilli(), ToUnixMS: now.UnixMilli()}
				if err := st.AppendGap(g); err != nil {
					t.Fatal(err)
				}
			}},
		{name: "duplicate", ids: []string{"t1", "t1"}, want: report.ReasonDuplicateDeclarations},
		{name: "skipped line", ids: []string{"t1"}, want: ReasonRecordsSkipped,
			setup: func(t *testing.T, st *store.Store, _ time.Time) {
				f, err := os.OpenFile(filepath.Join(st.RunDir(diffSession), store.FileRecords), os.O_APPEND|os.O_WRONLY, 0)
				if err != nil {
					t.Fatal(err)
				}
				if _, err := f.WriteString("not a record\n"); err != nil {
					t.Fatal(err)
				}
				if err := f.Close(); err != nil {
					t.Fatal(err)
				}
			}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			st, now := diffRun(t, true, c.ids...)
			if c.setup != nil {
				c.setup(t, st, now)
			}
			got, rep := bothReasons(t, st, now)
			if want := reportBasis(rep); !equalStrings(sortedReasons(got), want) {
				t.Errorf("digest reasons %v, want report's %v as digest reads them, %v", got, rep, want)
			}
			if c.want != "" && !hasReason(got, c.want) {
				t.Errorf("digest reasons %v lack %s: the case did not produce what it is for", got, c.want)
			}
			if c.want == "" && len(got) != 0 {
				t.Errorf("a clean closed turn has reasons %v", got)
			}
		})
	}
}

// TestRubricTEST12_AnOpenRunIsTheOneDocumentedDifference pins the difference
// H-84 makes on purpose: the report says run_not_closed of a run still open,
// and the turn digest, read at Stop when every run is open, never does.
// Every call here has terminated, so the digest's other open-run rule --
// unterminated and dropped calls count only once the run has ended -- has
// nothing to hold back, and everything else agrees.
func TestRubricTEST12_AnOpenRunIsTheOneDocumentedDifference(t *testing.T) {
	st, now := diffRun(t, false, "t1")
	got, rep := bothReasons(t, st, now)
	if !hasReason(rep, report.ReasonRunNotClosed) {
		t.Fatalf("premise: the report of an open run has reasons %v, without run_not_closed", rep)
	}
	if hasReason(got, report.ReasonRunNotClosed) {
		t.Errorf("the turn digest says run_not_closed (%v); H-84 keeps it off every turn", got)
	}
	if want := reportBasis(rep); !equalStrings(sortedReasons(got), want) {
		t.Errorf("digest reasons %v, want report's %v less run_not_closed, %v", got, rep, want)
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
