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
