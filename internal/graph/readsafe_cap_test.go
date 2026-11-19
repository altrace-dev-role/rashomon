package graph

import "testing"

// TestRS1a_C4_TheIndexCapIs64MiB pins the cap of the repository index (card
// READ-SAFE1a, condition c4): MaxGraphFileBytes is 64 MiB. The read side
// refuses over it by name (TestRS1a_R6_LoadFileOverTheCapIsRefused), and the
// writer is to hold the same name, so what one writes the other reads. A
// change of the value changes how large a repository may be indexed, and is
// made here on purpose.
func TestRS1a_C4_TheIndexCapIs64MiB(t *testing.T) {
	if MaxGraphFileBytes != 64<<20 {
		t.Fatalf("MaxGraphFileBytes = %d, want %d (64 MiB)", MaxGraphFileBytes, 64<<20)
	}
}
