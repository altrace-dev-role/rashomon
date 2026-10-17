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
	d := Declaration{
		Type: TypeDeclaration, SchemaVersion: SchemaVersion, Seq: 1 << 62, RecordedAtMS: 1 << 62,
		ToolUseID: id, SessionID: id, PromptID: &id, AgentID: &id, AgentType: &id,
		TranscriptPath: esc(MaxPathBytes), PermissionMode: id, ToolName: id,
		Shape: shape.Shape{Program: &program, VerbClass: "execute", Argc: &argc, Digest: digest,
			MayWrite: true, StatusMasked: &masked, RunnerDigest: &digest},
		Hosts: hosts(), SSHHosts: hosts(), CWDDigest: digest, FileLabel: &label,
		HostSource: []string{}, RulesVersion: 1 << 30,
	}
	d.DeclarationV5 = worstDeclarationV5()
	return d
}

// worstDeclarationV5 is the v5 half of worstDeclaration: the command at its
// cap with every marker span at its widest, every path and written path at
// its cap, every target value at its cap, and the closed-vocabulary fields
// at their longest values.
func worstDeclarationV5() DeclarationV5 {
	esc := func(n int) *string { s := strings.Repeat("<", n); return &s }
	big, yes := 1<<62, true
	spans := make([]MaskSpan, MaxMaskSpans)
	for i := range spans {
		spans[i] = MaskSpan{Rule: strings.Repeat("r", MaxMaskRuleBytes), Start: MaxCommandBytes, End: MaxCommandBytes}
	}
	written := make([]string, MaxWrittenPaths)
	for i := range written {
		written[i] = *esc(MaxWrittenPathsBytes / MaxWrittenPaths)
	}
	reason, via := "masker_error", "post"
	target := Target{
		AWSProfile: esc(MaxTargetValueBytes), AWSRegion: esc(MaxTargetValueBytes), AWSAccount: esc(MaxTargetValueBytes),
		EndpointHost: esc(MaxTargetValueBytes), S3Bucket: esc(MaxTargetValueBytes), KubeContext: esc(MaxTargetValueBytes),
		KubeNamespace: esc(MaxTargetValueBytes), GCPProject: esc(MaxTargetValueBytes), AzureSubscription: esc(MaxTargetValueBytes),
		Source: map[string]string{}, Dropped: []string{},
	}
	for _, k := range []string{"aws_profile", "aws_region", "aws_account", "endpoint_host", "s3_bucket",
		"kube_context", "kube_namespace", "gcp_project", "azure_subscription"} {
		target.Source[k] = "env_prefix"
		target.Dropped = append(target.Dropped, k)
	}
	marks := map[string]int{}
	for _, k := range []string{"skip", "only", "xfail", "nolint", "ts-ignore", "type-ignore", "noqa", "todo"} {
		marks[k] = big
	}
	return DeclarationV5{
		Writer: "1.2.0-rc.10", Command: esc(MaxCommandBytes), CommandBytes: &big,
		CommandCut: &Cut{Offset: big, RemovedBytes: big}, Masked: spans, CommandWithheld: &reason,
		CWD: esc(MaxPathBytes), Project: esc(MaxPathBytes), FilePath: esc(MaxPathBytes),
		WrittenPaths: written, WrittenPathsComplete: &yes, Target: &target, EditMarksBefore: marks,
		WrittenPathsWithheld: &reason, FilePathWithheld: &reason, CWDWithheld: &reason, ProjectWithheld: &reason,
		TargetWithheld: &reason, EditMarksBeforeWithheld: &reason, PromptIDWithheld: &reason,
		AgentIDWithheld: &reason, AgentTypeWithheld: &reason, HostsWithheld: &reason, SSHHostsWithheld: &reason,
		HarnessID: &reason, HarnessToolName: esc(MaxHarnessToolNameBytes), DeclaredVia: &via,
	}
}

