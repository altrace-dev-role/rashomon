package recap

import (
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/digest"
	"github.com/altrace-dev-role/rashomon/internal/report"
	"github.com/altrace-dev-role/rashomon/internal/store"
)

func cleanDigest() *digest.Digest {
	return &digest.Digest{
		SessionID: "sess-1",
		PromptID:  "prompt-1",
		Coverage:  digest.TurnCoverage{State: store.StateVerified, Reasons: []string{}},
		Declarations: digest.Declarations{
			WithoutExecution: []digest.Unexecuted{},
			Unterminated:     []string{},
			Dropped:          []string{},
			ByTool:           map[string]int{},
			ByLabel:          map[string]int{},
		},
		SilentFailures: report.SilentFailures{AbsentWords: []string{}},
		Gaps:           []store.Gap{},
	}
}

// TestLineClean is H-90's unit-level twin: a digest with none of the five
// triggers set produces no line at all.
func TestLineClean(t *testing.T) {
	d := cleanDigest()
	if line, ok := Line(d, d.SessionID, true); ok {
		t.Errorf("clean digest produced a line: %q", line)
	}
}

func TestLineCoverageUnverified(t *testing.T) {
	d := cleanDigest()
	d.Coverage.State = store.StateUnverified
	d.Coverage.Reasons = []string{store.ReasonProbeAbsent}

	line, ok := Line(d, d.SessionID, true)
	if !ok {
		t.Fatal("unverified coverage produced no line")
	}
	if !strings.Contains(line, "coverage unverified") || !strings.Contains(line, store.ReasonProbeAbsent) {
		t.Errorf("line = %q, want it to name coverage unverified and the reason", line)
	}
}

func TestLineUnknownSuppressesTheSeparateCoverageSentence(t *testing.T) {
	d := cleanDigest()
	d.Coverage.State = store.StateUnverified
	d.Coverage.Reasons = []string{store.ReasonProbeAbsent}
	d.Unknown = true

	line, ok := Line(d, d.SessionID, true)
	if !ok {
		t.Fatal("unknown digest produced no line")
	}
	if !strings.Contains(line, "digest unknown") {
		t.Errorf("line = %q, want it to say digest unknown", line)
	}
	if strings.Contains(line, "coverage unverified") {
		t.Errorf("line = %q, said both digest unknown and coverage unverified for the same "+
			"underlying reasons", line)
	}
}

func TestLineTruncated(t *testing.T) {
	d := cleanDigest()
	d.Truncated = true

	line, ok := Line(d, d.SessionID, true)
	if !ok {
		t.Fatal("truncated digest produced no line")
	}
	if !strings.Contains(line, "truncated") {
		t.Errorf("line = %q, want it to say truncated", line)
	}
}

// TestLineFiresOnRecordedFailureNamingTheCount matches the spec's own
// worked example: "1 recorded failure. 2 declarations without recorded
// execution."
func TestLineFiresOnRecordedFailureNamingTheCount(t *testing.T) {
	d := cleanDigest()
	d.SilentFailures = report.SilentFailures{Fires: true, Failed: 1, AbsentWords: []string{"fail", "error"}}
	d.Declarations.WithoutExecution = []digest.Unexecuted{
		{ToolUseID: "toolu_1", PermissionMode: "default"},
		{ToolUseID: "toolu_2", PermissionMode: "default"},
	}

	line, ok := Line(d, d.SessionID, true)
	if !ok {
		t.Fatal("expected a line")
	}
	if !strings.Contains(line, "1 recorded failure") {
		t.Errorf("line = %q, want %q", line, "1 recorded failure")
	}
	if !strings.Contains(line, "2 declarations without recorded execution") {
		t.Errorf("line = %q, want %q", line, "2 declarations without recorded execution")
	}
	// Never the inference this exists to avoid: a specific failure named as
	// unacknowledged, rather than a count set beside a vocabulary check.
	if strings.Contains(strings.ToLower(line), "did not mention") {
		t.Errorf("line = %q asserts an inference about intent, not a record", line)
	}
}

func TestLineSecondLineIndentMatchesThePrefix(t *testing.T) {
	d := cleanDigest()
	d.Truncated = true
	line, ok := Line(d, "abc123", true)
	if !ok {
		t.Fatal("expected a line")
	}
	rows := strings.SplitN(line, "\n", 2)
	if len(rows) != 2 {
		t.Fatalf("line has %d rows, want 2:\n%s", len(rows), line)
	}
	indent := len(rows[1]) - len(strings.TrimLeft(rows[1], " "))
	if indent != len([]rune(prefix)) {
		t.Errorf("second line indent = %d, want %d (len of %q)", indent, len([]rune(prefix)), prefix)
	}
	if !strings.Contains(rows[1], "--session abc123") {
		t.Errorf("second row = %q, want the session id passed through", rows[1])
	}
}

