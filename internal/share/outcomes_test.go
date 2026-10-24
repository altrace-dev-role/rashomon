package share

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/steer/pattern"
)

// fake is the Reader the tests use: Phase 2's labels do not exist yet.
type fake struct {
	o   Outcomes
	err error
}

func (f fake) Outcomes() (Outcomes, error) { return f.o, f.err }

var digest = strings.Repeat("ab", 32)

func tally(id pattern.ID, fired, fixed, ignored, overridden, labelled, true_ int) Tally {
	return Tally{id, fired, fixed, ignored, overridden, labelled, true_}
}

func outcomes(ts ...Tally) Outcomes {
	return Outcomes{InstallDigest: digest, Period: Period{"2026-10-01", "2026-10-24"}, Tallies: ts}
}

func render(t *testing.T, o Outcomes) []byte {
	t.Helper()
	b, err := Build(fake{o: o})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	out, err := Render(b)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	return out
}

// obj and list assert a decoded value's shape, so a wrong bundle fails the
// test that read it instead of panicking in it.
func obj(t *testing.T, v any) map[string]any {
	t.Helper()
	m, ok := v.(map[string]any)
	if !ok {
		t.Fatalf("%v is not an object", v)
	}
	return m
}

func list(t *testing.T, v any) []any {
	t.Helper()
	l, ok := v.([]any)
	if !ok {
		t.Fatalf("%v is not a list", v)
	}
	return l
}

func decode(t *testing.T, b []byte) any {
	t.Helper()
	var v any
	if err := json.Unmarshal(b, &v); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, b)
	}
	return v
}

// TestSH2_BundleCarriesTheCardsShape: regression-location 12 fired (7 fixed, 3
// ignored, 2 overridden; 6 labelled, 5 true) and cred-spray 3 fired (1, 1, 1;
// 2 labelled, 1 true). Totals, from the rule that counts are summed raw and
// then suppressed: fired 15, fixed 8, ignored 4, overridden 3. So the file
// shows 15 and 8 and "<5" for the other two, regression-location in full, and
// every cred-spray count as "<5". Rows follow pattern.All, not input order.
func TestSH2_BundleCarriesTheCardsShape(t *testing.T) {
	got := decode(t, render(t, outcomes(
		tally(pattern.CredSpray, 3, 1, 1, 1, 2, 1),
		tally(pattern.RegressionLocation, 12, 7, 3, 2, 6, 5),
	)))
	want := decode(t, []byte(`{
	  "schema": "outcomes/1",
	  "install_digest": "`+digest+`",
	  "period": {"start": "2026-10-01", "end": "2026-10-24"},
	  "counts": {"fired": 15, "fixed": 8, "ignored": "<5", "overridden": "<5"},
	  "by_pattern": [
	    {"pattern": "regression-location", "fired": 12, "precision": {"labelled": 6, "true": 5}},
	    {"pattern": "cred-spray", "fired": "<5", "precision": {"labelled": "<5", "true": "<5"}}
	  ]}`))
	if !reflect.DeepEqual(got, want) {
		t.Errorf("bundle = %v\nwant     %v", got, want)
	}
}

// TestSH2_SmallCountsPrintAsLessThanFive: 0 and 4 are under MinCount and 5 and
// 6 are not, in a count, in a total and in a precision field.
func TestSH2_SmallCountsPrintAsLessThanFive(t *testing.T) {
	for n, want := range map[int]any{0: "<5", 4: "<5", 5: float64(5), 6: float64(6), 1000: float64(1000)} {
		b, err := json.Marshal(Count(n))
		if err != nil || decode(t, b) != want {
			t.Errorf("Count(%d) = %s, %v; want %v", n, b, err, want)
		}
	}
	// The file says "<5" in those characters, not as a JSON escape.
	file := string(render(t, outcomes(tally(pattern.Integrity, 3, 0, 0, 0, 0, 0))))
	if !strings.Contains(file, `"fired": "<5"`) || strings.Contains(file, `\u003c`) {
		t.Errorf("the rendered bundle does not print a small count as \"<5\":\n%s", file)
	}
	// One pattern at the boundary: fired 5 prints, labelled 4 does not.
	got := obj(t, decode(t, render(t, outcomes(tally(pattern.Integrity, 5, 0, 0, 0, 4, 4)))))
	row := obj(t, list(t, got["by_pattern"])[0])
	prec := obj(t, row["precision"])
	if row["fired"] != float64(5) || prec["labelled"] != "<5" || prec["true"] != "<5" {
		t.Errorf("row at the boundary = %v", row)
	}
}

