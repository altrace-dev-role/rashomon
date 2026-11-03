package gateinfer

import "testing"

// TestGI1_AMakefileVariableIsNoTarget: `test := ...` and `test ?= ...` set a
// variable named test; only a rule line declares the target, and a recipe
// line (tab-led) is no rule. The decoys fall to go.mod. Break: read any
// "test" before a colon as the target.
func TestGI1_AMakefileVariableIsNoTarget(t *testing.T) {
	for _, mk := range []string{"test := go test ./...\n", "test ?= x\n", "all:\n\ttest: x\n"} {
		got, ok := Infer(project(t, map[string]string{"Makefile": mk, "go.mod": goMod}))
		if !ok || got.From != "go.mod" {
			t.Errorf("Makefile %q: Infer = %+v, %v; want go.mod's command", mk, got, ok)
		}
	}
	if got, ok := Infer(project(t, map[string]string{"Makefile": "test: build\n\tgo test ./...\n"})); !ok || got.From != "Makefile" {
		t.Errorf("control, a test rule: Infer = %+v, %v; want make test", got, ok)
	}
}
