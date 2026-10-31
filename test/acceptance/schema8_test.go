package acceptance

import (
	"slices"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/shape"
	"github.com/altrace-dev-role/rashomon/internal/store"
)

// TestSchema8_TheExecutionNamesItsRunnerAndTheStageThatHidItsStatus
// (MASKED-OUTCOME1, D229): the contract gates runner_name and masked_by at
// schema 8, holds masked_by to the binary's closed words and carries no more
// of the line than they are; and a piped runner's execution record, written
// by the built binary, holds both words, where the declaration holds
// neither. Break: write the stage's own text, and the enum and the
// leak check both fail.
func TestSchema8_TheExecutionNamesItsRunnerAndTheStageThatHidItsStatus(t *testing.T) {
	if store.CallSchemaVersion < 8 {
		t.Fatalf("CallSchemaVersion = %d, want 8 or later", store.CallSchemaVersion)
	}
	defs := schemaDefs(t)
	exec, _ := defs["execution"].(map[string]any)
	gate := versionGate(exec, 8)
	for _, k := range []string{"runner_name", "masked_by"} {
		if !slices.Contains(gate, k) {
			t.Errorf("$defs.execution's schema 8 gate does not require %s", k)
		}
	}
	for _, name := range []string{"declaration", "terminal"} {
		def, _ := defs[name].(map[string]any)
		props, _ := def["properties"].(map[string]any)
		for _, k := range []string{"runner_name", "masked_by"} {
			if _, ok := props[k]; ok {
				t.Errorf("$defs.%s declares %s, which only the execution carries", name, k)
			}
		}
	}
	props, _ := exec["properties"].(map[string]any)
	by, _ := props["masked_by"].(map[string]any)
	var enum []string
	var nullOK bool
	for _, raw := range toAnySlice(by["enum"]) {
		if raw == nil {
			nullOK = true
			continue
		}
		s, _ := raw.(string)
		enum = append(enum, s)
	}
	if !nullOK || !slices.Equal(enum, shape.MaskedByWords()) {
		t.Errorf("masked_by enum %v (null %v), want null and shape.MaskedByWords() %v", enum, nullOK, shape.MaskedByWords())
	}

	s := newTBSession(t)
	s.shell("make test 2>&1 | tail -40", true, "")
	s.shell("make test 2>&1 | head -40", true, "")
	s.shell("ls", true, "")
	execs := s.e.executions(testSession)
	if len(execs) != 3 {
		t.Fatalf("%d executions, want 3", len(execs))
	}
	for i, want := range []struct{ name, by any }{{"make test", "tail"}, {"make test", "head"}, {nil, nil}} {
		name, ok := execs[i].fields["runner_name"]
		if !ok || name != want.name {
			t.Errorf("execution %d: runner_name %v (present %v), want %v", i, name, ok, want.name)
		}
		by, ok := execs[i].fields["masked_by"]
		if !ok || by != want.by {
			t.Errorf("execution %d: masked_by %v (present %v), want %v", i, by, ok, want.by)
		}
	}
	for _, d := range s.e.declarations(testSession) {
		for _, k := range []string{"runner_name", "masked_by"} {
			if _, ok := nested(d, "shape."+k); ok {
				t.Errorf("a declaration's shape carries %s", k)
			}
			if _, ok := d.fields[k]; ok {
				t.Errorf("a declaration carries %s", k)
			}
		}
	}
}
