package acceptance

// The line items (H-157 to H-174) replay a run of the line corpus through
// the built binary and hold everything the run printed to a block the item
// writes out in full: the digest-level detectors that fired, the session
// start, every call event that printed, the Stop's whole output and the next
// prompt's, to the person (systemMessage) and to the model
// (additionalContext). A block is the snapshot's own rendering (renderTurn),
// so an item and the corpus sweep read one output one way; the item keeps it
// in its own text, so a change to what the run prints fails the item that
// owns the behaviour by name, not only the snapshot.

import (
	"strings"
	"testing"
)

// holdLine replays f under name and fails t for each turn whose block is not
// want's, in order. It returns the machine the run was replayed in.
func holdLine(t *testing.T, name string, f fixtureRun, want ...string) *env {
	t.Helper()
	turns, norm, e := replayRunEnv(t, f)
	got := renderRun(t, name, turns, norm)
	if len(got) != len(want) {
		t.Fatalf("%s: %d blocks, want %d:\n%s", name, len(got), len(want), strings.Join(got, "\n"))
	}
	for i := range got {
		if got[i] != want[i] {
			t.Errorf("%s turn %d:\n--- got\n%s--- want\n%s", name, i+1, got[i], want[i])
		}
	}
	return e
}

// lineRun is the run of the line fixture name, which an item may change
// before it replays it: a twin with one call's ending changed.
func lineRun(t *testing.T, name string) fixtureRun {
	t.Helper()
	return readLineFixture(t, name).fixtureRun
}

// lastStopLine is the line `rashomon report` prints for the session's last
// Stop, where the state line it computed is read back by a person even when
// a detector's sentence took the Stop's place.
func (e *env) lastStopLine() string {
	e.t.Helper()
	res := e.run("", nil, "report", "--session", testSession)
	if res.exitCode != 0 {
		e.t.Fatalf("report: exit %d, stderr %q", res.exitCode, res.stderr)
	}
	for _, l := range strings.Split(res.stdout, "\n") {
		if s := strings.TrimSpace(l); strings.HasPrefix(s, "last Stop:") {
			return s
		}
	}
	e.t.Fatalf("report names no last Stop:\n%s", res.stdout)
	return ""
}
