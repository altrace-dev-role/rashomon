package store

import (
	"reflect"
	"strconv"
	"testing"
)

// TestREC_PrefixReadsEveryKindVersion: prefixPair reads the pair of every
// kind at every version this reader accepts, from the line the store writes
// for it (marshalLine over the kind's own type), and of a version three
// digits long; it declines every other form, so those lines take the decode
// path instead (2.3b R1, A11). Each row of the second table is the input a
// prefix mutant misreads: a version of four digits, a missing comma, a kind
// not in the table, leading whitespace, an escaped kind name.
func TestREC_PrefixReadsEveryKindVersion(t *testing.T) {
	for _, kind := range sortedKinds() {
		for v := 1; v <= CallSchemaVersion+1; v++ {
			if !AcceptsKind(kind, v) {
				continue
			}
			line := writerLine(t, writerRecord(kind, v))
			if k, got, ok := prefixPair(line); !ok || k != kind || got != v {
				t.Errorf("%s: prefixPair = %q, %d, %v; want %q, %d, true", line, k, got, ok, kind, v)
			}
		}
	}
	if k, v, ok := prefixPair([]byte(`{"type":"declaration","schema_version":100,"seq":1}`)); !ok || k != TypeDeclaration || v != 100 {
		t.Errorf("three digits: prefixPair = %q, %d, %v; want declaration, 100, true", k, v, ok)
	}
	for _, c := range []struct{ name, line string }{
		{"four digits", `{"type":"declaration","schema_version":1000,"seq":1}`},
		{"no comma", `{"type":"declaration","schema_version":6}`},
		{"unknown kind", `{"type":"future_kind","schema_version":5,"seq":1}`},
		{"leading space", ` {"type":"declaration","schema_version":6,"seq":1}`},
		{"escaped kind", `{"type":"decl` + escapedA + `ration","schema_version":6,"seq":1}`},
		{"foreign order", `{"schema_version":6,"type":"declaration","seq":1}`},
		{"signed version", `{"type":"declaration","schema_version":-6,"seq":1}`},
		{"no digits", `{"type":"declaration","schema_version":,"seq":1}`},
		{"cut in kind", `{"type":"declar`},
		{"empty", ``},
	} {
		if k, v, ok := prefixPair([]byte(c.line)); ok {
			t.Errorf("%s: prefixPair(%s) = %q, %d, true; want false", c.name, c.line, k, v)
		}
	}
}

// TestREC_ForeignOrderFallsBack: a record whose keys are in another order
// than the writer's is not read from its prefix, and the decode path reads it
// whole, as it always has.
func TestREC_ForeignOrderFallsBack(t *testing.T) {
	line := []byte(`{"schema_version":6,"type":"declaration","seq":3,"tool_use_id":"toolu_f","session_id":"sess-1p"}`)
	if _, _, ok := prefixPair(line); ok {
		t.Fatalf("prefixPair read %s", line)
	}
	var run Run
	classifyRunLine(&run, line)
	if run.Skipped != 0 || len(run.Declarations) != 1 || run.Declarations[0].Seq != 3 || run.Declarations[0].ToolUseID != "toolu_f" {
		t.Errorf("read %+v, want the one declaration, seq 3, toolu_f", run)
	}
}

// TestREC_RefusedPairStillReadsCritical: a line in the writer's form whose
// pair AcceptsKind refuses still has its critical key read (1.2.2): false is
// Ignored, true or no key is Skipped, for a call kind above its version, a
// kind of 1.2.0 below its one, and coverage above its own. Every line here is
// read from its prefix, so these are the fast path's refused branch.
func TestREC_RefusedPairStillReadsCritical(t *testing.T) {
	next := strconv.Itoa(CallSchemaVersion + 1)
	for _, c := range []struct {
		name, line string
		want       Run
	}{
		{"call noncrit", `{"type":"declaration","schema_version":` + next + `,"critical":false}`, Run{Ignored: 1}},
		{"call crit", `{"type":"declaration","schema_version":` + next + `,"critical":true}`, Run{Skipped: 1}},
		{"call no key", `{"type":"execution","schema_version":` + next + `,"seq":1}`, Run{Skipped: 1}},
		{"event noncrit", `{"type":"turn_end","schema_version":4,"critical":false}`, Run{Ignored: 1}},
		{"coverage noncrit", `{"type":"coverage","schema_version":4,"critical":false}`, Run{Ignored: 1}},
	} {
		line := []byte(c.line)
		if _, _, ok := prefixPair(line); !ok {
			t.Errorf("%s: prefixPair declined %s; the case must take the fast path", c.name, line)
		}
		var got Run
		classifyRunLine(&got, line)
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s: read %+v, want %+v", c.name, got, c.want)
		}
	}
}

// TestREC_MalformedBodyAfterValidPrefixIsSkipped: a line whose prefix is a
// valid pair but whose body does not decode into its kind -- a field of the
// wrong type, a cut line, bytes after the object -- is Skipped and collects
// nothing, on the typed and the header branch alike.
func TestREC_MalformedBodyAfterValidPrefixIsSkipped(t *testing.T) {
	for _, line := range []string{
		`{"type":"declaration","schema_version":6,"seq":"x"}`,
		`{"type":"execution","schema_version":6,"seq":`,
		`{"type":"terminal","schema_version":6,"seq":1} x`,
		`{"type":"turn_end","schema_version":5,"critical":false,"seq":"one"}`,
	} {
		if _, _, ok := prefixPair([]byte(line)); !ok {
			t.Errorf("prefixPair declined %s; the case must take the fast path", line)
		}
		var got Run
		classifyRunLine(&got, []byte(line))
		if !reflect.DeepEqual(got, Run{Skipped: 1}) {
			t.Errorf("%s: read %+v, want only Skipped 1", line, got)
		}
	}
}
