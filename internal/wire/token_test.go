package wire

import (
	"testing"
	"time"
)

// The three-state per-row join.
//
// A run may mint an opaque token, hand it to the proxy as the password half of
// a Proxy-Authorization credential, and have the proxy write it into
// `causal_records.run_id`. Rows carrying it are this run's, exactly.
//
// The rule is per ROW and not per run, and that is the whole design. MEASURED,
// 2026-09-19, with a recording CONNECT proxy:
//
//	curl        CONNECT example.com:443             sends the credential
//	pip         CONNECT pypi.org:443                sends the credential
//	go net/http CONNECT example.com:443             sends the credential
//	git         CONNECT github.com:443              DOES NOT, via env or -c http.proxy
//
// Git transits the proxy and strips the userinfo, expecting proxy credentials
// from a credential helper instead. So in a tokened run, git's rows carry no
// token -- and a rule of "join on the token, fall back to the window only when
// no token was set" would exclude every one of them, because a token WAS set.
// The report would show a session that cloned three repositories as having
// reached nothing, and say so with full confidence.
//
// THE INVARIANT THE NEXT PERSON WILL BREAK: a tokened run is never worse
// informed than an untokened one.

const tok = "tok_thisrun"

// issuedByUs stands in for the MAC check. Only these two tags verify.
func issuedByUs(t string) bool { return t == tok || t == "tok_someoneelse" }

// TestToken_MatchedRowsAreThisRunsEvenOutsideTheWindow. The token is stronger
// evidence than the clock: it was issued to this process and no other. A row
// carrying it outside the window is this run's row with a bad timestamp, not
// somebody else's.
func TestToken_MatchedRowsAreThisRunsEvenOutsideTheWindow(t *testing.T) {
	rows := []row{
		{seq: 1, requestID: "a", runID: tok, host: "pypi.org", action: "ALLOW",
			when: base.Add(-time.Hour), whenOK: true},
	}

	obs := summarise(rows, Window{RunID: tok, Start: base}, "/tmp/causal.db")

	if len(obs.Hosts) != 1 || obs.Hosts[0].Attempts != 1 {
		t.Fatalf("hosts = %+v, want one attempt attributed to this run", obs.Hosts)
	}
	if obs.Hosts[0].InheritedAttempts != 0 {
		t.Errorf("inherited = %d, want 0. The token was issued to this process and no "+
			"other; a clock disagreeing does not make the row somebody else's.",
			obs.Hosts[0].InheritedAttempts)
	}
	if obs.TokenMatched != 1 {
		t.Errorf("token_matched = %d, want 1", obs.TokenMatched)
	}
}

// TestToken_UntokenedRowsInTheWindowAreStillAttributed is git, and it is the
// reason the rule is not "token or nothing".
func TestToken_UntokenedRowsInTheWindowAreStillAttributed(t *testing.T) {
	rows := []row{
		{seq: 1, requestID: "a", runID: tok, host: "pypi.org", action: "ALLOW",
			when: base.Add(time.Minute), whenOK: true},
		// git: transits the proxy, carries no credential.
		{seq: 2, requestID: "b", runID: "", host: "github.com", action: "ALLOW",
			when: base.Add(2 * time.Minute), whenOK: true},
	}

	obs := summarise(rows, Window{RunID: tok, Start: base}, "/tmp/causal.db")

	if obs.Attempts != 2 {
		t.Fatalf("attempts = %d, want 2. Excluding the untokened row would report a "+
			"session that reached github.com as having reached nothing -- and git is the "+
			"client that cannot carry the token.", obs.Attempts)
	}
	if obs.TokenMatched != 1 || obs.WindowMatched != 1 {
		t.Errorf("token_matched/window_matched = %d/%d, want 1/1: the report has to say "+
			"which rows it knows exactly and which it inferred from the clock",
			obs.TokenMatched, obs.WindowMatched)
	}
}

