package receipt

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/altrace-dev-role/rashomon/internal/store"
)

// ckpt is a checkpoint line for project cut by session at ms, with its gates
// given as the JSON array's body, and the integrity entries and cut counts
// the test names.
type ckpt struct {
	project, session string
	ms, seq          int64
	green            bool
	gates            string
	gatesOmitted     int
	unresolved       int
	integrityOmitted int
}

func (c ckpt) line() string {
	entries := make([]string, c.unresolved)
	for i := range entries {
		entries[i] = `{"kind":"x"}`
	}
	return fmt.Sprintf(`{"type":"verify","schema_version":%d,"recorded_at_unix_ms":%d,"session_id":%q,"project_root":%q,`+
		`"point_seq":%d,"spec_digest":null,"spec_trust":null,"record_green":%t,"gates":[%s],"gates_omitted":%d,`+
		`"integrity":{"unresolved":[%s]},"integrity_omitted":%d}`,
		store.EventSchemaVersion, c.ms, c.session, c.project, c.seq, c.green, c.gates, c.gatesOmitted,
		strings.Join(entries, ","), c.integrityOmitted)
}

func gate(name string, required bool, state string) string {
	return fmt.Sprintf(`{"gate":%q,"required":%t,"state":%q}`, name, required, state)
}

func openStore(t *testing.T) *store.Store {
	t.Helper()
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return st
}

// TestRP2g_PreviousNamesTheProjectsLastSession (card REPORT1b R1): runs of
// projects A, B, A (newest last) and the current session as a fourth run in
// A, the clear case with a checkpoint of its own. Previous with the current
// session as Skip returns A's third run, not the current one; without the
// skip it would. TestRG1e's fixture run through Previous with only a project
// still returns sess-best: a run written later is read on, not taken first.
// Break: stop at the first match, or do not skip the current session.
func TestRP2g_PreviousNamesTheProjectsLastSession(t *testing.T) {
	st := openStore(t)
	const a, b = "/work/a", "/work/b"
	seedRun(t, st, "s1", 1_000_000, ckpt{project: a, session: "s1", ms: 900_000, seq: 3, green: true}.line())
	seedRun(t, st, "s2", 2_000_000, ckpt{project: b, session: "s2", ms: 1_900_000, seq: 4, green: true}.line())
	seedRun(t, st, "s3", 3_000_000, ckpt{project: a, session: "s3", ms: 2_900_000, seq: 5, green: false,
		gates: gate("go", true, "red")}.line())
	seedRun(t, st, "cur", 4_000_000, ckpt{project: a, session: "cur", ms: 3_900_000, seq: 6, green: true}.line())

	got, sc, err := Previous(st, Query{Project: a, Skip: "cur"})
	if err != nil || !sc.Found || got.SessionID != "s3" || got.PointSeq != 5 || got.NotGreen != 1 {
		t.Errorf("Previous = %+v %+v err %v; want s3 at #5 with 1 gate not green", got, sc, err)
	}
	if got, _, _ := Previous(st, Query{Project: a}); got.SessionID != "cur" {
		t.Errorf("without a skip the current session is the newest: got %q", got.SessionID)
	}

	rg := openStore(t)
	const p, q = "/work/p", "/work/q"
	seedRun(t, rg, "sess-other", 4_000_000, checkpoint(q, 3_900_000))
	seedRun(t, rg, "sess-later-write", 3_000_000, checkpoint(p, 1_500_000), `{"type":"prompt","schema_version":5}`)
	seedRun(t, rg, "sess-best", 2_000_000, checkpoint(p, 1_000_000), checkpoint(p, 1_900_000))
	seedRun(t, rg, "sess-skewed", 1_000_000, checkpoint(p, 2_500_000))
	if got, sc, err := Previous(rg, Query{Project: p}); err != nil || !sc.Found || got.RecordedAtUnixMS != 1_900_000 {
		t.Errorf("TestRG1e's fixture: %+v %+v err %v; want recorded 1_900_000", got, sc, err)
	}
}

