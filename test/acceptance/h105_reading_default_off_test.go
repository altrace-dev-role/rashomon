package acceptance

import (
	"strings"
	"testing"
)

// H-105 -- the default install calls no model.
//
// Added beyond the spec's own H-95..H-99 range because Part 5 is gated on an
// amendment to two of README.md's published constraints ("No model in any
// path", "No account, no telemetry, no phone-home") that has not been
// signed off. Whatever else Part 5 does, a default install -- watch, a
// session, `report` -- must not be able to call a model, and this has to be
// checkable, not merely claimed.
//
// A model call, if any ever happens, happens entirely inside Claude Code's
// own runtime once it reads a "type": "prompt" hook out of the settings
// file -- see internal/install/reading.go's package doc. rashomon's own
// process never makes one and never could (H-99). So the only thing THIS
// binary's own behaviour can prove is the precondition: no such entry
// exists in the file Claude Code reads, unless the one command built for
// exactly that was run. That is what every assertion below checks.
func TestH105_APlainWatchInstallsNoPromptHook(t *testing.T) {
	e := newEnv(t)
	if res := e.watch(); res.exitCode != 0 {
		t.Fatalf("watch: exit %d, stderr %q", res.exitCode, res.stderr)
	}

	assertNoReadingEntry(t, e, "a plain watch")
}

// TestH105_AFullSessionOnADefaultInstallCallsNoModel runs every hook this
// program owns -- probe start, a tool call, its execution, and the
// exception-only recap -- on a machine that only ever ran `watch`, and
// checks the one place a model call could originate: the settings file
// Claude Code reads before any of those hooks fire. It never changes.
func TestH105_AFullSessionOnADefaultInstallCallsNoModel(t *testing.T) {
	e := newEnv(t)
	e.watched(testSession)

	before := e.settingsBytes()

	p := defaultPayload()
	e.mustHook(p.build(t))
	e.mustPost(defaultPost().build(t))
	// Whether Part 4's own line fires here is not this test's question --
	// H-90's own tests cover that. What matters is what follows: the
	// settings file, and so the set of hooks Claude Code could ever run,
	// must not have moved.
	e.recapLine(stopPayload(testSession, "Ran git status as requested.", false))

	after := e.settingsBytes()
	if string(before) != string(after) {
		t.Errorf("a full session changed settings.json:\nbefore: %s\nafter:  %s", before, after)
	}
	assertNoReadingEntry(t, e, "a full session on a default install")
}

// TestH105_ReportAndStatusInstallNoPromptHook covers the two other commands
// a user runs without ever touching enable-reading.
func TestH105_ReportAndStatusInstallNoPromptHook(t *testing.T) {
	e := newEnv(t)
	e.watched(testSession)
	e.mustHook(defaultPayload().build(t))
	e.mustPost(defaultPost().build(t))

	if res := e.run("", nil, "report", "--json"); res.exitCode != 0 {
		t.Fatalf("report: exit %d, stderr %q", res.exitCode, res.stderr)
	}
	if res := e.status(); res.exitCode != 0 {
		t.Fatalf("status: exit %d, stderr %q", res.exitCode, res.stderr)
	}
	assertNoReadingEntry(t, e, "report and status")

	// status itself must say so in words a user can read without knowing
	// what to look for in a JSON file -- the visible half of H-105.
	res := e.status()
	if !strings.Contains(res.stdout, "reading: disabled") {
		t.Errorf("status does not report reading as disabled by default:\n%s", res.stdout)
	}
}

// TestH105_EnableReadingIsTheOnlyWayInAndDisableUndoesIt is the control: the
// entry the tests above prove absent is reachable at all, through the one
// door built for it, and leaves through the same door.
func TestH105_EnableReadingIsTheOnlyWayInAndDisableUndoesIt(t *testing.T) {
	e := newEnv(t)
	e.watched(testSession)
	assertNoReadingEntry(t, e, "before enable-reading")

	if res := e.enableReading(); res.exitCode != 0 {
		t.Fatalf("enable-reading: exit %d, stderr %q", res.exitCode, res.stderr)
	}
	if !hasReadingEntry(t, e) {
		t.Fatal("enable-reading did not add the entry")
	}
	if !strings.Contains(e.status().stdout, "reading: enabled") {
		t.Error("status does not report reading as enabled after enable-reading")
	}

	if res := e.disableReading(); res.exitCode != 0 {
		t.Fatalf("disable-reading: exit %d, stderr %q", res.exitCode, res.stderr)
	}
	assertNoReadingEntry(t, e, "after disable-reading")
}

// hasReadingEntry reports whether settings.json carries a "type": "prompt"
// hook under Stop or StopFailure whose prompt text carries our marker --
// the same predicate internal/install.isReadingEntry uses, read back
// through the file a real Claude Code session would read, rather than
// through this program's own claim about what it wrote.
func hasReadingEntry(t *testing.T, e *env) bool {
	t.Helper()
	sf := e.settings()
	for _, event := range []string{"Stop", "StopFailure"} {
		for _, g := range sf.Hooks[event] {
			for _, h := range g.Hooks {
				if h.Type == "prompt" && strings.Contains(string(g.raw), "rashomon:model-reading:v1") {
					return true
				}
			}
		}
	}
	return false
}

func assertNoReadingEntry(t *testing.T, e *env, when string) {
	t.Helper()
	sf := e.settings()
	for _, event := range []string{"Stop", "StopFailure"} {
		for _, g := range sf.Hooks[event] {
			for _, h := range g.Hooks {
				if h.Type == "prompt" {
					t.Errorf("after %s, settings.json carries a \"type\": \"prompt\" hook under %s: %s",
						when, event, g.raw)
				}
			}
			if strings.Contains(string(g.raw), "rashomon:model-reading:v1") {
				t.Errorf("after %s, settings.json carries the model-reading marker under %s: %s",
					when, event, g.raw)
			}
		}
	}
}
