package acceptance

// Detector precision: the fixture format, the labelling sheet and the
// statistics. This file is the pure half -- no process, no store -- so every
// rule in it is tested without the binary; detector_precision_test.go is the
// half that replays a fixture through the built binary and reads which
// detectors fired.
//
// A detector here is anything that puts a judgement in front of a user: a
// sentence of the end-of-turn line, or a report section that says a thing was
// found. Its precision is the share of its fires that a person, reading the
// run and the final message, agrees were right. Nothing else counts as a
// measurement:
//
//   - a fixture is a minimal recorded run (the hook payloads a turn produced,
//     written compactly as calls) and the transcript excerpt the detector
//     reads (the turn's final assistant message), scrubbed, under
//     test/corpus/detectors/<detector>/;
//   - its label is the ground truth for that run: true_positive (the detector
//     fires and is right), false_positive (it fires and is wrong),
//     true_negative (it stays silent and is right) or false_negative (it
//     stays silent and should not have). The harness holds the label to what
//     the detector really does, so a label that presupposes a fire which does
//     not happen fails the build;
//   - labelled_by names who decided. "seed" means the label was derived from
//     a written rule by whoever built the fixture, with no person confirming
//     it: a seed label is checked, printed and never counted. A person counts
//     only by replacing "seed" with their own name after reading the fixture;
//   - a real fire seen outside a fixture goes in the labelling sheet
//     (test/corpus/detectors/labelling-sheet.tsv), one row per fire, and
//     counts the same way once its labelled_by is a person's.
//
// A detector is measured once it has minLabelledFires counted fires. Below
// that its row says unmeasured, however good its interval looks.

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
	"unicode/utf8"
)

// The labels. The first three are the three a precision figure is built from;
// false_negative is recorded so a blind spot has a fixture, and is never a
// fire.
const (
	labelTruePositive  = "true_positive"
	labelFalsePositive = "false_positive"
	labelTrueNegative  = "true_negative"
	labelFalseNegative = "false_negative"
)

// labelledBySeed marks a label nobody has confirmed.
const labelledBySeed = "seed"

// minLabelledFires is the number of counted fires below which a detector is
// unmeasured. A person-confirmed label counts; a seed label does not.
const minLabelledFires = 30

// Caps on what the parsers read, named so the over-cap path is tested.
const (
	maxFixtureBytes = 1 << 20
	maxFixtureCalls = 400
	maxCallRepeat   = 400
	maxSheetBytes   = 4 << 20
	maxSheetRows    = 20000
	maxFieldBytes   = 64 << 10
)

// fpClasses is the closed set of false-positive classes. A false positive
// names one, so the classes can be counted and the fixes ordered by how many
// fires each one costs; "other" is for a class not yet named, and its reason
// says what it was.
var fpClasses = []string{
	"non_english",
	"recovered_command",
	"empty_search",
	"status_query",
	"denied_call",
	"not_a_claim",
	"pass_word_other_sense",
	"output_shows_pass",
	"external_fix",
	"test_file_error",
	"session_predates_install",
	"other",
}

// callOutcomes is the closed set of ways a call can end in a fixture.
//
//	ok         PostToolUse: the call ran and the tool reported success.
//	failed     PostToolUseFailure: the call ran and failed (a shell call
//	           carries "Exit code N" and its output, as Claude Code sends it).
//	denied     a declaration and nothing after it: the user refused the call.
//	unrecorded a declaration and nothing after it: the call started and no
//	           ending arrived. The record cannot tell this from denied, which
//	           is why both exist: the fixture says which it was.
var callOutcomes = []string{"ok", "failed", "denied", "unrecorded"}

// fixtureCall is one tool call of a fixture's turn, in order.
type fixtureCall struct {
	Tool     string         `json:"tool"`
	Input    map[string]any `json:"input"`
	Outcome  string         `json:"outcome"`
	ExitCode int            `json:"exit_code,omitempty"`
	// Output is what a reader of the run would have seen of the call: a
	// shell call's stdout, or a failed call's error text. The recorder keeps
	// none of it; it is here so the person labelling can judge.
	Output string `json:"output,omitempty"`
	// Repeat replays the call that many times with distinct tool_use_ids.
	Repeat int `json:"repeat,omitempty"`
}

