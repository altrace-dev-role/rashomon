package measure

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/steer"
	"github.com/altrace-dev-role/rashomon/internal/steer/pattern"
)

// given is a note given to the agent on prompt at after_seq seq.
func given(prompt string, pat pattern.ID, seq int64) line {
	d := steer.DeliveryDelivered
	return line{Type: steer.LineType, Pattern: string(pat), Key: fmt.Sprint(pat, seq), PromptID: &prompt,
		Level: steer.LevelNudge.String(), AfterSeq: seq, State: steer.LineStateDelivered, Delivery: &d}
}

// withheld is a note arm off withheld on prompt at after_seq seq: what the
// steer arm gave, so it spends the window as a given note does.
func withheld(prompt string, pat pattern.ID, seq int64) line {
	by := steer.SuppressedByArmOff
	return line{Type: steer.LineType, Pattern: string(pat), Key: fmt.Sprint(pat, seq), PromptID: &prompt,
		Level: steer.LevelNudge.String(), AfterSeq: seq, State: steer.LineStateSuppressed, SuppressedBy: &by}
}

// steerFixture reads a steer-log fixture of test/fixtures/steer-logs.
func steerFixture(t *testing.T, name string) []line {
	t.Helper()
	b, err := os.ReadFile("../../test/fixtures/steer-logs/" + name)
	if err != nil {
		t.Fatal(err)
	}
	var out []line
	for _, raw := range bytes.Split(bytes.TrimSuffix(b, []byte("\n")), []byte("\n")) {
		var l line
		if err := json.Unmarshal(raw, &l); err != nil {
			t.Fatal(err)
		}
		out = append(out, l)
	}
	return out
}

// TestCheck_Window: the checker holds a session's log to the router's window
// limit and to nothing else (NOTE-BUDGET): a note given or withheld with
// three of its prompt's notes already above its after_seq less
// steer.NoteWindowSeqs is an error, the briefing counts in no window, and a
// log written under the shipped rule of three notes a prompt reads as valid:
// the alpha.2 log of release step e, and the G run's, three notes inside 167
// seqs. Break: return the checker to a count per prompt, and a fourth note
// past the window and a briefing beside three notes read as errors.
func TestCheck_Window(t *testing.T) {
	const w = steer.NoteWindowSeqs
	rl, se, ms := pattern.RegressionLocation, pattern.StaleEvidence, pattern.MaskedStatus
	three := func(p string, a int64) []line { return []line{given(p, rl, a), given(p, se, a+1), given(p, ms, a+2)} }
	for _, tc := range []struct {
		name  string
		lines []line
		over  bool
	}{
		{"three in one window", three("p1", 10), false},
		{"a fourth inside the window", append(three("p1", 10), given("p1", rl, 10+w-1)), true},
		{"a fourth once the oldest has left", append(three("p1", 10), given("p1", rl, 10+w)), false},
		{"a fourth at a lower seq written last", append(three("p1", 300), given("p1", rl, 290)), true},
		{"a withheld note spends the window", []line{given("p1", rl, 10), given("p1", se, 11), withheld("p1", ms, 12), given("p1", rl, 13)}, true},
		{"the briefing is outside", append([]line{given("p1", pattern.ContinuityBriefing, 5)}, three("p1", 10)...), false},
		{"a new prompt starts a new count", append(three("p1", 10), given("p2", rl, 13)), false},
		{"the alpha.2 log of release step e", steerFixture(t, "alpha2-step-e.scrubbed.ndjson"), false},
		{"the G run, written under the shipped rule", steerFixture(t, "g-run-063112.scrubbed.ndjson"), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := &Side{SessionID: "s"}
			err := s.notes(tc.lines)
			switch {
			case tc.over && (err == nil || !strings.Contains(err.Error(), "window")):
				t.Errorf("notes = %v, want an error naming the window", err)
			case !tc.over && err != nil:
				t.Errorf("notes = %v, want none", err)
			}
		})
	}
}
