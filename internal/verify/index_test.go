package verify

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"math/rand"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/baseline"
)

// The index tests' clock: idxNow is a fixed instant, dayMS one day.
const (
	idxNow = int64(1_793_000_000_000)
	dayMS  = int64(24 * 60 * 60 * 1000)
)

// idxRun is a gate run of the family runner@location at seq, of kind: the
// record's runner_kind when recorded, inferred when not.
func idxRun(seq int64, runner, location, kind string, recorded bool) gateRun {
	return gateRun{Seq: seq, Runner: ptr(runner), Location: location, Kind: kind, KindRecorded: recorded}
}

// entriesOf is idx's entries for family, found by a scan, so that a test does
// not lean on the order Required searches in.
func entriesOf(idx *ProjectIndex, family string) []IndexEntry {
	var out []IndexEntry
	for _, e := range idx.Entries {
		if e.Family == family {
			out = append(out, e)
		}
	}
	return out
}

// TestIDX1_AFamilySeenInTwoSessionsIsRequired (PF-PG1): a family run in two
// sessions is required, keyed by the Family gateStates gives the same runs,
// the key 1.5 looks up. Observed beside new families that sort before it, its
// entry is found rather than added twice.
func TestIDX1_AFamilySeenInTwoSessionsIsRequired(t *testing.T) {
	runs := []gateRun{idxRun(1, "r1", "l1", kindTest, true)}
	idx := &ProjectIndex{ProjectKey: "/p"}
	idx.Observe(runs, "s1", idxNow-dayMS)
	idx.Observe(append(slices.Clone(runs), idxRun(2, "a0", "l1", kindBuild, true), idxRun(3, "a1", "l1", kindBuild, true)), "s2", idxNow)
	gs := gateStates(runs, nil, nil, 1, nil)
	if len(gs) != 1 {
		t.Fatalf("gateStates gives %d families, want 1", len(gs))
	}
	es := entriesOf(idx, gs[0].Family)
	if len(es) != 1 || es[0].Sessions != 2 || len(idx.Entries) != 3 {
		t.Fatalf("entries %+v: want three, one of them %s with Sessions 2", idx.Entries, gs[0].Family)
	}
	if !idx.Required(gs[0].Family, idxNow) {
		t.Errorf("a family run in two sessions is not required")
	}
}

// TestIDX2_OneSessionIsNotRequired (PF-PG1): a family run in one session is
// advisory, and a second Observe for that session (a recap run again) does
// not count it twice.
func TestIDX2_OneSessionIsNotRequired(t *testing.T) {
	runs := []gateRun{idxRun(1, "r1", "l1", kindTest, true)}
	idx := &ProjectIndex{ProjectKey: "/p"}
	idx.Observe(runs, "s1", idxNow-dayMS)
	if idx.Required("r1@l1", idxNow) {
		t.Error("a family run in one session is required")
	}
	// Not required here, as one never seen: a missing index's default is Evaluate's (1.5).
	var none *ProjectIndex
	if none.Required("r1@l1", idxNow) || idx.Required("r2@l2", idxNow) {
		t.Error("a nil index, or a family not in the index, is required")
	}
	idx.Observe(runs, "s1", idxNow)
	if es := entriesOf(idx, "r1@l1"); len(es) != 1 || es[0].Sessions != 1 {
		t.Errorf("after one session observed twice: %+v, want one entry with Sessions 1", es)
	}
	if idx.Required("r1@l1", idxNow) {
		t.Error("one session observed twice is required")
	}
}

