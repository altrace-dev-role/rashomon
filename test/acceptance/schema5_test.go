package acceptance

import (
	"slices"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/store"
)

// REC-34 -- schema 5 is the flush_failed marker's own version
// (store.FlushFailedSchemaVersion), not the call records' (SchemaVersion).
//
// The hazard this file guards is the one an earlier shape of this fix
// actually shipped: stamping every declaration, execution and ordinary
// terminal at the marker's version the moment group commit landed, whether
// or not that run ever lost a flush. A 1.1.x reader has no idea what schema
// 5 is; Accepts runs before the type switch, so it would have skipped every
// one of those records, not only the one marker it was never going to
// understand anyway. Phase is the field that is actually new; gating it the
// way host_source and rule_match were gated at schema 3
// (TestSchema3IsRequiredOnlyAtVersion3) is what keeps a terminal already on
// disk validating against the contract it was written under.

// TestSchema5PhaseIsDeclaredButNotGloballyRequired is
// TestSchema3FieldsAreDeclaredButNotGloballyRequired's twin for REC-34's one
// new field. It must be declared, so a v5 terminal carrying it validates
// and additionalProperties:false does not reject it; and it must not be in
// terminal's top-level required, or every terminal ever written before this
// field existed fails the published contract.
func TestSchema5PhaseIsDeclaredButNotGloballyRequired(t *testing.T) {
	defs := schemaDefs(t)
	def, ok := defs["terminal"].(map[string]any)
	if !ok {
		t.Fatal("$defs.terminal is missing")
	}
	props, _ := def["properties"].(map[string]any)
	required := toStringSet(def["required"])

	if _, declared := props["phase"]; !declared {
		t.Error("terminal does not declare \"phase\". With additionalProperties false, a v5 " +
			"terminal carrying it is REJECTED by the published contract.")
	}
	if required["phase"] {
		t.Error("terminal lists \"phase\" in the top-level required. Every terminal already " +
			"on disk from before this field existed lacks it, so they all stop validating " +
			"against the contract they were written under.")
	}
}

// TestSchema5PhaseIsRequiredOnlyAtVersion5 pins the mechanism, not just the
// absence: there has to be a conditional keyed on schema_version that makes
// phase required when, and only when, the record says it is v5.
func TestSchema5PhaseIsRequiredOnlyAtVersion5(t *testing.T) {
	defs := schemaDefs(t)
	def, ok := defs["terminal"].(map[string]any)
	if !ok {
		t.Fatal("$defs.terminal is missing")
	}
	gated := versionGate(def, 5)
	var found bool
	for _, k := range gated {
		if k == "phase" {
			found = true
		}
	}
	if !found {
		t.Error("no `if schema_version == 5 then required: [... \"phase\" ...]` in " +
			"$defs.terminal. Without it the field is optional at every version, and a v5 " +
			"terminal that forgot it still passes the contract.")
	}
}

// TestSchemaVersionEnumAdmits5OnEveryCallRecord: the declaration, the
// execution and the terminal were written at 5 from their v5 keys (P-11)
// until runner_kind moved them to 6 (D11), and the flush_failed marker is
// still written at 5, so each must be allowed to say both; none may admit
// the version one above CallSchemaVersion, which nothing writes and would be
// a contract nobody can rely on (the same reasoning
// TestSchema3_WrittenRecordsMeetTheVersion3Gate's comment gives for comparing
// against a literal rather than the writer's own constant).
func TestSchemaVersionEnumAdmits5OnEveryCallRecord(t *testing.T) {
	defs := schemaDefs(t)

	admits := func(name string, v int) bool {
		def, ok := defs[name].(map[string]any)
		if !ok {
			t.Fatalf("$defs.%s is missing", name)
		}
		props, _ := def["properties"].(map[string]any)
		sv, _ := props["schema_version"].(map[string]any)
		if sv == nil {
			t.Fatalf("$defs.%s has no schema_version property", name)
		}
		for _, raw := range toAnySlice(sv["enum"]) {
			if n, ok := raw.(float64); ok && int(n) == v {
				return true
			}
		}
		return false
	}

	for _, name := range []string{"declaration", "execution", "terminal"} {
		if !admits(name, 5) {
			t.Errorf("$defs.%s.schema_version does not admit 5; a record this build writes "+
				"fails the contract even though the reader accepts it", name)
		}
		if !admits(name, 6) {
			t.Errorf("$defs.%s.schema_version does not admit 6; a record this build writes "+
				"fails the contract even though the reader accepts it", name)
		}
		// One above CallSchemaVersion: a version nothing writes yet.
		if next := store.CallSchemaVersion + 1; admits(name, next) {
			t.Errorf("$defs.%s.schema_version admits %d, but nothing writes one at that "+
				"version; the contract should not claim a shape nothing produces", name, next)
		}
	}
}

