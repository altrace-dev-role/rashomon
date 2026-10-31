package receipt

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/store"
)

// filler is n lines of about 1 KB that are not checkpoints.
func filler(n int) []string {
	pad := `{"type":"declaration","schema_version":2,"pad":"` + strings.Repeat("x", 1000) + `"}`
	out := make([]string, n)
	for i := range out {
		out[i] = pad
	}
	return out
}

// TestRP2zf_PointedWidensOnATailMissAndNeverSaysGoneForARunThatIsThere (card
// REPORT1b review): a run that cut its checkpoint and then wrote 100 KB of
// other records still has its record, past the first 64 KiB read. Pointed
// widens once, to at most PointedMax, and finds it; a run longer than that
// with the record before the window says "not found within the read bound",
// not that the record is gone; only a missing run file (or a file read whole
// with none in it) is gone. Break: never widen, call every miss gone, read
// the whole file, or lose the missing-file case.
func TestRP2zf_PointedWidensOnATailMissAndNeverSaysGoneForARunThatIsThere(t *testing.T) {
	st := openStore(t)
	const p = "/work/a"
	cp := ckpt{project: p, session: "s1", ms: 900_000, seq: 7}.line()
	seedRun(t, st, "s1", 1_000_000, append([]string{cp}, filler(100)...)...)
	got, sc, err := Pointed(st, p, "s1", 7)
	if err != nil || !sc.Found || sc.Bound != "" || got.PointSeq != 7 {
		t.Fatalf("a checkpoint then 100 KB of records: %+v %+v err %v; want found", got, sc, err)
	}
	if sc.ReadBytes <= TailBytes || sc.ReadBytes > TailBytes+PointedMax {
		t.Errorf("the widening read %d bytes; want between %d and %d", sc.ReadBytes, TailBytes, TailBytes+PointedMax)
	}

	big := int(PointedMax/1000) + 200
	seedRun(t, st, "s2", 1_000_000, append([]string{ckpt{project: p, session: "s2", ms: 900_000, seq: 7}.line()}, filler(big)...)...)
	got, sc, err = Pointed(st, p, "s2", 7)
	if err != nil || sc.Found || sc.Bound != BoundTail {
		t.Fatalf("a record before the widened window: %+v %+v err %v; want bound tail", got, sc, err)
	}
	if sc.ReadBytes > TailBytes+PointedMax {
		t.Errorf("read %d bytes; the bound is %d", sc.ReadBytes, TailBytes+PointedMax)
	}
	line := PreviousLine(got, sc, "ptr")
	if line != "last session: its record was not found within the 1 MiB read bound" || strings.Contains(line, "no longer") || len("  "+line) > 80 {
		t.Errorf("the tail-bound line: %q", line)
	}

	if _, sc, err = Pointed(st, p, "nosuch", 7); err != nil || sc.Found || sc.Bound != BoundGone {
		t.Errorf("a missing run: %+v err %v; want gone", sc, err)
	}
	seedRun(t, st, "s3", 1_000_000, filler(3)...)
	if _, sc, err = Pointed(st, p, "s3", 7); err != nil || sc.Found || sc.Bound != BoundGone {
		t.Errorf("a short run read whole with no checkpoint: %+v err %v; want gone", sc, err)
	}
	if err := os.Remove(filepath.Join(st.RunDir("s3"), store.FileRecords)); err != nil {
		t.Fatal(err)
	}
	if _, sc, err = Pointed(st, p, "s3", 7); err != nil || sc.Bound != BoundGone {
		t.Errorf("a run directory with no records file: %+v err %v; want gone", sc, err)
	}
}
