package receipt

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/store"
)

// pastTheCap is the name of a run directory that the filesystem lists after
// its first MaxWalk entries, in the order it lists them (hash order on APFS).
func pastTheCap(t *testing.T, st *store.Store) string {
	t.Helper()
	dir, err := os.Open(filepath.Join(st.Root(), store.DirRuns))
	if err != nil {
		t.Fatal(err)
	}
	defer dir.Close() //nolint:errcheck // read-only
	entries, err := dir.ReadDir(-1)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) <= MaxWalk {
		t.Fatalf("%d run directories, want more than %d", len(entries), MaxWalk)
	}
	return entries[len(entries)-1].Name()
}

// TestRP2zh_LatestWalksEveryRunDirectory (card REPORT1b review): with 1,100
// runs and the newest checkpoint in a directory the filesystem lists past the
// first 1,024, Latest returns it (the commit trailer cites what Latest
// returns, so a stale one would name the wrong checkpoint). The report
// baseline's capped walk is the one that leaves directories out, and says so
// whether it found a checkpoint or not; it is capped only when asked.
// Break: cap Latest's walk, or let an unasked walk cap itself.
func TestRP2zh_LatestWalksEveryRunDirectory(t *testing.T) {
	st := openStore(t)
	const p = "/work/p"
	for i := range 1100 {
		seedRun(t, st, fmt.Sprintf("r%04d", i), int64(1_000_000+i), checkpoint(p, int64(900_000+i)))
	}
	newest := pastTheCap(t, st)
	want := checkpoint(p, 5_000_000)
	seedRun(t, st, newest, 6_000_000, want)

	got, ok, err := Latest(st, p)
	if err != nil || !ok || string(got) != want {
		t.Fatalf("Latest with the newest past the first %d directories: %q ok %t err %v; want %q", MaxWalk, got, ok, err, want)
	}
	last, sc, err := Previous(st, Query{Project: p})
	if err != nil || !sc.Found || sc.Truncated || sc.Bound != "" || last.RecordedAtUnixMS != 5_000_000 {
		t.Errorf("an uncapped Previous: %+v %+v err %v; want the newest, not truncated", last, sc, err)
	}
	last, sc, err = Previous(st, Query{Project: p, CapWalk: true})
	if err != nil || !sc.Truncated || (sc.Found && last.RecordedAtUnixMS == 5_000_000) {
		t.Errorf("a capped Previous: %+v %+v err %v; want truncated and not the newest", last, sc, err)
	}
}
