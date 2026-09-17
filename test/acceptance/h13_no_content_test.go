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
		"type", "schema_version", "seq", "recorded_at_unix_ms",
		"tool_use_id", "session_id", "prompt_id", "agent_id", "agent_type",
		"transcript_path", "permission_mode", "tool_name",
		"shape", "shape.program", "shape.verb_class", "shape.argc", "shape.digest",
	}
	terminalKeys = []string{
		"type", "schema_version", "seq", "recorded_at_unix_ms",
		"tool_use_id", "session_id", "outcome", "reason",
	}
	coverageKeys = []string{
		"type", "schema_version", "recorded_at_unix_ms",
		"session_id", "install_id", "phase", "state", "reason", "hook_entry", "probe",
	}
)

func TestH13_RecordKeySetsAreClosed(t *testing.T) {
	e := newEnv(t)
	e.watched(testSession)
	e.mustHook(defaultPayload().build(t))
	e.probe("end", testSession)

	assertKeySet(t, e.declarations(testSession)[0], declarationKeys)
	assertKeySet(t, e.terminals(testSession)[0], terminalKeys)
	for _, phase := range []string{"start", "call", "end"} {
		assertKeySet(t, e.coverage(testSession, phase)[0], coverageKeys)
	}
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
	e := newEnv(t)
	short := "echo " + strings.Repeat("a", 20)
	long := "echo " + strings.Repeat("a", 20480)

	decls := e.declarationsAfter(short, long)
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
	e := newEnv(t)
	e.watched(testSession)

	p := defaultPayload()
	p.ToolInput = map[string]any{
		"command":     "echo " + canary,
		"description": canary,
		"nested":      map[string]any{"deep": canary},
	}
	res := e.hook(p.build(t))
	if res.exitCode != 0 {
		t.Fatalf("exit code %d, want 0", res.exitCode)
	}
	e.probe("end", testSession)
	rep := e.run("", nil, "report", "--session", testSession)

	for rel, f := range walkStore(t, e.home) {
		if bytes.Contains(f.body, []byte(canary)) {
			t.Errorf("%s contains the canary", rel)
		}
	}
	// Hook stdout and stderr are written to Claude Code's debug log, which puts
	// them on disk just as surely as the store does. And report is output.
	for name, s := range map[string]string{"hook stdout": res.stdout, "hook stderr": res.stderr, "report": rep.stdout} {
		if strings.Contains(s, canary) {
			t.Errorf("%s contains the canary", name)
		}
	}
}

// TestH12_InternalErrorCoverageCarriesNoCount covers the internal-error half of
// H-12; the SIGTERM half is in h12_signal_test.go.
//
// The guarantee is structural: store.Coverage has no field that could hold a
// count, so a run whose instrumentation failed cannot report a number even by
// mistake. This assertion is what stops someone adding one later.
func TestH12_InternalErrorCoverageCarriesNoCount(t *testing.T) {
	e := newEnv(t)
	e.watched(testSession)
	res := e.hook(defaultPayload().build(t), "ATTEST_FAULT="+pointStoreWrite+":plain_panic")
	if res.exitCode != 0 {
		t.Fatalf("exit code %d, want 0", res.exitCode)
	}

	cov := e.coverage(testSession, "call")
	if len(cov) != 1 {
		t.Fatalf("got %d call coverage records, want 1", len(cov))
	}
	assertKeySet(t, cov[0], coverageKeys)
	if got := cov[0].str("state"); got != "unverified" {
		t.Errorf("state is %q, want \"unverified\"", got)
	}
	if got := cov[0].str("reason"); got != "internal_error" {
		t.Errorf("reason is %q, want \"internal_error\"", got)
	}
}
