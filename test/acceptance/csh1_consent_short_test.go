package acceptance

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"
)

// CONSENT-SHORT1: the screen a yes is asked under is the first thing a
// stranger reads, and on main it is 146 lines (B260). It is now one default
// terminal, 24 rows of at most 80 columns: what a yes records, what it never
// records and how to turn it off. Every detail is one command away, --full,
// which writes nothing.

// shortScreenRows is a default terminal's rows, the ceiling the screen is
// held to here independently of consent.MaxShortLines.
const shortScreenRows = 24

// fullHeadings are the five sections --full prints.
var fullHeadings = []string{"1. HOOK ENTRIES", "2. WHAT IS RECORDED", "3. WHAT RASHOMON ADDS", "4. LESSONS", "5. WHAT YOU WILL SEE"}

// shortLandmarks are what the short screen says, in the inventory's words.
var shortLandmarks = []string{"RECORDED ON THIS MACHINE for each tool call", "NEVER STORED:", "NOTES TO YOUR AGENT",
	"rashomon steer off --user", "rashomon purge", "rashomon consent --full"}

// homeEnv is a machine whose home directory is 40 characters, with the store
// and the Claude configuration at their default places under it, so the
// paths the screen prints are as long as a person's are. t.TempDir's paths
// are longer than most homes, and would test the wrapping, not the screen.
func homeEnv(t *testing.T) *env {
	t.Helper()
	base, err := os.MkdirTemp("/tmp", "csh1-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(base) }) //nolint:errcheck // a test's scratch
	home := filepath.Join(base, strings.Repeat("h", max(1, 40-len(base)-1)))
	e := &env{t: t, home: filepath.Join(home, ".local", "state", "rashomon"), configDir: filepath.Join(home, ".claude"), cwd: t.TempDir()}
	for _, d := range []string{e.home, e.configDir} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(e.checkStore)
	return e
}

func screenFits(t *testing.T, what, screen string) {
	t.Helper()
	lines := strings.Split(strings.TrimSuffix(screen, "\n"), "\n")
	if len(lines) > shortScreenRows {
		t.Errorf("%s is %d lines, past a terminal's %d:\n%s", what, len(lines), shortScreenRows, screen)
	}
	for i, l := range lines {
		if n := utf8.RuneCountInString(l); n > 80 {
			t.Errorf("%s line %d is %d columns: %q", what, i+1, n, l)
		}
	}
	for _, w := range shortLandmarks {
		if !strings.Contains(screen, w) {
			t.Errorf("%s does not say %q:\n%s", what, w, screen)
		}
	}
	if strings.Contains(screen, fullHeadings[0]) {
		t.Errorf("%s carries the five sections; they are --full's", what)
	}
}

// TestCSH1_TheScreenAYesIsAskedUnderFitsOneTerminal: watch --show and
// consent --show, the bytes a yes is asked under, fit 24 rows of 80 columns
// through the binary, fresh and over an older consent, whose screen opens
// with what changed. Break: put the five sections back on the y-screen, or
// let a short paragraph grow past the ceiling, and this fails.
func TestCSH1_TheScreenAYesIsAskedUnderFitsOneTerminal(t *testing.T) {
	e := homeEnv(t)
	res := e.run("", []string{"CLAUDECODE=1"}, "watch", "--show")
	if res.exitCode != 0 {
		t.Fatalf("watch --show: exit %d, stderr %q", res.exitCode, res.stderr)
	}
	screenFits(t, "watch --show", res.stdout)
	if !strings.Contains(res.stdout, e.configDir) {
		t.Errorf("watch --show does not name the settings it writes, under %s:\n%s", e.configDir, res.stdout)
	}

	if res := e.watch(); res.exitCode != 0 {
		t.Fatalf("premise: watch --yes: exit %d, stderr %q", res.exitCode, res.stderr)
	}
	older := strings.Replace(string(e.consentBytes()), `"scope_version":5`, `"scope_version":4`, 1)
	if !strings.Contains(older, `"scope_version":4`) {
		t.Fatal("premise: the watch record does not carry scope_version 5")
	}
	if err := os.WriteFile(filepath.Join(e.home, "consent.json"), []byte(older), 0o600); err != nil {
		t.Fatal(err)
	}
	show := e.run("", nil, "consent", "--show")
	if show.exitCode != 0 || !strings.Contains(show.stdout, "CHANGED SINCE YOU LAST AGREED") {
		t.Fatalf("consent --show over an older scope: exit %d, stdout %q", show.exitCode, show.stdout)
	}
	screenFits(t, "consent --show over an older scope", show.stdout)
}

// TestCSH1_FullPrintsEveryDetailAndWritesNothing: the five sections the short
// screen sums up are one command away, --full, from watch and from consent,
// on a fresh machine and an installed one, from an agent session too, and
// --full writes nothing. --full with --yes or --show is refused, so a
// script cannot take --full for an answer. Break: let --full ask, write, or
// print the short screen, and this fails.
func TestCSH1_FullPrintsEveryDetailAndWritesNothing(t *testing.T) {
	e := newEnv(t)
	check := func(what string, args ...string) {
		t.Helper()
		before := e.machine()
		res := e.run("", []string{"CLAUDECODE=1"}, args...)
		if res.exitCode != 0 {
			t.Fatalf("%s: exit %d, stderr %q", what, res.exitCode, res.stderr)
		}
		for _, h := range append(fullHeadings, "rashomon purge") {
			if !strings.Contains(res.stdout, h) {
				t.Errorf("%s does not print %q", what, h)
			}
		}
		if strings.Contains(res.stdout, questionInstall) || strings.Contains(res.stdout, questionConsent) {
			t.Errorf("%s asks the question", what)
		}
		e.requireUnchanged(before, what)
	}
	check("watch --full, fresh", "watch", "--full")
	check("consent --full, fresh", "consent", "--full")
	if res := e.watch(); res.exitCode != 0 {
		t.Fatalf("premise: watch --yes: exit %d, stderr %q", res.exitCode, res.stderr)
	}
	check("consent --full, installed", "consent", "--full")
	check("watch --full, installed", "watch", "--full")

	before := e.machine()
	for _, args := range [][]string{{"watch", "--full", "--yes"}, {"consent", "--full", "--show"}} {
		res := e.run("", nil, args...)
		if res.exitCode == 0 || !strings.Contains(res.stderr, "--full prints every detail and writes nothing") {
			t.Errorf("%v: exit %d, stderr %q; want a refusal", args, res.exitCode, res.stderr)
		}
	}
	e.requireUnchanged(before, "--full with an answer flag")
}
