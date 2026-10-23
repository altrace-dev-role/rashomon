package acceptance

import (
	"strings"
	"testing"
)

// H-114 -- the probe says nothing on stdout.
//
// Claude Code hands a SessionStart hook's stdout to the model. A JSON
// object's additionalContext arrives as a system reminder, and so does plain
// text, labelled "SessionStart:startup hook success" (measured live on
// 2.1.304; test/live/hc-items.sh's HC-1 and fm-11 measure it again). A probe
// that printed would be writing into the context of every session it
// records, which is further than a recorder has any business reaching. What
// SessionEnd does with stdout has not been measured; it is held to the same
// rule because the probe has nothing to say there either.
//
// Break: one Fprintln to stdout -- in cmdProbe after the probe has run, in
// RunProbe's end phase, or in the refusal of a phase the probe does not know.
// The positive twin is the coverage record each phase writes: a probe that
// printed nothing because it did nothing would pass the stdout half alone.
func TestH114_ProbeStartAndEndPrintNothing(t *testing.T) {
	e := newEnv(t)
	if res := e.watch(); res.exitCode != 0 {
		t.Fatalf("watch: exit %d, stderr %q", res.exitCode, res.stderr)
	}
	for _, phase := range []string{"start", "end"} {
		res := e.probe(phase, testSession)
		if res.exitCode != 0 {
			t.Fatalf("probe %s: exit %d, stderr %q", phase, res.exitCode, res.stderr)
		}
		if res.stdout != "" {
			t.Errorf("probe %s wrote %q to stdout, which Claude Code would pass to the model", phase, res.stdout)
		}
		if got := e.coverage(testSession, phase); len(got) != 1 {
			t.Errorf("probe %s left %d %s-phase coverage records, want 1: it has to have run for its silence to mean anything", phase, len(got), phase)
		}
	}
}

// TestH114_ProbeThatDoesNotRecordPrintsNothing covers the two ways a probe
// declines to record -- paused, and an entry belonging to another install --
// because each has its own early return and its own line to say so, and that
// line belongs on stderr.
func TestH114_ProbeThatDoesNotRecordPrintsNothing(t *testing.T) {
	t.Run("paused", func(t *testing.T) {
		e := newEnv(t)
		e.watched(testSession)
		if res := e.pause(); res.exitCode != 0 {
			t.Fatalf("pause: exit %d, stderr %q", res.exitCode, res.stderr)
		}
		res := e.probe("end", testSession)
		if res.exitCode != 0 || res.stdout != "" {
			t.Errorf("paused probe end: exit %d, stdout %q; want 0 and nothing", res.exitCode, res.stdout)
		}
		paused := 0
		for _, c := range e.coverage(testSession, "end") {
			if c.str("reason") == "recording_paused" {
				paused++
			}
		}
		if paused != 1 {
			t.Errorf("got %d end-phase coverage records saying recording_paused, want 1", paused)
		}
	})
	t.Run("another install's entry", func(t *testing.T) {
		e := newEnv(t)
		e.watched(testSession)
		res := e.foreignProbe("end", testSession)
		if res.exitCode != 0 || res.stdout != "" {
			t.Errorf("foreign probe end: exit %d, stdout %q; want 0 and nothing", res.exitCode, res.stdout)
		}
		if !strings.Contains(res.stderr, "standing down") {
			t.Errorf("foreign probe end: stderr %q, want the stand-down line", res.stderr)
		}
	})
}

// TestH114_AnUnknownPhaseSaysSoOnStderrOnly: the phase check runs before the
// store is opened, so it is the one place in the probe a refusal could be
// printed without anything else having happened.
func TestH114_AnUnknownPhaseSaysSoOnStderrOnly(t *testing.T) {
	e := newEnv(t)
	for _, args := range [][]string{{"probe"}, {"probe", "middle"}} {
		res := e.run(e.sessionPayload("start", testSession), nil, args...)
		if res.exitCode != 0 {
			t.Errorf("%v: exit %d, want 0", args, res.exitCode)
		}
		if res.stdout != "" {
			t.Errorf("%v wrote %q to stdout", args, res.stdout)
		}
		if !strings.Contains(res.stderr, "probe expects start or end") {
			t.Errorf("%v: stderr %q, want the refusal", args, res.stderr)
		}
	}
}
