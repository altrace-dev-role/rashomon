package main

import (
	"bytes"
	"path/filepath"
	"runtime/debug"
	"testing"
)

// TestHookEntriesPinTheirRuntimeSettings: a hook entry process's GC percent
// and soft memory limit are fixed at hookGCPercent and hookMemoryLimitBytes
// (archive 85.3.12's RUNTIME block: "soft memory limit 32 MiB, GOGC 100 (env
// ignored)"), regardless of whatever GOMEMLIMIT or GOGC set them to before
// the dispatch ran. debug.SetGCPercent and debug.SetMemoryLimit each return
// the setting in force the moment before the call, so reapplying the pinned
// value after run() and checking that return is a read that leaves the
// pinned value in place -- no restore step needed on that axis.
func TestHookEntriesPinTheirRuntimeSettings(t *testing.T) {
	t.Setenv("RASHOMON_HOME", t.TempDir())
	t.Setenv("CLAUDE_CONFIG_DIR", t.TempDir())
	t.Setenv("RASHOMON_MANAGED_SETTINGS_PATH", filepath.Join(t.TempDir(), "absent"))

	// Simulate an environment that asked for something looser, the way
	// GOMEMLIMIT/GOGC would have left the runtime before main ran.
	prevGC := debug.SetGCPercent(400)
	prevMem := debug.SetMemoryLimit(1 << 30)
	t.Cleanup(func() {
		debug.SetGCPercent(prevGC)
		debug.SetMemoryLimit(prevMem)
	})

	for _, args := range [][]string{{"hook"}, {"post"}, {"probe", "start"}, {"probe", "end"}, {"recap"}} {
		// Reset to the looser setting before each case, so a case earlier
		// in the loop pinning it cannot make a later case's own pin look
		// like a no-op.
		debug.SetGCPercent(400)
		debug.SetMemoryLimit(1 << 30)

		var stdout, stderr bytes.Buffer
		run(args, bytes.NewReader(nil), &stdout, &stderr)

		if got := debug.SetGCPercent(hookGCPercent); got != hookGCPercent {
			t.Errorf("run(%q): GC percent was %d after dispatch, want %d (env-derived 400 must be overridden)", args, got, hookGCPercent)
		}
		if got := debug.SetMemoryLimit(hookMemoryLimitBytes); got != hookMemoryLimitBytes {
			t.Errorf("run(%q): memory limit was %d after dispatch, want %d (env-derived 1 GiB must be overridden)", args, got, hookMemoryLimitBytes)
		}
	}
}

// TestNonHookEntriesDoNotTouchRuntimeSettings: a command a person types
// (version is the cheapest one to dispatch) leaves GOGC and the memory
// limit exactly where they were: the pin is for the five hook-entry
// processes (archive 85.3.12), not for every subcommand this binary has.
func TestNonHookEntriesDoNotTouchRuntimeSettings(t *testing.T) {
	prevGC := debug.SetGCPercent(400)
	prevMem := debug.SetMemoryLimit(1 << 30)
	t.Cleanup(func() {
		debug.SetGCPercent(prevGC)
		debug.SetMemoryLimit(prevMem)
	})

	var stdout, stderr bytes.Buffer
	run([]string{"version"}, bytes.NewReader(nil), &stdout, &stderr)

	if got := debug.SetGCPercent(400); got != 400 {
		t.Errorf("run([\"version\"]) changed GC percent to %d, want it untouched at 400", got)
	}
	if got := debug.SetMemoryLimit(1 << 30); got != 1<<30 {
		t.Errorf("run([\"version\"]) changed the memory limit to %d, want it untouched at 1 GiB", got)
	}
}
