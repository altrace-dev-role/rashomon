package acceptance

import (
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// The dispatch of card RPT-1: which runs of `rashomon report` print the
// overview of one session and which print today's text. One test per row, through the built binary, whose stdout is a
// pipe here: an overview printed into a pipe carries no ESC byte.

// rpt1Overview is the overview's first line up to its padding: the overview
// of testSession. rpt1Today is the first line of today's text.
const (
	rpt1Overview = "rashomon · session " + testSession + " "
	rpt1Today    = "rashomon report -- generated "
)

// rpt1In runs `rashomon report args...` with dir as the working directory,
// or the test's own when dir is "".
func rpt1In(e *env, dir string, extraEnv []string, args ...string) result {
	e.t.Helper()
	cmd := e.command("", extraEnv, append([]string{"report"}, args...)...)
	if dir != "" {
		cmd.Dir = dir
	}
	return e.wait(cmd)
}

// rpt1Want fails t unless res exited 0 and its stdout opens with head, with
// no ESC byte anywhere in it.
func rpt1Want(t *testing.T, what string, res result, head string) {
	t.Helper()
	if res.exitCode != 0 || !strings.HasPrefix(res.stdout, head) || strings.Contains(res.stdout, "\x1b") {
		t.Errorf("%s: exit %d, want stdout opening %q with no ESC byte; stderr %q\n%s", what, res.exitCode, head, res.stderr, res.stdout)
	}
}

// TestRPT1_RowA_BareIsTheOverviewOfTheLastSession: no --session, no --json
// and no expanding flag prints the overview of the session that last names,
// --redact included, unpainted into a pipe even with TERM set; with no store,
// or from a project no session cut a checkpoint for, today's text.
func TestRPT1_RowA_BareIsTheOverviewOfTheLastSession(t *testing.T) {
	e := newEnv(t)
	fresh := t.TempDir()
	if res := e.run("", []string{"RASHOMON_HOME=" + fresh}, "report"); res.exitCode != 0 || strings.HasPrefix(res.stdout, "rashomon · session") {
		t.Errorf("bare with no store: exit %d, want today's text; stderr %q\n%s", res.exitCode, res.stderr, res.stdout)
	}
	e.rpSession(t)
	plain := e.run("", []string{"TERM=xterm", "NO_COLOR="}, "report")
	rpt1Want(t, "bare", plain, rpt1Overview)
	redacted := e.run("", nil, "report", "--redact")
	rpt1Want(t, "bare --redact", redacted, rpt1Overview)
	// The second line names the project; --redact turns it into a keyed
	// digest, so the two lines differ only if the overview was redacted.
	if p, r := rpt1Line(plain.stdout, 1), rpt1Line(redacted.stdout, 1); p == r {
		t.Errorf("bare --redact prints the plain second line %q", r)
	}
	rpt1Want(t, "bare from another project", rpt1In(e, t.TempDir(), nil), rpt1Today)
}

// TestRPT1_RowA_ALookupThatFailsPrintsNeitherText: a bare run whose
// checkpoint lookup fails on a store that opens returns that error and
// prints neither the overview nor today's text. Every records file of the
// store, unreadable (chmod 000), fails the lookup; today's text would fail on
// the same files, so this pins what the user sees, not which reader failed.
func TestRPT1_RowA_ALookupThatFailsPrintsNeitherText(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("chmod 000 does not stop this user reading the file")
	}
	e := newEnv(t)
	e.rpSession(t)
	n := 0
	err := filepath.WalkDir(e.home, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || d.Name() != "records.ndjson" {
			return err
		}
		fi, err := d.Info()
		if err != nil {
			return err
		}
		if err := os.Chmod(p, 0); err != nil {
			return err
		}
		t.Cleanup(func() { os.Chmod(p, fi.Mode().Perm()) }) //nolint:errcheck // the store check reads it next
		n++
		return nil
	})
	if err != nil || n == 0 {
		t.Fatalf("making the records files unreadable: %d made, %v", n, err)
	}
	res := e.run("", nil, "report")
	if res.exitCode == 0 || res.stdout != "" || !strings.Contains(res.stderr, "permission denied") {
		t.Errorf("bare with unreadable records: exit %d, stderr %q, want a permission error and no stdout\n%s", res.exitCode, res.stderr, res.stdout)
	}
}

