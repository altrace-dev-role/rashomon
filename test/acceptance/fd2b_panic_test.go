package acceptance

import (
	"strings"
	"testing"
)

// TestFD2b_PanicExitsZeroAndCounts (PRD PF-FD2; card P5B, 5.4, D-B3): a
// panic in a call-path entry exits 0 with no traceback and nothing on stdout,
// on the release binary's own exit code. In the handler, PreToolUse's or
// PostToolUse's, capture's Guard takes it and the fault count moves: the
// phase's recovered_panic coverage records go from none to one. Before
// PreToolUse's flush, outside every inner Guard, guardedHook's barrier takes
// it and says so in one stderr line; the count is capture's, so it does not
// move there.
func TestFD2b_PanicExitsZeroAndCounts(t *testing.T) {
	for _, c := range []struct {
		name, phase string
		counts      bool
		run         func(e *env) result
	}{
		{"PreToolUse handler", "call", true, func(e *env) result {
			return e.hook(defaultPayload().build(e.t), "RASHOMON_FAULT="+pointHookParsed+":plain_panic")
		}},
		{"PostToolUse handler", "post", true, func(e *env) result {
			e.mustHook(defaultPayload().build(e.t))
			return e.post(defaultPost().build(e.t), "RASHOMON_FAULT="+pointPostParsed+":plain_panic")
		}},
		{"PreToolUse before the flush", "call", false, func(e *env) result {
			return e.hook(defaultPayload().build(e.t), "RASHOMON_FAULT="+pointHookBeforeFlush+":plain_panic")
		}},
	} {
		t.Run(c.name, func(t *testing.T) {
			e := newEnv(t)
			e.watched(testSession)
			if n := recoveredPanics(e, c.phase); n != 0 {
				t.Fatalf("premise: %d recovered_panic %s records before the call", n, c.phase)
			}
			res := c.run(e)
			if res.exitCode != 0 {
				t.Fatalf("exit code %d, want 0 (2 would block the tool call)", res.exitCode)
			}
			assertNoTraceback(t, res)
			if !c.counts {
				if !strings.Contains(res.stderr, "rashomon: recovered panic of type ") {
					t.Errorf("stderr %q, want the barrier's one line", res.stderr)
				}
				return
			}
			if n := recoveredPanics(e, c.phase); n != 1 {
				t.Errorf("%d recovered_panic %s records after the panic, want 1", n, c.phase)
			}
		})
	}
}

// recoveredPanics counts the session's coverage records of phase whose
// reason is recovered_panic, the fault count a recovered panic moves.
func recoveredPanics(e *env, phase string) int {
	n := 0
	for _, r := range e.coverage(testSession, phase) {
		if r.str("reason") == "recovered_panic" {
			n++
		}
	}
	return n
}
