package session

import (
	"bytes"
	"encoding/json"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/graph"
)

const (
	frSID  = "0a1b2c3d-0000-4000-8000-0000000000f1"
	frPrev = "0a1b2c3d-0000-4000-8000-0000000000f0"
)

// sampleFrontier fills every field, with each slice out of its canonical
// order, so a round trip and the shape walk see all of them.
func sampleFrontier() Frontier {
	f := newFrontier(frSID)
	f.Through = Mark{Seq: 52, Steer: true, AtUnixMS: 1700, ID: "l9"}
	f.Continues = frPrev
	f.Intents = []IntentRef{
		{ID: string(graph.PromptNodeID(frSID, 2)), Digest: strings.Repeat("b", 16), StatedAt: 40, Status: ReqOpen},
		{ID: string(graph.PromptNodeID(frSID, 1)), Digest: strings.Repeat("a", 16), StatedAt: 3, Status: ReqOpen},
	}
	f.Reqs = []ReqState{
		{ID: "opened_unit:b", Pred: "opened_unit", Arg: "b", Status: ReqOpen, StatedAt: 41, LastStatedAt: 41},
		{ID: "gate_green:go-test", Pred: "gate_green", Arg: "go-test", Status: ReqMetOnce, StatedAt: 4, LastStatedAt: 4, MetAt: 9},
		{ID: "opened_unit:a", Pred: "opened_unit", Arg: "a", Status: ReqSuperseded, StatedAt: 4, LastStatedAt: 6,
			SupersededBy: string(graph.PromptNodeID(frSID, 2))},
	}
	f.Decisions = []DecisionRef{{IDs: []string{"n2", "n1"}, Action: "leave_alone"}, {IDs: []string{"n0"}, Action: "use"}}
	f.Touched = []string{"file:z.go", "file:a.go"}
	f.TouchedWithheld = 3
	f.TouchedOutside = 4
	f.Obligations = []ObligationRef{
		{ID: "ob3", Kind: "expected_evidence", State: "unknown"},
		{ID: "ob2", Kind: "gate_after_write", Unit: "unit:a", State: "open"},
		{ID: "ob1", Kind: "gate_after_write", Unit: "unit:z", State: "met"},
	}
	f.ObligationsAt = 30
	f.ObligationsOmitted = 2
	f.UnitRowsCut = 5
	f.Delivered = []LineRef{{ID: "l2", Key: "unit:b", Kind: "unit", Unit: "b", Seq: 41}, {ID: "l1", Key: "unit:a", Kind: "unit", Unit: "a", Seq: 4}}
	f.Verdicts = []VerdictRef{{Seq: 30, DoneFlagged: false, Unmet: 1}, {Seq: 12, DoneFlagged: true}}
	f.StopsWithoutVerdict = []int64{50, 40}
	f.LastBoundary = 20
	return f
}

// shuffled reverses every slice of f, a different order of the same state.
func shuffled(f Frontier) Frontier {
	rev := func(v reflect.Value) {
		for i, j := 0, v.Len()-1; i < j; i, j = i+1, j-1 {
			a, b := v.Index(i).Interface(), v.Index(j).Interface()
			v.Index(i).Set(reflect.ValueOf(b))
			v.Index(j).Set(reflect.ValueOf(a))
		}
	}
	b, err := json.Marshal(f)
	if err != nil {
		panic(err)
	}
	var g Frontier
	if err := json.Unmarshal(b, &g); err != nil {
		panic(err)
	}
	v := reflect.ValueOf(&g).Elem()
	for i := 0; i < v.NumField(); i++ {
		if v.Field(i).Kind() == reflect.Slice {
			rev(v.Field(i))
		}
	}
	for i := range g.Decisions {
		rev(reflect.ValueOf(g.Decisions[i].IDs))
	}
	return g
}

