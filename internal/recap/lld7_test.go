package recap

import (
	"encoding/json"
	"fmt"
	"os"
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

// TestRubricLLD7_AnOverCapFileKeepsTheClaim: the base bug was a drop in map
// order, which TestRubricLLD7_ClaimKeepsTheSessionItWrote catches only about
// one run in 65, because a file Claim keeps under the cap has one session to
// drop. A recap.json written over the cap -- by a build before the cap, or by
// hand -- makes the cap drop thousands at once. In map order the session just
// claimed then survives a trial with probability 64 in 4,097, and five trials
// leave it no realistic chance. The fixed cap never drops it, though its id
// sorts first, the order the cap drops in.
func TestRubricLLD7_AnOverCapFileKeepsTheClaim(t *testing.T) {
	const others, trials = 64 * maxTrackedSessions, 5
	for trial := 0; trial < trials; trial++ {
		root := t.TempDir()
		markInstalled(t, root)
		s := newState()
		for i := 0; i < others; i++ {
			id := fmt.Sprintf("sess-%05d", i)
			s.Spoken[id], s.Checked[id] = "p", "p"
		}
		b, err := json.Marshal(s)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(statePath(root), b, 0o600); err != nil {
			t.Fatal(err)
		}
		const current = "aaaa-current"
		if speak, err := Claim(root, current, "p-now", time.Now(), true); err != nil || !speak {
			t.Fatalf("trial %d: first Claim for the current turn: speak %v, %v", trial, speak, err)
		}
		got, err := readState(statePath(root))
		if err != nil {
			t.Fatal(err)
		}
		if got.Spoken[current] != "p-now" || got.Checked[current] != "p-now" {
			t.Fatalf("trial %d: the cap dropped the session just claimed from a %d-session file (spoken %q, checked %q)",
				trial, others, got.Spoken[current], got.Checked[current])
		}
		if len(got.Spoken) > maxTrackedSessions || len(got.Checked) > maxTrackedSessions {
			t.Errorf("trial %d: tracking %d spoken and %d checked sessions, over the cap of %d",
				trial, len(got.Spoken), len(got.Checked), maxTrackedSessions)
		}
	}
}
