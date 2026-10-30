package learn

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// The approved rows the delivery tests save, each entered at its own time so
// the order Lessons gives is the order written here.
var (
	provenCwd    = Lesson{ID: "go-test-cwd", Tier: TierProven, Text: "run go test from the module root", SinceUnixMS: 1_761_700_001_000}
	probationNpm = Lesson{ID: "npm-ci", Tier: TierProbation, Text: "npm ci before npm test", SinceUnixMS: 1_761_700_002_000}
)

// lessonsAt saves rows into project's ledger under root at the tiers they
// name, with no labels to move them, so active.json holds every row that is
// not proposed (Save's rule).
func lessonsAt(t *testing.T, root string, rows ...Lesson) *Ledger {
	t.Helper()
	l := open(t, root, t0)
	l.file.Lessons = append(l.file.Lessons, rows...)
	if err := l.Save(); err != nil {
		t.Fatal(err)
	}
	return l
}

func readActive(t *testing.T, root string) Active {
	t.Helper()
	a, err := ReadActive(root, project)
	if err != nil {
		t.Fatal(err)
	}
	return a
}

func lessonIDs(ls []Lesson) string {
	var s []string
	for _, l := range ls {
		s = append(s, l.ID+"="+string(l.Tier))
	}
	return strings.Join(s, " ")
}

// TestLRN17k_SaveBetweenReadsIsNotTamper: Save replaces lessons.json and
// active.json and then state.sum, so a prompt that reads between the two
// sees files the old sum does not sign, with no outside write made.
// ReadActive runs the whole read once more on a mismatch: a Save landing
// inside the first pass is read whole on the second, and its lessons
// arrive. A write the ledger did not make, seen on both passes, is still the
// signature case.
func TestLRN17k_SaveBetweenReadsIsNotTamper(t *testing.T) {
	root := t.TempDir()
	l := lessonsAt(t, root, probationNpm)
	saves := 0
	betweenReads = func() {
		if saves > 0 {
			return
		}
		saves++
		l.file.Lessons = append(l.file.Lessons, provenCwd)
		if err := l.Save(); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() { betweenReads = nil })
	if a := readActive(t, root); a.Unchecked != "" || lessonIDs(a.Lessons) != "go-test-cwd=proven npm-ci=probation" {
		t.Errorf("a Save inside the first read: unchecked %q, lessons %q; want the saved state read whole, both lessons", a.Unchecked, lessonIDs(a.Lessons))
	}
	if saves != 1 {
		t.Fatalf("premise: the Save ran %d times between the reads, want once", saves)
	}

	pad := 0
	betweenReads = func() {
		pad++
		path := filepath.Join(root, DirLearn, project, FileActive)
		b, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, append(bytes.TrimRight(b, "\n "), []byte(strings.Repeat(" ", pad)+"\n")...), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if a := readActive(t, root); a.Unchecked != UncheckedSignature || len(a.Lessons) != 0 {
		t.Errorf("an outside write on every read: unchecked %q, lessons %q; want the signature case and none", a.Unchecked, lessonIDs(a.Lessons))
	}
	if pad != 2 {
		t.Errorf("the state was read %d times, want twice (one retry)", pad)
	}
}

// TestLRN17l_OverCapSaysSizeNotSignature: the prompt path hashes
// lessons.json and labels.ndjson under 4 MiB each (L11). A correctly signed
// labels.ndjson one byte over is the size case, never the signature case,
// and gives no lesson; at the cap exactly it is read and its lessons arrive.
// Each case has its own sentence.
func TestLRN17l_OverCapSaysSizeNotSignature(t *testing.T) {
	for _, tc := range []struct {
		name      string
		size      int
		unchecked Unchecked
		lessons   string
	}{
		{"at the cap", StateMaxBytes, "", "npm-ci=probation"},
		{"one byte over", StateMaxBytes + 1, UncheckedSize, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			l := lessonsAt(t, root, probationNpm)
			labels := append(bytes.Repeat([]byte("x"), tc.size-1), '\n') // one line no label parses from
			if err := os.WriteFile(filepath.Join(l.dir, FileLabels), labels, 0o600); err != nil {
				t.Fatal(err)
			}
			if err := l.Save(); err != nil { // signs the padded labels file
				t.Fatal(err)
			}
			if l = open(t, root, t1); l.Quarantined {
				t.Fatal("premise: the padded state does not open clean")
			}
			if a := readActive(t, root); a.Unchecked != tc.unchecked || lessonIDs(a.Lessons) != tc.lessons {
				t.Errorf("labels.ndjson at %d bytes: unchecked %q, lessons %q; want %q, %q", tc.size, a.Unchecked, lessonIDs(a.Lessons), tc.unchecked, tc.lessons)
			}
		})
	}
	const (
		signature = "rashomon record: this project's lesson files fail their signature check, so no lesson was given to the agent; rashomon learn lists them."
		size      = "rashomon record: this project's lesson files are larger than the 4 MiB the prompt path reads, so no lesson was given to the agent; rashomon learn lists them."
	)
	for u, want := range map[Unchecked]string{UncheckedSignature: signature, UncheckedSize: size} {
		m, err := UncheckedMessage(u)
		if err != nil || m.Text() != want {
			t.Errorf("UncheckedMessage(%q) = %q, %v; want %q", u, m.Text(), err, want)
		}
	}
}

