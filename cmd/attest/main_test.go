package main

import (
	"bytes"
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
	cases := [][]string{
		nil,
		{},
		{"hook"},
		{"version"},
		{"watch"},
		{"detach"},
		{"report"},
		{"forget"},
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