// TestFrontier_JSONRoundTrip (W-M4.2): a frontier parses back to the state
// it was marshalled from, and two orders of the same state marshal to the
// same bytes, which TestFold_IncrementalEqualsFull's byte equality needs.
func TestFrontier_JSONRoundTrip(t *testing.T) {
	f := sampleFrontier()
	b, err := MarshalFrontier(f)
	if err != nil {
		t.Fatal(err)
	}
	g, err := ParseFrontier(b)
	if err != nil {
		t.Fatal(err)
	}
	b2, err := MarshalFrontier(g)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(b, b2) {
		t.Errorf("round trip changed the bytes:\n%s\n%s", b, b2)
	}
	b3, err := MarshalFrontier(shuffled(f))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(b, b3) {
		t.Errorf("another order of the same state marshals differently:\n%s\n%s", b, b3)
	}
	empty, err := MarshalFrontier(newFrontier(frSID))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(empty, []byte("null")) {
		t.Errorf("an empty frontier holds null, which a parsed one would read as []: %s", empty)
	}
}

// TestFrontier_TouchedCountKeys (W-M4.2, ruling 11:37 3c): the two cut
// counts follow Touched under their own keys, which a reader reads by name;
// a renamed tag round-trips and passes the other frontier tests.
func TestFrontier_TouchedCountKeys(t *testing.T) {
	b, err := MarshalFrontier(sampleFrontier())
	if err != nil {
		t.Fatal(err)
	}
	if want := `"touched":["file:a.go","file:z.go"],"touched_withheld":3,"touched_outside":4,`; !bytes.Contains(b, []byte(want)) {
		t.Errorf("frontier does not hold %s:\n%s", want, b)
	}
}

// TestFrontier_ObligationCountKeys (W-M4.2, D3; ruling 11:37 5c): the
// ledger's seq and its two cut counts follow Obligations under their own
// keys, unit_rows_cut beside obligations_omitted.
func TestFrontier_ObligationCountKeys(t *testing.T) {
	b, err := MarshalFrontier(sampleFrontier())
	if err != nil {
		t.Fatal(err)
	}
	if want := `"obligations_at":30,"obligations_omitted":2,"unit_rows_cut":5,"delivered":`; !bytes.Contains(b, []byte(want)) {
		t.Errorf("frontier does not hold %s:\n%s", want, b)
	}
}

// TestFrontier_BoundaryKey (W-M4.2, ruling 12:03): the last boundary's seq
// is under its own key, last in the frontier.
func TestFrontier_BoundaryKey(t *testing.T) {
	b, err := MarshalFrontier(sampleFrontier())
	if err != nil {
		t.Fatal(err)
	}
	if want := `"last_boundary":20}`; !bytes.Contains(b, []byte(want)) {
		t.Errorf("frontier does not hold %s:\n%s", want, b)
	}
}

// TestFrontier_VerdictKeys (W-M4.2, rulings 12:03, 12:12): the verdicts, in
// checkpoint order, under done_flagged and unmet, are followed by the Stop
// checkpoints no verdict names, in seq order, under their own key.
func TestFrontier_VerdictKeys(t *testing.T) {
	b, err := MarshalFrontier(sampleFrontier())
	if err != nil {
		t.Fatal(err)
	}
	if want := `"verdicts":[{"seq":12,"done_flagged":true,"unmet":0},{"seq":30,"done_flagged":false,"unmet":1}],"stops_without_verdict":[40,50],"last_boundary":20}`; !bytes.Contains(b, []byte(want)) {
		t.Errorf("frontier does not hold %s:\n%s", want, b)
	}
}

// TestFrontier_ReqKeys (W-M4.2, ruling 13:23 (3b), (3e)): a Req's seqs are
// under their own keys, last_stated_at between stated_at and met_at, and
// superseded_by holds the superseding prompt's node id.
func TestFrontier_ReqKeys(t *testing.T) {
	b, err := MarshalFrontier(sampleFrontier())
	if err != nil {
		t.Fatal(err)
	}
	want := `{"id":"opened_unit:a","pred":"opened_unit","arg":"a","status":"superseded","stated_at":4,"last_stated_at":6,"met_at":0,` +
		`"superseded_by":"` + string(graph.PromptNodeID(frSID, 2)) + `"}`
	if !bytes.Contains(b, []byte(want)) {
		t.Errorf("frontier does not hold %s:\n%s", want, b)
	}
}

