package share

import (
	"bytes"
	"encoding/json"
	"io"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/steer"
	"github.com/altrace-dev-role/rashomon/internal/steer/pattern"
	"github.com/altrace-dev-role/rashomon/internal/store"
)

// lineText is free text a steer line carries and a bundle never may.
const lineText = "rashomon record: go test failed in /home/user/private-project"

// steerLine is a line of pattern p in state s, with delivery d (nil for
// none), at ms, carrying the free text a bundle must leave out.
func steerLine(id string, p pattern.ID, s steer.LineState, d *steer.Delivery, ms int64) *steer.Line {
	return &steer.Line{Type: steer.LineType, V: steer.LineVersion, ID: id, AtUnixMS: ms, SessionID: "s",
		Pattern: string(p), State: s, Delivery: d, ModelText: lineText, UserText: lineText}
}

func delivery(d steer.Delivery) *steer.Delivery { return &d }

// appendLines writes lines to session's steer log as one batch, as the router
// appends.
func appendLines(t *testing.T, st *store.Store, session string, lines ...*steer.Line) {
	t.Helper()
	batch := make([]store.SteerLine, 0, len(lines))
	for _, l := range lines {
		batch = append(batch, l)
	}
	if err := st.AppendSteer(session, func(io.ReaderAt) ([]store.SteerLine, error) { return batch, nil }); err != nil {
		t.Fatal(err)
	}
}

func openStore(t *testing.T) *store.Store {
	t.Helper()
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return st
}

// sevenFired is SH1's store: two runs, five delivered regression-location
// lines and one delivered and one flag_only identical-retry line, between
// 1000 and 9000 ms; around them a holdout, a suppressed (suppressed_budget)
// and a candidate line, and a delivered line of a pattern outside the closed
// set, at 500 and 9500 ms, none of which counts. identical-retry is written
// first, though it comes later in pattern.All, so a bundle in log order is not
// one in the closed order; and neither run's first counted line is the
// earliest, so a period taken from whichever run Runs lists first is wrong.
func sevenFired(t *testing.T) *store.Store {
	t.Helper()
	st := openStore(t)
	d, f := delivery(steer.DeliveryDelivered), delivery(steer.DeliveryFlagOnly)
	appendLines(t, st, "run-a",
		steerLine("a1", pattern.IdenticalRetry, steer.LineStateDelivered, d, 2000),
		steerLine("a2", pattern.RegressionLocation, steer.LineStateDelivered, d, 9000),
		steerLine("a3", pattern.RegressionLocation, steer.LineStateDelivered, delivery(steer.DeliveryHoldout), 9500),
		steerLine("a4", pattern.DeclaredFact, steer.LineStateCandidate, nil, 9500),
		steerLine("a5", "made-up", steer.LineStateDelivered, d, 500))
	appendLines(t, st, "run-b",
		steerLine("b1", pattern.RegressionLocation, steer.LineStateDelivered, d, 3000),
		steerLine("b2", pattern.RegressionLocation, steer.LineStateDelivered, d, 1000),
		steerLine("b3", pattern.IdenticalRetry, steer.LineStateDelivered, f, 4000),
		steerLine("b4", pattern.IdenticalRetry, steer.LineStateSuppressed, delivery(steer.DeliverySuppressedBudget), 500),
		steerLine("b5", pattern.RegressionLocation, steer.LineStateDelivered, d, 5000),
		steerLine("b6", pattern.RegressionLocation, steer.LineStateDelivered, d, 6000))
	return st
}

// marshal is o as Encode prints it, compacted to one line for comparison.
func marshal(t *testing.T, o Outcomes) string {
	t.Helper()
	b, err := Encode(o)
	if err != nil {
		t.Fatal(err)
	}
	var line bytes.Buffer
	if err := json.Compact(&line, b); err != nil {
		t.Fatal(err)
	}
	return line.String()
}

