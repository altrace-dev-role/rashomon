package recap

import (
	"fmt"
	"os"
	"testing"
	"time"
)

// TestDONE1g_TakeUnverifiedIsOncePerFlaggedStop: a Stop that raised the done flag
// is taken once by its session's next prompt; a Stop replayed for the same
// turn is not taken again, a later flagged turn is, another session never
// sees it, and a Stop that did not flag keeps nothing.
func TestDONE1g_TakeUnverifiedIsOncePerFlaggedStop(t *testing.T) {
	root := t.TempDir()
	markInstalled(t, root)
	now := time.Now()
	claim := func(session, prompt string, flagged bool) {
		t.Helper()
		if _, err := ClaimTurn(root, session, prompt, now, true, "", flagged); err != nil {
			t.Fatal(err)
		}
	}
	if TakeUnverified(root, "s1") {
		t.Error("nothing flagged yet: taken")
	}
	claim("s1", "p1", false)
	if TakeUnverified(root, "s1") {
		t.Error("a Stop that did not flag: taken")
	}
	claim("s1", "p2", true)
	if TakeUnverified(root, "s2") {
		t.Error("another session took s1's flag")
	}
	if !TakeUnverified(root, "s1") {
		t.Fatal("the flagged Stop was not taken by the next prompt")
	}
	if TakeUnverified(root, "s1") {
		t.Error("the same flagged Stop was taken twice")
	}
	claim("s1", "p2", true) // the Stop replayed for the same turn
	if TakeUnverified(root, "s1") {
		t.Error("a replayed Stop raised the note again")
	}
	claim("s1", "p3", true)
	if !TakeUnverified(root, "s1") {
		t.Error("a later flagged turn was not taken")
	}
}

// TestDONE1h_TakeUnverifiedWithoutAStoreOrAFileAnswersFalseAndWritesNothing, and
// an unreadable file answers false: the take is never a reason to write over
// bookkeeping it could not read.
func TestDONE1h_TakeUnverifiedWithoutAStoreOrAFileAnswersFalseAndWritesNothing(t *testing.T) {
	root := t.TempDir()
	if TakeUnverified(root, "s1") {
		t.Error("no store: taken")
	}
	if entries, _ := os.ReadDir(root); len(entries) != 0 {
		t.Errorf("a take with no store wrote %v", entries)
	}
	markInstalled(t, root)
	if TakeUnverified(root, "s1") {
		t.Error("no recap.json: taken")
	}
	if _, err := os.Stat(statePath(root)); err == nil {
		t.Error("a take with nothing flagged wrote recap.json")
	}
	if err := os.WriteFile(statePath(root), []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	if TakeUnverified(root, "s1") {
		t.Error("an unreadable recap.json: taken")
	}
}

// TestDONE1i_UnverifiedIsCappedWithTheOtherMaps: the flagged and taken maps hold
// at most the tracked sessions, as the other maps do.
func TestDONE1i_UnverifiedIsCappedWithTheOtherMaps(t *testing.T) {
	root := t.TempDir()
	markInstalled(t, root)
	for i := 0; i < maxTrackedSessions+10; i++ {
		if _, err := ClaimTurn(root, fmt.Sprintf("s%03d", i), "p", time.Now(), true, "", true); err != nil {
			t.Fatal(err)
		}
		TakeUnverified(root, fmt.Sprintf("s%03d", i))
	}
	s, err := readState(statePath(root))
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Unverified) > maxTrackedSessions || len(s.Noted) > maxTrackedSessions {
		t.Errorf("unverified %d, noted %d sessions, want at most %d", len(s.Unverified), len(s.Noted), maxTrackedSessions)
	}
}
