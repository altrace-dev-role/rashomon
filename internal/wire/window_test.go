package wire

import (
	"testing"
	"time"
)

// H-39 -- the window bounds reached-versus-attempted, and it counts FOLDED
// REQUESTS rather than rows.
//
// The chain view needs to say, per declared host, whether this session
// actually reached it or only tried. `Destination` cannot express that today:
// `Unreached` is cleared by ANY row that is not a dial failure, including a row
// from outside the window, so a host reached yesterday and refused today reads
// as reached. That is the right answer for the destinations section, which is
// about the host, and the wrong one for a link, which is about this session.
//
// So `summarise` gains two per-host counters, incremented on the non-inherited
// branch after the request_id fold. Folded, because one attempt writes up to
// two rows -- the chain's verdict and, when the destination could not be
// reached, a dial outcome -- and counting rows would report one refused attempt
// as two.

// TestInWindow_SuccessBeforeTheWindowDoesNotCountAsReached is H-39 exactly: a
// request that succeeded BEFORE the window, and one refused dial inside it.
func TestInWindow_SuccessBeforeTheWindowDoesNotCountAsReached(t *testing.T) {
	windowStart := base
	rows := []row{
		// Before the window: another session's success. Inherited.
		{seq: 1, requestID: "old", host: "pypi.org", action: "ALLOW",
			when: base.Add(-time.Hour), whenOK: true},
		// Inside the window: one attempt, two rows, one request_id.
		{seq: 2, requestID: "now", host: "pypi.org", action: "ALLOW",
			when: base.Add(time.Minute), whenOK: true},
		{seq: 3, requestID: "now", host: "pypi.org",
			reason: dialFailedPrefix + "timeout", when: base.Add(time.Minute), whenOK: true},
	}

	obs := summarise(rows, Window{Start: windowStart}, "/tmp/causal.db")

	if len(obs.Hosts) != 1 {
		t.Fatalf("hosts = %v, want one", obs.Hosts)
	}
	d := obs.Hosts[0]
	if d.InWindowReached != 0 {
		t.Errorf("in_window_reached = %d, want 0. The only success is from before the "+
			"window; counting it would let another session's traffic answer this "+
			"session's question.", d.InWindowReached)
	}
	if d.InWindowFailed != 1 {
		t.Errorf("in_window_failed = %d, want 1: ONE attempt inside the window, which is "+
			"two rows folded on one request_id. Counting rows reports a single refused "+
			"attempt as two.", d.InWindowFailed)
	}
}

// TestInWindow_ReachedCountsTheRequestThatConnected is the positive twin.
func TestInWindow_ReachedCountsTheRequestThatConnected(t *testing.T) {
	rows := []row{
		{seq: 1, requestID: "a", host: "pypi.org", action: "ALLOW",
			when: base.Add(time.Minute), whenOK: true},
		{seq: 2, requestID: "b", host: "pypi.org", action: "ALLOW",
			when: base.Add(2 * time.Minute), whenOK: true},
		{seq: 3, requestID: "b", host: "pypi.org",
			reason: dialFailedPrefix + "dns", when: base.Add(2 * time.Minute), whenOK: true},
	}

	obs := summarise(rows, Window{Start: base}, "/tmp/causal.db")
	d := obs.Hosts[0]

	if d.InWindowReached != 1 {
		t.Errorf("in_window_reached = %d, want 1: request a connected", d.InWindowReached)
	}
	if d.InWindowFailed != 1 {
		t.Errorf("in_window_failed = %d, want 1: request b was refused", d.InWindowFailed)
	}
}

// TestInWindow_InheritedRowsCountForNeither. The counters exist to answer a
// question about THIS session, so a row outside the window contributes to
// neither -- the same rule the attempt counts already follow.
func TestInWindow_InheritedRowsCountForNeither(t *testing.T) {
	rows := []row{
		{seq: 1, requestID: "old-ok", host: "pypi.org", action: "ALLOW",
			when: base.Add(-time.Hour), whenOK: true},
		{seq: 2, requestID: "old-fail", host: "pypi.org",
			reason: dialFailedPrefix + "timeout", when: base.Add(-time.Hour), whenOK: true},
	}

	obs := summarise(rows, Window{Start: base}, "/tmp/causal.db")
	d := obs.Hosts[0]

	if d.InWindowReached != 0 || d.InWindowFailed != 0 {
		t.Errorf("reached/failed = %d/%d for inherited rows only; both must be 0",
			d.InWindowReached, d.InWindowFailed)
	}
	if d.InheritedAttempts == 0 {
		t.Error("premise: these rows should still be counted as inherited attempts")
	}
}

// TestInWindow_UnreachedKeepsItsOwnMeaning. The destinations section asks about
// the HOST and the link asks about the SESSION, so the new counters must not
// change what Unreached says.
func TestInWindow_UnreachedKeepsItsOwnMeaning(t *testing.T) {
	rows := []row{
		{seq: 1, requestID: "old", host: "pypi.org", action: "ALLOW",
			when: base.Add(-time.Hour), whenOK: true},
		{seq: 2, requestID: "now", host: "pypi.org",
			reason: dialFailedPrefix + "timeout", when: base.Add(time.Minute), whenOK: true},
	}

	obs := summarise(rows, Window{Start: base}, "/tmp/causal.db")
	d := obs.Hosts[0]

	if d.Unreached {
		t.Error("unreached = true, but a request DID reach this host -- outside the window. " +
			"That is the destinations section's question and its answer must not change.")
	}
	if d.InWindowReached != 0 {
		t.Errorf("in_window_reached = %d; the link's question is about this session and "+
			"the answer there is 0", d.InWindowReached)
	}
}