// TestIDX3_ThirtyDaysExpire (PF-PG1): a family is required while its last
// session is RequiredWindowMS old or less, and not a millisecond after. The
// last session's time is LastSeen, and the first's FirstSeen.
func TestIDX3_ThirtyDaysExpire(t *testing.T) {
	runs := []gateRun{idxRun(1, "r1", "l1", kindTest, true)}
	first, last := idxNow-RequiredWindowMS-7*dayMS, idxNow-RequiredWindowMS
	idx := &ProjectIndex{ProjectKey: "/p"}
	idx.Observe(runs, "s1", first)
	idx.Observe(runs, "s2", last)
	if es := entriesOf(idx, "r1@l1"); len(es) != 1 || es[0].FirstSeen != first || es[0].LastSeen != last {
		t.Fatalf("entries %+v: want FirstSeen %d and LastSeen %d", es, first, last)
	}
	if !idx.Required("r1@l1", idxNow) {
		t.Error("last seen exactly RequiredWindowMS ago: not required")
	}
	if idx.Required("r1@l1", idxNow+1) {
		t.Error("last seen a millisecond more than RequiredWindowMS ago: required")
	}
}

// TestIDX3b_AnOlderSessionRecappedLateMovesNothingBack (PF-PG1): sessions
// recapped out of order, each at its own time. An older session observed
// after a newer one widens FirstSeen but leaves LastSeen and Kind at the
// newer session's, so the window still runs from the newer one; as the
// session counted last, its retry counts once. Two sessions at one time
// take the later call's kind.
func TestIDX3b_AnOlderSessionRecappedLateMovesNothingBack(t *testing.T) {
	newer, older := idxNow-RequiredWindowMS, idxNow-RequiredWindowMS-5*dayMS
	idx := &ProjectIndex{ProjectKey: "/p"}
	idx.Observe([]gateRun{idxRun(1, "r1", "l1", kindTest, true)}, "n", newer)
	late := []gateRun{idxRun(1, "r1", "l1", kindBuild, true)}
	idx.Observe(late, "o", older)
	idx.Observe(late, "o", older)
	want := IndexEntry{Family: "r1@l1", Kind: kindTest, Sessions: 2, LastSession: "o", FirstSeen: older, LastSeen: newer}
	if es := entriesOf(idx, "r1@l1"); len(es) != 1 || es[0] != want {
		t.Fatalf("entries %+v, want %+v", es, want)
	}
	if !idx.Required("r1@l1", idxNow) {
		t.Error("the newer session was RequiredWindowMS ago, and the family is not required")
	}
	idx.Observe([]gateRun{idxRun(1, "r1", "l1", kindCustom, true)}, "tie", newer)
	if es := entriesOf(idx, "r1@l1"); len(es) != 1 || es[0].Kind != kindCustom || es[0].LastSeen != newer {
		t.Errorf("a session at the held LastSeen: entries %+v, want the later call's kind %s", es, kindCustom)
	}
}