// TestREC31_WorstCaseRecordFits: the record bound is derived, not guessed.
// worstCaseRecordBytes is computed from the same caps capture enforces, and a
// record built at every cap, out of the character JSON escapes widest, fits
// inside it; the bound fits under MaxRecordBytes, and that under the reader's
// maxLine, so a record the writer accepts is always one the reader can read.
func TestREC31_WorstCaseRecordFits(t *testing.T) {
	worst := worstCaseRecordBytes()
	// The derivation, spelled out: 6 x (7 ids x 256 + 4 paths x 4,096 +
	// program 256 + hosts 2 x 32 x 253 + command 65,536 + written paths
	// 16,384 + target 9 x 128 + harness tool name 128) + spans 256 x 64 +
	// fixed 4,096.
	if worst != 727424 {
		t.Errorf("worstCaseRecordBytes = %d, want the derived 727,424; a cap moved, so restate the derivation", worst)
	}
	if !(worst <= MaxRecordBytes && MaxRecordBytes <= maxLine) {
		t.Fatalf("worstCaseRecordBytes %d <= MaxRecordBytes %d <= maxLine %d does not hold", worst, MaxRecordBytes, maxLine)
	}
	capped := worstDeclaration()
	if CapDeclaration(&capped) {
		t.Error("the worst-case declaration is not at its caps: CapDeclaration withheld a field of it")
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
		t.Logf("%s at every cap: %d bytes, derived bound %d", name, len(line), worst)
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
	d.RuleMatch = map[string]any{"verdict": strings.Repeat("p", MaxRecordBytes)}
	if err := st.AppendDeclaration(d); !errors.Is(err, ErrRecordOversize) {
		t.Errorf("an oversize declaration: %v, want ErrRecordOversize", err)
	}
	c := Coverage{Type: TypeCoverage, SchemaVersion: SessionSchemaVersion, SessionID: "sess-big", CWD: strings.Repeat("p", MaxRecordBytes)}
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
	d.RuleMatch = nil
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

// TestREC31c_OversizeDegradesThenRefuses: a call record over MaxRecordBytes
// is not refused at once. Its capped fields are withheld, null with their
// *_withheld keys saying oversize, and it is marshalled again; only a record
// still over the limit after that is refused, with nothing written. A
// degraded record lands and the append says so with ErrRecordDegraded, so
// the caller marks the call record_oversize. The caps at capture make this
// unreachable from a hook; it is what keeps a missed cap from losing the
// call.
func TestREC31c_OversizeDegradesThenRefuses(t *testing.T) {
	st, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	huge := strings.Repeat("<", MaxRecordBytes)

	d := testDecl("sess-deg", "toolu_d")
	d.Command, d.CWD = &huge, &huge
	if err := st.AppendDeclaration(d); !errors.Is(err, ErrRecordDegraded) {
		t.Fatalf("an oversize declaration with capped fields: %v, want ErrRecordDegraded", err)
	}
	x := Execution{Type: TypeExecution, SchemaVersion: SchemaVersion, ToolUseID: "toolu_d", SessionID: "sess-deg"}
	x.ExecutedCommand = &huge
	if err := st.AppendExecution(x); !errors.Is(err, ErrRecordDegraded) {
		t.Fatalf("an oversize execution: %v, want ErrRecordDegraded", err)
	}
	if err := st.SpillExecution(x); !errors.Is(err, ErrRecordDegraded) {
		t.Fatalf("an oversize spilled execution: %v, want ErrRecordDegraded", err)
	}
	run, err := st.ReadRun("sess-deg")
	if err != nil {
		t.Fatal(err)
	}
	if len(run.Declarations) != 1 || len(run.Executions) != 2 || run.Skipped != 0 {
		t.Fatalf("read %d declarations, %d executions, %d skipped; want 1, 2, 0",
			len(run.Declarations), len(run.Executions), run.Skipped)
	}
	got := run.Declarations[0]
	if got.Command != nil || withheldAs(got.CommandWithheld) != withheldOversize ||
		got.CWD != nil || withheldAs(got.CWDWithheld) != withheldOversize {
		t.Errorf("degraded declaration: command %v (%q), cwd %v (%q); want both null and oversize",
			got.Command != nil, withheldAs(got.CommandWithheld), got.CWD != nil, withheldAs(got.CWDWithheld))
	}
	if got.ProjectWithheld != nil || got.FilePathWithheld != nil {
		t.Error("a field that held nothing reads as withheld")
	}
	for _, e := range run.Executions {
		if e.ExecutedCommand != nil || withheldAs(e.ExecutedCommandWithheld) != withheldOversize {
			t.Errorf("degraded execution: executed_command kept %v, reason %q", e.ExecutedCommand != nil, withheldAs(e.ExecutedCommandWithheld))
		}
	}

	// Still over after the capped fields went: refused whole.
	r := testDecl("sess-ref", "toolu_r")
	r.RuleMatch = map[string]any{"verdict": huge}
	if err := st.AppendDeclaration(r); !errors.Is(err, ErrRecordOversize) {
		t.Fatalf("a declaration over the limit in an uncapped field: %v, want ErrRecordOversize", err)
	}
	if run, err := st.ReadRun("sess-ref"); err != nil || len(run.Declarations)+run.Skipped != 0 {
		t.Errorf("a refused declaration left something behind (%v)", err)
	}
}
