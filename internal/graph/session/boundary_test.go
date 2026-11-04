package session

import (
	"strings"
	"testing"
)

// The lines both forms of the brief share: project content, never under
// a retention switch.
var sharedLines = []string{
	"rashomon: after compaction (boundary 1; rev C1+ov16).",
	"Owed: pytest services/auth; pytest jobs; pytest tests/integration -k session.",
	"R2 (inferred, conf 0.5): SESSION_TTL_SECONDS in services/auth/auth/sessions.py unchanged from C1. Holds at C1+ov16.",
	"Touched: services/auth/auth/sessions.py (ov12), services/auth/auth/cache.py (ov14), jobs/purge_sessions.py (ov16).",
}

// TestBoundary_RecoveryRetentionOff_BriefHasOwedAndPredicatesOnly:
// contracts-v2 section 1.3. With bodies off and the transcript unreadable
// the brief keeps every obligation, predicate and touched path and
// delivers no words of the person's or the model's; it says what it could
// not deliver. The 11-04a trace delivered the goal words with no record
// of where they came from.
func TestBoundary_RecoveryRetentionOff_BriefHasOwedAndPredicatesOnly(t *testing.T) {
	f := newFixture(t, retention{bodies: false, unreadable: true})
	f.reads()
	f.editsThroughPurge(t)
	f.MarkBoundary(17)
	brief := f.BoundaryBrief()
	want := append(sharedLines,
		"Task: 1 goal stated at prompt 1 (59 bytes). Words not retained; transcript not readable. Re-read your prompt 1 or ask the user to restate it.",
		"R1: 1 requirement stated at prompt 1 (38 bytes). Words not retained. Necessary condition: create_session or is_expired calls orgs.settings.get_org_settings. Holds at C1+ov16.",
		"Held decision: 1, from your plan, call 7 (89 bytes). Words not retained. It named services/orgs/orgs/settings.py and services/auth/auth/sessions.py.",
	)
	for _, w := range want {
		if !strings.Contains(brief, w) {
			t.Fatalf("brief lacks %q:\n%s", w, brief)
		}
	}
	for _, leak := range []string{"org-level timeout override", "get_org_settings(org_id)"} {
		if strings.Contains(brief, leak) {
			t.Fatalf("words delivered with retention off (%q):\n%s", leak, brief)
		}
	}
	if lost := f.KnowsIn(Lost); len(lost) != 12 {
		t.Fatalf("know lost = %d, want 12 (6 sent + 3 opened + 3 wrote)", len(lost))
	}
}

// TestBoundary_RecoveryRetentionByRef_BriefHasGoalWordsWhenDigestMatches:
// contracts-v2 section 1.2. Bodies off, transcript readable: the words
// are read back through the reference and delivered only when their
// digest and length match the record; a changed line falls back to the
// off form; bodies on delivers the copy when the transcript is gone.
func TestBoundary_RecoveryRetentionByRef_BriefHasGoalWordsWhenDigestMatches(t *testing.T) {
	f := newFixture(t, retention{})
	f.reads()
	f.editsThroughPurge(t)
	f.MarkBoundary(17)
	brief := f.BoundaryBrief()
	want := append(sharedLines,
		`Task (prompt 1, transcript line 3, digest ok): "make session expiry respect the org-level timeout override."`,
		`R1 (prompt 1, digest ok): "respect the org-level timeout override". Necessary condition: create_session or is_expired calls orgs.settings.get_org_settings. Holds at C1+ov16. Sufficiency not checked by the engine.`,
		`Held decision (your plan, call 7, digest ok): "read the org value through get_org_settings(org_id); do not import orgs.models into auth."`,
	)
	for _, w := range want {
		if !strings.Contains(brief, w) {
			t.Fatalf("brief lacks %q:\n%s", w, brief)
		}
	}
	if strings.Contains(brief, "Words not retained") {
		t.Fatalf("by-reference brief fell back:\n%s", brief)
	}

	f2 := newFixture(t, retention{})
	f2.editsThroughPurge(t)
	f2.corruptTranscriptLine(t, goalLine)
	f2.MarkBoundary(17)
	brief2 := f2.BoundaryBrief()
	if strings.Contains(brief2, "org-level timeout override") || !strings.Contains(brief2, "Words not retained; transcript changed.") {
		t.Fatalf("digest mismatch must fall back:\n%s", brief2)
	}
	if !strings.Contains(brief2, `Held decision (your plan, call 7, digest ok)`) {
		t.Fatalf("an untouched line must still resolve:\n%s", brief2)
	}

	f3 := newFixture(t, retention{bodies: true, unreadable: true})
	f3.editsThroughPurge(t)
	f3.MarkBoundary(17)
	if brief3 := f3.BoundaryBrief(); !strings.Contains(brief3, `Task (prompt 1): "make session expiry respect the org-level timeout override."`) {
		t.Fatalf("bodies on must deliver the copy when the transcript is unreadable:\n%s", brief3)
	}
}

// TestBoundary_RequirementFailureIsNamed: a requirement whose predicate
// no longer holds says so at the boundary.
func TestBoundary_RequirementFailureIsNamed(t *testing.T) {
	f := newFixture(t, retention{})
	f.edit(t, 12, sessionsPy, "SESSION_TTL_SECONDS = 3600", "SESSION_TTL_SECONDS = 7200")
	f.MarkBoundary(13)
	if brief := f.BoundaryBrief(); !strings.Contains(brief, "unchanged from C1. Fails at C1+ov12.") {
		t.Fatalf("R2 failure not named:\n%s", brief)
	}
}
