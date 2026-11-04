package acceptance

// PW1b-ON-230 step 2: the Stop reads the platform graph (PF-PL8 on the Stop,
// WED-SLICE row 2). Until now only `report --ledger`, which a person runs,
// read .pgraph; the Stop owed no platform row, so a session that edited a
// package and never ran its tests ended with nothing said. The Stop now
// names, on the person's channel only, each changed package whose go test
// has not run since the edit, and reads the log only when the walk file
// vouches for it, so a Stop that races the background sync 1b starts reads
// unknown and says nothing, never a torn pair.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/altrace-dev-role/rashomon/internal/disclose"
)

// pw1b2Files is h156Files with two more packages, each with a test file,
// so a session can change four.
var pw1b2Files = func() map[string]string {
	m := map[string]string{
		"c/c.go":      "package c\n\nfunc C() int { return 3 }\n",
		"c/c_test.go": "package c\n",
		"d/d.go":      "package d\n\nfunc D() int { return 4 }\n",
		"d/d_test.go": "package d\n",
		"b/b_test.go": "package b\n",
	}
	for k, v := range h156Files {
		m[k] = v
	}
	return m
}()

// pw1b2Edit is an Edit of dir's file that leaves its bytes as they are.
func pw1b2Edit(dir, file string) map[string]any {
	return map[string]any{"file_path": filepath.Join(dir, filepath.FromSlash(file)), "old_string": "package", "new_string": "package"}
}

// pw1b2Lines is the Stop's line split, each checked against 80 columns.
func pw1b2Lines(t *testing.T, msg string) []string {
	t.Helper()
	lines := strings.Split(strings.TrimRight(msg, "\n"), "\n")
	for _, l := range lines {
		if n := utf8.RuneCountInString(l); n > 80 {
			t.Errorf("a Stop line is %d columns: %q", n, l)
		}
	}
	return lines
}

// TestPW1b2_TheStopNamesAChangedPackageWhoseTestsDidNotRun: after a sync,
// an edit of a/a.go and no go test, the Stop's line names the command that
// owes package a, the WED-SLICE row 2 line fitted to 80 columns with the
// Stop's indent ("internal/recap changed (platform): go test
// ./internal/recap/... not run since the edit" is 84 alone; the command
// names the package). Break: leave the Stop's Platform unset, or render no
// platform row, and this fails.
func TestPW1b2_TheStopNamesAChangedPackageWhoseTestsDidNotRun(t *testing.T) {
	e := newSteerEnv(t)
	dir := gi1Project(t, h156Files)
	h156Sync(t, dir)
	e.dghCall(dir, "toolu_e", "Edit", pw1b2Edit(dir, "a/a.go"))
	got, _ := e.recapLine(stopEvent("Stop", "I changed a/a.go.", dir), stAgent, "CI=")
	t.Logf("Stop line:\n%s", got)
	lines := pw1b2Lines(t, got)
	want := "platform: go test ./a/... not run since the edit"
	n := 0
	for _, l := range lines {
		if strings.TrimSpace(l) == want {
			n++
		}
	}
	if n != 1 {
		t.Errorf("the Stop names package a %d times, want once as %q", n, want)
	}
}

// TestPW1b2_ATestRunAfterTheEditSilencesTheLine: the same session with
// go test ./a/... after the edit owes package a nothing, and the Stop's
// line, which still speaks, names no platform row. Negative twin of
// TestPW1b2_TheStopNames. Break: print every platform row, met included.
func TestPW1b2_ATestRunAfterTheEditSilencesTheLine(t *testing.T) {
	e := newSteerEnv(t)
	dir := gi1Project(t, h156Files)
	h156Sync(t, dir)
	e.dghCall(dir, "toolu_e", "Edit", pw1b2Edit(dir, "a/a.go"))
	e.dghCall(dir, "toolu_g", "Bash", map[string]any{"command": "go test ./a/..."})
	got, _ := e.recapLine(stopEvent("Stop", "I changed a/a.go and ran its tests.", dir), stAgent, "CI=")
	t.Logf("Stop line:\n%s", got)
	if got == "" {
		t.Fatal("premise: the Stop printed nothing")
	}
	if strings.Contains(got, "platform: go test") {
		t.Errorf("a package whose tests ran after the edit is named:\n%s", got)
	}
}

// TestPW1b2_TheStopNamesTwoAndPointsToTheLedger: four changed packages
// with no test run: two lines, then one naming how many more and where to
// read them. Break: print every row, and a session that touched forty
// packages floods the person's screen.
func TestPW1b2_TheStopNamesTwoAndPointsToTheLedger(t *testing.T) {
	e := newSteerEnv(t)
	dir := gi1Project(t, pw1b2Files)
	h156Sync(t, dir)
	for i, f := range []string{"a/a.go", "b/b.go", "c/c.go", "d/d.go"} {
		e.dghCall(dir, "toolu_e"+string(rune('0'+i)), "Edit", pw1b2Edit(dir, f))
	}
	got, _ := e.recapLine(stopEvent("Stop", "I changed four packages.", dir), stAgent, "CI=")
	t.Logf("Stop line:\n%s", got)
	named := 0
	more := false
	for _, l := range pw1b2Lines(t, got) {
		if strings.Contains(l, "platform: go test ./") {
			named++
		}
		if strings.TrimSpace(l) == "and 2 more: rashomon report --ledger" {
			more = true
		}
	}
	if named != 2 || !more {
		t.Errorf("named %d packages, the rest pointed to: %v; want 2 and \"and 2 more: rashomon report --ledger\"", named, more)
	}
}