// TestIDX4_IndexWriteIsAtomicAnd0600AndCapped (PF-PG1): WriteIndex leaves one
// file, at 0600, in a directory it made at 0700, and no temporary file; a
// write that cannot finish leaves the old content; over the cap it evicts the
// least recently seen first (LastSeen, then Family), no more than the cap
// needs, and says how many; a key over the cap alone writes nothing; and
// what it wrote reads back as it was.
func TestIDX4_IndexWriteIsAtomicAnd0600AndCapped(t *testing.T) {
	t.Run("one_file_0600_in_a_0700_directory", func(t *testing.T) {
		path := IndexPath(t.TempDir(), "/p")
		idx := &ProjectIndex{ProjectKey: "/p"}
		idx.Observe([]gateRun{idxRun(1, "r1", "l1", kindTest, true)}, "s1", idxNow)
		if n, err := WriteIndex(path, idx); n != 0 || err != nil {
			t.Fatalf("WriteIndex = %d, %v; want 0, nil", n, err)
		}
		wantMode(t, path, 0o600)
		wantMode(t, filepath.Dir(path), 0o700)
		des, err := os.ReadDir(filepath.Dir(path))
		if err != nil || len(des) != 1 || des[0].Name() != filepath.Base(path) {
			t.Errorf("the projects directory holds %v (%v), want only %s", des, err, filepath.Base(path))
		}
	})
	t.Run("a_failed_write_keeps_the_old_file", func(t *testing.T) {
		path := IndexPath(t.TempDir(), "/p")
		idx := &ProjectIndex{ProjectKey: "/p"}
		idx.Observe([]gateRun{idxRun(1, "r1", "l1", kindTest, true)}, "s1", idxNow-dayMS)
		if _, err := WriteIndex(path, idx); err != nil {
			t.Fatal(err)
		}
		want, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		dir := filepath.Dir(path)
		if err := os.Chmod(dir, 0o500); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { os.Chmod(dir, 0o700) }) //nolint:errcheck // TempDir's removal reports it
		if f, err := os.CreateTemp(dir, "probe"); err == nil {
			f.Close()           //nolint:errcheck // a probe
			os.Remove(f.Name()) //nolint:errcheck // a probe
			t.Skip("this user can write a 0500 directory (root), so a replace cannot be made to fail")
		}
		idx.Observe([]gateRun{idxRun(1, "r1", "l1", kindTest, true)}, "s2", idxNow)
		if _, err := WriteIndex(path, idx); err == nil {
			t.Error("WriteIndex into a read-only directory: nil error")
		}
		if got, err := os.ReadFile(path); err != nil || !bytes.Equal(got, want) {
			t.Errorf("after a failed write the file is %q (%v), want the old %q", got, err, want)
		}
	})
	t.Run("over_the_cap_the_least_recently_seen_go", func(t *testing.T) {
		idx := &ProjectIndex{ProjectKey: "/p"}
		for i := range 2000 {
			// Family order is a permutation of age order (7919 is prime to
			// 2000), so evicting by Family would keep the wrong ones.
			idx.Entries = append(idx.Entries, IndexEntry{Family: fmt.Sprintf("%064x@%064x", i*7919%2000, i),
				Kind: kindTest, Sessions: 2, LastSession: fmt.Sprintf("s-%034d", i),
				FirstSeen: idxNow - 40*dayMS - int64(i), LastSeen: idxNow - int64(2000-i)})
		}
		path := IndexPath(t.TempDir(), "/p")
		n, err := WriteIndex(path, idx)
		if err != nil || n == 0 {
			t.Fatalf("WriteIndex of 2000 entries = %d, %v; want some evicted, nil", n, err)
		}
		if b, err := os.ReadFile(path); err != nil || len(b) > MaxIndexBytes {
			t.Fatalf("the file is %d bytes (%v), over MaxIndexBytes", len(b), err)
		}
		got, err := ReadIndex(path)
		if err != nil || got == nil || len(got.Entries) != 2000-n {
			t.Fatalf("ReadIndex = %v; want %d entries", err, 2000-n)
		}
		was := map[string]IndexEntry{}
		for _, e := range idx.Entries {
			was[e.Family] = e
		}
		for _, e := range got.Entries {
			if e != was[e.Family] || e.LastSeen < idxNow-int64(2000-n) {
				t.Fatalf("kept %+v: want the %d most recently seen, as they were", e, 2000-n)
			}
		}
		// The youngest evicted one would not have fit: no more went than the cap needs.
		b, err := json.Marshal(ProjectIndex{ProjectKey: "/p", Entries: append(got.Entries, idx.Entries[n-1])})
		if err != nil || len(b)+len("\n") <= MaxIndexBytes {
			t.Errorf("with %d evicted, one more entry would still fit in MaxIndexBytes", n)
		}
		if len(idx.Entries) != 2000 {
			t.Errorf("WriteIndex changed the index it was given: %d entries", len(idx.Entries))
		}
	})
	t.Run("at_the_cap_nothing_goes_and_a_byte_over_one_does", func(t *testing.T) {
		// Two entries, the second's family padded so that the file is
		// exactly MaxIndexBytes, then one byte more: the older entry goes.
		pair := func(pad int) *ProjectIndex {
			return &ProjectIndex{ProjectKey: "/p", Entries: []IndexEntry{
				{Family: "a", Kind: kindTest, Sessions: 2, LastSession: "s1", LastSeen: idxNow - 1},
				{Family: "b" + strings.Repeat("x", pad), Kind: kindTest, Sessions: 2, LastSession: "s1", LastSeen: idxNow},
			}}
		}
		b, err := json.Marshal(pair(0))
		if err != nil {
			t.Fatal(err)
		}
		pad := MaxIndexBytes - len(b) - len("\n")
		for _, c := range []struct {
			pad, evicted int
			kept         string
		}{{pad, 0, "ab"}, {pad + 1, 1, "b"}} {
			path := IndexPath(t.TempDir(), "/p")
			n, err := WriteIndex(path, pair(c.pad))
			got, rerr := ReadIndex(path)
			if err != nil || rerr != nil || got == nil || n != c.evicted {
				t.Fatalf("a file of MaxIndexBytes%+d: WriteIndex = %d, %v; ReadIndex %v; want %d evicted",
					c.pad-pad, n, err, rerr, c.evicted)
			}
			kept := ""
			for _, e := range got.Entries {
				kept += e.Family[:1]
			}
			if kept != c.kept {
				t.Errorf("a file of MaxIndexBytes%+d kept %q, want %q", c.pad-pad, kept, c.kept)
			}
		}
	})
	t.Run("between_equals_the_first_family_goes", func(t *testing.T) {
		big := func(c string, seen int64) IndexEntry {
			return IndexEntry{Family: strings.Repeat(c, 30000), Kind: kindTest, Sessions: 2, LastSession: "s1", LastSeen: seen}
		}
		for _, c := range []struct {
			name string
			in   []IndexEntry
			want []string // the families kept, by first letter
		}{
			{"seen_together", []IndexEntry{big("c", idxNow), big("a", idxNow), big("b", idxNow)}, []string{"b", "c"}},
			{"seen_apart", []IndexEntry{big("c", idxNow-2), big("a", idxNow), big("b", idxNow-1)}, []string{"a", "b"}},
		} {
			path := IndexPath(t.TempDir(), "/p")
			n, err := WriteIndex(path, &ProjectIndex{ProjectKey: "/p", Entries: c.in})
			got, rerr := ReadIndex(path)
			if err != nil || rerr != nil || n != 1 || got == nil {
				t.Fatalf("%s: WriteIndex = %d, %v; ReadIndex %v; want one evicted", c.name, n, err, rerr)
			}
			var kept []string
			for _, e := range got.Entries {
				kept = append(kept, e.Family[:1])
			}
			if !slices.Equal(kept, c.want) {
				t.Errorf("%s: kept %v, want %v", c.name, kept, c.want)
			}
		}
	})
	t.Run("a_key_over_the_cap_writes_nothing", func(t *testing.T) {
		path := IndexPath(t.TempDir(), "/p")
		idx := &ProjectIndex{ProjectKey: strings.Repeat("k", MaxIndexBytes),
			Entries: []IndexEntry{{Family: "r1@l1", Kind: kindTest, Sessions: 1, LastSession: "s1"}}}
		if _, err := WriteIndex(path, idx); err == nil {
			t.Error("WriteIndex of a key over MaxIndexBytes: nil error")
		}
		if _, err := os.Stat(path); !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("a refused write left %s (%v)", path, err)
		}
	})
	t.Run("what_it_wrote_reads_back", func(t *testing.T) {
		runs := []gateRun{idxRun(1, "r1", "l1", kindTest, true), idxRun(2, "r2", "l2", kindBuild, false)}
		idx := &ProjectIndex{ProjectKey: "/p"}
		idx.Observe(runs, "s1", idxNow-dayMS)
		idx.Observe(runs[:1], "s2", idxNow)
		path := IndexPath(t.TempDir(), "/p")
		if _, err := WriteIndex(path, idx); err != nil {
			t.Fatal(err)
		}
		got, err := ReadIndex(path)
		want := []IndexEntry{
			{Family: "r1@l1", Kind: kindTest, Sessions: 2, LastSession: "s2", FirstSeen: idxNow - dayMS, LastSeen: idxNow},
			{Family: "r2@l2", Kind: kindBuild, Sessions: 1, LastSession: "s1", FirstSeen: idxNow - dayMS, LastSeen: idxNow - dayMS},
		}
		if err != nil || got == nil || got.ProjectKey != "/p" || !slices.Equal(got.Entries, want) {
			t.Errorf("ReadIndex = %+v, %v; want /p with %+v", got, err, want)
		}
	})
}