// TestToken_OtherTokensAreExcludedConfidently. This is what the token BUYS.
// Without one, a concurrent session's rows are indistinguishable from ours and
// the window has to guess. With one, they are positively someone else's.
func TestToken_OtherTokensAreExcludedConfidently(t *testing.T) {
	rows := []row{
		{seq: 1, requestID: "a", runID: tok, host: "pypi.org", action: "ALLOW",
			when: base.Add(time.Minute), whenOK: true},
		{seq: 2, requestID: "b", runID: "tok_someoneelse", host: "evil.example",
			action: "ALLOW", when: base.Add(time.Minute), whenOK: true},
	}

	// The verifier is what makes this exclusion legitimate: the tag is one THIS
	// INSTALL issued, so it is provably another run of ours.
	obs := summarise(rows, Window{RunID: tok, Start: base, IsOurs: issuedByUs}, "/tmp/causal.db")

	if obs.Attempts != 1 {
		t.Errorf("attempts = %d, want 1: the other run's row is inside the window and is "+
			"provably not ours", obs.Attempts)
	}
	if obs.OtherToken != 1 {
		t.Errorf("other_token = %d, want 1. The overlap is still reported -- an operator "+
			"must be able to see that another session was active without its rows "+
			"entering this one's view.", obs.OtherToken)
	}
	for _, h := range obs.Hosts {
		if h.Host == "evil.example" && h.Attempts > 0 {
			t.Error("another run's host was counted as this run's attempt")
		}
	}
}

// TestToken_NoTokenIsTodaysBehaviour. An untokened run must be unchanged: the
// window decides, and nothing claims a precision it does not have.
func TestToken_NoTokenIsTodaysBehaviour(t *testing.T) {
	rows := []row{
		{seq: 1, requestID: "a", runID: "", host: "pypi.org", action: "ALLOW",
			when: base.Add(time.Minute), whenOK: true},
		{seq: 2, requestID: "b", runID: "tok_other", host: "other.example",
			action: "ALLOW", when: base.Add(time.Minute), whenOK: true},
	}

	obs := summarise(rows, Window{Start: base}, "/tmp/causal.db")

	if obs.TokenMatched != 0 {
		t.Errorf("token_matched = %d with no token set", obs.TokenMatched)
	}
	// With no token of our own there is nothing to compare against, so another
	// run's row is just a row in the window -- which is exactly the ambiguity
	// the token exists to retire, and must not be silently pre-retired here.
	if obs.Attempts != 2 {
		t.Errorf("attempts = %d, want 2: without a token this run cannot tell the rows "+
			"apart, and pretending otherwise would claim precision it does not have",
			obs.Attempts)
	}
}

// TestToken_ATokenedRunIsNeverWorseInformed is the invariant, and the fixture
// now INCLUDES the row shape that can falsify it.
//
// The first version of this test contained only token-matched and untokened
// rows -- the one combination where the property cannot fail -- while a test
// four cases above demonstrated the violation and called it the feature. A
// review caught that, and it was right: the invariant as written was false,
// because an unverifiable foreign tag excluded a row that an untokened run
// would have counted.
//
// It is true now, and true by construction rather than by fixture: only a tag
// this install can prove it issued is excluded, and everything else falls back
// to the clock.
func TestToken_ATokenedRunIsNeverWorseInformed(t *testing.T) {
	rows := []row{
		{seq: 1, requestID: "a", runID: tok, host: "pypi.org", action: "ALLOW",
			when: base.Add(time.Minute), whenOK: true},
		{seq: 2, requestID: "b", runID: "", host: "github.com", action: "ALLOW",
			when: base.Add(2 * time.Minute), whenOK: true},
		// THE ROW THAT FALSIFIED THE OLD VERSION: a foreign tag this install
		// never issued. An agent that read HTTPS_PROXY and swapped the password
		// produces exactly this, and under the old rule it vanished.
		{seq: 3, requestID: "c", runID: "rt_forged_by_the_agent",
			host: "exfil.example", action: "ALLOW",
			when: base.Add(3 * time.Minute), whenOK: true},
	}

	withToken := summarise(rows, Window{RunID: tok, Start: base, IsOurs: issuedByUs}, "/tmp/causal.db")
	without := summarise(rows, Window{Start: base}, "/tmp/causal.db")

	if withToken.Attempts < without.Attempts {
		t.Errorf("a tokened run saw %d attempts and an untokened one saw %d over the same "+
			"rows. Opting in must never cost information; that is the invariant this "+
			"whole design rests on.", withToken.Attempts, without.Attempts)
	}
	if withToken.DistinctHosts < without.DistinctHosts {
		t.Errorf("tokened run saw %d hosts, untokened %d",
			withToken.DistinctHosts, without.DistinctHosts)
	}
}

