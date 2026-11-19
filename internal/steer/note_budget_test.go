package steer

import (
	"bytes"
	"encoding/json"
	"os"
	"reflect"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/steer/pattern"
	"github.com/altrace-dev-role/rashomon/internal/steer/render"
	"github.com/altrace-dev-role/rashomon/internal/steer/render/slot"
)

// gRunFixture is the G run's steer log, projected and scrubbed.
const gRunFixture = "g-run-063112.scrubbed.ndjson"

// lineSpan is the line numbers a through b. A run of them is written as a
// span, not as a literal list: test/acceptance/schema_readers_test.go reads
// five, six and seven written in a row anywhere under internal/ as a list of
// schema versions.
func lineSpan(a, b int) []int {
	out := make([]int, 0, b-a+1)
	for n := a; n <= b; n++ {
		out = append(out, n)
	}
	return out
}

// maskedNote is a masked-status finding under key: a third pattern with no
// reserve, beside regression-location and stale-evidence, and a note that
// reaches the model (integrity would not do: userOnly keeps it a user flag
// at every level, tech 4.2.4 step 5, so it never charges a window).
func maskedNote(t *testing.T, key string, afterSeq int64) Finding {
	t.Helper()
	m, err := render.New(pattern.MaskedStatus, render.MaskedStatusBinding{Gate: "go test",
		Stage: slot.NewEnum(slot.EnumMaskStage, "a pipe")})
	if err != nil {
		t.Fatal(err)
	}
	p, _ := pattern.Priority(pattern.MaskedStatus)
	f := Finding{Pattern: pattern.MaskedStatus, Key: key, Priority: p, MaxLevel: LevelNudge, StopForm: StopFormNone,
		Message: m, AfterSeq: afterSeq}
	if userOnly(f) || m.Text() == "" {
		t.Fatal("premise: the third pattern must be a note that reaches the model")
	}
	return f
}

// mustPrint routes f alone and fails unless its text reached the agent.
func mustPrint(t *testing.T, ctx *Context, f Finding) {
	t.Helper()
	if out := Route(ctx, []Finding{f}); out.AdditionalContext != f.Message.Text() {
		t.Fatalf("%s/%s at seq %d printed %+v, want its text", f.Pattern, f.Key, f.AfterSeq, out)
	}
}

// mustHold routes f alone and fails unless nothing printed and its line is
// suppressed by by, a budget's reason.
func mustHold(t *testing.T, ctx *Context, f Finding, by SuppressedBy) {
	t.Helper()
	if out := Route(ctx, []Finding{f}); out != (Output{}) {
		t.Fatalf("%s/%s at seq %d printed %+v, want nothing (%s)", f.Pattern, f.Key, f.AfterSeq, out, by)
	}
	wantLine(t, lastLines(t, ctx, 1)[0], LineStateSuppressed, by, DeliverySuppressedBudget)
}

// TestNoteBudget_Window: three notes reach the agent in one window of
// windowSeqs record seqs; a fourth inside it prints nothing
// and its line says budget_window; once the oldest has left the window, a
// note prints. A note at after_seq a is in the window of a finding at s while
// a > s - windowSeqs, so the edge is read on both sides: s = a + windowSeqs - 1
// is held, s = a + windowSeqs prints. Break: count a note at the edge
// (a >= s - windowSeqs), and the last note prints nothing; keep two notes in
// the scan, not three, and the fourth prints.
func TestNoteBudget_Window(t *testing.T) {
	ctx := routerContext(t)
	const a = 10
	mustPrint(t, ctx, regressionNote(t, "go test", a, "r1", LevelNudge))
	mustPrint(t, ctx, staleNote(t, "go test", a+1))
	mustPrint(t, ctx, maskedNote(t, "m1", a+2))
	mustHold(t, ctx, regressionNote(t, "go vet", a+windowSeqs-1, "r2", LevelNudge), SuppressedByBudgetWindow)
	mustPrint(t, ctx, regressionNote(t, "go vet", a+windowSeqs, "r3", LevelNudge))
}

// TestNoteBudget_ALaterSeqWrittenFirstCounts: a note counts while its
// after_seq is above s - windowSeqs, with no upper bound, so a parallel
// call's note written before a finding of a lower seq is in that finding's
// window, and no window of windowSeqs seqs holds more than three notes
// whatever order the lines are written in. Break: count only after_seq <= s,
// and the fourth note prints: four notes inside seqs 290 to 302.
func TestNoteBudget_ALaterSeqWrittenFirstCounts(t *testing.T) {
	ctx := routerContext(t)
	mustPrint(t, ctx, regressionNote(t, "go test", 300, "r1", LevelNudge))
	mustPrint(t, ctx, staleNote(t, "go test", 301))
	mustPrint(t, ctx, maskedNote(t, "m1", 302))
	mustHold(t, ctx, regressionNote(t, "go vet", 290, "r2", LevelNudge), SuppressedByBudgetWindow)
}

