package gather

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/receipt"
	"github.com/altrace-dev-role/rashomon/internal/report"
	"github.com/altrace-dev-role/rashomon/internal/store"
)

// TestRP2zi_ABaselineWalkTheCapCutSaysSo (card REPORT1b review): in a store of
// 1,100 runs whose baseline sits past the first 1,024 directories the
// filesystem lists, the knowledge block's baseline is null with
// BaselineWalkCap set, and the text says "not found in the first 1024 session
// directories walked", not "no earlier checkpoint". A store the cap does not
// cut keeps the plain "no earlier checkpoint" line. Break: drop the cap's
// flag, or print the plain line for a cut walk.
func TestRP2zi_ABaselineWalkTheCapCutSaysSo(t *testing.T) {
	st := kStore(t)
	for i := range 1100 {
		seed(t, st, fmt.Sprintf("f%04d", i), kc{project: "/other", at: kBase - 90_000 + int64(i), seq: 1})
	}
	dir, err := os.Open(filepath.Join(st.Root(), store.DirRuns))
	if err != nil {
		t.Fatal(err)
	}
	entries, err := dir.ReadDir(-1)
	_ = dir.Close()
	if err != nil || len(entries) <= receipt.MaxWalk {
		t.Fatalf("%d entries, err %v", len(entries), err)
	}
	seed(t, st, entries[len(entries)-1].Name(), kc{at: kBase - 5000, seq: 9})
	seed(t, st, "s1", kc{at: kBase + 1000, seq: 1})

	k := knowledgeOf(st, nil, "s1", kNow)
	if k.Baseline != nil || k.BaselineWalkCap != receipt.MaxWalk {
		t.Fatalf("baseline %+v cap %d; want none found, cap %d", k.Baseline, k.BaselineWalkCap, receipt.MaxWalk)
	}
	rep, err := Build(st, "s1", kNow)
	if err != nil {
		t.Fatal(err)
	}
	var b bytes.Buffer
	if err := report.Text(&b, rep); err != nil {
		t.Fatal(err)
	}
	if got := b.String(); !strings.Contains(got, "baseline: not found in the first 1024 session directories walked") || strings.Contains(got, "no earlier checkpoint") {
		t.Errorf("the knowledge text:\n%s", got)
	}

	small := kStore(t)
	seed(t, small, "s1", kc{at: kBase + 1000, seq: 1})
	if k := knowledgeOf(small, nil, "s1", kNow); k.Baseline != nil || k.BaselineWalkCap != 0 {
		t.Errorf("a store the cap does not cut: baseline %+v cap %d", k.Baseline, k.BaselineWalkCap)
	}
}
