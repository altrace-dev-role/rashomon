package main

import (
	"bytes"
	"runtime"
	"strings"
	"testing"
)

// TestRES30_HookAndPostRunOnOneProc: hook and post leave the runtime with one
// P, whatever they started with, on an empty stdin and on a call they record. With a P per core a hook's footprint grew with
// contention past RES-30's per-process rule; oneProc's comment has the
// measurements.
func TestRES30_HookAndPostRunOnOneProc(t *testing.T) {
	t.Setenv("RASHOMON_HOME", t.TempDir())
	t.Setenv("CLAUDE_CONFIG_DIR", t.TempDir())
	before := runtime.GOMAXPROCS(0)
	t.Cleanup(func() { runtime.GOMAXPROCS(before) })

	for _, entry := range []string{"hook", "post"} {
		for _, stdin := range []string{"", `{"session_id":"s","tool_use_id":"t","tool_name":"Bash","tool_input":{"command":"ls"}}`} {
			runtime.GOMAXPROCS(4)
			var stdout, stderr bytes.Buffer
			if got := run([]string{entry}, strings.NewReader(stdin), &stdout, &stderr); got != exitOK {
				t.Fatalf("run(%s) = %d, want %d", entry, got, exitOK)
			}
			if got := runtime.GOMAXPROCS(0); got != 1 {
				t.Errorf("%s with stdin %q left GOMAXPROCS at %d, want 1", entry, stdin, got)
			}
		}
	}
}