// wantMode fails the test unless path's permission bits are perm.
func wantMode(t *testing.T, path string, perm fs.FileMode) {
	t.Helper()
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm() != perm {
		t.Errorf("%s is %v, want %v", path, fi.Mode().Perm(), perm)
	}
}

// TestIDX5_ReadIndexAbsentMalformedAndOverCap (PF-PG1): no file is no index and
// no error; a file over MaxIndexBytes, or not the index's closed shape, is an
// error, which the caller reads as no index (every family required). A file
// of exactly MaxIndexBytes reads.
func TestIDX5_ReadIndexAbsentMalformedAndOverCap(t *testing.T) {
	dir := t.TempDir()
	if idx, err := ReadIndex(filepath.Join(dir, "absent.json")); idx != nil || err != nil {
		t.Errorf("ReadIndex of no file = %+v, %v; want nil, nil", idx, err)
	}
	padded := func(n int) string {
		s := `{"project_key":"/p","entries":[]}`
		return s + strings.Repeat(" ", n-len(s))
	}
	for _, c := range []struct{ name, body string }{
		{"unterminated", "{"},
		{"over_the_cap", padded(MaxIndexBytes + 1)},
		{"an_unknown_field", `{"project_key":"/p","entries":[],"extra":1}`},
		{"a_second_value", `{"project_key":"/p","entries":[]} {}`},
		{"out_of_order", `{"project_key":"/p","entries":[{"family":"b"},{"family":"a"}]}`},
		{"a_family_twice", `{"project_key":"/p","entries":[{"family":"a"},{"family":"a"}]}`},
	} {
		path := filepath.Join(dir, c.name+".json")
		if err := os.WriteFile(path, []byte(c.body), 0o600); err != nil {
			t.Fatal(err)
		}
		if idx, err := ReadIndex(path); idx != nil || err == nil {
			t.Errorf("%s: ReadIndex = %+v, %v; want nil and an error", c.name, idx, err)
		}
	}
	if idx, err := ReadIndex(dir); idx != nil || err == nil {
		t.Errorf("ReadIndex of a directory = %+v, %v; want nil and an error", idx, err)
	}
	path := filepath.Join(dir, "at_the_cap.json")
	if err := os.WriteFile(path, []byte(padded(MaxIndexBytes)), 0o600); err != nil {
		t.Fatal(err)
	}
	if idx, err := ReadIndex(path); err != nil || idx == nil || idx.ProjectKey != "/p" {
		t.Errorf("ReadIndex of exactly MaxIndexBytes = %+v, %v; want /p", idx, err)
	}
}

