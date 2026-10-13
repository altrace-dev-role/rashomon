package store

import (
	"errors"
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/shape"
)

// worstDeclaration is a declaration with every capped field at its cap,
// filled with '<', which JSON writes as six bytes, and every other field at
// the widest value it can hold.
func worstDeclaration() Declaration {
	esc := func(n int) string { return strings.Repeat("<", n) }
	hosts := func() []string {
		h := make([]string, MaxHosts)
		for i := range h {
			h[i] = esc(MaxHostBytes)
		}
		return h
	}
	id := esc(MaxIDBytes)
	program, digest := esc(MaxProgramBytes), strings.Repeat("f", 64)
	label, masked := "unknown", "masked_by_pipeline"
	argc := 1 << 30
	return Declaration{
		Type: TypeDeclaration, SchemaVersion: SchemaVersion, Seq: 1 << 62, RecordedAtMS: 1 << 62,
		ToolUseID: id, SessionID: id, PromptID: &id, AgentID: &id, AgentType: &id,
		TranscriptPath: esc(MaxPathBytes), PermissionMode: id, ToolName: id,
		Shape: shape.Shape{Program: &program, VerbClass: "execute", Argc: &argc, Digest: digest,
			MayWrite: true, StatusMasked: &masked, RunnerDigest: &digest},
		Hosts: hosts(), SSHHosts: hosts(), CWDDigest: digest, FileLabel: &label,
		HostSource: []string{}, RulesVersion: 1 << 30,
	}
}

// TestREC31_WorstCaseRecordFits: the record bound is derived, not guessed.
// worstCaseRecordBytes is computed from the same caps capture enforces, and a
// record built at every cap, out of the character JSON escapes widest, fits
// inside it; the bound fits under MaxRecordBytes, and that under the reader's
// maxLine, so a record the writer accepts is always one the reader can read.
func TestREC31_WorstCaseRecordFits(t *testing.T) {
	worst := worstCaseRecordBytes()
	if !(worst <= MaxRecordBytes && MaxRecordBytes <= maxLine) {
		t.Fatalf("worstCaseRecordBytes %d <= MaxRecordBytes %d <= maxLine %d does not hold", worst, MaxRecordBytes, maxLine)
	}
	esc := strings.Repeat("<", MaxPathBytes)
	for name, rec := range map[string]any{
		"declaration": worstDeclaration(),
		"execution": Execution{Type: TypeExecution, ToolUseID: esc[:MaxIDBytes], SessionID: esc[:MaxIDBytes],
			ToolName: esc[:MaxIDBytes], ExecutedDigest: strings.Repeat("f", 64)},
		"coverage": Coverage{Type: TypeCoverage, SessionID: esc[:MaxIDBytes], CWD: esc,
			InstallID: strings.Repeat("f", 32), HookEntry: EntryPresentSettings},
	} {
		line, err := marshalLine(rec)
		if err != nil {
			t.Fatalf("%s at every cap was refused: %v", name, err)
		}
		if len(line) > worst {
			t.Errorf("%s at every cap is %d bytes, over the derived worst case %d", name, len(line), worst)
		}
	}
}

// TestREC31_OversizeRecordRefused: a record over MaxRecordBytes is refused
// before it is written, with ErrRecordOversize, so no line the reader would
// have to skip is ever written; the caller records record_oversize. With the
// caps enforced at capture this is unreachable from a hook, and it is the
// guard that keeps it so if a cap is ever missed.
func TestREC31_OversizeRecordRefused(t *testing.T) {
	st, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	d := testDecl("sess-big", "toolu_big")
	d.TranscriptPath = strings.Repeat("p", MaxRecordBytes)
	if err := st.AppendDeclaration(d); !errors.Is(err, ErrRecordOversize) {
		t.Errorf("an oversize declaration: %v, want ErrRecordOversize", err)
	}
	c := Coverage{Type: TypeCoverage, SchemaVersion: SessionSchemaVersion, SessionID: "sess-big", CWD: d.TranscriptPath}
	if err := st.AppendCoverage(c); !errors.Is(err, ErrRecordOversize) {
		t.Errorf("an oversize coverage record: %v, want ErrRecordOversize", err)
	}
	run, err := st.ReadRun("sess-big")
	if err != nil {
		t.Fatal(err)
	}
	if len(run.Declarations)+len(run.Coverage)+run.Skipped != 0 {
		t.Errorf("a refused record left %d declarations, %d coverage records and %d skipped lines",
			len(run.Declarations), len(run.Coverage), run.Skipped)
	}
	// One byte under the limit, with its newline, is written.
	d.TranscriptPath = ""
	base, err := marshalLine(d)
	if err != nil {
		t.Fatal(err)
	}
	d.TranscriptPath = strings.Repeat("p", MaxRecordBytes-len(base))
	if line, err := marshalLine(d); err != nil || len(line) != MaxRecordBytes {
		t.Errorf("a %d-byte line was refused (%v); the limit includes the newline", len(line), err)
	}
	d.TranscriptPath += "p"
	if _, err := marshalLine(d); !errors.Is(err, ErrRecordOversize) {
		t.Errorf("a line one byte over the limit, newline included: %v, want ErrRecordOversize", err)
	}
}

// TestREC52b_OneHostOverItsCapWithholdsTheList: a host list is withheld whole
// when any one entry is over MaxHostBytes, not only when it has too many
// entries -- the per-host bound is the cap's, not the extractor's to keep.
func TestREC52b_OneHostOverItsCapWithholdsTheList(t *testing.T) {
	for _, n := range []int{MaxHostBytes, MaxHostBytes + 1} {
		d := Declaration{Hosts: []string{"a.example", strings.Repeat("h", n)}, SSHHosts: []string{strings.Repeat("s", n)}}
		withheld := CapDeclaration(&d)
		over := n > MaxHostBytes
		if withheld != over || (d.Hosts == nil) != over || (d.SSHHosts == nil) != over {
			t.Errorf("a %d-byte host: withheld %v, hosts %v, ssh_hosts %v", n, withheld, d.Hosts != nil, d.SSHHosts != nil)
		}
	}
}
