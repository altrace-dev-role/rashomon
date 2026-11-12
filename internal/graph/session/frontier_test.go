package session

import (
	"bytes"
	"encoding/json"
	"reflect"
	"sort"
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
	f.Continues = frPrev
	f.Intents = []IntentRef{
		{ID: string(graph.PromptNodeID(frSID, 2)), Digest: strings.Repeat("b", 16), StatedAt: 40, Status: ReqOpen},
		{ID: string(graph.PromptNodeID(frSID, 1)), Digest: strings.Repeat("a", 16), StatedAt: 3, Status: ReqMet},
	}
	f.Reqs = []ReqState{
		{ID: "unit:b", Pred: "opened_unit", Arg: "b", Status: ReqOpen, StatedAt: 41},
		{ID: "gate:go-test", Pred: "gate_green", Arg: "go-test", Status: ReqMet, StatedAt: 4, MetAt: 9},
		{ID: "unit:a", Pred: "opened_unit", Arg: "a", Status: ReqSuperseded, StatedAt: 4, SupersededBy: "unit:b"},
	}
	f.Decisions = []DecisionRef{{IDs: []string{"n2", "n1"}, Action: "leave_alone"}, {IDs: []string{"n0"}, Action: "use"}}
	f.Touched = []string{"unit:z", "unit:a"}
	f.Obligations = []ObligationRef{{ID: "ob2", Kind: "gate", Unit: "unit:a", State: "owed"}, {ID: "ob1", Kind: "gate", Unit: "unit:z", State: "met"}}
	f.ObligationsAt = 30
	f.ObligationsOmitted = 2
	f.Delivered = []LineRef{{ID: "l2", Key: "unit:b", Kind: "unit", Unit: "b", Seq: 41}, {ID: "l1", Key: "unit:a", Kind: "unit", Unit: "a", Seq: 4}}
	f.Verdicts = []VerdictRef{{Seq: 30, Done: false, Owed: 1}, {Seq: 12, Done: true}}
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
	"Session":             "session id",
	"Continues":           "session id",
	"Intents[].ID":        "prompt node id, graph.PromptNodeID",
	"Intents[].Digest":    "prompt digest",
	"Intents[].Status":    "ReqStatus",
	"Reqs[].ID":           "line key",
	"Reqs[].Pred":         "compliance predicate name",
	"Reqs[].Arg":          "unit, symbol, gate family or test id",
	"Reqs[].Status":       "ReqStatus",
	"Reqs[].SupersededBy": "line key",
	"Decisions[].IDs[]":   "graph node id",
	"Decisions[].Action":  "decision action name",
	"Touched[]":           "unit id",
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