// detectorFixture is one labelled run.
type detectorFixture struct {
	Detector   string `json:"detector"`
	Label      string `json:"label"`
	Reason     string `json:"reason"`
	LabelledBy string `json:"labelled_by"`
	FPClass    string `json:"fp_class,omitempty"`
	// Provenance says where the run came from: which committed capture or
	// test payload it reuses, or the rule it was constructed from.
	Provenance string `json:"provenance"`
	// Capture, when set, names a committed capture under test/fixtures
	// (relative to it) to replay whole instead of Calls.
	Capture string `json:"capture,omitempty"`
	// ProbeStart false leaves the session's start probe unrecorded.
	ProbeStart   *bool         `json:"probe_start,omitempty"`
	FinalMessage string        `json:"final_message"`
	Calls        []fixtureCall `json:"calls,omitempty"`
}

func (f detectorFixture) probeStart() bool { return f.ProbeStart == nil || *f.ProbeStart }

// isFire reports a label that presupposes the detector fired.
func isFireLabel(label string) bool {
	return label == labelTruePositive || label == labelFalsePositive
}

func inSet(s string, set []string) bool {
	for _, v := range set {
		if s == v {
			return true
		}
	}
	return false
}

// validLabel reports one of the four labels.
func validLabel(s string) bool {
	return inSet(s, []string{labelTruePositive, labelFalsePositive, labelTrueNegative, labelFalseNegative})
}

// checkLabelling is the rules a label, its reason, its author and its false-
// positive class answer to, shared by the fixture and the sheet so a row
// cannot say in one place what the other refuses.
func checkLabelling(label, reason, by, class string) error {
	switch {
	case !validLabel(label):
		return fmt.Errorf("label %q is none of true_positive, false_positive, true_negative, false_negative", label)
	case strings.TrimSpace(reason) == "":
		return errors.New("reason is empty: a label without its reason cannot be confirmed")
	case strings.TrimSpace(by) == "":
		return errors.New(`labelled_by is empty: "seed" for an unconfirmed label, or the person's name`)
	case label == labelFalsePositive && class == "":
		return errors.New("a false_positive names its fp_class")
	case label != labelFalsePositive && class != "":
		return fmt.Errorf("fp_class %q on a %s: only a false_positive has a class", class, label)
	case class != "" && !inSet(class, fpClasses):
		return fmt.Errorf("fp_class %q is not in the closed set %v", class, fpClasses)
	}
	for _, v := range []string{reason, by} {
		if len(v) > maxFieldBytes || !utf8.ValidString(v) || strings.ContainsAny(v, "\x00") {
			return errors.New("a field is over its cap, is not UTF-8, or holds NUL")
		}
	}
	return nil
}

