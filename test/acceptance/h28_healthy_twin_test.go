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
	"path/filepath"
	"strings"
	"testing"
)

// degradedMarkers are the exact substrings a healthy report must not contain.
// Each one is a line this report emits when it cannot answer a question.
var degradedMarkers = []string{
	"not observed (",               // destinations: the proxy store was unreadable
	"proxy on path: unknown",       // no rows in the window, or no store
	"window: NOT applied",          // the proxy's timestamps did not parse
	"suppressed:",                  // a host-scoped forget removed rows from the view
	"the agent's account: unknown", // the transcript could not be read
	// The novelty degradation, named by the literal a reader sees. "new for this
	// project: unknown" is composed from a format string in one file and a reason
	// in another, so no source check could confirm it and no fixture here can
	// produce it: the probe defaults cwd to the process's working directory, so
	// the current binary never records an empty one. It arrives only on records
	// written before that field existed.
	"the run recorded no working directory",
	"tool families: unknown", // the store could not be read
	"coverage: unverified",   // coverage degradation
	"outcome unobserved",     // a call whose ending was never recorded
	"could not be read",      // any reason string built from a read failure
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

// TestH28_EveryDegradedLineIsReachable guards this file against itself, and the
// guard's default is REACHABILITY MUST BE SHOWN.
//
// A marker list that had drifted from the renderer would make the test above
// pass by asserting the absence of strings the report can no longer produce.
// The first version of this test checked four hand-picked markers and left six
// unproven, and one of those six -- "declarations unverified" -- was a string
// no render path could emit. The absence assertion for it had been passing
// vacuously since it was written. So the list is now checked in full: every
// marker must appear in at least one render this test produces, and a marker
// that cannot be produced fails here rather than quietly weakening the test
// above.
func TestH28_EveryDegradedLineIsReachable(t *testing.T) {
	var renders []string

	// R1: no proxy store, an unreadable transcript, and a declaration whose
	// ending was never recorded. The shape that produces most of them at once.
	{
		e := newEnv(t)
		e.watched(testSession)
		p := defaultPayload()
		p.TranscriptPath = "/nonexistent/transcript.jsonl"
		e.mustHook(p.build(t))
		e.probe("end", testSession)
		renders = append(renders, e.run("", nil, "report", "--session", testSession,
			"--proxy-store", "/nonexistent/causal.db").stdout)
	}

	// R2: a failed call whose transcript cannot be read, which is the only way
	// the comparison reports that it did not happen.
	{
		e := newEnv(t)
		e.watched(testSession)
		p := defaultPayload()
		p.TranscriptPath = "/nonexistent/transcript.jsonl"
		e.mustHook(p.build(t))
		e.mustPost(failurePayload(t, testToolUseID, "Exit code 1", false, 30))
		e.probe("end", testSession)
		renders = append(renders, e.run("", nil, "report", "--session", testSession).stdout)
	}

	// R3: a host-scoped forget, which removes rows from the view and says so.
	{
		e := newEnv(t)
		e.watched(testSession)
		p := defaultPayload()
		p.ToolInput = map[string]any{"command": "curl https://gone.example"}
		e.mustHook(p.build(t))
		db := e.writeProxyStore(t, "gone.example")
		e.probe("end", testSession)
		if r := e.run("", nil, "forget", "--host", "gone.example"); r.exitCode != 0 {
			t.Fatalf("forget: exit %d, stderr %q", r.exitCode, r.stderr)
		}
		renders = append(renders, e.run("", nil, "report", "--session", testSession,
			"--proxy-store", db).stdout)
	}

	// R4: a proxy store whose timestamps do not parse, so the window cannot be
	// applied and every row is shown rather than silently dropped.
	{
		e := newEnv(t)
		e.watched(testSession)
		e.mustHook(defaultPayload().build(t))
		db := e.writeProxyStoreBadStamps(t, "unparseable.example")
		e.probe("end", testSession)
		renders = append(renders, e.run("", nil, "report", "--session", testSession,
			"--proxy-store", db).stdout)
	}

	all := strings.Join(renders, "\n")

	// A marker that no fixture here produces must at least exist as a literal in
	// the report package. The two that qualify are reachable only from state this
	// harness cannot create through the binary -- a record written before a field
	// existed -- and an allowlist naming them would be a judgement call that
	// decays. A source check is mechanical and admits no such call: the dead
	// marker that prompted this, "declarations unverified", fails BOTH halves.
	//
	// It reads the whole package rather than the renderer alone, because a
	// degradation line is a format string in text.go and a reason built beside
	// the code that discovered it. The known weakness is that a literal in a
	// COMMENT would satisfy the check; the render union above is the primary
	// defence, and it covers eight of the ten.
	source, err := packageSource(filepath.Join("..", "..", "internal", "report"))
	if err != nil {
		t.Fatalf("read the report package to check marker reachability: %v", err)
	}
	renderer := []byte(source)

	for _, marker := range degradedMarkers {
		if strings.Contains(all, marker) {
			continue
		}
		if strings.Contains(string(renderer), marker) {
			continue
		}
		t.Errorf("%q is produced by no render here AND appears in no literal in the "+
			"renderer, so the absence assertion above passes vacuously. Either correct "+
			"the marker or add a fixture that reaches it.", marker)
	}
}
