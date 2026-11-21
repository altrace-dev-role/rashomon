package main

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestRPT1q_UseColorPaintsOnlyATerminal (card RPT-1): the overview is
// painted when stdout is a character device, NO_COLOR is unset or empty and
// TERM is not dumb. /dev/null is a character device, so it stands in for a
// terminal here; a pipe, a regular file and a buffer are never painted.
func TestRPT1q_UseColorPaintsOnlyATerminal(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("useColor never paints on windows, by its own rule")
	}
	devnull, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer devnull.Close()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	defer w.Close()
	reg, err := os.Create(filepath.Join(t.TempDir(), "out"))
	if err != nil {
		t.Fatal(err)
	}
	defer reg.Close()
	for _, c := range []struct {
		name          string
		out           io.Writer
		noColor, term string
		want          bool
	}{
		{"a pipe", w, "", "xterm", false},
		{"a regular file", reg, "", "xterm", false},
		{"a buffer", &bytes.Buffer{}, "", "xterm", false},
		{"/dev/null with TERM=xterm", devnull, "", "xterm", true},
		{"/dev/null with TERM empty", devnull, "", "", true},
		{"NO_COLOR=1", devnull, "1", "xterm", false},
		{"TERM=dumb", devnull, "", "dumb", false},
	} {
		t.Setenv("NO_COLOR", c.noColor)
		t.Setenv("TERM", c.term)
		if got := useColor(c.out); got != c.want {
			t.Errorf("%s: useColor %v, want %v", c.name, got, c.want)
		}
	}
}

// TestRPT1r_OverviewRefusesAnotherView (card RPT-1, dispatch row e):
// --overview with --full, --json, --ledger or an expanding flag is an error
// naming the flag to drop, before any store is read; --ledger's own refusal
// names --overview and --full as it names the others. --redact and --session
// compose with --overview, so with nothing recorded they reach lastSession's
// error or the empty report, never the refusal.
func TestRPT1r_OverviewRefusesAnotherView(t *testing.T) {
	t.Setenv("RASHOMON_HOME", filepath.Join(t.TempDir(), "absent"))
	const refuse = "--overview takes only --session and --redact: drop "
	for _, c := range []struct {
		args []string
		want string
	}{
		{[]string{"--overview", "--full"}, refuse + "--full"},
		{[]string{"--full", "--overview"}, refuse + "--full"},
		{[]string{"--overview", "--json"}, refuse + "--json"},
		{[]string{"--overview", "--chain"}, refuse + "--chain"},
		{[]string{"--overview", "--timeline"}, refuse + "--timeline"},
		{[]string{"--overview", "--ids"}, refuse + "--ids"},
		{[]string{"--overview", "--nono-audit", "trail.jsonl"}, refuse + "--nono-audit"},
		{[]string{"--overview", "--json", "--chain"}, refuse + "--json"},
		{[]string{"--overview", "--session", "s", "--json"}, refuse + "--json"},
		{[]string{"--ledger", "--session", "s", "--overview"}, "--ledger prints alone: drop --overview"},
		{[]string{"--ledger", "--session", "s", "--full"}, "--ledger prints alone: drop --full"},
		{[]string{"--overview"}, "--session last: nothing has been recorded here"},
		{[]string{"--overview", "--redact"}, "--session last: nothing has been recorded here"},
		{[]string{"--overview", "--session", "s"}, "report: no session to show"},
	} {
		var out bytes.Buffer
		err := cmdReport(c.args, &out)
		if err == nil || err.Error() != c.want {
			t.Errorf("report %s: error %v, want %q", strings.Join(c.args, " "), err, c.want)
		}
		if out.Len() != 0 {
			t.Errorf("report %s printed %q before its error", strings.Join(c.args, " "), out.String())
		}
	}
}

// TestRPT1s_OverviewOfANamedSession (card RPT-1, dispatch rows b and c): with
// a session recorded, --overview --session prints that session's overview,
// unpainted into a buffer, and --session alone prints today's text. The
// fixture's checkpoint is for its own directory, not the test's, so the bare
// run here has no checkpoint of its project and prints today's text too.
func TestRPT1s_OverviewOfANamedSession(t *testing.T) {
	f := newLW(t)
	f.bind(lwSpec("unit"))
	f.land("toolu_e", "Edit", lwEdit(f.file("a.go", "package a\n")), nil)
	f.stop()
	var o, e bytes.Buffer
	if code := run([]string{"report", "--overview", "--session", "s"}, strings.NewReader(""), &o, &e); code != 0 {
		t.Fatalf("report --overview --session s: exit %d: %s", code, e.String())
	}
	if out := o.String(); !strings.HasPrefix(out, "rashomon · session s ") || strings.Contains(out, "\x1b") {
		t.Errorf("--overview --session s is not the unpainted overview of s:\n%s", out)
	}
	for _, args := range [][]string{{"report", "--session", "s"}, {"report"}, {"report", "--full"}} {
		o.Reset()
		e.Reset()
		if code := run(args, strings.NewReader(""), &o, &e); code != 0 {
			t.Fatalf("%s: exit %d: %s", strings.Join(args, " "), code, e.String())
		}
		if strings.HasPrefix(o.String(), "rashomon · session") {
			t.Errorf("%s printed the overview, want today's text:\n%s", strings.Join(args, " "), o.String())
		}
	}
}