// TestSchema5_OrdinaryCallRecordsAreWrittenAtTheCallVersion: an ordinary
// call, with no flush failure anywhere near it, writes its declaration,
// execution and terminal at schema 6, each naming the binary that wrote it. Compared
// against the literal, not store.SchemaVersion, for the reason
// TestSchema3_WrittenRecordsMeetTheVersion3Gate's comment gives: comparing
// the writer with itself cannot catch the writer moving. Break: leave
// SchemaVersion at 5, or drop writer from any of the three.
func TestSchema5_OrdinaryCallRecordsAreWrittenAtTheCallVersion(t *testing.T) {
	e := newEnv(t)
	e.watched(testSession)
	e.mustHook(defaultPayload().build(t))
	e.mustPost(defaultPost().build(t))

	for _, c := range []struct {
		kind string
		recs []record
	}{
		{"declaration", e.declarations(testSession)},
		{"execution", e.executions(testSession)},
		{"terminal", recordsOfType(e.records(testSession), "terminal")},
	} {
		if len(c.recs) == 0 {
			t.Fatalf("no %s record was written", c.kind)
		}
		for i, r := range c.recs {
			if got := r.fields["schema_version"]; got != float64(8) {
				t.Errorf("%s %d written at schema_version %v, want 8", c.kind, i, got)
			}
			if w, _ := r.fields["writer"].(string); w == "" {
				t.Errorf("%s %d names no writer: %v", c.kind, i, r.fields["writer"])
			}
		}
	}
}

// TestSchema6_EveryCallGateHoldsTheGateBeforeIt: a record at 6 carries every
// key a record at 5 does, and runner_kind on the declaration's shape and the
// execution (D11), and the execution's content_sha256 (card P5A, added in
// place: v6 is unreleased, no tag contains a65f9ac9): each call kind's v6
// gate is its v5 gate and those keys.
// The terminal gains nothing, so its v6 gate is its v5 gate: without it a v6
// terminal that names no phase or writer would pass the contract. Break:
// drop a key from a v6 gate, or runner_kind from either.
func TestSchema6_EveryCallGateHoldsTheGateBeforeIt(t *testing.T) {
	defs := schemaDefs(t)
	for _, c := range []struct {
		def   string
		added []string
	}{
		{"declaration", []string{"shape.runner_kind"}},
		{"execution", []string{"content_sha256", "runner_kind"}},
		{"terminal", nil},
	} {
		def, ok := defs[c.def].(map[string]any)
		if !ok {
			t.Fatalf("$defs.%s is missing", c.def)
		}
		v5, v6 := versionGate(def, 5), versionGate(def, 6)
		if len(v5) == 0 || len(v6) == 0 {
			t.Fatalf("$defs.%s: %d keys gated at 5 and %d at 6; want both gates", c.def, len(v5), len(v6))
		}
		var added []string
		for _, k := range v6 {
			if !slices.Contains(v5, k) {
				added = append(added, k)
			}
		}
		for _, k := range v5 {
			if !slices.Contains(v6, k) {
				t.Errorf("$defs.%s: %q is required at 5 and not at 6", c.def, k)
			}
		}
		if slices.Sort(added); !slices.Equal(added, c.added) {
			t.Errorf("$defs.%s: the v6 gate adds %q to the v5 gate, want %q", c.def, added, c.added)
		}
	}
}
