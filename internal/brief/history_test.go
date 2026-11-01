package brief

// SEED1-KEEP: the brief says the person's kept before count, at startup, to
// the person only (systemMessage), and names the command until one is kept,
// by this binary's path as the spec trust line does (D189).
// A brief with no store root has no such line.

import (
	"strings"
	"testing"
	"time"

	"github.com/altrace-dev-role/rashomon/internal/baseline"
	"github.com/altrace-dev-role/rashomon/internal/seed"
)

func TestSEED2e_TheBriefCarriesTheKeptCount(t *testing.T) {
	root, storeRoot := t.TempDir(), t.TempDir()
	if b := Build(root, "", Pointer{}, nil); hasKind(b, KindHistory) {
		t.Errorf("a brief with no store root has a history line: %+v", b.Facts)
	}
	cmd, _ := self()
	if f := fact(t, Build(root, storeRoot, Pointer{}, nil), KindHistory); f.Text != "no before count kept yet; run in your own terminal:" || f.Next != cmd+" audit --keep" {
		t.Errorf("no kept count: %+v", f)
	}
	k := seed.Kept{Version: 1, KeptAt: time.Now().UTC(), Days: 14,
		All: seed.Counts{Sessions: 9, Kinds: map[string]int{"interrupted": 4, "rejected": 3, "pushed-back": 2}},
		Projects: map[string]seed.Counts{baseline.Key(root): {Sessions: 4, Kinds: map[string]int{"rejected": 3},
			Rows: []seed.KeptRow{{Kind: "failed-again", Class: "stale", Times: 3, Sessions: 3}}}}}
	if err := seed.WriteKept(storeRoot, k); err != nil {
		t.Fatal(err)
	}
	b := Build(root, storeRoot, Pointer{}, nil)
	f := fact(t, b, KindHistory)
	if f.Text != "9 corrections in the 14 days before AutoSteer, 3 in this project" ||
		f.Next != "most here: a write to a file changed since it was read, then tried again" {
		t.Errorf("kept count: %+v", f)
	}
	if b.Facts[len(b.Facts)-1].Kind != KindVerify || b.Facts[len(b.Facts)-2].Kind != KindHistory {
		t.Errorf("history is not the line before verify: %+v", b.Facts)
	}
	for _, l := range strings.Split(Render(b), "\n") {
		if n := len([]rune(l)); n > Width {
			t.Errorf("a brief line is %d columns, over %d: %q", n, Width, l)
		}
	}
}

func hasKind(b Brief, k string) bool {
	for _, f := range b.Facts {
		if f.Kind == k {
			return true
		}
	}
	return false
}
