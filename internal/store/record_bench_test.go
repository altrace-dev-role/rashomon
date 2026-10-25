package store

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/shape"
)

// Field values at the widths a real session writes: a UUID session and prompt
// id, a 30-character tool_use_id, a transcript path under ~/.claude/projects.
const (
	benchSession    = "4b1f0c2e-9a7d-4e3b-8c21-5d6f7a8b9c0d"
	benchPrompt     = "72fea482-07bb-4309-8de3-529636979121"
	benchTranscript = "/Users/dev/.claude/projects/-Users-dev-src-project/" + benchSession + ".jsonl"
	benchCWD        = "/Users/dev/src/project"
	benchStartMS    = 1791380383430

	// benchDurationMS is how long the call ran: its execution and post
	// coverage are recorded that long after its declaration.
	benchDurationMS = 1200

	// benchSpread is how many consecutive declaration lengths the store
	// benchmarks cycle through: at least one whole size-class width at the
	// store's declaration lengths, where the allocator's classes are 128 to
	// 256 B apart (1,408, 1,536, 1,792). A line that fills its class is
	// copied once more by marshalLine's newline append, so a whole cycle pays
	// that at its expected rate, one declaration in 128 to 256, and no single
	// boundary decides a row the way one fixed-length declaration on the
	// 1,408 B edge did, in every op at once. At the gate's 20 iterations a
	// row sees 20 consecutive lengths, so a boundary can still move it by one
	// copy in 20, never by every one.
	benchSpread = 256

	// benchIDSpread is how many consecutive lengths the terminal, the
	// execution and the coverage records cycle through: copy i's tool_use_id
	// is i mod benchIDSpread bytes wider than a real one, and its coverage cwd
	// as many bytes longer. That is one whole size-class width at the
	// execution's length, about 1 KB, where the classes are 128 B apart
	// (1,024, 1,152), and several at the terminal's and the coverage record's,
	// where they are 16 to 32 B apart. The widest id, 30 bytes and 127 more,
	// stays under MaxIDBytes.
	benchIDSpread = 128
)

// benchCall is what one Bash tool call writes, as the hook path builds it: a
// declaration, its terminal and a call coverage record on PreToolUse, then an
// execution and a post coverage record on PostToolUse.
type benchCall struct {
	decl Declaration
	term Terminal
	exec Execution
	cov  Coverage

	// idPad is the zeros this copy puts after toolu_01 in every tool_use_id a
	// benchmark writes for it (benchCalls). It is an any, boxed once here,
	// because a string boxed in the loop allocates, and so does fmt for a
	// zero-padded width past its own scratch buffer: either would count an
	// allocation of the fixture's in the row.
	idPad any
}

// newBenchCall builds bash_short's records, its shape derived under st's key.
func newBenchCall(st *Store) benchCall {
	input := json.RawMessage(`{"command":"git status --short && git diff --stat","description":"Show working tree status"}`)
	s := shape.Derive("Bash", input, st.Key())
	wire, ssh := shape.Hosts("Bash", input)
	prompt := benchPrompt
	duration := int64(benchDurationMS)
	return benchCall{
		decl: Declaration{
			Type: TypeDeclaration, SchemaVersion: SchemaVersion, SessionID: benchSession, PromptID: &prompt,
			TranscriptPath: benchTranscript, PermissionMode: "default", ToolName: "Bash", Shape: s,
			Hosts: wire, SSHHosts: ssh, CWDDigest: shape.CWDDigest(st.Key(), benchCWD), RulesVersion: shape.RulesVersion,
		},
		term: Terminal{Type: TypeTerminal, SchemaVersion: SchemaVersion, SessionID: benchSession, Outcome: OutcomeOK},
		exec: Execution{
			Type: TypeExecution, SchemaVersion: SchemaVersion, SessionID: benchSession, ToolName: "Bash",
			Outcome: ExecOK, DurationMS: &duration, ExecutedDigest: s.Digest,
			StatusMasked: s.StatusMasked, RunnerDigest: s.RunnerDigest, RunnerKind: s.RunnerKind, RulesVersion: shape.RulesVersion,
		},
		cov: Coverage{
			Type: TypeCoverage, SchemaVersion: SessionSchemaVersion, SessionID: benchSession, InstallID: st.InstallID(),
			State: StateVerified, HookEntry: EntryPresentSettings, Probe: ProbeFresh, CWD: benchCWD,
		},
	}
}

