package recap

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func markInstalled(t *testing.T, root string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(root, "install.json"), []byte(`{}`), 0o600); err != nil {
		t.Fatal(err)
	}
}

// TestClaimNoStoreNeverWrites is H-87's rule applied to this package's own
// bookkeeping file: asking recap to decide must not be what plants the first
// file in an otherwise-empty store root.
func TestClaimNoStoreNeverWrites(t *testing.T) {
	root := t.TempDir()
	speak, err := Claim(root, "sess-1", "prompt-1", time.Now(), true)
	if err != nil {
		t.Fatalf("Claim: %v", err)
	}
	if !speak {
		t.Error("speak = false with no bookkeeping to dedupe against; wantSpeak should pass through unchanged")
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Errorf("root is no longer empty after Claim with no store: %v", entries)
	}
}

// TestClaimDedupesTheSamePromptTwice is H-101's unit-level twin: a second
// Claim for the exact same (session, prompt) pair that already spoke must
// not speak again, even though the caller still asks for it.
func TestClaimDedupesTheSamePromptTwice(t *testing.T) {
	root := t.TempDir()
	markInstalled(t, root)
	now := time.Now()

	speak, err := Claim(root, "sess-1", "prompt-1", now, true)
	if err != nil {
		t.Fatalf("first Claim: %v", err)
	}
	if !speak {
		t.Fatal("first Claim for a new (session, prompt) pair should speak")
	}

	speak, err = Claim(root, "sess-1", "prompt-1", now.Add(time.Second), true)
	if err != nil {
		t.Fatalf("second Claim: %v", err)
	}
	if speak {
		t.Error("second Claim for the same (session, prompt) pair spoke again -- H-101's break")
	}
}

// TestClaimANewPromptInTheSameSessionStillSpeaks guards against the
// over-broad fix: deduping by session_id alone rather than the pair would
// silence every later turn in a long-running session.
func TestClaimANewPromptInTheSameSessionStillSpeaks(t *testing.T) {
	root := t.TempDir()
	markInstalled(t, root)
	now := time.Now()

	if _, err := Claim(root, "sess-1", "prompt-1", now, true); err != nil {
		t.Fatal(err)
	}
	speak, err := Claim(root, "sess-1", "prompt-2", now, true)
	if err != nil {
		t.Fatal(err)
	}
	if !speak {
		t.Error("a new prompt id in the same session was suppressed by the previous prompt's claim")
	}
}

// TestClaimASilentTurnClaimsNothing is the other half of H-101's fix: a turn
// that found nothing must not poison a LATER firing of the same prompt (a
// continuation that goes on to produce a real finding) into staying silent.
func TestClaimASilentTurnClaimsNothing(t *testing.T) {
	root := t.TempDir()
	markInstalled(t, root)
	now := time.Now()

	speak, err := Claim(root, "sess-1", "prompt-1", now, false)
	if err != nil {
		t.Fatal(err)
	}
	if speak {
		t.Fatal("wantSpeak was false; speak must stay false")
	}
	speak, err = Claim(root, "sess-1", "prompt-1", now, true)
	if err != nil {
		t.Fatal(err)
	}
	if !speak {
		t.Error("a prompt that never spoke was still treated as already claimed")
	}
}

// TestStatusDistinguishesNotEvaluatedFromEvaluated is H-92's unit-level
// twin.
func TestStatusDistinguishesNotEvaluatedFromEvaluated(t *testing.T) {
	root := t.TempDir()
	markInstalled(t, root)

	if evaluated, _, _, err := Status(root); err != nil || evaluated {
		t.Fatalf("Status before any Claim: evaluated=%v err=%v, want false, nil", evaluated, err)
	}

	now := time.Now()
	if _, err := Claim(root, "sess-1", "prompt-1", now, false); err != nil {
		t.Fatal(err)
	}

	evaluated, lastAt, healthy, err := Status(root)
	if err != nil {
		t.Fatalf("Status after a silent Claim: %v", err)
	}
	if !evaluated {
		t.Error("evaluated = false after a Claim that found nothing -- silence must still move " +
			"the evaluated clock (H-92)")
	}
	if !healthy {
		t.Error("healthy = false after a normal Claim")
	}
	if lastAt.UnixMilli() != now.UnixMilli() {
		t.Errorf("lastAt = %v, want %v", lastAt, now)
	}
}

func TestRecordFailureMarksUnhealthy(t *testing.T) {
	root := t.TempDir()
	markInstalled(t, root)
	now := time.Now()

	RecordFailure(root, now)

	evaluated, _, healthy, err := Status(root)
	if err != nil {
		t.Fatal(err)
	}
	if !evaluated {
		t.Error("evaluated = false after RecordFailure; the failed run itself must still count " +
			"as an evaluation")
	}
	if healthy {
		t.Error("healthy = true after RecordFailure")
	}
}

func TestClaimSurvivesACorruptStateFile(t *testing.T) {
	root := t.TempDir()
	markInstalled(t, root)
	if err := os.WriteFile(statePath(root), []byte("not json"), 0o600); err != nil {
		t.Fatal(err)
	}

	speak, err := Claim(root, "sess-1", "prompt-1", time.Now(), true)
	if err != nil {
		t.Fatalf("Claim over a corrupt state file returned an error rather than recovering: %v", err)
	}
	if !speak {
		t.Error("a corrupt bookkeeping file suppressed a real finding")
	}
}
