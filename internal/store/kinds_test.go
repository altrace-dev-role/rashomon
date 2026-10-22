package store

import (
	"reflect"
	"testing"
)

// Every kind the 1.2.0 record carries, a literal copy of the Contracts and
// names list in the technical spec. A kind added without a version, or a
// version moved, is a contract change and shows up here.
var frozenKindVersions = map[string]int{
	"declaration": 5, "execution": 5, "terminal": 5,
	"coverage": 3, "gap": 3,
	"session": 5, "prompt": 5, "turn_end": 5, "turn_cost": 5, "human": 5, "subagent": 5,
	"batch": 5, "task": 5, "compact": 5, "instructions": 5, "model": 5,
	"verify": 5, "outcome_join": 5, "write_observed": 5,
}

func TestKindVersionsAreTheFrozenTable(t *testing.T) {
	if len(kindVersion) != len(frozenKindVersions) {
		t.Fatalf("%d kinds, want %d", len(kindVersion), len(frozenKindVersions))
	}
	for kind, want := range frozenKindVersions {
		if got, ok := KindVersion(kind); !ok || got != want {
			t.Errorf("KindVersion(%q) = %d (present %v), want %d", kind, got, ok, want)
		}
	}
	if _, steer := KindVersion("steer"); steer {
		t.Error("a steer kind is in the table; ED-1 keeps steer lines out of the record")
	}
}

// TestKindVersionIsReadOnly: the table behind KindVersion is a function,
// not an exported map, so no importer can change what a kind accepts by
// writing into it (adversarial 8).
func TestKindVersionIsReadOnly(t *testing.T) {
	typ := reflect.TypeOf(KindVersion)
	if typ.Kind() != reflect.Func {
		t.Fatalf("KindVersion is a %s, want a func", typ.Kind())
	}
	if typ.String() != "func(string) (int, bool)" {
		t.Errorf("KindVersion is %s, want func(string) (int, bool)", typ)
	}
}

func TestAcceptsKindFollowsTheAcceptanceTable(t *testing.T) {
	cases := []struct {
		kind    string
		version int
		want    bool
	}{
		{"declaration", 1, true}, {"declaration", 4, true}, {"declaration", 5, true}, {"declaration", 6, false}, {"declaration", 0, false},
		{"execution", 5, true}, {"terminal", 3, true},
		{"coverage", 1, true}, {"coverage", 3, true}, {"coverage", 4, false},
		{"gap", 3, true}, {"gap", 5, false},
		{"session", 5, true}, {"session", 4, false}, {"session", 6, false},
		{"verify", 5, true}, {"write_observed", 5, true},
		{"steer", 1, false}, {"nothing", 5, false},
	}
	for _, c := range cases {
		if got := AcceptsKind(c.kind, c.version); got != c.want {
			t.Errorf("AcceptsKind(%q, %d) = %v, want %v", c.kind, c.version, got, c.want)
		}
	}
}

// TestREC1_WriterStampsTheCallKindsAtFive: the call records are written at
// CallSchemaVersion, which is KindVersion's entry for each of them, and the
// session records stay at 3, so an older reader keeps every session (1.2.2).
func TestREC1_WriterStampsTheCallKindsAtFive(t *testing.T) {
	if SchemaVersion != CallSchemaVersion {
		t.Errorf("SchemaVersion = %d, want CallSchemaVersion %d", SchemaVersion, CallSchemaVersion)
	}
	for _, kind := range []string{TypeDeclaration, TypeExecution, TypeTerminal} {
		if v, _ := KindVersion(kind); v != SchemaVersion {
			t.Errorf("KindVersion(%s) = %d, but the writer stamps %d", kind, v, SchemaVersion)
		}
	}
	if FlushFailedSchemaVersion != 5 {
		t.Errorf("FlushFailedSchemaVersion = %d; REC-34's flush_failed marker is written at 5", FlushFailedSchemaVersion)
	}
	if CallSchemaVersion != 5 || EventSchemaVersion != 5 || SessionSchemaVersion != 3 {
		t.Errorf("version constants moved: call %d event %d session %d", CallSchemaVersion, EventSchemaVersion, SessionSchemaVersion)
	}
}
