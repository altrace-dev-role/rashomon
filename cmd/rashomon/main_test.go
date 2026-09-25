package main

import (
	"bytes"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// TestRunNeverReturnsTwo covers every dispatch path in one place.
//
// Exit code 2 from a PreToolUse hook blocks the tool call, and this binary is
// invoked as one. A usage error is the easy way to reintroduce it: 2 is the
// conventional status for bad arguments, and a contributor reaching for that
// convention would be writing a bug that only shows up as Claude Code
// mysteriously refusing to run commands.
func TestRunNeverReturnsTwo(t *testing.T) {
	// Every path below that touches disk must touch a throwaway. A test that
	// opened the real store or the real settings file would be H-18's own
	// failure mode, committed by the suite meant to prevent it.
	t.Setenv("RASHOMON_HOME", t.TempDir())
	t.Setenv("CLAUDE_CONFIG_DIR", t.TempDir())
	t.Setenv("RASHOMON_MANAGED_SETTINGS_PATH", filepath.Join(t.TempDir(), "absent"))

	cases := [][]string{
		nil,
		{},
		{"hook"},
		{"hook", "--install", foreignInstallID},
		{"hook", "--install"},
		{"hook", "--install", "not an id"},
		{"post"},
		{"post", "--install", foreignInstallID},
		{"post", "--install"},
		{"post", "--install", "not an id"},
		{"probe"},
		{"probe", "start"},
		{"probe", "end"},
		{"probe", "start", "--install", foreignInstallID},
		{"probe", "end", "--install"},
		{"probe", "start", "--install", "--install"},
		{"probe", "sideways"},
		{"version"},
		{"watch"},
		{"detach"},
		{"pause"},
		{"resume"},
		{"status"},
		{"report"},
		{"report", "--json"},
		{"report", "--session"},
		{"forget"},
		{"forget", "--since", "nonsense"},
		{"forget", "--before"},
		{"forget", "--before", "nonsense"},
		{"forget", "--since", "1h", "--before", "1h"},
		{"not-a-subcommand"},
		{"--help"},
		{"-h"},
		{""},
	}

	for _, args := range cases {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			got := run(args, strings.NewReader(""), &stdout, &stderr)

			if got == 2 {
				t.Fatalf("run(%q) returned 2, which blocks the tool call", args)
			}
			if got != exitOK && got != exitFail {
				t.Errorf("run(%q) returned %d, want %d or %d", args, got, exitOK, exitFail)
			}
		})
	}
}

// dormantCommand matches a usage line that offers env or run as a COMMAND.
//
// A word boundary, never the bare substring: "rashomon run" is a prefix of
// "rashomon running", and a check that could not tell the two apart once
// forced a skill's trigger phrase to be renamed to satisfy it.
var dormantCommand = regexp.MustCompile(`\brashomon (run|env)\b`)

// TestUsage_AdvertisesNoDormantProxyPath keeps the help text to what this
// alpha can serve.
//
// env, run and --proxy-store stay in the binary as the bridge to the observing
// proxy, but the proxy does not ship, so a usage screen that offered them
// would send a reader to commands that either refuse (env), run without
// recording destinations (run), or read a database they do not have
// (--proxy-store). The dormant commands keep their own --help text; only the
// top-level screen is trimmed.
//
// The positive half is not decoration: a usage that printed nothing would
// pass every absence check below.
func TestUsage_AdvertisesNoDormantProxyPath(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if got := run([]string{"--help"}, strings.NewReader(""), &stdout, &stderr); got != exitOK {
		t.Fatalf("--help returned %d, want %d", got, exitOK)
	}
	out := stdout.String()

	for _, want := range []string{"rashomon watch", "rashomon report", "--nono-audit", "rashomon forget"} {
		if !strings.Contains(out, want) {
			t.Errorf("usage no longer names %q, so the absences below prove nothing:\n%s", want, out)
		}
	}
	if m := dormantCommand.FindString(out); m != "" {
		t.Errorf("usage advertises %q, a command this alpha does not serve:\n%s", m, out)
	}
	for _, gone := range []string{"--proxy-store", "--proxy-status"} {
		if strings.Contains(out, gone) {
			t.Errorf("usage advertises %q, a path this alpha does not serve:\n%s", gone, out)
		}
	}
}