// rpt1Line is line i of s, counted from 0, or "" past its end.
func rpt1Line(s string, i int) string {
	if lines := strings.Split(s, "\n"); i < len(lines) {
		return lines[i]
	}
	return ""
}

// TestRPT1_RowB_ANamedSessionOrAnExpandingFlagIsTodaysText: --session with an
// id, last included, and each expanding flag print today's text.
func TestRPT1_RowB_ANamedSessionOrAnExpandingFlagIsTodaysText(t *testing.T) {
	e := newEnv(t)
	e.rpSession(t)
	for _, args := range [][]string{{"--session", testSession}, {"--session", "last"}, {"--chain"}, {"--timeline"}, {"--ids"}} {
		rpt1Want(t, "report "+strings.Join(args, " "), rpt1In(e, "", nil, args...), rpt1Today)
	}
}

// TestRPT1_RowC_OverviewForcesTheOverview: --overview prints the overview of
// the named session from any directory, of the session last names without
// one, with --redact composing; where last has no session, the error is
// lastSession's own and nothing is printed.
func TestRPT1_RowC_OverviewForcesTheOverview(t *testing.T) {
	e := newEnv(t)
	e.rpSession(t)
	elsewhere := t.TempDir()
	rpt1Want(t, "--overview --session from another project", rpt1In(e, elsewhere, nil, "--overview", "--session", testSession), rpt1Overview)
	rpt1Want(t, "--overview", rpt1In(e, "", nil, "--overview"), rpt1Overview)
	rpt1Want(t, "--overview --redact", rpt1In(e, "", nil, "--overview", "--redact"), rpt1Overview)
	res := rpt1In(e, elsewhere, nil, "--overview")
	if res.exitCode == 0 || res.stdout != "" || !strings.Contains(res.stderr, "--session last: no session of this project holds a checkpoint") {
		t.Errorf("--overview from another project: exit %d, stderr %q, want lastSession's refusal and no stdout\n%s", res.exitCode, res.stderr, res.stdout)
	}
}

// TestRPT1_RowD_FullForcesTodaysText: --full prints today's text of every
// session, and with --session the same text as --session alone.
func TestRPT1_RowD_FullForcesTodaysText(t *testing.T) {
	e := newEnv(t)
	e.rpSession(t)
	rpt1Want(t, "--full", rpt1In(e, "", nil, "--full"), rpt1Today)
	full := rpt1In(e, "", nil, "--full", "--session", testSession)
	named := rpt1In(e, "", nil, "--session", testSession)
	rpt1Want(t, "--full --session", full, rpt1Today)
	if dropGenerated(full.stdout) != dropGenerated(named.stdout) {
		t.Errorf("--full --session differs from --session:\n%s\n----\n%s", full.stdout, named.stdout)
	}
}

// TestRPT1_RowE_OverviewWithAnotherViewIsRefused: --overview with --full,
// --json, --ledger or an expanding flag exits non-zero naming the flag to
// drop, and prints nothing on stdout.
func TestRPT1_RowE_OverviewWithAnotherViewIsRefused(t *testing.T) {
	e := newEnv(t)
	e.rpSession(t)
	for _, c := range []struct {
		args []string
		want string
	}{
		{[]string{"--overview", "--full"}, "--overview takes only --session and --redact: drop --full"},
		{[]string{"--overview", "--json"}, "--overview takes only --session and --redact: drop --json"},
		{[]string{"--overview", "--timeline"}, "--overview takes only --session and --redact: drop --timeline"},
		{[]string{"--ledger", "--session", testSession, "--overview"}, "--ledger prints alone: drop --overview"},
	} {
		res := rpt1In(e, "", nil, c.args...)
		if res.exitCode == 0 || res.stdout != "" || !strings.Contains(res.stderr, c.want) {
			t.Errorf("report %s: exit %d, stderr %q, want %q and no stdout", strings.Join(c.args, " "), res.exitCode, res.stderr, c.want)
		}
	}
}