// TestRP2h_PreviousIsBounded (card REPORT1b R1): 17 newer runs of another
// project give the runs bound; a records file of 20 MiB whose tail holds no
// checkpoint is read from its end only as far as the budget, 8 MiB in all,
// and gives the bytes bound; a run with Before set is read whole and
// answers with the newest record before it. Break: drop the 16-run bound,
// read the whole file at the fallback, or ignore Before.
func TestRP2h_PreviousIsBounded(t *testing.T) {
	st := openStore(t)
	const a, b = "/work/a", "/work/b"
	seedRun(t, st, "old", 1_000_000, ckpt{project: a, session: "old", ms: 900_000, seq: 1, green: true}.line())
	for i := range 17 {
		seedRun(t, st, fmt.Sprintf("b%02d", i), int64(2_000_000+i), ckpt{project: b, session: "b", ms: int64(1_900_000 + i), seq: 1}.line())
	}
	q := Query{Project: a, MaxRuns: MaxRuns, MaxBytes: MaxBytes}
	if got, sc, err := Previous(st, q); err != nil || sc.Found || sc.Bound != BoundRuns {
		t.Errorf("17 newer runs of B: %+v %+v err %v; want none, bound runs", got, sc, err)
	}
	if line := PreviousLine(Last{}, Scan{Bound: BoundRuns}, "ptr"); line != "last session: none in the 16 most recent sessions on this machine" {
		t.Errorf("the runs line: %q", line)
	}
	q.MaxRuns = 18
	if got, sc, err := Previous(st, q); err != nil || !sc.Found || got.SessionID != "old" {
		t.Errorf("18 runs reach A's: %+v %+v err %v", got, sc, err)
	}

	big := openStore(t)
	dir := big.RunDir("huge")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	f, err := os.Create(filepath.Join(dir, store.FileRecords))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString(ckpt{project: a, session: "huge", ms: 5, seq: 1, green: true}.line() + "\n"); err != nil {
		t.Fatal(err)
	}
	pad := `{"type":"declaration","schema_version":5,"pad":"` + strings.Repeat("x", 4000) + `"}` + "\n"
	for written := 0; written < 20<<20; written += len(pad) {
		if _, err := f.WriteString(pad); err != nil {
			t.Fatal(err)
		}
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	got, sc, err := Previous(big, Query{Project: a, MaxRuns: MaxRuns, MaxBytes: MaxBytes})
	if err != nil || sc.Found || sc.Bound != BoundBytes || sc.ReadBytes > MaxBytes || sc.ReadBytes < MaxBytes/2 {
		t.Errorf("a 20 MiB file with no checkpoint in its tail: %+v %+v err %v; want none, bound bytes, at most 8 MiB read", got, sc, err)
	}
	if line := PreviousLine(Last{}, sc, "ptr"); line != "last session: none within the 8 MiB read bound on this machine" {
		t.Errorf("the bytes line: %q", line)
	}
	if _, sc, err := Previous(big, Query{Project: a}); err != nil || !sc.Found {
		t.Errorf("unbounded, the first record is found: %+v err %v", sc, err)
	}

	bf := openStore(t)
	seedRun(t, bf, "s", 9_000_000, ckpt{project: a, session: "s", ms: 100, seq: 1}.line(), ckpt{project: a, session: "s", ms: 200, seq: 2}.line(),
		ckpt{project: a, session: "s", ms: 300, seq: 3}.line())
	if got, _, err := Previous(bf, Query{Project: a, Before: 250}); err != nil || got.PointSeq != 2 {
		t.Errorf("Before 250: %+v err %v; want the record at #2", got, err)
	}
}

// TestRP2i_StartupLineFitsAndReasonsAreRight (card REPORT1b R1): every form
// is at most 80 columns with its indent, under both pointers; a checkpoint
// whose only non-green gate is stale reads as a gate not green, never red;
// integrity unresolved with every gate green names it; no required gate names
// that; cut gates all stored non-green read "at least"; the pointer is on the
// found forms and not on the none forms. Break: read red only, or drop the
// width fallback.
func TestRP2i_StartupLineFitsAndReasonsAreRight(t *testing.T) {
	const plugin, cli = "/rashomon:report last", "rashomon report --session last"
	at := time.Date(2026, 10, 29, 16, 20, 0, 0, time.UTC).UnixMilli()
	cases := []struct {
		name string
		c    ckpt
		want string
	}{
		{"green", ckpt{green: true, gates: gate("go", true, "green")}, "record green"},
		{"stale", ckpt{gates: gate("go", true, "stale")}, "1 gate not green"},
		{"two", ckpt{gates: gate("go", true, "red") + "," + gate("npm", true, "stale")}, "2 gates not green"},
		{"optional red", ckpt{unresolved: 1, gates: gate("go", false, "red") + "," + gate("npm", true, "green")}, "integrity unresolved"},
		{"integrity cut", ckpt{integrityOmitted: 2, gates: gate("npm", true, "green")}, "integrity unresolved"},
		{"no gate", ckpt{}, "no required gate"},
		{"at least", ckpt{gatesOmitted: 3, gates: gate("go", true, "red") + "," + gate("npm", true, "red")}, "at least 2 gates not green"},
		{"cut with a green", ckpt{gatesOmitted: 3, gates: gate("go", true, "red") + "," + gate("npm", true, "green")}, "1 gate not green"},
	}
	for _, tc := range cases {
		tc.c.project, tc.c.session, tc.c.ms, tc.c.seq = "/p", "s", at, 9
		p, ok := Decode([]byte(tc.c.line()))
		if !ok {
			t.Fatalf("%s: line does not decode", tc.name)
		}
		for _, ptr := range []string{plugin, cli} {
			line := PreviousLine(p, Scan{Found: true}, ptr)
			if w := utf8.RuneCountInString("  " + line); w > 80 {
				t.Errorf("%s with %q: %d columns: %q", tc.name, ptr, w, line)
			}
			if !strings.HasSuffix(line, "; "+ptr) || !strings.HasPrefix(line, "last session: 10-29 16:20") {
				t.Errorf("%s: %q lacks the time or the pointer", tc.name, line)
			}
		}
		if got := PreviousLine(p, Scan{Found: true}, plugin); !strings.Contains(got, ", "+tc.want+";") {
			t.Errorf("%s: %q does not read %q", tc.name, got, tc.want)
		}
	}
	p, _ := Decode([]byte(ckpt{project: "/p", ms: at, gatesOmitted: 3, gates: gate("go", true, "red") + "," + gate("npm", true, "red")}.line()))
	if line := PreviousLine(p, Scan{Found: true}, cli); line != "last session: 10-29 16:20; "+cli {
		t.Errorf("a reason that does not fit is dropped, the pointer kept: %q", line)
	}
	if line := PreviousLine(Last{}, Scan{}, plugin); line != "" {
		t.Errorf("a walk that read every run and found none has no line of its own: %q", line)
	}
	for _, s := range []Scan{{Bound: BoundRuns}, {Bound: BoundBytes}} {
		if line := PreviousLine(Last{}, s, plugin); strings.Contains(line, "report") || !strings.HasPrefix(line, "last session: none") {
			t.Errorf("a none form %+v carries a pointer or no 'none': %q", s, line)
		}
	}
}

// TestRP2j_TheLineCarriesOneReason (card REPORT1b R1, RB12): two required
// gates not green and integrity unresolved print only the gates, by
// precedence, with no counts clause. Break: join the reasons.
func TestRP2j_TheLineCarriesOneReason(t *testing.T) {
	c := ckpt{project: "/p", ms: 1, seq: 1, unresolved: 1, gates: gate("go", true, "red") + "," + gate("npm", true, "red")}
	p, _ := Decode([]byte(c.line()))
	line := PreviousLine(p, Scan{Found: true}, "/rashomon:report last")
	if !strings.Contains(line, ", 2 gates not green; ") || strings.Contains(line, "integrity") {
		t.Errorf("%q: want only '2 gates not green'", line)
	}
}

// TestRP2z_TheWalkLooksAtAtMostMaxWalkDirectories (card REPORT1b, the
// startup cost): a store of more than MaxWalk run directories is not walked
// past MaxWalk, and a walk that found nothing says so ("walk") instead of
// claiming the project has no session; one of exactly MaxWalk, read whole,
// gives no bound. Which MaxWalk directories are looked at is the directory's
// order, not the newest, and the line says "the first". The line names the
// bound and carries no pointer. Break: lift the cap, or drop its flag.
func TestRP2z_TheWalkLooksAtAtMostMaxWalkDirectories(t *testing.T) {
	st := openStore(t)
	const a, b = "/work/a", "/work/b"
	for i := range MaxWalk {
		seedRun(t, st, fmt.Sprintf("r%04d", i), int64(1_000_000+i), ckpt{project: b, session: "b", ms: int64(900_000 + i), seq: 1}.line())
	}
	q := Query{Project: a, MaxRuns: 2 * MaxWalk, MaxBytes: MaxBytes, CapWalk: true}
	if got, sc, err := Previous(st, q); err != nil || sc.Found || sc.Bound != "" {
		t.Fatalf("exactly MaxWalk runs, all read: %+v %+v err %v; want none, no bound", got, sc, err)
	}
	seedRun(t, st, "extra", 3_000_000, ckpt{project: b, session: "b", ms: 2_900_000, seq: 1}.line())
	got, sc, err := Previous(st, q)
	if err != nil || sc.Found || sc.Bound != BoundWalk {
		t.Fatalf("one run past the cap: %+v %+v err %v; want none, bound walk", got, sc, err)
	}
	line := PreviousLine(got, sc, "ptr")
	if line != "last session: none among the first 1024 session directories on this machine" || len("  "+line) > 80 {
		t.Errorf("the walk line: %q", line)
	}
}
