package harness

import (
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/hookio"
	"github.com/altrace-dev-role/rashomon/test/perf/payload"
)

// BenchmarkDetect pins Detect's own cost against the 0.2ms p99 budget
// Technical spec 6.4 states for it, now that hostguard.Evaluate is a real
// caller (detect.go's own comment used to note no such benchmark existed
// yet to pin that budget).
//
// The 64 KiB fixture is test/perf/payload's Pre class -- an EditPre payload
// at smallShape's upper edge, padded in new_string to exactly 64 KiB, the
// realistic PreToolUse shape above the small-Bash-call size (Pre's own
// doc). Intake runs once, outside the timed loop: this benchmark measures
// Detect alone, the one call hostguard.Evaluate makes per invocation, not
// the jsonscan pass every entry point already pays for and already has its
// own benchmarks (BenchmarkRunHook).
func BenchmarkDetect(b *testing.B) {
	raw, err := payload.Pre(64<<10, "toolu_0100000000000000000000bench1")
	if err != nil {
		b.Fatalf("payload.Pre: %v", err)
	}
	p, err := hookio.Intake(strings.NewReader(string(raw)))
	if err != nil {
		b.Fatalf("hookio.Intake: %v", err)
	}
	// /Users/dev/.claude matches test/perf/payload's own transcript path
	// (payload.go's unexported "transcript" const), so this fixture is read
	// as ClaudeCode by BOTH the entrypoint allowlist and the transcript
	// check, the same double-evidence case BRD1's "claude code cli" row
	// pins in detect_test.go.
	env := Env{Entrypoint: "cli", ConfigDir: "/Users/dev/.claude"}

	b.ReportAllocs()
	b.SetBytes(int64(len(raw)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		id := Detect(p, env)
		if id.ID != ClaudeCodePresumed && id.ID != ClaudeCode {
			b.Fatalf("Detect(fixture) = %s, want a claude identity", id.ID)
		}
	}
}