// TestSH2_TotalsAreSummedBeforeSuppression: two patterns of 3 firings each are
// "<5" in their rows and 6 in the total; suppressing first would print "<5"
// for both and a total of "<5" too.
func TestSH2_TotalsAreSummedBeforeSuppression(t *testing.T) {
	got := obj(t, decode(t, render(t, outcomes(
		tally(pattern.Integrity, 3, 0, 0, 0, 0, 0),
		tally(pattern.CredSpray, 3, 0, 0, 0, 0, 0),
	))))
	if c := obj(t, got["counts"]); c["fired"] != float64(6) {
		t.Errorf("counts.fired = %v, want 6", c["fired"])
	}
}

// TestSH2_OutputIsDeterministic: the same tallies in two orders, and the same
// call twice, give the same bytes (and an empty reader gives an empty list,
// not null).
func TestSH2_OutputIsDeterministic(t *testing.T) {
	a := tally(pattern.RegressionLocation, 12, 7, 3, 2, 6, 5)
	b := tally(pattern.CredSpray, 9, 1, 1, 1, 2, 1)
	c := tally(pattern.SubtaskGate, 30, 4, 4, 4, 10, 9)
	one, two := render(t, outcomes(a, b, c)), render(t, outcomes(c, a, b))
	if !bytes.Equal(one, two) || !bytes.Equal(one, render(t, outcomes(a, b, c))) {
		t.Errorf("two orders or two runs differ:\n%s\n%s", one, two)
	}
	var seen []string
	for _, row := range list(t, obj(t, decode(t, one))["by_pattern"]) {
		seen = append(seen, fmt.Sprint(obj(t, row)["pattern"]))
	}
	if want := []string{"subtask-gate", "regression-location", "cred-spray"}; !slices.Equal(seen, want) {
		t.Errorf("rows %v, want pattern.All order %v", seen, want)
	}
	if !strings.Contains(string(render(t, outcomes())), `"by_pattern": []`) {
		t.Error("no tallies did not give an empty by_pattern list")
	}
}

// TestSH2_ReaderIsClosed: Build refuses what a closed set does not allow, and
// each row has a value that is allowed beside the one that is not.
func TestSH2_ReaderIsClosed(t *testing.T) {
	good := func() Outcomes { return outcomes(tally(pattern.Integrity, 9, 1, 1, 1, 4, 3)) }
	if _, err := Build(fake{o: good()}); err != nil {
		t.Fatalf("the baseline is refused: %v", err)
	}
	for _, tc := range []struct {
		name string
		mut  func(*Outcomes)
		want string // a word of the refusal, so the right check is the one that fired
	}{
		{"unknown pattern", func(o *Outcomes) { o.Tallies[0].Pattern = "my-private-project" }, "outside the closed set"},
		{"empty pattern", func(o *Outcomes) { o.Tallies[0].Pattern = "" }, "outside the closed set"},
		{"duplicate pattern", func(o *Outcomes) { o.Tallies = append(o.Tallies, o.Tallies[0]) }, "two tallies"},
		{"negative fired", func(o *Outcomes) { o.Tallies[0].Fired = -1 }, "outside 0.."},
		{"negative fixed", func(o *Outcomes) { o.Tallies[0].Fixed = -1 }, "outside 0.."},
		{"count over the cap", func(o *Outcomes) { o.Tallies[0].Fired, o.Tallies[0].Labelled, o.Tallies[0].True = MaxCount+1, 0, 0 }, "outside 0.."},
		{"true over labelled", func(o *Outcomes) { o.Tallies[0].True = 5 }, "true of"},
		{"labelled over fired", func(o *Outcomes) { o.Tallies[0].Labelled, o.Tallies[0].True = 10, 3 }, "true of"},
		{"digest too short", func(o *Outcomes) { o.InstallDigest = digest[:63] }, "install_digest"},
		{"digest upper case", func(o *Outcomes) { o.InstallDigest = strings.ToUpper(digest) }, "lower-case hex"},
		{"digest not hex", func(o *Outcomes) { o.InstallDigest = strings.Repeat("g", 64) }, "lower-case hex"},
		{"digest is text", func(o *Outcomes) { o.InstallDigest = "/home/luis/secret-project" + strings.Repeat("a", 39) }, "lower-case hex"},
		{"period is a timestamp", func(o *Outcomes) { o.Period.Start = "2026-10-01T00:00:00Z" }, "YYYY-MM-DD"},
		{"period is not padded", func(o *Outcomes) { o.Period.End = "2026-10-4" }, "YYYY-MM-DD"},
		{"period is words", func(o *Outcomes) { o.Period.End = "last tuesday" }, "YYYY-MM-DD"},
		{"period day does not exist", func(o *Outcomes) { o.Period.End = "2026-02-30" }, "YYYY-MM-DD"},
		{"period runs backwards", func(o *Outcomes) { o.Period.Start, o.Period.End = o.Period.End, o.Period.Start }, "starts after"},
		{"more tallies than patterns", func(o *Outcomes) {
			for i := 0; i < len(pattern.All()); i++ {
				o.Tallies = append(o.Tallies, o.Tallies[0])
			}
		}, "patterns"},
	} {
		o := good()
		tc.mut(&o)
		if _, err := Build(fake{o: o}); err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("Build with %s: err = %v, want a refusal naming %q", tc.name, err, tc.want)
		}
	}
	boom := errors.New("store unreadable")
	if _, err := Build(fake{err: boom}); !errors.Is(err, boom) {
		t.Errorf("a reader's error was swallowed: %v", err)
	}
	// A total over the cap from tallies each under it.
	var many []Tally
	for _, id := range pattern.All()[:2] {
		many = append(many, tally(id, MaxCount, 0, 0, 0, 0, 0))
	}
	if _, err := Build(fake{o: outcomes(many...)}); err == nil || !strings.Contains(err.Error(), "a total of") {
		t.Error("Build accepted a total over the cap")
	}
}

