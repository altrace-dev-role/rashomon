package acceptance

import (
	"math"
	"strings"
	"testing"
)

// The pure half of detector precision, tested without the binary: the Wilson
// interval, what counts, when a detector is measured, and what the fixture
// and sheet parsers refuse. Every expected value below was computed from the
// stated rule before it was typed; the Wilson values by a second algebraic
// route (the closed form lo/hi = (2np + z^2 -/+ z*sqrt(z^2 + 4np(1-p))) /
// (2(n + z^2)), run in Python), not by reading wilson95.

// TestDetectorPrecision_Wilson: known intervals at n = 1, 10 and 30, at both
// ends and in the middle, and the two inputs that have no interval.
func TestDetectorPrecision_Wilson(t *testing.T) {
	for _, tc := range []struct {
		k, n   int
		lo, hi float64
	}{
		{30, 30, 0.886487, 1.0},
		{27, 30, 0.743789, 0.965400},
		{15, 30, 0.331541, 0.668459},
		{0, 30, 0.0, 0.113513},
		{1, 1, 0.206549, 1.0},
		{0, 10, 0.0, 0.277533},
		{9, 10, 0.595850, 0.982124},
	} {
		lo, hi, ok := wilson95(tc.k, tc.n)
		if !ok || math.Abs(lo-tc.lo) > 1e-6 || math.Abs(hi-tc.hi) > 1e-6 {
			t.Errorf("wilson95(%d, %d) = [%.6f, %.6f] ok=%v, want [%.6f, %.6f]", tc.k, tc.n, lo, hi, ok, tc.lo, tc.hi)
		}
	}
	for _, bad := range [][2]int{{0, 0}, {5, 3}, {-1, 4}, {1, -2}} {
		if _, _, ok := wilson95(bad[0], bad[1]); ok {
			t.Errorf("wilson95(%d, %d) gave an interval; k outside 0..n or n <= 0 has none", bad[0], bad[1])
		}
	}
}

// TestDetectorPrecision_SeedLabelsNeverCount: a seed label is tallied apart
// and the row prints n/a, never a number; a person's label counts.
func TestDetectorPrecision_SeedLabelsNeverCount(t *testing.T) {
	var tl fireTally
	for i := 0; i < 40; i++ {
		tl.add(labelTruePositive, labelledBySeed)
	}
	tl.add(labelFalsePositive, labelledBySeed)
	tl.add(labelTrueNegative, "luis") // not a fire: ignored by the fireTally
	tl.add(labelFalseNegative, "luis")
	if tl.N() != 0 || tl.SeedTP != 40 || tl.SeedFP != 1 {
		t.Fatalf("fireTally = %+v, want N=0 and 40 seed TP, 1 seed FP", tl)
	}
	row := precisionRow{"silent_failure", tl, true}.String()
	for _, want := range []string{"N=0 ", "precision=n/a", "wilson95=n/a", "unmeasured: 0 of 30 labelled fires", "41 seed fires (40 TP, 1 FP) await a person"} {
		if !strings.Contains(row, want) {
			t.Errorf("row %q lacks %q: 41 seed fires must not become a measurement", row, want)
		}
	}
	tl.add(labelTruePositive, "luis")
	tl.add(labelFalsePositive, "luis")
	if tl.TP != 1 || tl.FP != 1 || tl.N() != 2 {
		t.Errorf("a person's labels did not count: %+v", tl)
	}
}

// TestDetectorPrecision_FloorIsThirty: 29 counted fires is unmeasured however
// perfect, and 30 is measured; a detector with no fixtures says so.
func TestDetectorPrecision_FloorIsThirty(t *testing.T) {
	for _, tc := range []struct {
		tp, fp   int
		fixtures bool
		want     string
	}{
		{29, 0, true, "unmeasured: 29 of 30 labelled fires"},
		{30, 0, true, "measured"},
		{25, 5, true, "measured"},
		{0, 0, false, "unmeasured: 0 of 30 labelled fires; no fixtures"},
	} {
		row := precisionRow{"masked_run", fireTally{TP: tc.tp, FP: tc.fp}, tc.fixtures}
		got := row.status()
		if !strings.HasPrefix(got, tc.want) || (tc.want == "measured" && got != "measured") {
			t.Errorf("status of %d TP, %d FP = %q, want %q", tc.tp, tc.fp, got, tc.want)
		}
	}
	// The printed row for 27 of 30: precision 0.900, interval from the table above.
	got := precisionRow{"masked_run", fireTally{TP: 27, FP: 3}, true}.String()
	for _, want := range []string{"N=30 ", "precision=0.900", "wilson95=[0.744, 0.965]", "measured"} {
		if !strings.Contains(got, want) {
			t.Errorf("row %q lacks %q", got, want)
		}
	}
}

const goodFixture = `{"detector":"silent_failure","label":"false_positive","reason":"r","labelled_by":"seed",
"fp_class":"empty_search","provenance":"p","final_message":"m",
"calls":[{"tool":"Bash","input":{"command":"grep x y"},"outcome":"failed","exit_code":1}]}`