// TestPW1b2_ATornPairReadsUnknownAndSaysNothing: a log or digests file the
// walk file does not vouch for (a Stop that read while a sync was between
// its writes), or a walk file that vouches for no pair (the last sync was
// refused), reads unknown: the Stop names no platform row and the ledger has
// one unknown row with no family. Break: skip either sha check, or read a
// refused walk file as no walk file, and the edit is named from a pair no
// sync finished.
func TestPW1b2_ATornPairReadsUnknownAndSaysNothing(t *testing.T) {
	for name, edit := range map[string]func(line string) string{
		"log sha": func(l string) string {
			if strings.HasPrefix(l, "log\t") {
				return "log\t" + strings.Repeat("0", 64)
			}
			return l
		},
		"digests sha": func(l string) string {
			if strings.HasPrefix(l, "digests\t") {
				return "digests\t" + strings.Repeat("0", 64)
			}
			return l
		},
		"refused": func(l string) string {
			if strings.HasPrefix(l, "log\t") {
				return "refused"
			}
			if strings.HasPrefix(l, "digests\t") {
				return "\x00"
			}
			return l
		},
	} {
		t.Run(strings.ReplaceAll(name, " ", "_"), func(t *testing.T) {
			e := newSteerEnv(t)
			dir := gi1Project(t, h156Files)
			h156Sync(t, dir)
			walk := filepath.Join(dir, ".pgraph", "pgraph.walk")
			b, err := os.ReadFile(walk)
			if err != nil {
				t.Fatalf("premise: the sync wrote no walk file: %v", err)
			}
			var out []string
			for _, l := range strings.Split(string(b), "\n") {
				if l = edit(l); l != "\x00" {
					out = append(out, l)
				}
			}
			if err := os.WriteFile(walk, []byte(strings.Join(out, "\n")), 0o644); err != nil {
				t.Fatal(err)
			}
			e.dghCall(dir, "toolu_e", "Edit", pw1b2Edit(dir, "a/a.go"))
			got, _ := e.recapLine(stopEvent("Stop", "I changed a/a.go.", dir), stAgent, "CI=")
			t.Logf("walk file:\n%s\nStop line:\n%s", strings.Join(out, "\n"), got)
			if got == "" {
				t.Fatal("premise: the Stop printed nothing")
			}
			if strings.Contains(got, "platform: go test") {
				t.Errorf("a pair the walk file does not vouch for was read:\n%s", got)
			}
			if rows := e.h156Rows(testSession); len(rows) != 1 || rows[0][4] != "unknown" || rows[0][6] != "-" {
				t.Errorf("ledger rows %q, want one unknown row with no family", rows)
			}
		})
	}
}

// TestPW1b2_TheDoneFlagNamesThePackageToo: a final message that claims done
// prints the done flag, not the value line; it names the package all the
// same, in a line of at most 80 columns. Only that line is held to 80 here:
// the done flag's own first line is not this card's. Break: add the lines
// to the value line alone.
func TestPW1b2_TheDoneFlagNamesThePackageToo(t *testing.T) {
	e := newSteerEnv(t)
	dir := gi1Project(t, h156Files)
	h156Sync(t, dir)
	e.dghCall(dir, "toolu_e", "Edit", pw1b2Edit(dir, "a/a.go"))
	got, _ := e.recapLine(stopEvent("Stop", "Done.", dir), stAgent, "CI=")
	t.Logf("Stop line:\n%s", got)
	n := 0
	for _, l := range strings.Split(strings.TrimRight(got, "\n"), "\n") {
		if strings.TrimSpace(l) == "platform: go test ./a/... not run since the edit" {
			n++
			if c := utf8.RuneCountInString(l); c > 80 {
				t.Errorf("the platform line is %d columns: %q", c, l)
			}
		}
	}
	if n != 1 {
		t.Errorf("the done flag names package a %d times, want once", n)
	}
}

// TestPW1b2_TheReadIsDisclosed: the full screen's pgraph paragraph says the
// recorder reads .pgraph at the end of each reply, and what it says from
// it, to the person and then, at the next prompt, to the agent (step 3).
// Break: wire the read without the words.
func TestPW1b2_TheReadIsDisclosed(t *testing.T) {
	inv, err := disclose.Default()
	if err != nil {
		t.Fatal(err)
	}
	var text string
	for _, p := range inv.Paragraphs {
		if strings.Contains(p.Text, ".pgraph") {
			text = p.Text
		}
	}
	for _, w := range []string{"At the end of each reply rashomon reads .pgraph", "go test", "tells you", "your next prompt's note tells the agent the same"} {
		if !strings.Contains(text, w) {
			t.Errorf("the full screen's pgraph paragraph does not say %q:\n%s", w, text)
		}
	}
}
