package acceptance

import "testing"

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

// TestSchemaVersionEnumAdmits5OnTerminalOnly: a terminal declaring
// schema_version 5 -- the flush_failed marker -- must be allowed to say so.
// Declaration and execution make no such claim: this build never writes
// either of them at 5, and admitting a version nothing writes is a contract
// nobody can rely on (the same reasoning TestSchema3_WrittenRecordsMeetTheVersion3Gate's
// comment gives for comparing against a literal rather than the writer's
// own constant).
func TestSchemaVersionEnumAdmits5OnTerminalOnly(t *testing.T) {
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

	if !admits("terminal", 5) {
		t.Error("$defs.terminal.schema_version does not admit 5; the flush_failed marker " +
			"fails the contract even though the reader accepts it")
	}
	for _, name := range []string{"declaration", "execution"} {
		if admits(name, 5) {
			t.Errorf("$defs.%s.schema_version admits 5, but this build never writes one at "+
				"that version; the contract should not claim a shape nothing produces", name)
		}
	}
}

// TestSchema5_OrdinaryCallRecordsStayAtSchemaFour is the direct regression
// test for the hazard this file's own doc comment names: an ordinary call,
// with no flush failure anywhere near it, must write its declaration,
// execution and terminal at schema 4, never 5. Compared against the
// literal, not store.SchemaVersion, for the reason
// TestSchema3_WrittenRecordsMeetTheVersion3Gate's comment gives: comparing
// the writer with itself cannot catch the writer moving. Break: stamp any
// of the three at store.FlushFailedSchemaVersion, or collapse
// FlushFailedSchemaVersion back onto SchemaVersion.
func TestSchema5_OrdinaryCallRecordsStayAtSchemaFour(t *testing.T) {
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
			if got := r.fields["schema_version"]; got != float64(4) {
				t.Errorf("%s %d written at schema_version %v, want 4", c.kind, i, got)
			}
		}
	}
}
