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
		// v2. Hostnames only: the extractor returns a canonical hostname or
		// nothing, so neither list can carry a path, a query or a credential.
		// They are here rather than under shape because the report joins on
		// them and a nested list is harder to read back than a top-level one.
		"hosts", "ssh_hosts",
		// v3, RESERVED and not yet written. host_source says which entries of
		// `hosts` were read from a URL-valued field and which were extracted
		// from command text -- a closed two-value enum, so it can carry no
		// text. rule_match is the rule-match layer's verdict, an object whose
		// shape that layer owns; it is reserved here so that layer adds
		// behaviour rather than a second schema bump. Both are null on every
		// record this build writes.
		// file_label is the label layer's reservation: what the file a call
		// named LOOKS LIKE, from a closed vocabulary, never the path. Null on
		// every record this build writes.
		"host_source", "rule_match", "file_label",
	}
	executionKeys = []string{
		"type", "schema_version", "seq", "recorded_at_unix_ms",
		"tool_use_id", "session_id", "tool_name",
		// v2. An enum from a closed set, an integer parsed out of a fixed
		// prefix, a boolean, and the client's own millisecond count. None can
		// carry a substring of a tool response: the failure MESSAGE is read to
		// produce exit_code and has no field it could be assigned to.
		"outcome", "exit_code", "is_interrupt", "duration_ms",
		// v2. A keyed digest of the executed input: 64 hex characters, fixed
		// width, and the only thing that survives reading tool_input on the
		// post path.
		"executed_digest",
		// v3, RESERVED. See the note on the declaration list.
		"rule_match",
	}
	terminalKeys = []string{
		"type", "schema_version", "seq", "recorded_at_unix_ms",
		"tool_use_id", "session_id", "outcome", "reason",
	}
	coverageKeys = []string{
		"type", "schema_version", "recorded_at_unix_ms",
		"session_id", "install_id", "phase", "state", "reason", "hook_entry", "probe",
		// v2. The working directory, which is the novelty baseline's project
		// key. Metadata of the same class as transcript_path, which this store
		// has always kept verbatim. Note what is still absent: Coverage has no
		// numeric field, so the rule that a run whose instrumentation failed
		// cannot report a count is untouched.
		"cwd",
	}
	gapKeys = []string{
		"type", "schema_version", "recorded_at_unix_ms",
		"session_id", "reason", "from_unix_ms", "to_unix_ms", "removed_records",
		// v2. A keyed HMAC of the forgotten host, empty for a window forget.
		// Not the hostname: the report must recognise the host again while
		// `forget --host` must make the name leave the store's bytes.
		"host_digest",
	}
)

func TestH13_RecordKeySetsAreClosed(t *testing.T) {
	e := newEnv(t)
	e.watched(testSession)
	e.mustHook(defaultPayload().build(t))
	e.mustPost(defaultPost().build(t))
	e.probe("end", testSession)

	// A path-naming tool as well as the Bash call defaultPayload builds.
	//
	// Without this second declaration the item is blind to one whole class of
	// regression. shape.Label returns the empty string for Bash, so a Bash
	// declaration's file_label is null whatever the hook path does with the
	// field -- and a null is indistinguishable from a value nobody wrote. The
	// allowlist comparison cannot see it either: it compares the schema
	// document against the static slice above, never against a record.
	pathCall := defaultPayload()
	pathCall.ToolName = "Read"
	pathCall.ToolUseID = "toolu_2"
	pathCall.ToolInput = map[string]any{"file_path": "/home/u/.ssh/id_rsa"}
	e.mustHook(pathCall.build(t))

	for _, d := range e.declarations(testSession) {
		assertKeySet(t, d, declarationKeys)
	}
	assertKeySet(t, e.executions(testSession)[0], executionKeys)
	assertKeySet(t, e.terminals(testSession)[0], terminalKeys)
	// The post phase writes a coverage record like any other phase, and phase
	// is a key the allowlist already carries: a new phase adds no key.
	for _, phase := range []string{"start", "call", "post", "end"} {
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
//
// The canary is placed everywhere a command line carries a value that is not
// the program. An argument is the obvious place, and was once the only one
// here -- which left blind the place a parser that finds the program by
// skipping what cannot be one is likeliest to go wrong: a redirect whose
// operator the tokenizer emits as two tokens, so that skipping "the operator
// and the word after it" skips the operator's second half and keeps the word.
// That shipped once, and put a redirect target's filename in the program field
// of the store and of every render.
func TestH13_CanaryNeverReachesDisk(t *testing.T) {
	const canary = "CANARY-7f3a1e2b-must-not-persist"
	for _, tc := range []struct{ name, command string }{
		{"argument", "echo " + canary},
		{"redirect target after >&", ">& /tmp/" + canary + " ls"},
		{"redirect target after >|", ">| /tmp/" + canary + " ls"},
		{"redirect target after <>", "<> /tmp/" + canary + " ls"},
		{"here-string", "<<< " + canary + " cat"},
		{"process substitution", "<(cat /tmp/" + canary + ") ls"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := newEnv(t)
			e.watched(testSession)

			p := defaultPayload()
			p.ToolInput = map[string]any{
				"command":     tc.command,
				"description": canary,
				"nested":      map[string]any{"deep": canary},
			}
			res := e.hook(p.build(t))
			if res.exitCode != 0 {
				t.Fatalf("exit code %d, want 0", res.exitCode)
			}
			e.probe("end", testSession)
			// Every form of the report: they render the same records through
			// different code, and a renderer is exactly where a field nobody
			// meant to print gets printed. --redact is the one written to be
			// pasted somewhere public.
			repJSON := e.run("", nil, "report", "--json", "--session", testSession)
			repText := e.run("", nil, "report", "--session", testSession)
			repRedact := e.run("", nil, "report", "--redact", "--session", testSession)

			for rel, f := range walkStore(t, e.home) {
				if bytes.Contains(f.body, []byte(canary)) {
					t.Errorf("%s contains the canary", rel)
				}
			}
			// Hook stdout and stderr are written to Claude Code's debug log,
			// which puts them on disk just as surely as the store does. And
			// report is output.
			for name, s := range map[string]string{
				"hook stdout":     res.stdout,
				"hook stderr":     res.stderr,
				"report --json":   repJSON.stdout,
				"report text":     repText.stdout,
				"report --redact": repRedact.stdout,
			} {
				if strings.Contains(s, canary) {
					t.Errorf("%s contains the canary", name)
				}
			}
		})
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
	res := e.hook(defaultPayload().build(t), "RASHOMON_FAULT="+pointStoreWrite+":plain_panic")
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