// TestLRN17j_ApprovalStampsItsTime: Approve stamps the approval's time, in
// milliseconds, beside the tier it gives today; Save and Open keep it, in
// lessons.json and in active.json's row. A quarantine sends the tier back to
// proposed and keeps the stamp. A lessons.json written before the stamp
// existed opens with it 0 and approves as before.
func TestLRN17j_ApprovalStampsItsTime(t *testing.T) {
	root := t.TempDir()
	l := open(t, root, t0)
	importText(t, l, twoLessons)
	at := t1.Add(1234 * time.Millisecond)
	if err := l.Approve("npm-ci", at); err != nil {
		t.Fatal(err)
	}
	if err := l.Save(); err != nil {
		t.Fatal(err)
	}
	stamp := func(ls []Lesson, id string) (Tier, int64) {
		for _, x := range ls {
			if x.ID == id {
				return x.Tier, x.ApprovedUnixMS
			}
		}
		return "", -1
	}
	l = open(t, root, t1)
	if tier, ms := stamp(l.Lessons(), "npm-ci"); tier != TierProbation || ms != at.UnixMilli() {
		t.Errorf("lessons.json after Open: npm-ci %s at %d, want probation at %d", tier, ms, at.UnixMilli())
	}
	if tier, ms := stamp(l.Lessons(), "go-test-cwd"); tier != TierProposed || ms != 0 {
		t.Errorf("the lesson not approved: %s at %d, want proposed with no stamp", tier, ms)
	}
	if tier, ms := stamp(activeOf(t, root).Lessons, "npm-ci"); tier != TierProbation || ms != at.UnixMilli() {
		t.Errorf("active.json's row: npm-ci %s at %d, want probation at %d", tier, ms, at.UnixMilli())
	}

	if err := edit(filepath.Join(l.dir, FileActive), `"quarantined":false`, `"quarantined":false `); err != nil {
		t.Fatal(err)
	}
	l = open(t, root, t1.Add(time.Hour))
	if tier, ms := stamp(l.Lessons(), "npm-ci"); !l.Quarantined || tier != TierProposed || ms != at.UnixMilli() {
		t.Errorf("after a quarantine: quarantined %v, npm-ci %s at %d; want proposed, the stamp kept", l.Quarantined, tier, ms)
	}

	old := t.TempDir()
	lo := open(t, old, t0)
	importText(t, lo, twoLessons)
	if b, err := os.ReadFile(filepath.Join(lo.dir, FileLessons)); err != nil || bytes.Contains(b, []byte("approved_unix_ms")) {
		t.Fatalf("premise: a ledger with nothing approved writes no stamp key (%v): %s", err, b)
	}
	lo = open(t, old, t1)
	if tier, ms := stamp(lo.Lessons(), "npm-ci"); lo.Quarantined || tier != TierProposed || ms != 0 {
		t.Errorf("a lessons.json with no stamp: quarantined %v, npm-ci %s at %d; want proposed, 0", lo.Quarantined, tier, ms)
	}
	if err := lo.Approve("npm-ci", t1); err != nil {
		t.Errorf("approving in a ledger with no stamp: %v", err)
	}
}
