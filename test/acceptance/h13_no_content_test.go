package acceptance

import (
	"bytes"
	"strings"
	"testing"
)

// The complete set of keys each record type may carry, as dotted paths.
//
// These are assertions about the key set, not about the absence of one named
// key. Asserting that "difference_count" is absent is vacuously true whenever
// the code never produces that key, and stays true while a different content
// field is added beside it.
var (
	declarationKeys = []string{
		"type", "schema_version", "recorded_at_unix_ms",
		"tool_use_id", "session_id", "prompt_id", "agent_id", "agent_type",
		"transcript_path", "permission_mode", "tool_name",
		"shape", "shape.program", "shape.verb_class", "shape.argc", "shape.digest",
	}
	terminalKeys = []string{
		"type", "schema_version", "recorded_at_unix_ms",
		"tool_use_id", "session_id", "outcome", "reason",
	}
	coverageKeys = []string{
		"type", "schema_version", "recorded_at_unix_ms",
		"session_id", "install_id", "state", "reason", "hook_entry",
	}
)

func TestH13_RecordKeySetsAreClosed(t *testing.T) {
	home := t.TempDir()

	if res := runHook(t, home, defaultPayload().build(t)); res.exitCode != 0 {
		t.Fatalf("exit code %d, want 0", res.exitCode)
	}

	recs := readRecords(t, home, testSession, "records.ndjson")
	assertKeySet(t, recordsOfType(recs, "declaration")[0], declarationKeys)
	assertKeySet(t, recordsOfType(recs, "terminal")[0], terminalKeys)

	cov := recordsOfType(readRecords(t, home, testSession, "coverage.ndjson"), "coverage")
	assertKeySet(t, cov[0], coverageKeys)
}

// TestH13_RecordWidthIsIndependentOfInputSize is the assertion that survives an
// encoding. A canary search proves only that one literal is absent, which any
// base64 or hex or chunked representation defeats; a record whose serialized
// width does not move when the input grows by three orders of magnitude cannot
// be carrying the input.
//
// The premise is guarded first. Width is allowed to depend on argument count,
// which is a count and not content, so the two commands are constructed to have
// the same program and the same argc and to differ only in how many bytes the
// argument occupies.
func TestH13_RecordWidthIsIndependentOfInputSize(t *testing.T) {
	home := t.TempDir()

	short := "echo " + strings.Repeat("a", 20)
	long := "echo " + strings.Repeat("a", 20480)

	decls := declarationsAfter(t, home, short, long)
	a, b := decls[0], decls[1]

	argcA, _ := nested(a, "shape.argc")
	argcB, _ := nested(b, "shape.argc")
	if argcA != argcB {
		t.Fatalf("premise broken: argc differs (%v vs %v), so width is allowed to differ", argcA, argcB)
	}
	progA, _ := nested(a, "shape.program")
	progB, _ := nested(b, "shape.program")
	if progA != progB {
		t.Fatalf("premise broken: program differs (%v vs %v)", progA, progB)
	}

	if len(a.raw) != len(b.raw) {
		t.Errorf("record width tracks input size: %d bytes for a %d-byte command, %d bytes for a %d-byte command",
			len(a.raw), len(short), len(b.raw), len(long))
	}
}

// TestH13_CanaryNeverReachesDisk is the second line of defence, and only that.
func TestH13_CanaryNeverReachesDisk(t *testing.T) {
	const canary = "CANARY-7f3a1e2b-must-not-persist"

	home := t.TempDir()
	p := defaultPayload()
	p.ToolInput = map[string]any{
		"command":     "echo " + canary,
		"description": canary,
		"nested":      map[string]any{"deep": canary},
	}

	res := runHook(t, home, p.build(t))
	if res.exitCode != 0 {
		t.Fatalf("exit code %d, want 0", res.exitCode)
	}

	for rel, f := range walkStore(t, home) {
		if bytes.Contains(f.body, []byte(canary)) {
			t.Errorf("%s contains the canary", rel)
		}
	}

	// Hook stdout and stderr are written to Claude Code's debug log, which puts
	// them on disk just as surely as the store does.
	if strings.Contains(res.stdout, canary) {
		t.Errorf("stdout contains the canary")
	}
	if strings.Contains(res.stderr, canary) {
		t.Errorf("stderr contains the canary")
	}
}

// TestH12_InternalErrorCoverageCarriesNoCount covers the internal-error half of
// H-12. The SIGTERM half needs a way to hold the handler open long enough to
// signal it and is not covered yet.
//
// The guarantee is structural: store.Coverage has no field that could hold a
// count, so a run whose instrumentation failed cannot report a number even by
// mistake. This assertion is what stops someone adding one later.
func TestH12_InternalErrorCoverageCarriesNoCount(t *testing.T) {
	home := t.TempDir()

	res := runHook(t, home, defaultPayload().build(t), "ATTEST_FAULT="+pointStoreWrite+":plain_panic")
	if res.exitCode != 0 {
		t.Fatalf("exit code %d, want 0", res.exitCode)
	}

	cov := recordsOfType(readRecords(t, home, testSession, "coverage.ndjson"), "coverage")
	if len(cov) != 1 {
		t.Fatalf("got %d coverage records, want 1", len(cov))
	}

	assertKeySet(t, cov[0], coverageKeys)
	if got := cov[0].fields["state"]; got != "unverified" {
		t.Errorf("state is %v, want \"unverified\"", got)
	}
	if got := cov[0].fields["reason"]; got != "internal_error" {
		t.Errorf("reason is %v, want \"internal_error\"", got)
	}
}
