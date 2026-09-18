package acceptance

// H-25 — forget one destination.
//
// The operation has three halves and is worth nothing without all of them: the
// records naming the host leave rashomon's own store, its project baseline
// entry goes, and the report keeps suppressing it from the wire view.
//
// That third half exists because of a boundary this tool does not cross. The
// proxy's causal.db is the closed product's hash-chained audit store, opened
// read-only, and deleting a row would break the chain it exists to provide. So
// a forgotten host would otherwise reappear in the very next report as a
// destination reached and never named -- the forget reading as undone, or as a
// finding. The gap record carries a KEYED DIGEST of the host -- not its name,
// which has to leave the store -- and the report recomputes that digest from
// each destination it observed.

import (
	"strings"
	"testing"
)

const forgettable = "acme-secret.internal"

// seedHostSession records a session that named two hosts, one to forget.
func seedHostSession(t *testing.T, e *env) {
	t.Helper()
	e.watched(testSession)
	for i, h := range []string{forgettable, "keep-me.example"} {
		p := defaultPayload()
		p.ToolUseID = "toolu_fh_" + string(rune('a'+i))
		p.ToolInput = map[string]any{"command": "curl https://" + h + "/x"}
		e.mustHook(p.build(t))
	}
	e.probe("end", testSession)
}

func TestH25_ForgetHostRemovesTheRecordsAndLeavesAGap(t *testing.T) {
	e := newEnv(t)
	seedHostSession(t, e)

	before := len(e.declarations(testSession))
	if before != 2 {
		t.Fatalf("premise: %d declarations, want 2", before)
	}

	res := e.run("", nil, "forget", "--host", forgettable)
	if res.exitCode != 0 {
		t.Fatalf("forget --host: exit %d, stderr %q", res.exitCode, res.stderr)
	}

	// The call that named it is gone; the other call is untouched.
	after := e.declarations(testSession)
	if len(after) != 1 {
		t.Fatalf("got %d declarations after the forget, want 1", len(after))
	}

	// A gap record says that something left and identifies it well enough for
	// the report to keep suppressing it.
	gaps := e.gaps()
	if len(gaps) != 1 {
		t.Fatalf("got %d gap records, want 1", len(gaps))
	}
	g := gaps[0]
	if g.str("reason") != "forget_host" {
		t.Errorf("gap reason = %q, want forget_host", g.str("reason"))
	}
	// A KEYED DIGEST, not the hostname. The report has to recognise the host
	// again, but `forget --host` also has to make the name leave the store's
	// bytes -- writing the name here would satisfy the first by breaking the
	// second. The next test is what proves the name is gone.
	if d := g.str("host_digest"); d == "" {
		t.Error("gap carries no host_digest, so the report cannot keep the destination suppressed")
	} else if len(d) != 64 {
		t.Errorf("host_digest = %q, want 64 hex characters", d)
	}
	if strings.Contains(g.str("host_digest"), forgettable) {
		t.Error("the gap's digest field contains the plaintext host")
	}
	if g.fields["removed_records"] == float64(0) {
		t.Error("gap says 0 records removed")
	}
}

// TestH25_HostIsGoneFromTheFileBytes is the assertion that survives an
// encoding. Checking that a reader no longer returns the host is weaker: the
// record could still be on disk and merely filtered.
func TestH25_HostIsGoneFromTheFileBytes(t *testing.T) {
	e := newEnv(t)
	seedHostSession(t, e)

	if res := e.run("", nil, "forget", "--host", forgettable); res.exitCode != 0 {
		t.Fatalf("forget --host: exit %d, stderr %q", res.exitCode, res.stderr)
	}

	for rel, f := range walkStore(t, e.home) {
		if strings.Contains(string(f.body), forgettable) {
			t.Errorf("%s still contains %q after forget --host; the records must leave "+
				"the file, not merely be filtered out of a reader", rel, forgettable)
		}
	}
	// The premise, so this cannot pass by the host never having been stored:
	// the host that was NOT forgotten is still on disk.
	var sawKeeper bool
	for _, f := range walkStore(t, e.home) {
		if strings.Contains(string(f.body), "keep-me.example") {
			sawKeeper = true
		}
	}
	if !sawKeeper {
		t.Error("premise broken: the host that was not forgotten is also absent, so the " +
			"assertion above proves nothing")
	}
}

// TestH25_ForgottenHostStaysSuppressedInTheReport is the third half, and the
// one the proxy boundary forces.
func TestH25_ForgottenHostStaysSuppressedInTheReport(t *testing.T) {
	e := newEnv(t)
	seedHostSession(t, e)
	db := e.writeProxyStore(t, forgettable, "keep-me.example")

	// Premise: before the forget, the wire shows it.
	plain := e.run("", nil, "report", "--session", testSession, "--proxy-store", db).stdout
	if !strings.Contains(plain, forgettable) {
		t.Fatalf("premise: the report does not mention %q before the forget:\n%s",
			forgettable, plain)
	}

	if res := e.run("", nil, "forget", "--host", forgettable); res.exitCode != 0 {
		t.Fatalf("forget --host: exit %d, stderr %q", res.exitCode, res.stderr)
	}

	after := e.run("", nil, "report", "--session", testSession, "--proxy-store", db)
	if strings.Contains(after.stdout, forgettable) {
		t.Errorf("the forgotten host is back in the report. The proxy's row still exists "+
			"-- it is not ours to delete -- so the report must suppress it from the "+
			"view or the forget reads as undone:\n%s", after.stdout)
	}
	// And it says something was withheld, rather than silently omitting rows.
	if !strings.Contains(after.stdout, "suppressed") {
		t.Errorf("the report does not say a destination was suppressed; a view that "+
			"silently omits rows is the same failure as one that prints nothing when "+
			"it was not watching:\n%s", after.stdout)
	}
	// The host that was not forgotten is still reported.
	if !strings.Contains(after.stdout, "keep-me.example") {
		t.Errorf("the forget removed more than it was asked to:\n%s", after.stdout)
	}
}

// TestH25_ForgetHostRefusesToMixWithAWindow: --host removes by destination and
// --since/--before remove by time. Accepting both would make the scope of a
// deletion ambiguous, which is the one thing a deletion must never be.
func TestH25_ForgetHostRefusesToMixWithAWindow(t *testing.T) {
	e := newEnv(t)
	seedHostSession(t, e)

	res := e.run("", nil, "forget", "--host", forgettable, "--since", "1h")
	if res.exitCode == 0 {
		t.Error("forget --host --since was accepted; the scope of the deletion would be " +
			"ambiguous")
	}
	if len(e.declarations(testSession)) != 2 {
		t.Error("the refused command still removed records")
	}
}

// TestH25_ForgetUnknownHostIsNotAnError: asking to forget something that was
// never there is a no-op, not a failure. A user cleaning up cannot be expected
// to know which hosts a store holds.
func TestH25_ForgetUnknownHostIsNotAnError(t *testing.T) {
	e := newEnv(t)
	seedHostSession(t, e)

	res := e.run("", nil, "forget", "--host", "never-seen.example")
	if res.exitCode != 0 {
		t.Errorf("forget --host on an unknown host: exit %d, stderr %q",
			res.exitCode, res.stderr)
	}
	if len(e.declarations(testSession)) != 2 {
		t.Error("forgetting an unknown host removed records")
	}
	if len(e.gaps()) != 0 {
		t.Error("forgetting an unknown host wrote a gap record; a gap must mean records " +
			"actually left")
	}
}
