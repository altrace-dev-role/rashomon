package intent

import (
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/graph/engine"
	"github.com/altrace-dev-role/rashomon/internal/spec"
)

// TestTrusted_EqualsEngineRuleTrust: intent's trusted set and engine's
// RuleTrust are two copies of one rule (engine does not import intent), so
// they must agree on every value. spec.Trusts leaves Builtin out, the value
// a copy is most likely to drop, so it is added here beside the empty and an
// unknown word. Controls: Trusted is true on both sides, Untrusted and the
// unknown word false on both.
func TestTrusted_EqualsEngineRuleTrust(t *testing.T) {
	values := append(append([]spec.Trust{}, spec.Trusts...), spec.Builtin, spec.Trust(""), spec.Trust("bogus"))
	for _, v := range values {
		if a, b := trusted(v), engine.RuleTrust(v); a != b {
			t.Errorf("trust %q: intent.trusted = %v, engine.RuleTrust = %v", v, a, b)
		}
	}
	for v, want := range map[spec.Trust]bool{spec.Trusted: true, spec.Builtin: true, spec.Untrusted: false, spec.Trust("bogus"): false} {
		if trusted(v) != want || engine.RuleTrust(v) != want {
			t.Errorf("control %q: intent %v engine %v, want both %v", v, trusted(v), engine.RuleTrust(v), want)
		}
	}
}