// parseFixture reads one fixture file of the detector's directory. Strict:
// an unknown key, a second document or a field out of its set is an error,
// and nothing is a default except probe_start.
func parseFixture(detector string, b []byte) (detectorFixture, error) {
	var f detectorFixture
	if len(b) > maxFixtureBytes {
		return f, fmt.Errorf("fixture is %d bytes, over the %d cap", len(b), maxFixtureBytes)
	}
	// The decoder repairs invalid UTF-8 into U+FFFD, so the bytes are checked
	// before it sees them: a fixture holds what its author wrote.
	if !utf8.Valid(b) {
		return f, errors.New("fixture is not UTF-8")
	}
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&f); err != nil {
		return f, fmt.Errorf("decoding: %w", err)
	}
	if dec.More() {
		return f, errors.New("more than one JSON document")
	}
	if f.Detector != detector {
		return f, fmt.Errorf("detector %q in a %q directory", f.Detector, detector)
	}
	if err := checkLabelling(f.Label, f.Reason, f.LabelledBy, f.FPClass); err != nil {
		return f, err
	}
	if strings.TrimSpace(f.Provenance) == "" {
		return f, errors.New("provenance is empty: say which capture, payload or rule the run came from")
	}
	if len(f.FinalMessage) > maxFieldBytes || !utf8.ValidString(f.FinalMessage) || strings.Contains(f.FinalMessage, "\x00") {
		return f, errors.New("final_message is over its cap, is not UTF-8, or holds NUL")
	}
	if f.Capture != "" && f.Capture != realCaptureName {
		return f, fmt.Errorf("capture %q is not a replayable capture (%q)", f.Capture, realCaptureName)
	}
	if f.Capture != "" && len(f.Calls) > 0 {
		return f, errors.New("capture and calls are alternatives: give one (no calls at all is a turn that used no tool)")
	}
	if len(f.Calls) > maxFixtureCalls {
		return f, fmt.Errorf("%d calls, over the %d cap", len(f.Calls), maxFixtureCalls)
	}
	total := 0
	for i, c := range f.Calls {
		if c.Tool == "" {
			return f, fmt.Errorf("call %d names no tool", i)
		}
		if !inSet(c.Outcome, callOutcomes) {
			return f, fmt.Errorf("call %d outcome %q is not one of %v", i, c.Outcome, callOutcomes)
		}
		if c.Repeat < 0 || c.Repeat > maxCallRepeat {
			return f, fmt.Errorf("call %d repeat %d is outside 0..%d", i, c.Repeat, maxCallRepeat)
		}
		if c.ExitCode < 0 || c.ExitCode > 255 || (c.ExitCode != 0 && c.Outcome != "failed") {
			return f, fmt.Errorf("call %d exit_code %d: only a failed call has one, 0..255", i, c.ExitCode)
		}
		if len(c.Output) > maxFieldBytes {
			return f, fmt.Errorf("call %d output is over its cap", i)
		}
		n := c.Repeat
		if n == 0 {
			n = 1
		}
		total += n
		if total > maxFixtureCalls {
			return f, fmt.Errorf("repeats expand to over %d calls", maxFixtureCalls)
		}
	}
	return f, nil
}

// sheetColumns is the labelling sheet's header, in order. One row per real
// fire; the first six columns are filled when the fire is seen, the last
// four by the person who judges it.
var sheetColumns = []string{
	"detector", "fire_ref", "fired_on", "line_text",
	"label", "fp_class", "reason", "labelled_by",
}

// sheetRow is one labelled real fire.
type sheetRow struct {
	Detector, FireRef, FiredOn, LineText string
	Label, FPClass, Reason, LabelledBy   string
}

// parseSheet reads the labelling sheet. A row with no label is a fire not yet
// judged and is skipped, not an error: the sheet is filled in over time. A
// row with a label answers to the same rules as a fixture's, and to the
// registry's detector ids (known).
func parseSheet(b []byte, known func(string) bool) ([]sheetRow, error) {
	if len(b) > maxSheetBytes {
		return nil, fmt.Errorf("sheet is %d bytes, over the %d cap", len(b), maxSheetBytes)
	}
	if !utf8.Valid(b) || bytes.IndexByte(b, 0) >= 0 {
		return nil, errors.New("sheet is not UTF-8 or holds NUL")
	}
	lines := strings.Split(strings.ReplaceAll(string(b), "\r\n", "\n"), "\n")
	if len(lines) == 0 || lines[0] != strings.Join(sheetColumns, "\t") {
		return nil, fmt.Errorf("header is not %q", strings.Join(sheetColumns, "\t"))
	}
	if len(lines) > maxSheetRows {
		return nil, fmt.Errorf("sheet has %d lines, over the %d cap", len(lines), maxSheetRows)
	}
	var rows []sheetRow
	for i, line := range lines[1:] {
		if strings.TrimSpace(line) == "" {
			continue
		}
		cols := strings.Split(line, "\t")
		if len(cols) != len(sheetColumns) {
			return nil, fmt.Errorf("line %d has %d columns, want %d", i+2, len(cols), len(sheetColumns))
		}
		r := sheetRow{cols[0], cols[1], cols[2], cols[3], cols[4], cols[5], cols[6], cols[7]}
		if !known(r.Detector) {
			return nil, fmt.Errorf("line %d names detector %q, which is not in the inventory", i+2, r.Detector)
		}
		if r.Label == "" {
			if r.FPClass != "" || r.Reason != "" || r.LabelledBy != "" {
				return nil, fmt.Errorf("line %d has judgement columns filled but no label", i+2)
			}
			continue
		}
		if !isFireLabel(r.Label) {
			return nil, fmt.Errorf("line %d: a sheet row is a fire the detector made, so its label is true_positive or false_positive, not %q", i+2, r.Label)
		}
		if err := checkLabelling(r.Label, r.Reason, r.LabelledBy, r.FPClass); err != nil {
			return nil, fmt.Errorf("line %d: %w", i+2, err)
		}
		rows = append(rows, r)
	}
	return rows, nil
}