// TestFrontier_SchemaIsOne (W-M4.2): the frontier says its schema first, and
// a file of any other schema does not parse, so the cache misses and the
// session folds in full.
func TestFrontier_SchemaIsOne(t *testing.T) {
	if FrontierSchema != 1 {
		t.Fatalf("FrontierSchema = %d, want 1", FrontierSchema)
	}
	b, err := MarshalFrontier(newFrontier(frSID))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(b, []byte(`{"schema":1,`)) {
		t.Errorf("frontier does not open with its schema: %.60s", b)
	}
	for _, s := range []string{`"schema":0,`, `"schema":2,`} {
		other := bytes.Replace(b, []byte(`"schema":1,`), []byte(s), 1)
		if _, err := ParseFrontier(other); err == nil {
			t.Errorf("a frontier with %s parsed", s)
		}
	}
}

// TestFrontier_KeyOpensIt (W-M4.2, rulings 13:59 (3), 14:09 (5)): a frontier
// opens with its key, schema then fold_version, ledger_version and through,
// the last applied event's full place under Mark's own keys; an empty fold's
// through is the zero Mark, never null.
func TestFrontier_KeyOpensIt(t *testing.T) {
	b, err := MarshalFrontier(sampleFrontier())
	if err != nil {
		t.Fatal(err)
	}
	want := `{"schema":1,"fold_version":` + strconv.Itoa(foldVersion) + `,"ledger_version":` + strconv.Itoa(ledgerVersion) +
		`,"through":{"seq":52,"steer":true,"at_unix_ms":1700,"id":"l9"},"session":`
	if !bytes.HasPrefix(b, []byte(want)) {
		t.Errorf("frontier does not open with %s:\n%s", want, b)
	}
	empty, err := MarshalFrontier(newFrontier(frSID))
	if err != nil {
		t.Fatal(err)
	}
	if want := `"through":{"seq":0,"steer":false,"at_unix_ms":0,"id":""},`; !bytes.Contains(empty, []byte(want)) {
		t.Errorf("an empty frontier does not hold %s:\n%s", want, empty)
	}
}

// TestFrontier_KeyMismatchMisses (W-M4.2, ruling 13:59 (3)): a frontier of
// another fold_version or ledger_version does not parse, so a cache reader
// misses and folds in full instead of continuing state folded under other
// rules or another ledger's ids.
func TestFrontier_KeyMismatchMisses(t *testing.T) {
	b, err := MarshalFrontier(sampleFrontier())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ParseFrontier(b); err != nil {
		t.Fatalf("the frontier of this key did not parse: %v", err)
	}
	for _, c := range []struct {
		key string
		v   int
	}{{"fold_version", foldVersion}, {"ledger_version", ledgerVersion}} {
		this := `"` + c.key + `":` + strconv.Itoa(c.v) + `,`
		for _, v := range []int{0, c.v + 1} {
			other := bytes.Replace(b, []byte(this), []byte(`"`+c.key+`":`+strconv.Itoa(v)+`,`), 1)
			if bytes.Equal(other, b) {
				t.Fatalf("no %s in %s", this, b)
			}
			if _, err := ParseFrontier(other); err == nil {
				t.Errorf("a frontier with %s %d parsed", c.key, v)
			}
		}
	}
}

// TestFrontier_ParseRejectsMalformed (W-M4.2): a frontier file with a field
// this schema does not name, or anything but space after its object, does not
// parse, so a cache reader misses and folds in full instead of trusting a torn
// or foreign file. A trailing newline is space and parses.
func TestFrontier_ParseRejectsMalformed(t *testing.T) {
	b, err := MarshalFrontier(sampleFrontier())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ParseFrontier(append(append([]byte{}, b...), '\n')); err != nil {
		t.Errorf("a frontier with a trailing newline did not parse: %v", err)
	}
	for _, c := range []struct {
		name string
		b    []byte
	}{
		{"unknown field", bytes.Replace(b, []byte(`{"schema":1,`), []byte(`{"schema":1,"repo":"r",`), 1)},
		{"stray brace", append(append([]byte{}, b...), '}')},
		{"stray bracket", append(append([]byte{}, b...), ']')},
		{"second object", append(append([]byte{}, b...), b...)},
		{"cut short", b[:len(b)-1]},
	} {
		if _, err := ParseFrontier(c.b); err == nil {
			t.Errorf("%s: parsed", c.name)
		}
	}
}