// bundleStrings are the only string-typed fields of a Bundle, by path. Each is
// a closed set or a fixed form that Build checks; a string field added
// anywhere else is a place free text could enter, and fails here first.
var bundleStrings = []string{"Schema", "InstallDigest", "Period.Start", "Period.End", "ByPattern[].Pattern"}

// stringFields lists the string-kinded fields under t by path.
func stringFields(t reflect.Type, path string) []string {
	var out []string
	switch t.Kind() {
	case reflect.String:
		out = append(out, path)
	case reflect.Slice:
		out = append(out, stringFields(t.Elem(), path+"[]")...)
	case reflect.Struct:
		for i := 0; i < t.NumField(); i++ {
			p := t.Field(i).Name
			if path != "" {
				p = path + "." + p
			}
			out = append(out, stringFields(t.Field(i).Type, p)...)
		}
	}
	return out
}

// closedViolations walks a decoded bundle and returns every place it holds
// something outside the schema: a key it does not name, a string that is not
// its slot's closed set or fixed form, a number where a count is not.
func closedViolations(doc any) []string {
	var bad []string
	isCount := func(path string, v any) {
		if s, ok := v.(string); ok && s == SmallCount {
			return
		}
		if n, ok := v.(float64); !ok || n < MinCount || n != float64(int64(n)) {
			bad = append(bad, fmt.Sprintf("%s = %v is not a count", path, v))
		}
	}
	object := func(path string, v any, keys ...string) map[string]any {
		m, ok := v.(map[string]any)
		if !ok {
			bad = append(bad, path+" is not an object")
			return nil
		}
		for k := range m {
			if !slices.Contains(keys, k) {
				bad = append(bad, path+"."+k+" is not a key of the schema")
			}
		}
		for _, k := range keys {
			if _, ok := m[k]; !ok {
				bad = append(bad, path+"."+k+" is missing")
			}
		}
		return m
	}
	top := object("$", doc, "schema", "install_digest", "period", "counts", "by_pattern")
	if top == nil {
		return bad
	}
	if top["schema"] != SchemaOutcomes {
		bad = append(bad, fmt.Sprintf("schema = %v", top["schema"]))
	}
	if s, _ := top["install_digest"].(string); checkDigest(s) != nil {
		bad = append(bad, fmt.Sprintf("install_digest = %v is not a digest", top["install_digest"]))
	}
	if p := object("period", top["period"], "start", "end"); p != nil {
		for _, k := range []string{"start", "end"} {
			if s, _ := p[k].(string); checkPeriod(Period{s, s}) != nil {
				bad = append(bad, fmt.Sprintf("period.%s = %v is not a day", k, p[k]))
			}
		}
	}
	if c := object("counts", top["counts"], "fired", "fixed", "ignored", "overridden"); c != nil {
		for k, v := range c {
			isCount("counts."+k, v)
		}
	}
	rows, ok := top["by_pattern"].([]any)
	if !ok {
		return append(bad, "by_pattern is not a list")
	}
	for i, r := range rows {
		path := fmt.Sprintf("by_pattern[%d]", i)
		row := object(path, r, "pattern", "fired", "precision")
		if row == nil {
			continue
		}
		if s, _ := row["pattern"].(string); !pattern.Valid(pattern.ID(s)) {
			bad = append(bad, fmt.Sprintf("%s.pattern = %v is not a pattern id", path, row["pattern"]))
		}
		isCount(path+".fired", row["fired"])
		if p := object(path+".precision", row["precision"], "labelled", "true"); p != nil {
			for k, v := range p {
				isCount(path+".precision."+k, v)
			}
		}
	}
	return bad
}

