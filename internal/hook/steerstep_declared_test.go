package hook

import (
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/spec"
)

// dgLine is stale-evidence's note for a trusted spec's gate "unit": named by
// its id, not its program (D222).
const dgLine = "rashomon record: gate 'unit' exited 0 earlier in this session (2 min ago); this call then wrote 'internal/a.go'. " +
	"No later run of the gate is recorded."

// dgFix is a fixture in a git project whose spec declares one required gate,
// "unit", `go test ./...`, bound as trusted when bound, with no project index.
func dgFix(t *testing.T, doc string, bound bool) *stFix {
	t.Helper()
	s := newSpecTree(t)
	s.mkdir(".git")
	path := s.put(s.base, doc)
	f := newST(t)
	f.cwd = s.base
	if bound {
		_, digest, err := spec.Parse([]byte(doc))
		if err != nil {
			t.Fatal(err)
		}
		writeBindings(t, f.st.Root(), spec.Binding{ProjectRoot: s.base, SpecPath: path, SpecDigest: digest, BoundAtMS: 1}, false)
	}
	return f
}

// TestDG2d_TheWriteAfterADeclaredGreenNotesWithNoIndex (card D222): in a
// project with no learned index, a trusted spec's required gate, green, then
// an edit of a counted file prints the stale-evidence note naming the gate
// by its id, and takes the arm.
func TestDG2d_TheWriteAfterADeclaredGreenNotesWithNoIndex(t *testing.T) {
	f := dgFix(t, specOneGate, true)
	f.green()
	f.editOf("toolu_e", "internal/a.go").Steer(snEnv())
	if got := staleLines(t, f.st); len(got) != 1 || got[0].ModelText != dgLine {
		t.Fatalf("%d stale-evidence lines (%+v), want one carrying %q", len(got), got, dgLine)
	}
	if armed, _ := f.st.StaleArmed(stSession); armed {
		t.Error("the judged edit left the trigger armed")
	}
}

// TestDG2e_NoDeclaredRequiredGateNotesNothingWithNoIndex: the same session
// raises nothing where the spec is not bound (untrusted), where there is no
// spec, or where its only gate is advisory; and the arm is taken
// all the same, so a later write does not read the run again.
func TestDG2e_NoDeclaredRequiredGateNotesNothingWithNoIndex(t *testing.T) {
	advisory := `{"rashomon_spec": 1, "gates": [{"id": "unit", "kind": "test", "argv": ["go", "test", "./..."], "required": false}]}`
	for _, c := range []struct {
		name  string
		doc   string
		bound bool
	}{
		{"untrusted spec", specOneGate, false},
		{"advisory gate", advisory, true},
		{"no spec", "", false},
	} {
		t.Run(c.name, func(t *testing.T) {
			f := dgFix(t, c.doc, c.bound)
			if c.doc == "" {
				f = newSN(t)
			}
			f.green()
			f.editOf("toolu_e", "internal/a.go").Steer(snEnv())
			if got := staleLines(t, f.st); len(got) != 0 {
				t.Errorf("%d stale-evidence lines, want 0", len(got))
			}
			if armed, _ := f.st.StaleArmed(stSession); armed {
				t.Error("the trigger is still armed after the first counted write")
			}
		})
	}
}