// TestSH1_CountsFiredByDeliveryAndPattern: fired counts delivered and
// flag_only lines of the closed set, 7 here; the holdout, the suppressed
// line, the candidate and the unknown pattern count nowhere and widen no
// period; by_pattern has the two patterns that fired, in pattern.All's order,
// a number at five and "<5" under it.
func TestSH1_CountsFiredByDeliveryAndPattern(t *testing.T) {
	st := sevenFired(t)
	o, err := BuildOutcomes(st)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"schema":"outcomes/1","install_digest":"` + st.InstallDigest() + `",` +
		`"period":{"from_unix_ms":1000,"to_unix_ms":9000},` +
		`"counts":{"fired":7,"fixed":"<5","ignored":"<5","overridden":"<5"},` +
		`"by_pattern":[` +
		`{"pattern":"regression-location","fired":5,"precision":{"labelled":"<5","true":"<5"}},` +
		`{"pattern":"identical-retry","fired":"<5","precision":{"labelled":"<5","true":"<5"}}]}`
	if got := marshal(t, o); got != want {
		t.Errorf("bundle:\n got %s\nwant %s", got, want)
	}
	if o.Counts.Fired != 7 || len(o.ByPattern) != 2 || o.ByPattern[1].Fired != 2 {
		t.Errorf("counts %+v, by_pattern %+v; want fired 7 and identical-retry 2", o.Counts, o.ByPattern)
	}
}

// TestSH1b_UnderFivePrintsLess: a count under five prints as "<5" and five
// prints as itself; with four fired lines the bundle's fired is "<5", and
// fixed, ignored and overridden are "<5" whatever fired is.
func TestSH1b_UnderFivePrintsLess(t *testing.T) {
	for c, want := range map[Count]string{0: `"<5"`, 1: `"<5"`, 4: `"<5"`, 5: `5`, 12: `12`} {
		if b, err := c.MarshalJSON(); err != nil || string(b) != want {
			t.Errorf("Count(%d) marshals as %s (%v), want %s", c, b, err, want)
		}
	}
	st := openStore(t)
	d := delivery(steer.DeliveryDelivered)
	appendLines(t, st, "run-a",
		steerLine("a1", pattern.Integrity, steer.LineStateDelivered, d, 1),
		steerLine("a2", pattern.Integrity, steer.LineStateDelivered, d, 2),
		steerLine("a3", pattern.Integrity, steer.LineStateDelivered, d, 3),
		steerLine("a4", pattern.Integrity, steer.LineStateDelivered, d, 4))
	o, err := BuildOutcomes(st)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := marshal(t, o), `"counts":{"fired":"<5","fixed":"<5","ignored":"<5","overridden":"<5"}`; !strings.Contains(got, want) {
		t.Errorf("four fired: %s\nwant it to hold %s", got, want)
	}
	if got, want := marshal(t, mustBuild(t, sevenFired(t))), `"fixed":"<5","ignored":"<5","overridden":"<5"`; !strings.Contains(got, want) {
		t.Errorf("seven fired: %s\nwant it to hold %s", got, want)
	}
}

func mustBuild(t *testing.T, st *store.Store) Outcomes {
	t.Helper()
	o, err := BuildOutcomes(st)
	if err != nil {
		t.Fatal(err)
	}
	return o
}

// TestSH2b_NoFreeTextField: every string in a marshalled bundle is the
// schema, a pattern id of the closed set, "<5" or the install digest; every
// other leaf is a number. No null, no boolean, and none of the lines' text.
func TestSH2b_NoFreeTextField(t *testing.T) {
	st := sevenFired(t)
	b, err := Encode(mustBuild(t, st))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), "private-project") || strings.Contains(string(b), "made-up") {
		t.Fatalf("the bundle carries a line's text or an unknown pattern: %s", b)
	}
	var doc any
	if err := json.Unmarshal(b, &doc); err != nil {
		t.Fatal(err)
	}
	digest := st.InstallDigest()
	leaves := 0
	var walk func(path string, v any)
	walk = func(path string, v any) {
		switch v := v.(type) {
		case map[string]any:
			for k, x := range v {
				walk(path+"."+k, x)
			}
		case []any:
			for i, x := range v {
				walk(path+"["+strconv.Itoa(i)+"]", x)
			}
		case string:
			leaves++
			if v != SchemaOutcomes && v != "<5" && !pattern.Valid(pattern.ID(v)) && (v != digest || digest == "") {
				t.Errorf("%s is the free string %q", path, v)
			}
		case float64:
			leaves++
		default:
			t.Errorf("%s is %v (%T), neither a closed string nor a number", path, v, v)
		}
	}
	walk("$", doc)
	if leaves != 16 {
		t.Errorf("walked %d leaves of %s; the bundle has 16", leaves, b)
	}
}

var hex64 = regexp.MustCompile(`^[0-9a-f]{64}$`)

// TestSH1c_EmptyStoreIsAnEmptyBundle: a store with no runs gives the schema,
// the digest, a zero period, every count "<5" and an empty by_pattern, never
// null. The digest is 64 hex, the same for every open of one store, another
// for another store, and neither the install id nor the forget-host digest of
// it: its domain is its own.
func TestSH1c_EmptyStoreIsAnEmptyBundle(t *testing.T) {
	st := openStore(t)
	digest := st.InstallDigest()
	want := `{"schema":"outcomes/1","install_digest":"` + digest + `",` +
		`"period":{"from_unix_ms":0,"to_unix_ms":0},` +
		`"counts":{"fired":"<5","fixed":"<5","ignored":"<5","overridden":"<5"},"by_pattern":[]}`
	if got := marshal(t, mustBuild(t, st)); got != want {
		t.Errorf("empty store:\n got %s\nwant %s", got, want)
	}
	if !hex64.MatchString(digest) {
		t.Fatalf("install digest %q is not 64 hex", digest)
	}
	again, err := store.Open(st.Root())
	if err != nil {
		t.Fatal(err)
	}
	if again.InstallDigest() != digest {
		t.Errorf("a second open of the store digests the install as %q, the first as %q", again.InstallDigest(), digest)
	}
	if other := openStore(t).InstallDigest(); other == digest {
		t.Errorf("two installs share the digest %q", digest)
	}
	if digest == st.InstallID() || digest == st.HostDigest(st.InstallID()) {
		t.Errorf("install digest %q is the id or its forget-host digest: a domain of its own is required", digest)
	}
}
