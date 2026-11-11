package acceptance

// H-175: the regression-location note names the writes between a gate's
// green run and its red run, and a write is an edit only where its call
// ended ok. A green `go test ./...`, then `./gen > internal/gen.go` failing
// with exit 1, then the same test red: no completed write stands between the
// two runs, so the red's post raises no regression note. The shell truncated
// gen.go before gen ran, so the base's "Files written between the two runs:
// 'internal/gen.go'" was true here, and is now lost: the note has no closed
// words for a call that may have written. That is the stated cost of the
// rule (regressionLocations), pinned here so it is a decision and not an
// accident. The twin is the same run with the redirect ending ok, which must
// still raise the note naming internal/gen.go, so the absence above is the
// call's ending and not a note that is never raised.

import (
	"slices"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/steer/pattern"
)

var h175Gen = map[string]any{"command": "./gen > internal/gen.go"}

// h175Run lands a green go test, the redirect ending with exit genExit, and
// a red go test, and returns the red's post.
func (e *env) h175Run(genExit int) result {
	e.t.Helper()
	if res := e.stCall("toolu_g", "Bash", stGoTest, 0); res.stdout != "" {
		e.t.Fatalf("the green run printed %q, want nothing", res.stdout)
	}
	if res := e.stCall("toolu_w", "Bash", h175Gen, genExit); res.stdout != "" {
		e.t.Fatalf("the redirect printed %q, want nothing", res.stdout)
	}
	e.backdate("toolu_g", 180_000)
	return e.stCall("toolu_r", "Bash", stGoTest, 1)
}

func TestH175_AFailedRedirectBetweenTheRunsRaisesNoNote(t *testing.T) {
	e := newSteerEnv(t)
	if res := e.h175Run(1); res.stdout != "" {
		t.Errorf("the red's post printed %q, want nothing: no completed write stands between the runs", res.stdout)
	}
	if lines := e.steerLines(); len(lines) != 0 {
		t.Errorf("%d steer lines, want 0: a call that failed and may have written is no write between the runs", len(lines))
	}
}

// TestH175_TwinTheRedirectEndedOK: the same run with the redirect ending ok
// raises the note, and it names the file the redirect wrote.
func TestH175_TwinTheRedirectEndedOK(t *testing.T) {
	e := newSteerEnv(t)
	e.h175Run(0)
	lines := e.steerLines()
	if len(lines) != 1 {
		t.Fatalf("%d steer lines, want 1: the regression note for the write that ended ok", len(lines))
	}
	l := lines[0]
	if l.str("pattern") != string(pattern.RegressionLocation) {
		t.Errorf("pattern %q, want %q", l.str("pattern"), pattern.RegressionLocation)
	}
	slots, _ := l.fields["slots"].(map[string]any)
	if paths := strs(slots["paths"]); !slices.Equal(paths, []string{"internal/gen.go"}) {
		t.Errorf("paths %v, want [internal/gen.go]", paths)
	}
}