// TestNoteBudget_RepeatReserve: a pattern with a note in the window reserves
// one more slot, so no pattern takes a whole window. With
// two notes in the window, one of them stale-evidence, a second
// stale-evidence note prints nothing and its line says budget_repeat, while a
// note of a third pattern still prints; with one note in the window the
// repeat prints. Blast-radius, which reserves one, reserves two on a repeat:
// its second note is held with only its first in the window, where the
// shipped rule gave it. Break: drop the repeat reserve, and the second
// stale-evidence note and the second blast-radius note print.
func TestNoteBudget_RepeatReserve(t *testing.T) {
	t.Run("held at two in the window", func(t *testing.T) {
		ctx := routerContext(t)
		mustPrint(t, ctx, staleNote(t, "go test", 10))
		mustPrint(t, ctx, regressionNote(t, "go vet", 11, "r1", LevelNudge))
		mustHold(t, ctx, staleNote(t, "go build", 12), SuppressedByBudgetRepeat)
		mustPrint(t, ctx, maskedNote(t, "m1", 13))
	})
	t.Run("given at one in the window", func(t *testing.T) {
		ctx := routerContext(t)
		mustPrint(t, ctx, staleNote(t, "go test", 10))
		mustPrint(t, ctx, staleNote(t, "go build", 11))
	})
	t.Run("blast-radius reserves two on a repeat", func(t *testing.T) {
		ctx := routerContext(t)
		mustPrint(t, ctx, blastNote(t, "example.com/m/a@0123456789ab", 10))
		mustHold(t, ctx, blastNote(t, "example.com/m/b@0123456789ab", 11), SuppressedByBudgetRepeat)
		mustPrint(t, ctx, staleNote(t, "go test", 12))
	})
	t.Run("the window's reason when both bite", func(t *testing.T) {
		ctx := routerContext(t)
		mustPrint(t, ctx, staleNote(t, "go test", 10))
		mustPrint(t, ctx, regressionNote(t, "go vet", 11, "r1", LevelNudge))
		mustPrint(t, ctx, maskedNote(t, "m1", 12))
		mustHold(t, ctx, staleNote(t, "go build", 13), SuppressedByBudgetWindow)
	})
}

// TestNoteBudget_CountClauses walks the rule's count clauses with 0, 1 and 2
// notes in the window, for a plain pattern and for
// blast-radius (reserve 1), new to the window and repeated in it: n + reserve
// >= 3 is budget_window, else a repeat with n + reserve + 1 >= 3 is
// budget_repeat, else the note prints. The reason texts are read against the
// held rows (docs-out/evidence-m3/pr4/nb-strings-r1.txt).
func TestNoteBudget_CountClauses(t *testing.T) {
	regression := func(t *testing.T, seq int64) Finding { return regressionNote(t, "go vet", seq, "r1", LevelNudge) }
	masked := func(t *testing.T, seq int64) Finding { return maskedNote(t, "m1", seq) }
	stale := func(t *testing.T, seq int64) Finding { return staleNote(t, "go build", seq) }
	blast := func(t *testing.T, seq int64) Finding { return blastNote(t, "example.com/m/a@0123456789ab", seq) }
	plainNote := func(t *testing.T) Finding { return staleNote(t, "go test", 20) }
	blastUnder := func(t *testing.T) Finding { return blastNote(t, "example.com/m/b@0123456789ab", 20) }
	type before func(*testing.T, int64) Finding
	for _, tc := range []struct {
		name  string
		prior []before
		note  func(*testing.T) Finding
		by    SuppressedBy // "" prints
	}{
		{"0 plain", nil, plainNote, ""},
		{"0 blast-radius", nil, blastUnder, ""},
		{"1 plain new", []before{regression}, plainNote, ""},
		{"1 plain repeat", []before{stale}, plainNote, ""},
		{"1 blast-radius new", []before{regression}, blastUnder, ""},
		{"1 blast-radius repeat", []before{blast}, blastUnder, SuppressedByBudgetRepeat},
		{"2 plain new", []before{regression, masked}, plainNote, ""},
		{"2 plain repeat", []before{regression, stale}, plainNote, SuppressedByBudgetRepeat},
		{"2 blast-radius new", []before{regression, masked}, blastUnder, SuppressedByBudgetWindow},
		{"2 blast-radius repeat", []before{regression, blast}, blastUnder, SuppressedByBudgetWindow},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := routerContext(t)
			for i, f := range tc.prior {
				mustPrint(t, ctx, f(t, int64(10+i)))
			}
			if tc.by == "" {
				mustPrint(t, ctx, tc.note(t))
				return
			}
			mustHold(t, ctx, tc.note(t), tc.by)
		})
	}
}

