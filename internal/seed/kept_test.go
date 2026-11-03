package seed

// SEED1-KEEP (PF-HD17's typed rows, PF-HD8 in the brief): the count of what
// the person corrected before AutoSteer, kept as the before of a before and
// after: bounded by the install time, per project, by stable ids.

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

// stamped is x with every line given a timestamp, at from and one second
// apart, and a cwd.
func stamped(t *testing.T, x *tx, from time.Time, cwd string) string {
	t.Helper()
	var out []string
	for i, l := range x.lines {
		ts := from.Add(time.Duration(i) * time.Second).UTC().Format(time.RFC3339Nano)
		out = append(out, strings.Replace(l, "{", `{"timestamp":"`+ts+`","cwd":"`+cwd+`",`, 1))
	}
	return strings.Join(out, "\n") + "\n"
}

// TestSEED2a_TheKeptCountIsBoundedPerProjectAndRoundTrips: a session before
// the boundary counts and one after it does not (the control is the same
// session before it); each project keeps its own counts; the record reads
// back as written; every kind and class id parses back to itself.
func TestSEED2a_TheKeptCountIsBoundedPerProjectAndRoundTrips(t *testing.T) {
	install := time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)
	tl := New()
	tl.Until = install
	tl.Key = func(cwd string) string { return "key:" + cwd }
	for _, f := range []struct {
		x    *tx
		from time.Time
		cwd  string
	}{
		{sessionA(t), install.Add(-time.Hour), "/w/a"},
		{sessionB(t), install.Add(-2 * time.Hour), "/w/b"},
		// The same session written after the boundary: none of it counts.
		{sessionB(t), install.Add(time.Minute), "/w/b"},
	} {
		if err := tl.Read(strings.NewReader(stamped(t, f.x, f.from, f.cwd)), "p", false); err != nil {
			t.Fatal(err)
		}
	}
	// The same session with no timestamps: it cannot be shown to be before.
	if err := tl.Read(strings.NewReader(strings.Join(sessionB(t).lines, "\n")+"\n"), "p", false); err != nil {
		t.Fatal(err)
	}
	s := tl.Summary()
	if s.Sessions != 2 || s.PushedBack != 3 || s.Rejected != 2 || s.FailedAgain != 2 {
		t.Errorf("summary %+v, want the after-boundary and unstamped copies left out: 2 sessions, 3 pushed back, 2 rejected, 2 failed again", s)
	}
	k := tl.Kept(install.Add(time.Hour), install.Add(-14*24*time.Hour), install, 14)
	if k.Version != 1 || k.Days != 14 || !k.Until.Equal(install) || k.All.Sessions != 2 {
		t.Errorf("kept header %+v", k)
	}
	a, b := k.Projects["key:/w/a"], k.Projects["key:/w/b"]
	if len(k.Projects) != 2 || a.Sessions != 1 || b.Sessions != 1 || a.Kinds["pushed-back"] != 2 || b.Kinds["pushed-back"] != 1 {
		t.Errorf("projects %+v, want /w/a with 2 pushed back and /w/b with 1, one session each", k.Projects)
	}
	if got := b.Rows; len(got) != 3 || got[0] != (KeptRow{Kind: "failed-again", Class: "edit-not-found", Times: 1, Sessions: 1}) {
		t.Errorf("project b rows %+v, want its three, the edit retry first by text", got)
	}
	root := t.TempDir()
	if err := WriteKept(root, k); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(filepath.Join(root, KeptFile))
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("kept file %v, mode %v; want 0600", err, info)
	}
	back, ok, err := ReadKept(root)
	if err != nil || !ok || !reflect.DeepEqual(back, k) {
		t.Errorf("read back %+v ok %v err %v\nwant %+v", back, ok, err, k)
	}
	for c := class(0); c < numClasses; c++ {
		if p, ok := parseClass(c.id()); !ok || p != c {
			t.Errorf("class %d id %q parses to %d %v", c, c.id(), p, ok)
		}
	}
	for g := kind(0); g < numKinds; g++ {
		if p, ok := parseKind(g.id()); !ok || p != g {
			t.Errorf("kind %d id %q parses to %d %v", g, g.id(), p, ok)
		}
	}
}

// TestSEED2b_ReadKeptRefusesWhatItDidNotWrite: no file is no record and no
// error; a file past KeptMaxBytes, of another version, or not JSON is an
// error and no record. The oversize file is a good record and blank space
// past the cap, which only the cap refuses.
func TestSEED2b_ReadKeptRefusesWhatItDidNotWrite(t *testing.T) {
	root := t.TempDir()
	if _, ok, err := ReadKept(root); ok || err != nil {
		t.Errorf("no file: ok %v err %v, want none and no error", ok, err)
	}
	for name, body := range map[string]string{
		"oversize": `{"seed_kept":1,"days":14}` + strings.Repeat(" ", KeptMaxBytes),
		"version":  `{"seed_kept":2,"days":14}`,
		"garbage":  `not json`,
	} {
		if err := os.WriteFile(filepath.Join(root, KeptFile), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, ok, err := ReadKept(root); ok || err == nil {
			t.Errorf("%s: ok %v err %v, want an error and no record", name, ok, err)
		}
	}
}

// TestSEED2e_HistoryLines: the brief's words for a kept record, this
// project's count and its most-often row, the machine's row when the project
// has none that recurs. The brief
// prints a fact's second line whole (a command cut short runs nothing), so
// the row line fits by construction: every signature's, for every kind and
// class, is within historyNextMax.
func TestSEED2e_HistoryLines(t *testing.T) {
	k := Kept{Version: 1, Days: 14,
		All: Counts{Sessions: 9, Kinds: map[string]int{"interrupted": 4, "rejected": 3, "pushed-back": 2, "blocked": 7},
			Rows: []KeptRow{{"blocked", "shell", 7, 4}, {"rejected", "vcs", 3, 3}}},
		Projects: map[string]Counts{
			"/w/a": {Sessions: 4, Kinds: map[string]int{"rejected": 3, "pushed-back": 1}, Rows: []KeptRow{{"rejected", "vcs", 3, 3}}},
			"/w/b": {Sessions: 1, Kinds: map[string]int{"interrupted": 1}, Rows: []KeptRow{{"interrupted", "reply", 1, 1}}},
		}}
	for _, c := range []struct{ key, text, next string }{
		{"/w/a", "9 corrections in the 14 days before AutoSteer, 4 here",
			"most here: you said no to a version-control command"},
		{"/w/b", "9 corrections in the 14 days before AutoSteer, 1 here",
			"most: your rules blocked a shell command"},
		{"/w/c", "9 corrections in the 14 days before AutoSteer",
			"most: your rules blocked a shell command"},
	} {
		text, next := History(k, c.key)
		if text != c.text || next != c.next {
			t.Errorf("History(%s) =\n %q\n %q\nwant\n %q\n %q", c.key, text, next, c.text, c.next)
		}
	}
	for k := kind(0); k < numKinds; k++ {
		for c := class(0); c < numClasses; c++ {
			if n := utf8.RuneCountInString("most here: " + rowText(k, c)); n > historyNextMax {
				t.Errorf("history line for %q is %d columns, over %d", rowText(k, c), n, historyNextMax)
			}
		}
	}
}
