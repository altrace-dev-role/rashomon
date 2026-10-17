package spend

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// usageKeySpellings are spellings of message.usage that encoding/json's
// decode accepts and the exact byte test `"usage"` does not: its key match is
// case-insensitive under Unicode folding (U+017F folds to s), and a key may
// carry escapes.
var usageKeySpellings = []string{`Usage`, `USAGE`, `us\u0061ge`, `\u0055SAGE`, "uſage"}

// usageLineWithKey is one assistant usage line of a main transcript, its
// usage key spelt as given.
func usageLineWithKey(key string) string {
	return `{"type":"assistant","timestamp":"2026-09-29T10:00:01.000Z","sessionId":"s1",` +
		`"message":{"id":"msg_1","model":"claude-sonnet-4-5","stop_reason":"end_turn","` + key +
		`":{"input_tokens":10,"output_tokens":20,"cache_read_input_tokens":0,"cache_creation_input_tokens":0}}}`
}

// TestRubricPERF3_AUsageLineTheDecodeAcceptsIsCounted holds the main
// transcript's byte prefilter to the decode behind it: once the file is
// dated, a line is skipped unparsed only when it cannot carry usage. The
// exact test for the bytes "usage" dropped a usage line whose key the decode
// accepts -- a capital, an escape, a folded letter -- without counting it
// anywhere, priced or unparsed.
func TestRubricPERF3_AUsageLineTheDecodeAcceptsIsCounted(t *testing.T) {
	for _, key := range usageKeySpellings {
		t.Run(key, func(t *testing.T) {
			dir := t.TempDir()
			p := filepath.Join(dir, "s1.jsonl")
			first := `{"type":"user","timestamp":"2026-09-29T10:00:00.000Z","promptId":"p1","message":{"role":"user","content":"go"}}`
			if err := os.WriteFile(p, []byte(first+"\n"+usageLineWithKey(key)+"\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			sc, err := Read(&Found{Files: []TranscriptFile{{Path: p, Session: "s1", Main: p}}})
			if err != nil {
				t.Fatal(err)
			}
			if sc.UsageLines != 1 || len(sc.Responses) != 1 || sc.Unparsed != 0 {
				t.Errorf("usage key %q: %d usage lines, %d responses, %d unparsed; want 1, 1, 0",
					key, sc.UsageLines, len(sc.Responses), sc.Unparsed)
			}
		})
	}
}

// checkUsagePrefilter is the property FuzzUsagePrefilter checks: a line the
// decode reads a usage object from is never one the prefilter skips.
func checkUsagePrefilter(t *testing.T, raw []byte) {
	var l line
	if json.Unmarshal(raw, &l) != nil || l.Message.Usage == nil {
		return
	}
	if !mayCarryUsage(raw) {
		t.Errorf("the decode reads usage from %q and the prefilter skips it", raw)
	}
}

// FuzzUsagePrefilter fuzzes the prefilter against the decode it stands in
// front of, seeded with a real usage line, each accepted spelling of its key
// and a line that carries none.
func FuzzUsagePrefilter(f *testing.F) {
	for _, key := range append([]string{"usage"}, usageKeySpellings...) {
		f.Add([]byte(usageLineWithKey(key)))
	}
	f.Add([]byte(`{"type":"user","message":{"role":"user","content":"the usage of usage"}}`))
	f.Add([]byte(`{"message":{"USAGE":null,"usage":{}}}`))
	f.Fuzz(func(t *testing.T, raw []byte) {
		checkUsagePrefilter(t, raw)
	})
}

// TestRubricPERF3_UsagePrefilterAgreesWithTheDecode runs the fuzz property
// over its seeds without the fuzzer, so the sweep can pin it.
func TestRubricPERF3_UsagePrefilterAgreesWithTheDecode(t *testing.T) {
	for _, key := range usageKeySpellings {
		checkUsagePrefilter(t, []byte(usageLineWithKey(key)))
		checkUsagePrefilter(t, []byte(strings.Replace(`{"message":{"K":{}}}`, "K", key, 1)))
	}
}

// toolResultLine is a tool_result line of the shape a Bash call with
// colourised output leaves in a real transcript: n repeats of a short ANSI
// escape sequence, written the way Claude Code's own JSON encoder writes
// control characters -- \u001b, never a literal ESC byte -- inside a
// tool_result block. It carries no usage key.
func toolResultLine(n int) []byte {
	var b strings.Builder
	b.WriteString(`{"type":"user","timestamp":"2026-09-29T10:00:01.000Z","message":{"role":"user","content":[` +
		`{"type":"tool_result","tool_use_id":"toolu_1","content":"`)
	for i := 0; i < n; i++ {
		b.WriteString(`\u001b[32mPASS\u001b[0m `)
	}
	b.WriteString(`"}]}}`)
	return []byte(b.String())
}

// TestMayCarryUsage_ToolResultHeavyLineStaysBounded covers a read raised
// against this prefilter: a tool-result-heavy line, carrying no usage key,
// whose \u001b escapes are real transcript content rather than the widening
// they were first suspected of costing. It is a Test, not a Benchmark --
// TestRES36_BudgetsFileCoversEveryBenchmark (test/perf/gates_test.go) and
// benchgate itself both fail closed on any Benchmark function with no row
// in test/perf/budgets.json, and this is a one-off check on a worst-case
// input, not a figure meant to join the tracked budget surface. A direct
// timing loop here answers the one question that matters without adding a
// benchmark that would need a row.
//
// Isolated from other load on this machine, the escape-heavy case ran
// several times slower than its same-length escape-free comparison, and
// the gap tracked to bytes.Contains(raw, usageMarker) -- the check
// mayCarryUsage runs FIRST, matching neither line -- not to the
// escape-marker check after it, which matches in a few nanoseconds on its
// own. Under load from other work on a shared machine the ratio is
// noisier than that isolated reading, which is why the bound below is
// generous: it exists to catch a catastrophic regression, not to pin the
// ratio down.
func TestMayCarryUsage_ToolResultHeavyLineStaysBounded(t *testing.T) {
	heavy := toolResultLine(400) // ~8 KB, a realistic single tool_result
	plain := []byte(strings.Repeat("x", len(heavy)))
	const rounds = 2000
	timeIt := func(raw []byte) time.Duration {
		start := time.Now()
		for i := 0; i < rounds; i++ {
			mayCarryUsage(raw)
		}
		return time.Since(start) / rounds
	}
	heavyPerOp := timeIt(heavy)
	plainPerOp := timeIt(plain)
	t.Logf("mayCarryUsage: %v/op on the ANSI-heavy tool_result, %v/op on the same-length escape-free line", heavyPerOp, plainPerOp)
	// Generous on purpose: measured here (go1.25, Apple M3, native) at
	// single-digit microseconds, and under GOARCH=amd64 on this machine
	// (Rosetta, every architecture this module's own tests cross-build
	// for) at low single-digit milliseconds -- emulation overhead, not a
	// defect in mayCarryUsage. The ceiling exists to catch a change that
	// makes this scan unboundedly worse (quadratic in line length, say),
	// not to pin the measured figure down.
	const ceiling = 50 * time.Millisecond
	if heavyPerOp > ceiling {
		t.Errorf("mayCarryUsage took %v on an 8 KB tool_result; want under %v", heavyPerOp, ceiling)
	}
}