// benchCalls is n copies of newBenchCall(st) whose records step through
// consecutive lengths. Copy i pads by i mod benchIDSpread bytes (its pad):
// its tool_use_ids carry that many more zeros (idPad) and its coverage cwd is
// benchCWD with "-" and pad x's after it, which spreads the terminal, the
// execution and the coverage record. Its transcript path is benchTranscript
// with "-" and the other i-pad x's after it, so the declaration, which
// carries both the id and the path, is still i bytes longer than copy 0's: n
// consecutive lengths. Every path stays under MaxPathBytes and every id under
// MaxIDBytes for n up to benchSpread.
func benchCalls(st *Store, n int) []benchCall {
	calls := make([]benchCall, n)
	for i := range calls {
		pad := i % benchIDSpread
		calls[i] = newBenchCall(st)
		calls[i].idPad = strings.Repeat("0", pad)
		calls[i].decl.TranscriptPath = benchTranscript + "-" + strings.Repeat("x", i-pad)
		calls[i].cov.CWD = benchCWD + "-" + strings.Repeat("x", pad)
	}
	return calls
}

// appendBenchCall writes one call's five records through the append API, in
// the order the two hooks write them.
func appendBenchCall(st *Store, c benchCall, id string, at int64) error {
	c.decl.ToolUseID, c.decl.RecordedAtMS = id, at
	c.term.ToolUseID, c.term.RecordedAtMS = id, at
	c.exec.ToolUseID, c.exec.RecordedAtMS = id, at+benchDurationMS
	if err := st.AppendDeclaration(c.decl); err != nil {
		return err
	}
	if err := st.AppendTerminal(c.term); err != nil {
		return err
	}
	c.cov.Phase, c.cov.RecordedAtMS = PhaseCall, at
	if err := st.AppendCoverage(c.cov); err != nil {
		return err
	}
	if err := st.AppendExecution(c.exec); err != nil {
		return err
	}
	c.cov.Phase, c.cov.RecordedAtMS = PhasePost, at+benchDurationMS
	return st.AppendCoverage(c.cov)
}

// benchLines reports a file's size and its line count. A missing file has
// none, as it does to the store's own reader.
func benchLines(b *testing.B, path string) (int, int) {
	b.Helper()
	data, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		b.Fatal(err)
	}
	return len(data), bytes.Count(data, []byte("\n"))
}

// BenchmarkRecordBytesPerCall appends what one tool call writes and reports
// what that leaves on disk per call, records plus coverage, the basis of
// v1.1.0's 1,850 B and of RES-31's 6,000 B budget; records-B/call is
// records.ndjson alone. SetBytes is the same per-call figure. B/call includes
// the bytes benchCalls pads the records by to spread their lengths: 50.5 B a
// call over the gate's 20 calls, 384.5 B over a whole cycle of benchSpread.
func BenchmarkRecordBytesPerCall(b *testing.B) {
	st, err := Open(b.TempDir())
	if err != nil {
		b.Fatal(err)
	}
	calls := benchCalls(st, benchSpread)
	ids := make([]string, b.N)
	for i := range ids {
		ids[i] = fmt.Sprintf("toolu_01%s%022d", calls[i%benchSpread].idPad, i)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := appendBenchCall(st, calls[i%benchSpread], ids[i], benchStartMS+int64(i)); err != nil {
			b.Fatal(err)
		}
	}
	b.StopTimer()

	dir := st.RunDir(benchSession)
	recBytes, recLines := benchLines(b, filepath.Join(dir, FileRecords))
	covBytes, covLines := benchLines(b, filepath.Join(dir, FileCoverage))
	if recLines != 3*b.N || covLines != 2*b.N {
		b.Fatalf("%d record and %d coverage lines after %d calls, want %d and %d", recLines, covLines, b.N, 3*b.N, 2*b.N)
	}
	run, err := st.ReadRun(benchSession)
	if err != nil {
		b.Fatal(err)
	}
	if run.Skipped != 0 || len(run.Declarations) != b.N || len(run.Terminals) != b.N ||
		len(run.Executions) != b.N || len(run.Coverage) != 2*b.N {
		b.Fatalf("read back %d declarations, %d terminals, %d executions, %d coverage, %d skipped; want %d, %d, %d, %d, 0",
			len(run.Declarations), len(run.Terminals), len(run.Executions), len(run.Coverage), run.Skipped, b.N, b.N, b.N, 2*b.N)
	}
	b.SetBytes(int64((recBytes + covBytes) / b.N))
	b.ReportMetric(float64(recBytes+covBytes)/float64(b.N), "B/call")
	b.ReportMetric(float64(recBytes)/float64(b.N), "records-B/call")
}