// TestSH2b_NoFreeTextField: a bundle built over every pattern, with every
// count both under and over the line, walks clean; the types hold no string
// field outside the five closed ones; and the walker itself flags each way a
// free-text field could be added or a closed one filled with text.
func TestSH2b_NoFreeTextField(t *testing.T) {
	var all []Tally
	for i, id := range pattern.All() {
		all = append(all, tally(id, 3+i*2, i, i, i, i, i/2))
	}
	doc := decode(t, render(t, outcomes(all...)))
	if bad := closedViolations(doc); len(bad) > 0 {
		t.Errorf("a real bundle has free text or a stray key: %v", bad)
	}
	if rows := list(t, obj(t, doc)["by_pattern"]); len(rows) != len(pattern.All()) {
		t.Fatalf("premise: %d rows for %d patterns", len(rows), len(pattern.All()))
	}

	if got := stringFields(reflect.TypeOf(Bundle{}), ""); !slices.Equal(got, bundleStrings) {
		t.Errorf("Bundle's string fields = %v, want exactly %v", got, bundleStrings)
	}

	base := func() map[string]any {
		return obj(t, decode(t, render(t, outcomes(tally(pattern.Integrity, 9, 1, 1, 1, 4, 3)))))
	}
	row := func(m map[string]any) map[string]any { return obj(t, list(t, m["by_pattern"])[0]) }
	for name, mut := range map[string]func(map[string]any){
		"a note at the top":      func(m map[string]any) { m["note"] = "fixed the auth bug in acme/payments" },
		"a note in a row":        func(m map[string]any) { row(m)["example"] = "rm -rf /home/luis" },
		"a note in the period":   func(m map[string]any) { obj(t, m["period"])["label"] = "q4" },
		"a text pattern":         func(m map[string]any) { row(m)["pattern"] = "my-private-project" },
		"a text digest":          func(m map[string]any) { m["install_digest"] = "luis@example.com" },
		"a text count":           func(m map[string]any) { obj(t, m["counts"])["fired"] = "many" },
		"a text precision":       func(m map[string]any) { obj(t, row(m)["precision"])["true"] = "most" },
		"a count under the line": func(m map[string]any) { row(m)["fired"] = float64(2) },
		"a fraction":             func(m map[string]any) { row(m)["fired"] = 7.5 },
		"a text day":             func(m map[string]any) { obj(t, m["period"])["end"] = "yesterday" },
		"another schema":         func(m map[string]any) { m["schema"] = "outcomes/2" },
		"a missing key":          func(m map[string]any) { delete(m, "counts") },
	} {
		m := base()
		mut(m)
		if len(closedViolations(m)) == 0 {
			t.Errorf("the walker passed a bundle with %s", name)
		}
	}
	if bad := closedViolations(base()); len(bad) > 0 {
		t.Errorf("the walker's own baseline is flagged: %v", bad)
	}
}

// TestSH2_PeriodOfOneDay: a period whose ends are the same day is one day, not
// an error; and the last day of a leap year is a day.
func TestSH2_PeriodOfOneDay(t *testing.T) {
	for _, p := range []Period{{"2026-10-24", "2026-10-24"}, {"2028-02-29", "2028-03-01"}} {
		o := outcomes()
		o.Period = p
		if _, err := Build(fake{o: o}); err != nil {
			t.Errorf("period %v refused: %v", p, err)
		}
	}
}
