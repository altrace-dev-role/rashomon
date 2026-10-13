package steer

import (
	"reflect"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/steer/pattern"
)

// TestContext_IsEmptyPlaceholder pins that Context is not frozen in P-07
// (the gaps table closes it before P-30 opens): zero fields today, so adding
// the real ones later is additive, not a breaking change to anything that
// depends only on the type existing.
func TestContext_IsEmptyPlaceholder(t *testing.T) {
	if n := reflect.TypeOf(Context{}).NumField(); n != 0 {
		t.Errorf("Context has %d fields, want 0 (not frozen until P-30)", n)
	}
}

// TestRoute_StubReturnsEmptyOutput pins this package's one deliberate
// behavioural gap: Route decides nothing yet (tech 4.2.4's router is P-30's),
// and the zero Output is the only honest value for "no decision was made."
func TestRoute_StubReturnsEmptyOutput(t *testing.T) {
	findings := []Finding{{Pattern: pattern.SubtaskGate, AfterSeq: 1}}
	got := Route(&Context{}, findings)
	if got != (Output{}) {
		t.Errorf("Route() = %+v, want the zero Output", got)
	}
}

func TestRoute_StubIgnoresNilInputs(t *testing.T) {
	got := Route(nil, nil)
	if got != (Output{}) {
		t.Errorf("Route(nil, nil) = %+v, want the zero Output", got)
	}
}