// TestIDX6_EntriesSortedAndOrderIndependent (PF-PG1): Observe over 100 shuffles
// of one session's runs, into an index that already holds entries, leaves
// them sorted by Family and writes byte-identical files; WriteIndex sorts an
// index built out of order, without changing it; an empty index writes
// "entries":[], never null.
func TestIDX6_EntriesSortedAndOrderIndependent(t *testing.T) {
	runs := []gateRun{
		idxRun(1, "r3", "l1", kindTest, true), idxRun(2, "r1", "l2", kindBuild, true),
		idxRun(3, "r2", "l1", kindCustom, false), idxRun(4, "r0", "l9", kindTest, false),
		// One family, two inferred kinds: its kind is the later run's.
		idxRun(5, "r4", "l1", kindBuild, false), idxRun(6, "r4", "l1", kindCustom, false),
		idxRun(7, "r5", "l3", kindTest, true),
	}
	held := func() *ProjectIndex {
		return &ProjectIndex{ProjectKey: "/p", Entries: []IndexEntry{
			{Family: "r1@l2", Kind: kindBuild, Sessions: 1, LastSession: "s0", FirstSeen: idxNow - dayMS, LastSeen: idxNow - dayMS},
			{Family: "r9@l9", Kind: kindTest, Sessions: 3, LastSession: "s0", FirstSeen: idxNow - 9*dayMS, LastSeen: idxNow - dayMS},
		}}
	}
	dir := t.TempDir()
	var want []byte
	rng := rand.New(rand.NewSource(1))
	for i := range 100 {
		rng.Shuffle(len(runs), func(i, j int) { runs[i], runs[j] = runs[j], runs[i] })
		idx := held()
		idx.Observe(runs, "s1", idxNow)
		if !slices.IsSortedFunc(idx.Entries, func(a, b IndexEntry) int { return strings.Compare(a.Family, b.Family) }) {
			t.Fatalf("shuffle %d: Observe left the entries out of order: %+v", i, idx.Entries)
		}
		path := filepath.Join(dir, strconv.Itoa(i)+".json")
		if _, err := WriteIndex(path, idx); err != nil {
			t.Fatal(err)
		}
		got, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if i == 0 {
			want = got
		} else if !bytes.Equal(got, want) {
			t.Fatalf("shuffle %d wrote %s, want %s", i, got, want)
		}
	}
	if got, err := ReadIndex(filepath.Join(dir, "0.json")); err != nil || got == nil || len(got.Entries) != 7 {
		t.Fatalf("the shuffles' index = %+v, %v; want seven entries", got, err)
	}
	reversed := held()
	slices.Reverse(reversed.Entries)
	path := filepath.Join(dir, "reversed.json")
	if _, err := WriteIndex(path, reversed); err != nil {
		t.Fatal(err)
	}
	if got, err := ReadIndex(path); err != nil || got == nil || !slices.Equal(got.Entries, held().Entries) {
		t.Errorf("an index written out of order reads back as %+v, %v; want it sorted", got, err)
	}
	if reversed.Entries[0].Family != "r9@l9" {
		t.Errorf("WriteIndex reordered the index it was given: %+v", reversed.Entries)
	}
	path = filepath.Join(dir, "empty.json")
	if _, err := WriteIndex(path, &ProjectIndex{ProjectKey: "/p"}); err != nil {
		t.Fatal(err)
	}
	if got, err := os.ReadFile(path); err != nil || string(got) != `{"project_key":"/p","entries":[]}`+"\n" {
		t.Errorf("an empty index writes %q (%v), want entries []", got, err)
	}
}

