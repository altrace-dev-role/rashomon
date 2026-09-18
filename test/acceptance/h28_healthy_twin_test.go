package acceptance

// H-28 — the healthy twin, in aggregate.
//
// Every degraded line in this report has a positive counterpart, and most are
// tested in pairs where they are built. This test asserts the property those
// pairs cannot: that on a FULLY HEALTHY run, not one degradation line appears.
//
// It matters because the individual pairs are checked against hand-built
// values, while a real render composes a dozen sections that each decide
// independently whether they are degraded. A section that defaulted to its
// degraded form -- a nil map read as "unavailable", a zero count read as
// "unknown" -- would pass its own pair and still make every real report look
// broken. That failure is worse than a missing line: a reader who sees
// "unknown" on a healthy session learns to ignore the word, and then misses it
// when it is true.

import (
	"strings"
	"testing"
)

// degradedMarkers are the exact substrings a healthy report must not contain.
// Each one is a line this report emits when it cannot answer a question.
var degradedMarkers = []string{
	"not observed (",                // destinations: the proxy store was unreadable
	"proxy on path: unknown",        // no rows in the window, or no store
	"window: NOT applied",           // the proxy's timestamps did not parse
	"suppressed:",                   // a host-scoped forget removed rows from the view
	"the agent's account: unknown",  // the transcript could not be read
	"new for this project: unknown", // the baseline could not be read
	"tool families: unknown",        // the store could not be read
	"declarations unverified",       // coverage degradation
	"outcome unobserved",            // a call whose ending was never recorded
	"could not be read",             // any reason string built from a read failure
}

// TestH28_AHealthyRunShowsNoDegradationLine builds the healthiest session this
// harness can produce and asserts every marker is absent.
func TestH28_AHealthyRunShowsNoDegradationLine(t *testing.T) {
	e := newEnv(t)
	e.watched(testSession)

	// A transcript that MATCHES the recorded call: the accounting equation is
	// checked per transcript, so one naming none of the recorded ids renders as
	// a mismatch in both directions and could not be a healthy fixture.
	p := defaultPayload()
	transcript := e.writeFullTranscript(t, p.ToolUseID,
		"Fetched the package index and wrote the notes file.")
	p.TranscriptPath = transcript
	p.ToolInput = map[string]any{"command": "curl https://pypi.org/simple/"}
	e.mustHook(p.build(t))

	post := defaultPost()
	post.ToolInput = p.ToolInput
	post.ToolName = p.ToolName
	post.ToolUseID = p.ToolUseID
	e.mustPost(post.build(t))

	// The wire saw exactly the host that was declared, and the row is written
	// BEFORE the session ends so it falls inside the coverage window. A row
	// written after probe end is correctly inherited, which would make this
	// fixture a degraded session rather than a healthy one.
	db := e.writeProxyStore(t, "pypi.org")

	e.probe("end", testSession)

	// Two renders: the text form a human reads and the JSON a consumer parses.
	// A renderer is exactly where a degraded default shows up in one form only.
	text := e.run("", nil, "report", "--session", testSession, "--proxy-store", db)
	if text.exitCode != 0 {
		t.Fatalf("report: exit %d, stderr %q", text.exitCode, text.stderr)
	}

	// Premise: this really is the healthy path. If coverage is unverified the
	// assertions below would be checking the wrong session.
	rep := e.report(testSession)
	if rep.Coverage.State != "verified" {
		t.Fatalf("premise broken: coverage is %s (%v), so this is not a healthy run",
			rep.Coverage.State, rep.Coverage.Reasons)
	}

	for _, marker := range degradedMarkers {
		if strings.Contains(text.stdout, marker) {
			t.Errorf("a healthy run rendered the degradation marker %q. A reader who sees "+
				"it on a clean session learns to ignore the word, and then misses it "+
				"when it is true.\n%s", marker, text.stdout)
		}
	}
}

// TestH28_AHealthyRunStillShowsWhatCannotBeObserved is the other half, and the
// two together are the whole rule.
//
// "No degradation lines" must not become "no caveats". The not-observable list
// is a property of the instrument rather than of the session, so it belongs on
// a healthy report too -- a reader told only what was observed reads the rest
// as an absence of traffic rather than an absence of observation.
func TestH28_AHealthyRunStillShowsWhatCannotBeObserved(t *testing.T) {
	e := newEnv(t)
	e.watched(testSession)
	p := defaultPayload()
	p.ToolInput = map[string]any{"command": "curl https://pypi.org/simple/"}
	e.mustHook(p.build(t))
	db := e.writeProxyStore(t, "pypi.org")
	e.probe("end", testSession)

	out := e.run("", nil, "report", "--session", testSession, "--proxy-store", db).stdout

	for _, want := range []string{
		"not observable",
		"fetch",
		"ssh",
		// The comparison lines print their zero rather than vanishing, so a
		// reader can tell a clean session from an unchecked one.
		"executed differently from declared: 0",
		"reached but never named:",
		"declared but not observed:",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("a healthy report omits %q; no degradation lines must not mean no "+
				"caveats:\n%s", want, out)
		}
	}
}

// TestH28_EveryDegradedLineIsReachable guards this file against itself.
//
// A marker list that had drifted from the renderer would make the test above
// pass by asserting the absence of strings the report can no longer produce.
// So each marker is shown to appear in SOME render: the degraded session below
// has no proxy store, no transcript and no baseline, which is the shape that
// produces most of them at once.
func TestH28_EveryDegradedLineIsReachable(t *testing.T) {
	e := newEnv(t)
	e.watched(testSession)
	p := defaultPayload()
	p.TranscriptPath = "/nonexistent/transcript.jsonl"
	e.mustHook(p.build(t))
	e.probe("end", testSession)

	// No --proxy-store, so the wire side is unavailable.
	out := e.run("", nil, "report", "--session", testSession,
		"--proxy-store", "/nonexistent/causal.db").stdout

	// The subset this shape is expected to produce. Not all of them: some need
	// a forget, or an unparseable timestamp, and those are covered where they
	// are built.
	for _, want := range []string{
		"not observed (",
		"proxy on path: unknown",
		"the agent's account: unknown",
		"tool families: unknown",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("the degraded session did not produce %q, so the healthy-twin test "+
				"may be asserting the absence of a string the report can no longer "+
				"emit:\n%s", want, out)
		}
	}
}