// TestDormantCommandMatchesCommandsNotWords holds the matcher to its reason.
// Without the negative rows a regex that matched nothing would make the
// usage test above pass vacuously.
func TestDormantCommandMatchesCommandsNotWords(t *testing.T) {
	for line, want := range map[string]bool{
		"  rashomon run -- claude":          true,
		"  rashomon run [--proxy-status P]": true,
		"  rashomon env [--port N]":         true,
		"  rashomon env":                    true,
		"is rashomon running":               false,
		"rashomon environment":              false,
		"  rashomon report":                 false,
	} {
		if got := dormantCommand.MatchString(line); got != want {
			t.Errorf("dormantCommand.MatchString(%q) = %v, want %v", line, got, want)
		}
	}
}

// TestReport_RefusesAnEmptyProxyStore: "" is what NOT naming a store means,
// so accepting `--proxy-store ""` -- an unset variable in a script -- would
// collapse the proxy block the caller asked for, with nothing to say so. The
// refusal names the flag, and it comes before any store is opened.
func TestReport_RefusesAnEmptyProxyStore(t *testing.T) {
	t.Setenv("RASHOMON_HOME", t.TempDir())
	var stdout, stderr bytes.Buffer
	if got := run([]string{"report", "--proxy-store", ""}, strings.NewReader(""), &stdout, &stderr); got != exitFail {
		t.Errorf("report --proxy-store \"\" returned %d, want %d", got, exitFail)
	}
	if !strings.Contains(stderr.String(), "--proxy-store") {
		t.Errorf("the refusal does not name the flag; stderr %q", stderr.String())
	}
	if stdout.Len() != 0 {
		t.Errorf("report --proxy-store \"\" rendered a report:\n%s", stdout.String())
	}
}

// foreignInstallID is an id this environment cannot be holding: the store's id
// is random and created on first open, so any literal names another install.
const foreignInstallID = "ffffffffffffffffffffffffffffffff"

// TestHookPathsAlwaysExitZero is the stricter rule for the paths Claude Code
// itself invokes: not merely never 2, but always 0, whatever they were given.
//
// An --install naming another install is the ordinary case rather than an
// abusive one -- it is what a second install's entry passes -- and it stands
// down. A malformed one still exits 0: an argument this program cannot read is
// not grounds for blocking a tool call.
func TestHookPathsAlwaysExitZero(t *testing.T) {
	t.Setenv("RASHOMON_HOME", t.TempDir())
	t.Setenv("CLAUDE_CONFIG_DIR", t.TempDir())

	for _, args := range [][]string{
		{"hook"},
		{"hook", "--install", "x"},
		{"hook", "--install", foreignInstallID},
		{"hook", "--install"},
		{"hook", "--install", "not an id"},
		{"post"},
		{"post", "--install", "x"},
		{"post", "--install", foreignInstallID},
		{"post", "--install"},
		{"post", "--install", "not an id"},
		{"probe", "start"},
		{"probe", "end"},
		{"probe", "start", "--install", foreignInstallID},
		{"probe", "end", "--install"},
		{"probe", "start", "--install", "--install"},
		{"probe"},
		{"probe", "neither"},
	} {
		for _, stdin := range []string{"", "not json", `{"session_id":"s"}`} {
			var stdout, stderr bytes.Buffer
			if got := run(args, strings.NewReader(stdin), &stdout, &stderr); got != 0 {
				t.Errorf("run(%q) with stdin %q returned %d, want 0", args, stdin, got)
			}
			if stdout.Len() != 0 {
				t.Errorf("run(%q) wrote to stdout, which Claude Code parses as control output: %q", args, stdout.String())
			}
		}
	}
}