// TestIDX7_KindIsTheFamilys (PF-PG1): an entry's kind is its family's as
// kindOf reads it over the runs in Seq order, whatever order they arrive in:
// a recorded kind outranks a later inferred one, and with none recorded the
// last run's inferred kind is the family's. It is the kind gateStates gives
// the family's gate, and it replaces the kind an earlier session left (D33).
func TestIDX7_KindIsTheFamilys(t *testing.T) {
	runs := []gateRun{
		idxRun(4, "r2", "l1", kindCustom, false), idxRun(3, "r2", "l1", kindBuild, false),
		idxRun(2, "r1", "l1", kindCustom, false), idxRun(1, "r1", "l1", kindTest, true),
	}
	idx := &ProjectIndex{ProjectKey: "/p", Entries: []IndexEntry{
		{Family: "r1@l1", Kind: kindCustom, Sessions: 1, LastSession: "s0", FirstSeen: idxNow - dayMS, LastSeen: idxNow - dayMS},
	}}
	idx.Observe(runs, "s1", idxNow)
	want := map[string]string{"r1@l1": kindTest, "r2@l1": kindCustom}
	gs := gateStates(runs, nil, nil, 4, nil)
	if len(gs) != 2 {
		t.Fatalf("gateStates gives %d families, want 2", len(gs))
	}
	for _, g := range gs {
		if es := entriesOf(idx, g.Family); len(es) != 1 || es[0].Kind != want[g.Family] || es[0].Kind != g.Kind {
			t.Errorf("%s: entries %+v, want one of kind %s (the gate's is %s)", g.Family, es, want[g.Family], g.Kind)
		}
	}
}