// TestToken_AnUnverifiableTagCannotDeleteAHost is the security half, stated on
// its own because it is the finding rather than a corollary.
//
// The observed agent runs arbitrary shell. It reads HTTPS_PROXY out of its own
// environment, substitutes any password, and routes its traffic through the
// same proxy. If an unequal tag were enough to mean "another session", that row
// would leave Attempts, leave DistinctHosts, and -- the part that matters --
// become ineligible for the reached-but-never-named finding and for novelty.
// The feature would have handed the subject a one-line opt-out from the
// product's central claim, and printed a confident false attribution to a third
// party while doing it.
func TestToken_AnUnverifiableTagCannotDeleteAHost(t *testing.T) {
	rows := []row{
		{seq: 1, requestID: "a", runID: "rt_forged", host: "exfil.example",
			action: "ALLOW", when: base.Add(time.Minute), whenOK: true},
	}

	obs := summarise(rows, Window{RunID: tok, Start: base, IsOurs: issuedByUs}, "/tmp/causal.db")

	if obs.Attempts != 1 {
		t.Errorf("attempts = %d, want 1. A tag this install never issued is not evidence "+
			"about anybody; the clock decides, exactly as it did before tags existed.",
			obs.Attempts)
	}
	if obs.OtherToken != 0 {
		t.Errorf("other_token = %d; an unverifiable tag was counted as another session",
			obs.OtherToken)
	}
	if len(obs.Hosts) != 1 || obs.Hosts[0].Inherited {
		t.Errorf("hosts = %+v; the forged row was excluded, which removes exfil.example "+
			"from the finding the product exists to make", obs.Hosts)
	}
}

// TestToken_AForeignDialFailureDoesNotMarkOurHostUnreached.
//
// The outcome maps are built before classification and hostOutcome is keyed by
// HOST across the proxy's whole database, so another run's refused dial to a
// host this session reached successfully inverted our Unreached verdict. The
// row had already been classified as somebody else's by the time the attempt
// counters ran; the outcome path simply never asked.
func TestToken_AForeignDialFailureDoesNotMarkOurHostUnreached(t *testing.T) {
	rows := []row{
		{seq: 1, requestID: "a", runID: tok, host: "pypi.org", action: "ALLOW",
			when: base.Add(time.Minute), whenOK: true},
		{seq: 2, requestID: "", runID: "tok_someoneelse", host: "pypi.org",
			reason: dialFailedPrefix + "refused", when: base.Add(time.Minute), whenOK: true},
	}

	obs := summarise(rows, Window{RunID: tok, Start: base, IsOurs: issuedByUs}, "/tmp/causal.db")

	d := obs.Hosts[0]
	if d.Unreached {
		t.Error("another run's refused dial marked this session's successfully reached host " +
			"as never reached -- the one distinction Unreached exists to make, inverted")
	}
	if d.InWindowReached != 1 || d.InWindowFailed != 0 {
		t.Errorf("reached/failed = %d/%d, want 1/0", d.InWindowReached, d.InWindowFailed)
	}
}