// frontierStrings names what every string in a frontier refers to (ruling
// 2026-11-12 09:38 Q2: words by reference, kept as shape). A string field is
// where prompt words would go; one not listed here fails by its path.
var frontierStrings = map[string]string{
	"Through.ID":          "steer line id of the last applied event, empty for a record",
	"Session":             "session id",
	"Continues":           "session id",
	"Intents[].ID":        "prompt node id, graph.PromptNodeID",
	"Intents[].Digest":    "prompt digest",
	"Intents[].Status":    "ReqStatus, open only in v1",
	"Reqs[].ID":           "a Req's key, Pred:Arg (D6)",
	"Reqs[].Pred":         "compliance predicate name",
	"Reqs[].Arg":          "a unit's path as its line holds it or its keyed digest (record scope v4), a symbol, a gate family or a test id",
	"Reqs[].Status":       "ReqStatus",
	"Reqs[].SupersededBy": "prompt node id, graph.PromptNodeID, of the prompt that superseded it",
	"Decisions[].IDs[]":   "graph node id",
	"Decisions[].Action":  "decision action name",
	"Touched[]":           "file id, graph.FileID of a project-relative path",
	"Obligations[].ID":    "obligation id",
	"Obligations[].Kind":  "obligation kind name",
	"Obligations[].Unit":  "unit id",
	"Obligations[].State": "obligation state name",
	"Delivered[].ID":      "steer line id",
	"Delivered[].Key":     "line key",
	"Delivered[].Kind":    "line kind name",
	"Delivered[].Unit":    "unit id",
}

func stringLeaves(t reflect.Type, path string, out *[]string) {
	switch t.Kind() {
	case reflect.String:
		*out = append(*out, path)
	case reflect.Slice:
		stringLeaves(t.Elem(), path+"[]", out)
	case reflect.Struct:
		for i := 0; i < t.NumField(); i++ {
			p := t.Field(i).Name
			if path != "" {
				p = path + "." + p
			}
			stringLeaves(t.Field(i).Type, p, out)
		}
	case reflect.Map, reflect.Interface, reflect.Pointer:
		*out = append(*out, path+" ("+t.Kind().String()+")")
	}
}

// TestFrontier_WordsByReference (W-M4.2, AC-M4.1): every string a frontier
// holds is an id, a digest or a closed name; an intent is the prompt's node
// id and digest, never its text. The frontier holds no map, interface or
// pointer, so its marshalled order is its sorted order.
func TestFrontier_WordsByReference(t *testing.T) {
	var got []string
	stringLeaves(reflect.TypeOf(Frontier{}), "", &got)
	sort.Strings(got)
	seen := map[string]bool{}
	for _, p := range got {
		seen[p] = true
		if _, ok := frontierStrings[p]; !ok {
			t.Errorf("%s: a string not named as a reference; name what it refers to or keep words out", p)
		}
	}
	for p := range frontierStrings {
		if !seen[p] {
			t.Errorf("%s: named but gone; drop it from frontierStrings", p)
		}
	}
	var fields []string
	for i := 0; i < reflect.TypeOf(IntentRef{}).NumField(); i++ {
		fields = append(fields, reflect.TypeOf(IntentRef{}).Field(i).Name)
	}
	if want := []string{"ID", "Digest", "StatedAt", "Status"}; !reflect.DeepEqual(fields, want) {
		t.Errorf("IntentRef fields = %v, want %v", fields, want)
	}
}

// TestReqStatus_MetOnceIsHistory (W-M4.2, ruling 13:11 (3)): the fold's mark
// is history and its stored form says so, met_once, never "met", which a
// reader could take for a requirement that holds now.
func TestReqStatus_MetOnceIsHistory(t *testing.T) {
	b, err := json.Marshal(ReqState{Status: ReqMetOnce})
	if err != nil || !strings.Contains(string(b), `"status":"met_once"`) {
		t.Errorf("ReqState JSON %s, %v; want status met_once", b, err)
	}
}
