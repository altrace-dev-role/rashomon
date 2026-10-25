package report

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

// TestText_HarnessUnknown renders the three states of the BRD-4 line (ruling
// 2026-10-24), each distinct: a non-zero count says it is a detector gap to
// chase, zero says none, and an unreadable counter is unknown, never none.
func TestText_HarnessUnknown(t *testing.T) {
	one, three, zero := 1, 3, 0
	for _, tc := range []struct {
		name string
		n    *int
		want string
	}{
		{"one", &one, "harness unknown: 1 session in the last 30 days (each one is a detector gap to chase)"},
		{"several", &three, "harness unknown: 3 sessions in the last 30 days (each one is a detector gap to chase)"},
		{"none", &zero, "harness unknown: none"},
		{"unknown", nil, "harness unknown: unknown (harness_counts.json could not be read)"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rep := &Report{HarnessUnknownSessions: tc.n, Sessions: []Session{{SessionID: "sess-1"}}}
			var out bytes.Buffer
			if err := Text(&out, rep); err != nil {
				t.Fatal(err)
			}
			lines := strings.Split(out.String(), "\n")
			// Header, the sessionless line, then this one.
			if len(lines) < 3 || lines[2] != tc.want {
				t.Errorf("the third line = %q, want %q\n%s", lines[min(2, len(lines)-1)], tc.want, out.String())
			}
		})
	}
}

// TestText_HarnessUnknownWithNoSessions: "no sessions recorded" is the whole
// answer for a clean store, but a non-zero or unreadable count is still said,
// because a session that was counted and never recorded is the gap to chase.
func TestText_HarnessUnknownWithNoSessions(t *testing.T) {
	two := 2
	for name, tc := range map[string]struct {
		n        *int
		wantLine string
	}{
		"clean":      {n: intp(0)},
		"non-zero":   {n: &two, wantLine: "harness unknown: 2 sessions in the last 30 days"},
		"unreadable": {n: nil, wantLine: "harness unknown: unknown"},
	} {
		t.Run(name, func(t *testing.T) {
			rep := Empty(time.Now())
			rep.HarnessUnknownSessions = tc.n
			var out bytes.Buffer
			if err := Text(&out, rep); err != nil {
				t.Fatal(err)
			}
			has := strings.Contains(out.String(), "harness unknown:")
			if tc.wantLine == "" && has {
				t.Errorf("a clean empty report speaks of the harness:\n%s", out.String())
			}
			if tc.wantLine != "" && !strings.Contains(out.String(), tc.wantLine) {
				t.Errorf("report lacks %q:\n%s", tc.wantLine, out.String())
			}
		})
	}
}

func intp(n int) *int { return &n }

// TestEmptyReport_HarnessUnknownIsARealZero: nothing recorded at a location
// means nothing was counted there either.
func TestEmptyReport_HarnessUnknownIsARealZero(t *testing.T) {
	if n := Empty(time.Now()).HarnessUnknownSessions; n == nil || *n != 0 {
		t.Errorf("Empty: harness unknown sessions = %v, want 0", n)
	}
}

// TestAssemble_CarriesTheHarnessUnknownCount: the read step's count reaches the
// report through Assemble as it does through gather.Build.
func TestAssemble_CarriesTheHarnessUnknownCount(t *testing.T) {
	rep := Assemble(Input{HarnessUnknownSessions: intp(4)}, time.UnixMilli(0))
	if rep.HarnessUnknownSessions == nil || *rep.HarnessUnknownSessions != 4 {
		t.Errorf("Assemble: harness unknown sessions = %v, want 4", rep.HarnessUnknownSessions)
	}
	if rep := Assemble(Input{}, time.UnixMilli(0)); rep.HarnessUnknownSessions != nil {
		t.Errorf("Assemble with no count read: %d, want unknown (nil)", *rep.HarnessUnknownSessions)
	}
}