// TestNoteBudget_KeyHoldsForPrompt: budget_pattern is the prompt's, not the
// window's: a key given at seq 10 is still held two windows
// later.
func TestNoteBudget_KeyHoldsForPrompt(t *testing.T) {
	ctx := routerContext(t)
	mustPrint(t, ctx, regressionNote(t, "go test", 10, "same", LevelNudge))
	mustHold(t, ctx, regressionNote(t, "go vet", 10+2*windowSeqs, "same", LevelNudge), SuppressedByBudgetPattern)
}

// TestNoteBudget_NewPromptResets: a new prompt starts a new count: the window
// and the keys are the prompt's.
func TestNoteBudget_NewPromptResets(t *testing.T) {
	ctx := routerContext(t)
	mustPrint(t, ctx, regressionNote(t, "go test", 10, "r1", LevelNudge))
	mustPrint(t, ctx, staleNote(t, "go test", 11))
	mustPrint(t, ctx, maskedNote(t, "m1", 12))
	mustHold(t, ctx, regressionNote(t, "go vet", 13, "r2", LevelNudge), SuppressedByBudgetWindow)
	ctx.PromptID = ptr("p2")
	mustPrint(t, ctx, regressionNote(t, "go test", 14, "r1", LevelNudge))
}

// seqLine is a delivered nudge of session s1 at after_seq seq, as the writer
// stores it.
func seqLine(t *testing.T, prompt *string, seq int64, pat, key string) []byte {
	t.Helper()
	b, err := json.Marshal(Line{
		Type: LineType, V: LineVersion, SessionID: "s1", PromptID: prompt, AtUnixMS: 1_000 + seq,
		Pattern: pat, Key: key, Level: LevelNudge.String(), State: LineStateDelivered, AfterSeq: seq,
	})
	if err != nil {
		t.Fatal(err)
	}
	return append(b, '\n')
}

// TestNoteBudget_ScanKeepsTheLargestThree: the scan keeps the three largest
// after_seqs of the prompt's notes, not the last three written, so a line
// written out of order cannot push a note of the window out of the count.
// Break: keep the last three written, and the window of seq 500 counts 2.
func TestNoteBudget_ScanKeepsTheLargestThree(t *testing.T) {
	p := ptr("p1")
	var log []byte
	for _, s := range []int64{300, 299, 10, 298} {
		log = append(log, seqLine(t, p, s, "regression-location", "k")...)
	}
	c := countOf(t, log, p)
	if n, _ := c.spent(500, "stale-evidence"); n != 3 {
		t.Errorf("spent(500) = %d, want 3 (seqs 298, 299, 300)", n)
	}
	if n, _ := c.spent(298+windowSeqs, "stale-evidence"); n != 2 {
		t.Errorf("spent(%d) = %d, want 2 (seq 298 has left the window)", 298+windowSeqs, n)
	}
}

// TestNoteBudget_BriefingOutside: the continuity briefing stays outside the
// window, as it was outside the prompt's count (tech 4.2.5): it spends no
// slot, and its pattern reserves nothing on a repeat.
func TestNoteBudget_BriefingOutside(t *testing.T) {
	p := ptr("p1")
	log := seqLine(t, p, 5, string(pattern.ContinuityBriefing), "briefing")
	log = append(log, seqLine(t, p, 6, "regression-location", "r1")...)
	log = append(log, seqLine(t, p, 7, "stale-evidence", "go test")...)
	c := countOf(t, log, p)
	if n, repeat := c.spent(8, string(pattern.ContinuityBriefing)); n != 2 || repeat {
		t.Errorf("spent(8, briefing) = %d, %v; want 2, false", n, repeat)
	}
	if n, repeat := c.spent(8, "stale-evidence"); n != 2 || !repeat {
		t.Errorf("spent(8, stale-evidence) = %d, %v; want 2, true", n, repeat)
	}
}

// TestNoteBudget_AnUnreadSeqIsSpent: a delivered note whose after_seq is not
// a whole number, or is absent, is in every window: a budget that cannot be
// read counts as spent (tech 4.2.5).
func TestNoteBudget_AnUnreadSeqIsSpent(t *testing.T) {
	for _, seq := range []string{`"after_seq":1.5,`, `"after_seq":"7",`, `"after_seq":null,`, ``} {
		raw := `{"type":"steer","session_id":"s1","prompt_id":"p1","at_unix_ms":1,` + seq +
			`"state":"delivered","level":"nudge","pattern":"regression-location","key":"k"}` + "\n"
		c := countOf(t, []byte(raw), ptr("p1"))
		if n, repeat := c.spent(1_000_000, "regression-location"); n != 1 || !repeat {
			t.Errorf("after_seq %q: spent = %d, %v; want 1, true", seq, n, repeat)
		}
	}
}

