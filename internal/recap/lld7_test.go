package recap

import (
	"fmt"
	"testing"
	"time"
)

// TestRubricLLD7_ClaimKeepsTheSessionItWrote: recap.json tracks at most
// maxTrackedSessions sessions, and the cap used to delete any of them --
// including the one Claim had just written. A replayed Stop for that same
// turn then found no claim and printed its line again (about 1% of trials
// when the order was random). The session being claimed is now never the
// one dropped, and the cap still holds. The current session's id sorts
// first here, which is the order the cap drops in.
func TestRubricLLD7_ClaimKeepsTheSessionItWrote(t *testing.T) {
	for _, others := range []int{maxTrackedSessions - 1, maxTrackedSessions, maxTrackedSessions + 1} {
		t.Run(fmt.Sprintf("%d sessions", others+1), func(t *testing.T) {
			root := t.TempDir()
			markInstalled(t, root)
			now := time.Now()
			for i := 0; i < others; i++ {
				if _, err := Claim(root, fmt.Sprintf("sess-%03d", i), "p", now, true); err != nil {
					t.Fatal(err)
				}
			}
			const current = "aaaa-current"
			if speak, err := Claim(root, current, "p-now", now, true); err != nil || !speak {
				t.Fatalf("first Claim for the current turn: speak %v, %v", speak, err)
			}
			if speak, err := Claim(root, current, "p-now", now, true); err != nil || speak {
				t.Errorf("a replayed Stop for the same turn spoke again (speak %v, %v): the cap dropped the claim just written", speak, err)
			}
			s, err := readState(statePath(root))
			if err != nil {
				t.Fatal(err)
			}
			if len(s.Spoken) > maxTrackedSessions || len(s.Checked) > maxTrackedSessions {
				t.Errorf("tracking %d spoken and %d checked sessions, over the cap of %d", len(s.Spoken), len(s.Checked), maxTrackedSessions)
			}
			if s.Checked[current] != "p-now" {
				t.Errorf("the current session's check was dropped: %q", s.Checked[current])
			}
		})
	}
}
