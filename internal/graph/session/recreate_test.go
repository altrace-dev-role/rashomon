//go:build unix

package session

import (
	"bytes"
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/store"
)

// inodeOf is path's inode number.
func inodeOf(t *testing.T, path string) uint64 {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	return uint64(info.Sys().(*syscall.Stat_t).Ino) //nolint:unconvert // width differs by unix flavour
}

// recreateCycles is how many times TestCompose_RecreatedRecordsFoldWhole
// removes and recreates the records file.
const recreateCycles = 100

// TestCompose_RecreatedRecordsFoldWhole: records.ndjson is removed and a new
// one written in its place, recreateCycles times, each the length of the one
// before with its lines' ends at the same offsets but its paths another
// pair, so the saved records cursor still lands on a line end at the size it
// saw and G has not moved. Each compose must fold the new file whole and
// never continue the frontier of lines that are gone: the mark of the line
// that ends at the cursor tells the files apart. How often the new file got
// the old one's inode is logged, a reading of the file system the test ran
// on and not a check: an inode, alone or with its device, would not tell
// the files apart whenever it came back.
func TestCompose_RecreatedRecordsFoldWhole(t *testing.T) {
	st := composeStore(t)
	declareEdit(t, st, "toolu_a", "a.go", 1000)
	declareEdit(t, st, "toolu_b", "b.go", 5000)
	if _, _, saved := mustCompose(t, st); !saved {
		t.Fatal("the first compose saved nothing")
	}
	rec := filepath.Join(st.RunDir(composeID), store.FileRecords)
	ab, err := os.ReadFile(rec)
	if err != nil {
		t.Fatal(err)
	}
	cd := bytes.Replace(bytes.Replace(ab, []byte("/w/p/a.go"), []byte("/w/p/c.go"), 1), []byte("/w/p/b.go"), []byte("/w/p/d.go"), 1)
	if len(cd) != len(ab) || bytes.Equal(cd, ab) || ab[len(ab)-1] != '\n' {
		t.Fatalf("control: the second pair's file is %d bytes against %d, or equal to the first, or has no line end", len(cd), len(ab))
	}
	reused := 0
	for i := 1; i <= recreateCycles; i++ {
		next := ab
		if i%2 == 1 {
			next = cd
		}
		before := inodeOf(t, rec)
		if err := os.Remove(rec); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(rec, next, 0o600); err != nil {
			t.Fatal(err)
		}
		if inodeOf(t, rec) == before {
			reused++
		}
		got, stats, saved := mustCompose(t, st)
		if want := wholeFold(t, st); !stats.full || !saved || got != want {
			t.Fatalf("recreation %d was continued (full %v, saved %v):\n got %s\nwant %s", i, stats.full, saved, got, want)
		}
	}
	t.Logf("INODE reused in %d of %d recreations", reused, recreateCycles)
}

// splitLines is b cut after each line end, the line ends kept.
func splitLines(b []byte) [][]byte {
	var out [][]byte
	for len(b) > 0 {
		i := bytes.IndexByte(b, '\n') + 1
		if i == 0 {
			i = len(b)
		}
		out = append(out, b[:i])
		b = b[i:]
	}
	return out
}

// TestCompose_RecreatedRecordsKeptLine: records.ndjson is replaced by two
// lines the lengths of the old two, one of them the old line byte for byte
// and the other a line of a different edit. With the first line kept, the
// mark of the last line tells the files apart and the compose folds the new
// file whole. With the last line kept, the compose continues the old
// frontier: that is the mark's limit, as catchUp and fold.CursorTailValid
// state it, pinned here so a change to it changes this test and those words
// together.
func TestCompose_RecreatedRecordsKeptLine(t *testing.T) {
	for _, tc := range []struct {
		name      string
		keep      int
		continued bool
	}{{"head kept", 0, false}, {"tail kept, the limit", 1, true}} {
		t.Run(tc.name, func(t *testing.T) {
			st := composeStore(t)
			declareEdit(t, st, "toolu_a", "a.go", 1000)
			declareEdit(t, st, "toolu_b", "b.go", 5000)
			if _, _, saved := mustCompose(t, st); !saved {
				t.Fatal("the first compose saved nothing")
			}
			rec := filepath.Join(st.RunDir(composeID), store.FileRecords)
			old, err := os.ReadFile(rec)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.Remove(rec); err != nil {
				t.Fatal(err)
			}
			declareEdit(t, st, "toolu_c", "c.go", 1000)
			declareEdit(t, st, "toolu_d", "d.go", 5000)
			donor, err := os.ReadFile(rec)
			if err != nil {
				t.Fatal(err)
			}
			ol, dl := splitLines(old), splitLines(donor)
			if len(ol) != 2 || len(dl) != 2 {
				t.Fatalf("control: %d and %d lines, want 2 and 2", len(ol), len(dl))
			}
			for i := range ol {
				if len(ol[i]) != len(dl[i]) || bytes.Equal(ol[i], dl[i]) || ol[i][len(ol[i])-1] != '\n' {
					t.Fatalf("control: line %d is %d bytes against %d, or equal, or has no line end", i+1, len(dl[i]), len(ol[i]))
				}
			}
			lines := [][]byte{dl[0], dl[1]}
			lines[tc.keep] = ol[tc.keep]
			if err := os.Remove(rec); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(rec, bytes.Join(lines, nil), 0o600); err != nil {
				t.Fatal(err)
			}
			got, stats, _ := mustCompose(t, st)
			want := wholeFold(t, st)
			if continued := !stats.full && got != want; continued != tc.continued || stats.full == tc.continued {
				t.Errorf("old line %d kept: continued %v (full %v), want continued %v:\n got %s\nwant %s", tc.keep+1, continued, stats.full, tc.continued, got, want)
			}
		})
	}
}