// TestIDX8_NoFamilyRunsAreSkipped (PF-PG1): a run with no runner, or with no
// location, has no family (gateRun.family) and adds no entry.
func TestIDX8_NoFamilyRunsAreSkipped(t *testing.T) {
	noRunner := gateRun{Seq: 1, Location: "l1", Kind: kindTest}
	noLocation := idxRun(2, "r1", "", kindTest, true)
	idx := &ProjectIndex{ProjectKey: "/p"}
	idx.Observe([]gateRun{noRunner, noLocation, idxRun(3, "r2", "l2", kindTest, true)}, "s1", idxNow)
	if len(idx.Entries) != 1 || idx.Entries[0].Family != "r2@l2" {
		t.Errorf("entries %+v, want r2@l2 alone", idx.Entries)
	}
}

// TestIDX9_IndexPathIsSanitised (PF-PG1): a project key, an absolute path with
// separators (baseline.Key), names one file directly under verify/projects,
// named as the baseline names its files; two keys equal in their last 80
// characters differ by the digest suffix.
func TestIDX9_IndexPathIsSanitised(t *testing.T) {
	root := t.TempDir()
	path := IndexPath(root, "/Users/k/src/my project/a:b")
	if dir := filepath.Join(root, "verify", "projects"); filepath.Dir(path) != dir {
		t.Errorf("IndexPath = %s, not one element under %s", path, dir)
	}
	if name := filepath.Base(path); !baseline.IsFileName(name) {
		t.Errorf("%s is not named as a baseline file is (sanitised key, digest, one .json)", name)
	}
	tail := strings.Repeat("x", 100)
	a, b := filepath.Base(IndexPath(root, "/a/"+tail)), filepath.Base(IndexPath(root, "/b/"+tail))
	if a == b {
		t.Errorf("two keys equal in their last 80 characters share %s", a)
	}
	if strings.LastIndex(a, "-") < 0 || a[:strings.LastIndex(a, "-")] != b[:strings.LastIndex(b, "-")] {
		t.Errorf("%s and %s differ before the digest, so this row does not isolate it", a, b)
	}
}

// BenchmarkReadIndex is ReadIndex of an index at its cap, the read post (2.2)
// makes once per PostToolUse call. Its entries are shaped as real ones are
// (64-hex runner and location digests, a UUID-length session, millisecond
// times), as many as WriteIndex keeps under MaxIndexBytes, reported as
// entries. Top-level, as BenchmarkGateStates is: a sub-benchmark's suffix
// reads as a secret to TestH113's entropy check over budgets.json.
func BenchmarkReadIndex(b *testing.B) {
	idx := &ProjectIndex{ProjectKey: "/Users/dev/src/project"}
	for i := range 400 {
		idx.Entries = append(idx.Entries, IndexEntry{Family: fmt.Sprintf("%064x@%064x", i, i+1<<20),
			Kind: kindTest, Sessions: 2 + i%7, LastSession: fmt.Sprintf("%08x-0000-4000-8000-%012x", i, i),
			FirstSeen: idxNow - 40*dayMS + int64(i), LastSeen: idxNow - int64(i)})
	}
	path := IndexPath(b.TempDir(), idx.ProjectKey)
	evicted, err := WriteIndex(path, idx)
	if err != nil || evicted == 0 {
		b.Fatalf("WriteIndex of 400 entries = %d, %v; want some evicted, nil", evicted, err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		if _, err := ReadIndex(path); err != nil {
			b.Fatal(err)
		}
	}
	// After the loop: ResetTimer deletes reported metrics.
	b.ReportMetric(float64(len(idx.Entries)-evicted), "entries")
}