// TestDetectorPrecision_ParseFixture: one accepted fixture, then one refusal
// per rule, each differing from the accepted one in a single place.
func TestDetectorPrecision_ParseFixture(t *testing.T) {
	if f, err := parseFixture("silent_failure", []byte(goodFixture)); err != nil || f.Label != labelFalsePositive || len(f.Calls) != 1 || !f.probeStart() {
		t.Fatalf("the accepted fixture was refused or misread: %+v, %v", f, err)
	}
	// A turn that used no tool is a fixture: no calls is not an error.
	if _, err := parseFixture("digest_unknown", []byte(`{"detector":"digest_unknown","label":"true_negative","reason":"r","labelled_by":"luis","provenance":"p","final_message":"m"}`)); err != nil {
		t.Errorf("a fixture with no calls was refused: %v", err)
	}
	mut := func(old, new string) string { return strings.Replace(goodFixture, old, new, 1) }
	for name, tc := range map[string]struct {
		detector, doc, want string
	}{
		"unknown key":              {"silent_failure", mut(`"reason":"r"`, `"reason":"r","note":"x"`), "unknown field"},
		"wrong directory":          {"masked_run", goodFixture, "in a \"masked_run\" directory"},
		"second document":          {"silent_failure", goodFixture + goodFixture, "more than one JSON document"},
		"label outside the four":   {"silent_failure", mut("false_positive", "maybe"), "label"},
		"empty reason":             {"silent_failure", mut(`"reason":"r"`, `"reason":" "`), "reason is empty"},
		"empty labelled_by":        {"silent_failure", mut(`"labelled_by":"seed"`, `"labelled_by":""`), "labelled_by is empty"},
		"false positive no class":  {"silent_failure", mut(`"fp_class":"empty_search",`, ""), "names its fp_class"},
		"class outside the set":    {"silent_failure", mut("empty_search", "weather"), "closed set"},
		"class on a true positive": {"silent_failure", mut("false_positive", "true_positive"), "only a false_positive has a class"},
		"empty provenance":         {"silent_failure", mut(`"provenance":"p"`, `"provenance":""`), "provenance is empty"},
		"call names no tool":       {"silent_failure", mut(`"tool":"Bash"`, `"tool":""`), "names no tool"},
		"outcome outside the set":  {"silent_failure", mut(`"outcome":"failed"`, `"outcome":"crashed"`), "outcome"},
		"exit code on an ok call":  {"silent_failure", mut(`"outcome":"failed"`, `"outcome":"ok"`), "only a failed call has one"},
		"exit code over 255":       {"silent_failure", mut(`"exit_code":1`, `"exit_code":256`), "exit_code"},
		"repeat over the cap":      {"silent_failure", mut(`"outcome":"failed"`, `"outcome":"failed","repeat":401`), "repeat"},
		"repeats expand past cap":  {"silent_failure", mut(`"outcome":"failed"`, `"outcome":"failed","repeat":400},{"tool":"Bash","input":{},"outcome":"failed"`), "expand"},
		"capture and calls":        {"silent_failure", mut(`"provenance"`, `"capture":"`+realCaptureName+`","provenance"`), "alternatives"},
		"NUL in reason":            {"silent_failure", mut(`"reason":"r"`, `"reason":"r\u0000"`), "NUL"},
		"invalid UTF-8":            {"silent_failure", mut(`"final_message":"m"`, "\"final_message\":\"m\xff\""), ""},
		"truncated JSON":           {"silent_failure", goodFixture[:40], ""},
		"empty":                    {"silent_failure", "", ""},
	} {
		_, err := parseFixture(tc.detector, []byte(tc.doc))
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: got %v, want an error containing %q", name, err, tc.want)
		}
	}
	// A capture with no calls, and the named capture, is the one accepted spelling.
	cap := `{"detector":"silent_failure","label":"true_negative","reason":"r","labelled_by":"seed","provenance":"p","capture":"` + realCaptureName + `","final_message":""}`
	if _, err := parseFixture("silent_failure", []byte(cap)); err != nil {
		t.Errorf("the named capture was refused: %v", err)
	}
	if _, err := parseFixture("silent_failure", []byte(strings.Replace(cap, realCaptureName, "claude-code/9.9.9", 1))); err == nil {
		t.Error("a capture that is not the committed one was accepted")
	}
	if _, err := parseFixture("silent_failure", []byte(strings.Repeat(" ", maxFixtureBytes)+goodFixture)); err == nil {
		t.Error("a fixture over the byte cap was accepted")
	}
	// Hostile shapes within the byte cap must be answered, not crash the
	// parser: a deeply nested input, a million-key object, a lone surrogate.
	deep := strings.Repeat(`{"a":`, 5000) + "1" + strings.Repeat("}", 5000)
	for _, hostile := range []string{
		mut(`{"command":"grep x y"}`, deep),
		mut(`"m"`, `"\ud800"`),
		mut(`"calls":[`, `"calls":[null,`),
		"\x00\x00\x00",
	} {
		_, _ = parseFixture("silent_failure", []byte(hostile))
	}
}