// fireTally is one detector's labelled fires. Counted ones were confirmed by a
// person; seed ones were not, and are kept apart so they are visible and
// cannot become a number.
type fireTally struct {
	TP, FP         int
	SeedTP, SeedFP int
}

// add counts one fire's label. A label that is no fire is ignored here.
func (t *fireTally) add(label, by string) {
	if !isFireLabel(label) {
		return
	}
	seed := by == labelledBySeed
	switch {
	case label == labelTruePositive && seed:
		t.SeedTP++
	case label == labelTruePositive:
		t.TP++
	case seed:
		t.SeedFP++
	default:
		t.FP++
	}
}

// N is the counted fires.
func (t fireTally) N() int { return t.TP + t.FP }

// z95 is the standard normal quantile for a two-sided 95% interval.
const z95 = 1.959963984540054

// wilson95 is the Wilson score interval for k true of n, at 95%. ok is false
// for n == 0, where there is no interval to give. Wilson and not the normal
// approximation because it stays inside [0, 1] and is honest at small n and
// at p near 1, which is exactly where a detector's precision lives.
func wilson95(k, n int) (lo, hi float64, ok bool) {
	if n <= 0 || k < 0 || k > n {
		return 0, 0, false
	}
	nf, p := float64(n), float64(k)/float64(n)
	z2 := z95 * z95
	den := 1 + z2/nf
	centre := (p + z2/(2*nf)) / den
	half := z95 * math.Sqrt(p*(1-p)/nf+z2/(4*nf*nf)) / den
	return math.Max(0, centre-half), math.Min(1, centre+half), true
}

// precisionRow is one printed line of the per-detector table.
type precisionRow struct {
	Detector string
	Tally    fireTally
	// Fixtures is false for a detector with no fixture directory at all.
	Fixtures bool
}

// status is measured or unmeasured, and for unmeasured says why in numbers.
func (r precisionRow) status() string {
	if r.Tally.N() >= minLabelledFires {
		return "measured"
	}
	s := fmt.Sprintf("unmeasured: %d of %d labelled fires", r.Tally.N(), minLabelledFires)
	if !r.Fixtures {
		return s + "; no fixtures"
	}
	if seed := r.Tally.SeedTP + r.Tally.SeedFP; seed > 0 {
		s += fmt.Sprintf("; %d seed fires (%d TP, %d FP) await a person", seed, r.Tally.SeedTP, r.Tally.SeedFP)
	}
	return s
}

// String renders the row: detector, counted N, precision, interval, status.
// Precision and interval are printed from counted fires only, and as n/a at
// N == 0: a seed label is never a number.
func (r precisionRow) String() string {
	prec, ival := "n/a", "n/a"
	if n := r.Tally.N(); n > 0 {
		lo, hi, _ := wilson95(r.Tally.TP, n)
		prec = fmt.Sprintf("%.3f", float64(r.Tally.TP)/float64(n))
		ival = fmt.Sprintf("[%.3f, %.3f]", lo, hi)
	}
	return fmt.Sprintf("%-31s N=%-3d precision=%-6s wilson95=%-16s %s", r.Detector, r.Tally.N(), prec, ival, r.status())
}
