package acceptance

import "testing"

// H-103 -- the migration window's double-count must be visible, not silent.
//
// H-71 through H-77 are Part 1's own items. This one is not: it answers a
// question raised reviewing that work -- during the migration window (a real
// settings install, the plugin enabled afterward, both live at once), a
// single user-visible tool call genuinely double-fires through both origins,
// and it was measured against the real binary that the report rendered that
// session as coverage: verified, reasons: none, with every count silently
// doubled. Numbered 103 because 100 and 102 are already taken and 101
// belongs to Part 4.
//
// The detector is a record-level invariant, not a coverage-record check and
// not a plugin-aware one: a run whose declaration-record count exceeds its
// count of distinct tool_use_ids has more than one recorder writing into it,
// whatever that turns out to be -- a plugin and a settings install both live,
// a future third origin, a duplicated settings entry. The reporting layer
// does not need to know about plugins to catch it, and does not name one:
// report.go's own comment on ReasonDuplicateDeclarations says why.
//
// Measured against a real store before this reason existed (two sessions,
// 1,433 declarations): every tool_use_id had exactly one declaration,
// including in a session with denied calls -- so in practice a duplicate is
// anomalous, not a normal shape this check would misfire on. The other
// reason it does not misfire on ordinary failures: chains.go documents that
// one tool_use_id legitimately carries two EXECUTIONS (PostToolUse and
// PostToolUseFailure), so the invariant is applied to declarations only.

// TestH103_DuplicateDeclarationsAreNamedNotHidden reproduces the exact
// migration-window scenario H-74 already builds -- watch first, the plugin
// enabled after, one tool call fired through both origins -- and checks the
// report names it rather than rendering the doubled session as healthy.
//
// Break: remove the loop that adds ReasonDuplicateDeclarations in
// internal/report/report.go's build(), and this session -- the one already
// reproduced against the real binary -- renders coverage: verified,
// reasons: none, declarations recorded: 2, exactly the silent inflation this
// item exists to catch.
func TestH103_DuplicateDeclarationsAreNamedNotHidden(t *testing.T) {
	e := newEnv(t)
	e.watched(testSession) // watch, then the probe -- before the plugin exists
	fx := newPluginFixture(t, "rashomon@test")
	fx.installed(t, e)
	fx.enable(t, e, true)

	// One user-visible tool call, delivered to BOTH configured entries: the
	// same session and tool_use_id, once through each origin's own command.
	if res := e.hook(defaultPayload().build(t)); res.exitCode != 0 {
		t.Fatalf("hook via the settings entry: exit %d, stderr %q", res.exitCode, res.stderr)
	}
	if res := e.runBin(fx.bin, defaultPayload().build(t), "hook"); res.exitCode != 0 {
		t.Fatalf("hook via the plugin entry: exit %d, stderr %q", res.exitCode, res.stderr)
	}
	if res := e.probe("end", testSession); res.exitCode != 0 {
		t.Fatalf("probe end: exit %d, stderr %q", res.exitCode, res.stderr)
	}

	rep := e.report(testSession)
	if rep.Coverage.State != "unverified" {
		t.Fatalf("coverage is %s (%v), want unverified -- two declarations for one tool call must not read as a clean session",
			rep.Coverage.State, rep.Coverage.Reasons)
	}
	if !e.hasReason(rep, "duplicate_declarations") {
		t.Errorf("coverage reasons %v do not name duplicate_declarations", rep.Coverage.Reasons)
	}

	// The totals stay doubled, honestly, rather than de-duplicated: choosing
	// which of the two records was "the real one" is data this program does
	// not have, and inventing it would be worse than a count the reason
	// explains.
	if rep.Declarations.Recorded != 2 {
		t.Errorf("declarations recorded is %d, want 2 -- the reason explains the count, it does not hide it", rep.Declarations.Recorded)
	}
	if rep.Declarations.ByTool["Bash"] != 2 {
		t.Errorf("by_tool[Bash] is %d, want 2", rep.Declarations.ByTool["Bash"])
	}
}

// TestH103_FailurePairIsNotMistakenForADuplicate is the assertion that keeps
// the one above honest: an ordinary failed call, whose PostToolUseFailure
// execution record shares its tool_use_id with the declaration rather than
// with a second declaration, must not trip the same reason.
func TestH103_FailurePairIsNotMistakenForADuplicate(t *testing.T) {
	e := newEnv(t)
	e.watched(testSession)
	e.mustHook(defaultPayload().build(t))
	e.mustPost(failurePayload(t, testToolUseID, "Exit code 1", false, 30))
	if res := e.probe("end", testSession); res.exitCode != 0 {
		t.Fatalf("probe end: exit %d, stderr %q", res.exitCode, res.stderr)
	}

	rep := e.report(testSession)
	if rep.Coverage.State != "verified" {
		t.Fatalf("coverage is %s (%v), want verified -- one declaration and one failed execution is not two writers",
			rep.Coverage.State, rep.Coverage.Reasons)
	}
	if e.hasReason(rep, "duplicate_declarations") {
		t.Errorf("a single declaration with a failed execution was read as duplicate_declarations")
	}
	if rep.Declarations.Recorded != 1 {
		t.Errorf("declarations recorded is %d, want 1", rep.Declarations.Recorded)
	}
}
