package main

import (
	"bytes"
	"path/filepath"
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
	t.Setenv("ATTEST_HOME", t.TempDir())
	t.Setenv("CLAUDE_CONFIG_DIR", t.TempDir())
	t.Setenv("ATTEST_MANAGED_SETTINGS_PATH", filepath.Join(t.TempDir(), "absent"))

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
		{"report"},
		{"report", "--session"},
		{"forget"},
		{"forget", "--since", "nonsense"},
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
	t.Setenv("ATTEST_HOME", t.TempDir())
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
