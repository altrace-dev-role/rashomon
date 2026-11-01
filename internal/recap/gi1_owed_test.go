package recap

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"time"
)

// TestGI1s_TheTurnEndKeepsWhatItOwesTheNextPrompt: what a turn end owes its
// session's next prompt (card GATE-INFER1, W5) is kept per session in
// recap.json, read with the catch-up's own read (Turn), and replaced or
// cleared by the session's next turn end, whatever the prompt; another
// session's stays, and the cap keeps the session just claimed. Controls: no
// session id reads none, and the checked prompt is not pending. Break: drop
// the map from the write, key it by prompt, or leave it out of the cap.
func TestGI1s_TheTurnEndKeepsWhatItOwesTheNextPrompt(t *testing.T) {
	root := t.TempDir()
	markInstalled(t, root)
	now := time.Now()
	ci := Owed{Seq: 12, Gate: "pytest", From: ".github/workflows/ci.yml"}
	mk := Owed{Seq: 3, Stale: true, Gate: "make test", From: "Makefile"}
	for _, c := range []struct {
		session string
		owed    Owed
	}{{"s1", ci}, {"s2", mk}} {
		if _, err := ClaimShown(root, c.session, "p1", now, true, "", c.owed); err != nil {
			t.Fatal(err)
		}
	}
	if pending, got := Turn(root, "s1", "p2"); !pending || got != ci {
		t.Fatalf("Turn(s1, p2) = %v, %+v, want pending and %+v", pending, got, ci)
	}
	if pending, got := Turn(root, "s1", "p1"); pending || got != ci {
		t.Errorf("Turn(s1, p1) = %v, %+v, want the checked prompt not pending, the owed note kept per session", pending, got)
	}
	if _, err := ClaimShown(root, "s1", "p2", now, false, "", Owed{}); err != nil {
		t.Fatal(err)
	}
	if _, got := Turn(root, "s1", "p3"); got != (Owed{}) {
		t.Errorf("after a turn end that owes nothing: %+v, want none", got)
	}
	if _, got := Turn(root, "s2", "p2"); got != mk {
		t.Errorf("another session's: %+v, want %+v kept", got, mk)
	}
	if pending, got := Turn(root, "", "p2"); pending || got != (Owed{}) {
		t.Errorf("no session id: %v, %+v, want nothing", pending, got)
	}

	for i := range maxTrackedSessions + 6 {
		if _, err := ClaimShown(root, fmt.Sprintf("x%02d", i), "p1", now, true, "", mk); err != nil {
			t.Fatal(err)
		}
	}
	last := fmt.Sprintf("x%02d", maxTrackedSessions+5)
	if _, got := Turn(root, last, "p2"); got != mk {
		t.Errorf("the session just claimed: %+v, want %+v kept by the cap", got, mk)
	}
	s, err := readState(statePath(root))
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Owed) > maxTrackedSessions {
		t.Errorf("%d owed sessions, want at most %d", len(s.Owed), maxTrackedSessions)
	}
	b, err := os.ReadFile(statePath(root))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), `"owed":{`) {
		t.Errorf("recap.json %s holds no owed map", b)
	}
}