const sheetHeader = "detector\tfire_ref\tfired_on\tline_text\tlabel\tfp_class\treason\tlabelled_by\n"

func knownInTest(id string) bool { return id == "masked_run" || id == "silent_failure" }

// TestDetectorPrecision_ParseSheet: an unjudged row is skipped, a judged one
// is read and counts by who judged it, and every malformed row is refused.
func TestDetectorPrecision_ParseSheet(t *testing.T) {
	rows, err := parseSheet([]byte(sheetHeader+
		"silent_failure\tref-1\t2026-10-26\t1 recorded failure\t\t\t\t\n"+
		"silent_failure\tref-2\t2026-10-26\t1 recorded failure\ttrue_positive\t\tthe build failed\tluis\n"+
		"masked_run\tref-3\t2026-10-26\tpass claimed\tfalse_positive\toutput_shows_pass\tthe log showed ok\tseed\n\n"), knownInTest)
	if err != nil || len(rows) != 2 {
		t.Fatalf("rows = %+v, err = %v; want the two judged rows", rows, err)
	}
	var tl fireTally
	for _, r := range rows {
		tl.add(r.Label, r.LabelledBy)
	}
	if tl.TP != 1 || tl.SeedFP != 1 || tl.N() != 1 {
		t.Errorf("fireTally = %+v: luis's row counts, the seed row does not", tl)
	}

	row := func(cols ...string) string { return sheetHeader + strings.Join(cols, "\t") + "\n" }
	for name, tc := range map[string]struct{ doc, want string }{
		"wrong header":       {"detector\tlabel\n", "header"},
		"empty":              {"", "header"},
		"short row":          {row("silent_failure", "r", "d"), "columns"},
		"long row":           {row("silent_failure", "r", "d", "l", "true_positive", "", "why", "luis", "extra"), "columns"},
		"unknown detector":   {row("weather", "r", "d", "l", "true_positive", "", "why", "luis"), "not in the inventory"},
		"true negative":      {row("silent_failure", "r", "d", "l", "true_negative", "", "why", "luis"), "not \"true_negative\""},
		"fp without class":   {row("silent_failure", "r", "d", "l", "false_positive", "", "why", "luis"), "fp_class"},
		"class on a tp":      {row("silent_failure", "r", "d", "l", "true_positive", "empty_search", "why", "luis"), "only a false_positive"},
		"no reason":          {row("silent_failure", "r", "d", "l", "true_positive", "", "", "luis"), "reason is empty"},
		"no labeller":        {row("silent_failure", "r", "d", "l", "true_positive", "", "why", ""), "labelled_by is empty"},
		"judgement no label": {row("silent_failure", "r", "d", "l", "", "", "why", "luis"), "no label"},
		"NUL":                {row("silent_failure", "r", "d", "l\x00", "", "", "", ""), "NUL"},
		"invalid UTF-8":      {row("silent_failure", "r", "d", "l\xff", "", "", "", ""), "UTF-8"},
		"over the byte cap":  {sheetHeader + strings.Repeat("\n", maxSheetBytes), "over the"},
	} {
		if _, err := parseSheet([]byte(tc.doc), knownInTest); err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: got %v, want an error containing %q", name, err, tc.want)
		}
	}
}

// TestDetectorPrecision_LabelIsHeldToTheDetector: a label that presupposes a
// fire the detector did not make, a silence it did not keep, and a line that
// disagrees with the digest each fail; the matching cases do not. Two values
// per rule, run on a synthetic replay.
func TestDetectorPrecision_LabelIsHeldToTheDetector(t *testing.T) {
	spec, _ := detectorSpecByID("silent_failure")
	fires := fixtureReplay{spoke: true, line: "※ rashomon: 1 recorded failure."}
	fires.digest.SilentFailures.Fires = true
	silent := fixtureReplay{}
	liar := fixtureReplay{spoke: true, line: "※ rashomon: coverage unverified (gap)."}
	liar.digest.SilentFailures.Fires = true // digest says fired, the line has no such sentence

	for name, tc := range map[string]struct {
		label    string
		r        fixtureReplay
		wantFire bool
		problems int
	}{
		"tp that fired":             {labelTruePositive, fires, true, 0},
		"fp that fired":             {labelFalsePositive, fires, true, 0},
		"tn that stayed silent":     {labelTrueNegative, silent, false, 0},
		"fn that stayed silent":     {labelFalseNegative, silent, false, 0},
		"tp that did not fire":      {labelTruePositive, silent, false, 1},
		"fp that did not fire":      {labelFalsePositive, silent, false, 1},
		"tn that fired":             {labelTrueNegative, fires, true, 1},
		"fn that fired":             {labelFalseNegative, fires, true, 1},
		"line disagrees the digest": {labelTruePositive, liar, true, 1},
	} {
		fired, problems := fixtureProblems(spec, detectorFixture{Label: tc.label}, tc.r)
		if fired != tc.wantFire || len(problems) != tc.problems {
			t.Errorf("%s: fired=%v with %d problems %q, want fired=%v with %d", name, fired, len(problems), problems, tc.wantFire, tc.problems)
		}
	}
}