func TestSanitizeSessionID(t *testing.T) {
	cases := []struct{ in, want string }{
		{"a3f2", "a3f2"},
		{"sess-1_2.3:4", "sess-1_2.3:4"},
		{"evil\r\n\x1b[31mFAKE", "evil31mFAKE"},
		{"has space", "hasspace"},
		{"", "unknown"},
	}
	for _, c := range cases {
		if got := sanitizeSessionID(c.in); got != c.want {
			t.Errorf("sanitizeSessionID(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// TestLinePointsAtACommandTheOriginHas: the line names a command the user who
// sees it can run. /rashomon:report is a plugin skill; a settings install has
// no plugin, but does have the rashomon binary it was installed from. A
// plugin's binary lives under the plugin's own directory, not on PATH, so the
// reverse holds there.
func TestLinePointsAtACommandTheOriginHas(t *testing.T) {
	d := cleanDigest()
	d.Truncated = true
	for _, tc := range []struct {
		fromPlugin bool
		want, not  string
	}{
		{fromPlugin: false, want: "rashomon report --session abc123", not: "/rashomon:report"},
		{fromPlugin: true, want: "/rashomon:report --session abc123"},
	} {
		line, ok := Line(d, "abc123", tc.fromPlugin)
		if !ok {
			t.Fatal("expected a line")
		}
		if !strings.Contains(line, tc.want) {
			t.Errorf("fromPlugin=%v: line = %q, want it to point at %q", tc.fromPlugin, line, tc.want)
		}
		if tc.not != "" && strings.Contains(line, tc.not) {
			t.Errorf("fromPlugin=%v: line = %q points at %q, which this origin does not have", tc.fromPlugin, line, tc.not)
		}
	}
}

// TestLineTestBending: each test-bending pattern is one short sentence joined
// into the line, naming the first pair by seq, with the same pointer. The
// wording is the record's -- "no recorded file edit between", never "nothing changed"
// -- and a truncated list still speaks, counted from its omitted total.
func TestLineTestBending(t *testing.T) {
	for _, tc := range []struct {
		name string
		tb   digest.TestBending
		want []string
	}{
		{
			name: "tests only then green",
			tb:   digest.TestBending{TestsOnlyThenGreen: []report.SeqPair{{12, 19}}},
			want: []string{"test command failed, then the only recorded edits were to files named like tests, then it passed (#12 → #19)."},
		},
		{
			name: "flaky, passed first",
			tb:   digest.TestBending{Flaky: []report.FlakyPair{{Seqs: report.SeqPair{8, 14}}}},
			want: []string{"same test command had both outcomes with no recorded file edit between (#8 passed, #14 failed)."},
		},
		{
			name: "flaky, failed first",
			tb:   digest.TestBending{Flaky: []report.FlakyPair{{Seqs: report.SeqPair{8, 14}, FirstFailed: true}}},
			want: []string{"same test command had both outcomes with no recorded file edit between (#8 failed, #14 passed)."},
		},
		{
			name: "both, and more than one",
			tb: digest.TestBending{
				TestsOnlyThenGreen: []report.SeqPair{{12, 19}, {20, 25}},
				Flaky:              []report.FlakyPair{{Seqs: report.SeqPair{8, 14}}}, FlakyOmitted: 2,
			},
			want: []string{
				"then it passed (#12 → #19, 1 more). same test command",
				"no recorded file edit between (#8 passed, #14 failed, 2 more).",
			},
		},
		{
			name: "cut whole by truncate",
			tb:   digest.TestBending{TestsOnlyThenGreenOmitted: 3},
			want: []string{"then it passed (3 times)"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := cleanDigest()
			d.TestBending = tc.tb
			line, ok := Line(d, d.SessionID, false)
			if !ok {
				t.Fatal("a test-bending pattern produced no line")
			}
			for _, w := range tc.want {
				if !strings.Contains(line, w) {
					t.Errorf("line = %q\nwant it to contain %q", line, w)
				}
			}
			if strings.Contains(line, "nothing changed") {
				t.Errorf("line = %q claims nothing changed, which the record cannot see", line)
			}
			if !strings.Contains(line, "→ rashomon report --session sess-1") {
				t.Errorf("line = %q, want the same report pointer", line)
			}
			if n := strings.Count(line, "\n"); n != 1 {
				t.Errorf("line has %d newlines, want the sentence line and the pointer line", n)
			}
		})
	}
}

// TestLineNoTestBendingIsSilent: empty lists and zero omitted counts are not a
// trigger. Break: test the lists for nil rather than length, and every digest
// read back from JSON speaks.
func TestLineNoTestBendingIsSilent(t *testing.T) {
	d := cleanDigest()
	d.TestBending = digest.TestBending{TestsOnlyThenGreen: []report.SeqPair{}, Flaky: []report.FlakyPair{}}
	if line, ok := Line(d, d.SessionID, true); ok {
		t.Errorf("a digest with no test-bending pair produced a line: %q", line)
	}
}

// The line's count is the failures that made it fire, not the lookups beside
// them: the report lists those, and "3 recorded failures" over one failed
// build and two failed Reads of a directory would point at three things when
// one is the finding. Break: print Failed and this reads 3.
func TestLineCountsOnlyTheFailuresThatFire(t *testing.T) {
	d := cleanDigest()
	d.SilentFailures = report.SilentFailures{Fires: true, Failed: 3, FailedLookups: 2, FinalMessageAvailable: true, AbsentWords: []string{"fail"}}

	line, ok := Line(d, d.SessionID, false)
	if !ok {
		t.Fatal("expected a line")
	}
	if !strings.Contains(line, "rashomon: 1 recorded failure.") {
		t.Errorf("line = %q, want %q", line, "1 recorded failure.")
	}
}
