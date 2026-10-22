package recap

import (
	"fmt"
	"path/filepath"
	"testing"
)

// TestRubricTEST8_TrackedSessionsAtNAndNPlusOne: recap.json remembers at most
// maxTrackedSessions sessions in each of its maps: 64 are all kept, and at
// 65 the map is cut back to 64.
func TestRubricTEST8_TrackedSessionsAtNAndNPlusOne(t *testing.T) {
	if maxTrackedSessions != 64 {
		t.Fatalf("maxTrackedSessions = %d, want 64", maxTrackedSessions)
	}
	for _, n := range []int{maxTrackedSessions, maxTrackedSessions + 1} {
		s := state{Spoken: map[string]string{}, Checked: map[string]string{}}
		for i := 0; i < n; i++ {
			id := fmt.Sprintf("sess-%03d", i)
			s.Spoken[id], s.Checked[id] = "turn", "turn"
		}
		path := filepath.Join(t.TempDir(), stateFileName)
		if err := writeState(path, s, ""); err != nil {
			t.Fatal(err)
		}
		got, err := readState(path)
		if err != nil {
			t.Fatal(err)
		}
		want := min(n, maxTrackedSessions)
		if len(got.Spoken) != want || len(got.Checked) != want {
			t.Errorf("%d sessions written: %d spoken and %d checked kept, want %d", n, len(got.Spoken), len(got.Checked), want)
		}
	}
}
