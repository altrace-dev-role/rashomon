package acceptance

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/shape"
	"github.com/altrace-dev-role/rashomon/internal/store"
)

// H-88 -- the ceiling holds with a correct omitted count.
//
// Break: cap the fields independently and a turn wide in five dimensions
// exceeds the total while every field is within its own cap.
//
// The fixture is written directly to records.ndjson rather than through N
// subprocess hook/post invocations: what matters here is the SHAPE of the
// records (many distinct tools, many unterminated, many dropped, many
// without an execution, all in one turn), not how they were recorded, and
// writing them directly is the difference between this test taking whole
// seconds and taking whole minutes.
func TestH88_TheCeilingHoldsWithACorrectOmittedCount(t *testing.T) {
	e := newEnv(t)
	e.watched(testSession) // a real install + a PhaseStart coverage record

	// n declarations, each naming a distinct tool, none terminated and none
	// executed: each one counts in Unterminated, WithoutExecution AND
	// ByTool at once. Separately, n terminals matching no declaration at
	// all: Dropped. Wide in four of the five growable fields simultaneously
	// -- ByLabel is excluded on purpose: shape.Labels() is a small, closed
	// vocabulary (7 values), so it can never by itself be the field that
	// blows the ceiling, independently-capped or not.
	const n = 90
	var lines bytes.Buffer
	promptID := "prompt-1"
	var seq int64
	for i := 0; i < n; i++ {
		seq++
		d := store.Declaration{
			Type:           store.TypeDeclaration,
			SchemaVersion:  store.SchemaVersion,
			Seq:            seq,
			ToolUseID:      fmt.Sprintf("toolu_d%04d", i),
			SessionID:      testSession,
			PromptID:       &promptID,
			TranscriptPath: "/tmp/transcripts/sess-1.jsonl",
			PermissionMode: "default",
			ToolName:       fmt.Sprintf("Tool%04d", i), // distinct: widens by_tool
			Shape:          shape.Shape{VerbClass: "other"},
		}
		b, err := json.Marshal(d)
		if err != nil {
			t.Fatal(err)
		}
		lines.Write(b)
		lines.WriteByte('\n')
	}
	for i := 0; i < n; i++ {
		seq++
		term := store.Terminal{
			Type:          store.TypeTerminal,
			SchemaVersion: store.SchemaVersion,
			Seq:           &seq,
			ToolUseID:     fmt.Sprintf("toolu_t%04d", i), // matches no declaration: dropped
			SessionID:     testSession,
			Outcome:       store.OutcomeOK,
		}
		b, err := json.Marshal(term)
		if err != nil {
			t.Fatal(err)
		}
		lines.Write(b)
		lines.WriteByte('\n')
	}

	recordsPath := filepath.Join(e.home, "runs", testSession, "records.ndjson")
	f, err := os.OpenFile(recordsPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.Write(lines.Bytes()); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}

	res := e.digestRaw("--session", testSession, "--prompt", promptID)
	if res.exitCode != 0 {
		t.Fatalf("digest: exit %d, stderr %q", res.exitCode, res.stderr)
	}
	// -1: stdout carries one trailing newline the JSON document itself does
	// not; CapBytes bounds the document, not the line it is printed on.
	if len(res.stdout)-1 > 8<<10 {
		t.Fatalf("digest output is %d bytes, want <= 8192 (CapBytes) + a trailing newline", len(res.stdout))
	}

	var d digestOutput
	if err := json.Unmarshal([]byte(res.stdout), &d); err != nil {
		t.Fatalf("digest output is not JSON: %v\n%s", err, res.stdout)
	}
	if !d.Truncated {
		t.Fatalf("truncated = false for a turn with %d declarations -- each unterminated, "+
			"without an execution, and naming a distinct tool -- plus %d dropped terminals. "+
			"This fixture is built to exceed the ceiling in four dimensions at once.", n, n)
	}
	if d.Declarations.Recorded != n {
		t.Errorf("recorded = %d, want %d: the COUNT is never truncated, only the listings",
			d.Declarations.Recorded, n)
	}
	if got, want := len(d.Declarations.WithoutExecution)+d.Declarations.WithoutExecutionOmitted, n; got != want {
		t.Errorf("without_execution kept+omitted = %d, want %d (nothing invented or lost)", got, want)
	}
	if got, want := len(d.Declarations.Unterminated)+d.Declarations.UnterminatedOmitted, n; got != want {
		t.Errorf("unterminated kept+omitted = %d, want %d", got, want)
	}
	if got, want := len(d.Declarations.Dropped)+d.Declarations.DroppedOmitted, n; got != want {
		t.Errorf("dropped kept+omitted = %d, want %d", got, want)
	}
	if got, want := len(d.Declarations.ByTool)+d.Declarations.ByToolOmitted, n; got != want {
		t.Errorf("by_tool kept+omitted = %d, want %d", got, want)
	}
}