// fixtureLines reads a steer-log fixture of test/fixtures/steer-logs.
func fixtureLines(t *testing.T, name string) []Line {
	t.Helper()
	return linesAt(t, "../../test/fixtures/steer-logs/"+name)
}

// linesAt is the steer lines of the log at path.
func linesAt(t *testing.T, path string) []Line {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var out []Line
	for _, raw := range bytes.Split(bytes.TrimSuffix(b, []byte("\n")), []byte("\n")) {
		var l Line
		if err := json.Unmarshal(raw, &l); err != nil {
			t.Fatal(err)
		}
		out = append(out, l)
	}
	return out
}

// TestNoteBudget_GRunReplay replays the G run's 22 steer lines through the
// budget, one call per at_unix_ms, in the order the router wrote them. The
// fixture is the run's steer.ndjson (T2 smoke 063112-g, sha16
// fa29614bd11084b1, copied to docs-out/evidence-m2/t2-smoke-063112-g-store)
// projected to the fields the scan reads, then scrubbed by test/tools/scrub
// (its name says so). The deliveries were fixed before this code under the
// window of NOTE-BUDGET (W = 225, 75 calls of three records), by a
// simulator that first reproduced the hand table under W = 233, read once
// and not tuned: lines 1, 2, 3, 11, 17 and 22, against 1, 2 and 3 under the
// shipped rule. Line 17 (seq 393) prints because the masked-status note at
// 167 has left the window (393 - 225 = 168); 21 is then a repeat of 17, and
// 22 prints since no line of its key was delivered. Of the rest, five are
// budget_pattern, eight budget_window and three budget_repeat.
// RASHOMON_NB_REPLAY_LOG names another log to replay instead, the real store
// copy, so the forecast is read from it and not from the fixture;
// the result is logged either way, for the diff before and after a
// scrub.
// Limit: the candidates are the recorded lines; a gate that fires on news
// alone may raise others once more notes are given.
func TestNoteBudget_GRunReplay(t *testing.T) {
	lines := fixtureLines(t, gRunFixture)
	if p := os.Getenv("RASHOMON_NB_REPLAY_LOG"); p != "" {
		lines = linesAt(t, p)
	}
	if len(lines) != 22 {
		t.Fatalf("premise: the fixture holds %d lines, want 22", len(lines))
	}
	var log []byte
	got := map[string][]int{}
	for i := 0; i < len(lines); {
		j := i + 1
		for j < len(lines) && lines[j].AtUnixMS == lines[i].AtUnixMS {
			j++
		}
		have, err := countBudget(bytes.NewReader(log), &Context{SessionID: lines[i].SessionID, PromptID: lines[i].PromptID})
		if err != nil {
			t.Fatal(err)
		}
		plans := make([]plan, 0, j-i)
		for _, l := range lines[i:j] {
			f := Finding{Pattern: PatternID(l.Pattern), Kind: Kind(l.Kind), Key: l.Key, AfterSeq: l.AfterSeq}
			plans = append(plans, plan{f: f, key: findingKey{l.Pattern, l.Kind, l.Key}, level: LevelNudge, decide: true,
				direct: ownChannel(f.Pattern, Event(l.Event))})
		}
		charge(plans, have)
		for k := range plans {
			p, l := &plans[k], lines[i+k]
			name := string(p.state)
			if p.by != nil {
				name = string(*p.by)
			}
			got[name] = append(got[name], i+k+1)
			b, err := json.Marshal(Line{Type: LineType, V: LineVersion, SessionID: l.SessionID, PromptID: l.PromptID,
				AtUnixMS: l.AtUnixMS, Pattern: l.Pattern, Kind: l.Kind, Key: l.Key, Level: l.Level,
				AfterSeq: l.AfterSeq, State: p.state, SuppressedBy: p.by})
			if err != nil {
				t.Fatal(err)
			}
			log = append(log, append(b, '\n')...)
		}
		i = j
	}
	want := map[string][]int{
		"delivered":      {1, 2, 3, 11, 17, 22},
		"budget_window":  append(lineSpan(4, 10), 14),
		"budget_repeat":  {15, 16, 21},
		"budget_pattern": {12, 13, 18, 19, 20},
	}
	t.Logf("replay = %v", got)
	if !reflect.DeepEqual(got, want) {
		t.Errorf("replay = %v\nwant     %v", got, want)
	}
	var shipped []int
	for i, l := range lines {
		if l.State == LineStateDelivered {
			shipped = append(shipped, i+1)
		}
	}
	if !reflect.DeepEqual(shipped, []int{1, 2, 3}) {
		t.Errorf("premise: the shipped rule delivered lines %v, want 1, 2, 3", shipped)
	}
}
