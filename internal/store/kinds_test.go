package store

import "testing"

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
	if len(KindVersion) != len(frozenKindVersions) {
		t.Fatalf("%d kinds, want %d", len(KindVersion), len(frozenKindVersions))
	}
	for kind, want := range frozenKindVersions {
		if got, ok := KindVersion[kind]; !ok || got != want {
			t.Errorf("KindVersion[%q] = %d (present %v), want %d", kind, got, ok, want)
		}
	}
	if _, steer := KindVersion["steer"]; steer {
		t.Error("a steer kind is in the table; ED-1 keeps steer lines out of the record")
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

// The writer has not moved: a stubs-only change must not change what the
// store writes. P-11 flips SchemaVersion to CallSchemaVersion with the v5
// fields, and this test is the one that changes then.
func TestWriterStillStampsSchemaVersionFour(t *testing.T) {
	if SchemaVersion != 4 {
		t.Errorf("SchemaVersion = %d; the call-record writer moves to 5 in P-11, not here", SchemaVersion)
	}
	if CallSchemaVersion != 5 || EventSchemaVersion != 5 || SessionSchemaVersion != 3 {
		t.Errorf("version constants moved: call %d event %d session %d", CallSchemaVersion, EventSchemaVersion, SessionSchemaVersion)
	}
}
