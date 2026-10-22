package store

import "errors"

// MaxRecordBytes bounds one record line, its newline included. marshalLine
// refuses anything longer (ErrRecordOversize), so the writer can never
// produce a line the reader would have to skip: it sits above the derived
// worst case of a capped record (worstCaseRecordBytes) and below the
// reader's maxLine, and TestREC31_WorstCaseRecordFits holds the three in
// that order (lld-3, REC-31).
//
// The writer's and reader's bounds used to be set apart from each other: one
// tool call inside the 8 MiB intake limit could produce a 17 MB line, because
// JSON writes `<` as six bytes, and the reader then failed on it.
const MaxRecordBytes = 1 << 20

// The field caps, in raw bytes before JSON escaping, of every value a payload
// hands a record verbatim (1.2.11; REC-52). A value over its cap is withheld
// at capture: null with its *_withheld key saying "oversize" where the field
// is nullable, emptied where a v4 field cannot be null, and in either case
// the call's coverage record carries ReasonRecordOversize. A session id over
// its cap is not withheld but bucketed, by the hook, under its unattributed
// session.
const (
	// MaxIDBytes caps session_id, tool_use_id, prompt_id, agent_id,
	// agent_type, permission_mode and tool_name.
	MaxIDBytes = 256
	// MaxPathBytes caps transcript_path, cwd, project, file_path, a git
	// state's root and the coverage record's cwd: PATH_MAX on Linux, so no
	// path a file can be opened at is cut.
	MaxPathBytes = 4096
	// MaxProgramBytes caps shape.program, a path element (NAME_MAX is 255).
	MaxProgramBytes = 256
	// MaxHosts and MaxHostBytes cap hosts and ssh_hosts: 32 entries, of at
	// most a DNS name's 253 bytes each.
	MaxHosts     = 32
	MaxHostBytes = 253

	// MaxCommandBytes caps command and executed_command, measured on the
	// masked string as stored (1.2.3): capture cuts a longer one to its
	// head and tail windows, so only a command capture failed to cut
	// reaches this cap, and it is withheld.
	MaxCommandBytes = 65536
	// MaxWrittenPaths and MaxWrittenPathsBytes cap written_paths: 64
	// entries and 16 KiB in all, withheld whole over either.
	MaxWrittenPaths      = 64
	MaxWrittenPathsBytes = 16384
	// MaxTargetValueBytes caps each of a target's values; a Target has
	// MaxTargetValues of them.
	MaxTargetValues     = 9
	MaxTargetValueBytes = 128
	// MaxHarnessToolNameBytes caps harness_tool_name, the harness's own
	// tool name, null over it (tech 6.2.1).
	MaxHarnessToolNameBytes = 128
	// MaxMaskSpans caps a command's marker spans, and MaxMaskRuleBytes the
	// rule id each names. A span is a rule id and two offsets into the
	// stored command, which is at most MaxCommandBytes, so each offset is at
	// most five digits. It marshals to at most MaxMaskSpanBytes: the rule id
	// escaped to six bytes a byte (the cap checks its length, not its
	// charset), plus maskSpanFrameBytes.
	MaxMaskSpans     = 256
	MaxMaskRuleBytes = 24
	MaxMaskSpanBytes = MaxMaskRuleBytes*jsonEscapeFactor + maskSpanFrameBytes
)

// maskSpanFrameBytes is a span's bytes besides its rule id:
// {"rule":"","start":65536,"end":65536} and the comma after it in the list.
const maskSpanFrameBytes = 38

// cappedIDs is how many fields MaxIDBytes caps on a declaration, the widest
// record: session_id, tool_use_id, prompt_id, agent_id, agent_type,
// permission_mode and tool_name.
const cappedIDs = 7

// cappedPaths is how many fields MaxPathBytes caps on a declaration:
// transcript_path, cwd, project and file_path.
const cappedPaths = 4

// jsonEscapeFactor is the most bytes json.Marshal writes for one input
// byte: `<`, `>`, `&` and control bytes go out as six, `<` as \u003c.
const jsonEscapeFactor = 6

// fixedRecordBytes bounds everything in a declaration no cap governs: the
// keys, the punctuation around every list entry, the integers, three hex
// digests, and the values this program writes from closed vocabularies --
// the withheld reasons, the target's source and dropped key names, the
// closed mark ids of edit_marks_before. A declaration's keys and fixed
// values come to under 3 KB (TestREC31_WorstCaseRecordFits).
const fixedRecordBytes = 4096

// withheldOversize is the reason a *_withheld key gives for a value over its
// cap (1.2.11).
const withheldOversize = "oversize"

// ErrRecordOversize is a record over MaxRecordBytes, refused before any of it
// was written. The caller records ReasonRecordOversize.
var ErrRecordOversize = errors.New("store: record is over MaxRecordBytes")

// ErrRecordDegraded reports a call record that was over MaxRecordBytes and
// was written after its capped fields were withheld (degrade). The record
// landed: a caller treats the append as done and marks the call
// ReasonRecordOversize, because the record holds less than the call did.
var ErrRecordDegraded = errors.New("store: record was over MaxRecordBytes and was written with its capped fields withheld")

// worstCaseRecordBytes is the longest line a record can marshal to once the
// caps hold (lld-3): every capped field at its cap and every byte of it
// escaped to six, the marker spans at their count and width, plus the fixed
// part. The largest record is a declaration.
//
// It is computed from the constants CapDeclaration enforces, so it moves
// with them. Hosts, target values and span rule ids are counted escaped as
// well, though their extractors keep to charsets that never escape: the caps
// do not check a charset, so the bound does not lean on one.
//
// At today's caps the capped bytes are 7*256 + 4*4,096 + 256 + 2*32*253 +
// 65,536 + 16,384 + 9*128 + 128 = 117,824; escaped, 706,944; with 256 spans
// of 24*6 + 38 = 182 bytes, 753,536; with the fixed part, 757,632, inside the
// 1 MiB line. Technical spec 1.2.11's 639,936 is 117,696 less: it counts the
// hosts and the target values at one byte per byte, which holds only if
// their charsets are checked where they are capped, and they are not; it
// leaves out the harness tool name, which schema 5 added after it; and it
// gives no span width, where a rule id is counted escaped here for the same
// reason as a host.
//
// The bound covers the fields a writer fills today. Some payload-derived
// schema 5 fields have no cap yet because every writer leaves them null:
// marker spans and a cut beside a null command, head_after's branch, a
// target's source and dropped lists, harness_id, declared_via and the keys
// of edit_marks_before. A change that fills one caps it here and counts it
// above; until it does, a record made long by one is refused whole
// (ErrRecordOversize), not degraded.
func worstCaseRecordBytes() int {
	capped := cappedIDs*MaxIDBytes + cappedPaths*MaxPathBytes + MaxProgramBytes +
		2*MaxHosts*MaxHostBytes + MaxCommandBytes + MaxWrittenPathsBytes +
		MaxTargetValues*MaxTargetValueBytes + MaxHarnessToolNameBytes
	return jsonEscapeFactor*capped + MaxMaskSpans*MaxMaskSpanBytes + fixedRecordBytes
}

// CapID is s, or a digest of s when it is over MaxIDBytes: an id as
// CapDeclaration and CapExecution leave it, for a caller that has to name
// the call before its record is built. The digest, not "", is what keeps a
// declaration, its execution and its terminal paired on the same key
// instead of every over-cap tool_use_id colliding through one empty one.
func CapID(s string) string {
	if len(s) > MaxIDBytes {
		return digestID(s)
	}
	return s
}

// CapDeclaration withholds every field of d over its cap, and reports
// whether it withheld any. d.SessionID is the hook's to bucket and is left
// as it is. d.ToolUseID is capped by CapID rather than emptied like the rest
// (withholder.id): it is the key an execution and a terminal join to it by,
// and emptying it over the cap paired every over-cap call through "".
//
// A nullable field over its cap becomes null with its *_withheld key set to
// "oversize" (1.2.11); the v4 strings that cannot be null are emptied, and
// only the call's coverage record says so (ReasonRecordOversize).
func CapDeclaration(d *Declaration) bool {
	var w withholder
	d.ToolUseID = w.id(d.ToolUseID, MaxIDBytes)
	d.PromptID = w.ptr(d.PromptID, MaxIDBytes, &d.PromptIDWithheld)
	d.AgentID = w.ptr(d.AgentID, MaxIDBytes, &d.AgentIDWithheld)
	d.AgentType = w.ptr(d.AgentType, MaxIDBytes, &d.AgentTypeWithheld)
	d.TranscriptPath = w.str(d.TranscriptPath, MaxPathBytes)
	d.PermissionMode = w.str(d.PermissionMode, MaxIDBytes)
	d.ToolName = w.str(d.ToolName, MaxIDBytes)
	d.Shape.Program = w.ptr(d.Shape.Program, MaxProgramBytes, nil)
	d.Hosts = w.hosts(d.Hosts, &d.HostsWithheld)
	d.SSHHosts = w.hosts(d.SSHHosts, &d.SSHHostsWithheld)

	d.Command, d.CommandCut, d.Masked = w.command(d.Command, d.CommandCut, d.Masked, &d.CommandWithheld)
	d.CWD = w.ptr(d.CWD, MaxPathBytes, &d.CWDWithheld)
	d.Project = w.ptr(d.Project, MaxPathBytes, &d.ProjectWithheld)
	d.FilePath = w.ptr(d.FilePath, MaxPathBytes, &d.FilePathWithheld)
	if !w.writtenPathsFit(d.WrittenPaths) {
		incomplete := false
		d.WrittenPaths, d.WrittenPathsComplete = nil, &incomplete
		d.WrittenPathsWithheld = w.oversize()
	}
	if !w.targetFits(d.Target) {
		d.Target, d.TargetWithheld = nil, w.oversize()
	}
	d.HarnessToolName = w.ptr(d.HarnessToolName, MaxHarnessToolNameBytes, nil)
	return w.withheld
}

// CapExecution is CapDeclaration for an execution record.
func CapExecution(x *Execution) bool {
	var w withholder
	x.ToolUseID = w.id(x.ToolUseID, MaxIDBytes)
	x.ToolName = w.str(x.ToolName, MaxIDBytes)
	x.ExecutedCommand, x.ExecutedCommandCut, x.ExecutedMasked = w.command(
		x.ExecutedCommand, x.ExecutedCommandCut, x.ExecutedMasked, &x.ExecutedCommandWithheld)
	if x.HeadAfter != nil {
		x.HeadAfter.Root = w.ptr(x.HeadAfter.Root, MaxPathBytes, &x.HeadAfter.RootWithheld)
	}
	x.HarnessToolName = w.ptr(x.HarnessToolName, MaxHarnessToolNameBytes, nil)
	return w.withheld
}

// degrader is a record that can withhold its capped fields to fit under
// MaxRecordBytes: the call records, the only ones that carry free text from
// a payload.
type degrader interface {
	// degraded is the record with every capped free-text field that holds a
	// value withheld, and whether there was any to withhold.
	degraded() (any, bool)
}

// degraded withholds every capped free-text field of a declaration that
// holds a value: null with its *_withheld key saying oversize where it has
// one, empty where a v4 string cannot be null (transcript_path). The join
// keys (session_id, tool_use_id) and the closed or id-sized fields stay.
func (d Declaration) degraded() (any, bool) {
	var w withholder
	d.PromptID = w.drop(d.PromptID, &d.PromptIDWithheld)
	d.AgentID = w.drop(d.AgentID, &d.AgentIDWithheld)
	d.AgentType = w.drop(d.AgentType, &d.AgentTypeWithheld)
	d.TranscriptPath = w.str(d.TranscriptPath, 0)
	d.Shape.Program = w.drop(d.Shape.Program, nil)
	if d.Hosts != nil {
		d.Hosts, d.HostsWithheld = nil, w.oversize()
	}
	if d.SSHHosts != nil {
		d.SSHHosts, d.SSHHostsWithheld = nil, w.oversize()
	}
	if d.Command != nil {
		d.Command, d.CommandCut, d.Masked, d.CommandWithheld = nil, nil, nil, w.oversize()
	}
	d.CWD = w.drop(d.CWD, &d.CWDWithheld)
	d.Project = w.drop(d.Project, &d.ProjectWithheld)
	d.FilePath = w.drop(d.FilePath, &d.FilePathWithheld)
	if d.WrittenPaths != nil {
		incomplete := false
		d.WrittenPaths, d.WrittenPathsComplete, d.WrittenPathsWithheld = nil, &incomplete, w.oversize()
	}
	if d.Target != nil {
		d.Target, d.TargetWithheld = nil, w.oversize()
	}
	d.HarnessToolName = w.drop(d.HarnessToolName, nil)
	return d, w.withheld
}

// degraded is Declaration.degraded for an execution.
func (x Execution) degraded() (any, bool) {
	var w withholder
	if x.ExecutedCommand != nil {
		x.ExecutedCommand, x.ExecutedCommandCut, x.ExecutedMasked = nil, nil, nil
		x.ExecutedCommandWithheld = w.oversize()
	}
	if x.HeadAfter != nil && x.HeadAfter.Root != nil {
		head := *x.HeadAfter
		head.Root, head.RootWithheld = nil, w.oversize()
		x.HeadAfter = &head
	}
	x.HarnessToolName = w.drop(x.HarnessToolName, nil)
	return x, w.withheld
}

// drop is ptr with no cap: any value is withheld.
func (w *withholder) drop(s *string, key **string) *string {
	return w.ptr(s, -1, key)
}

// withholder applies the caps and remembers whether any value was over one.
type withholder struct{ withheld bool }

func (w *withholder) str(s string, limit int) string {
	if len(s) > limit {
		w.withheld = true
		return ""
	}
	return s
}

// id is str for a value a reader joins records by: over limit, it becomes a
// digest of itself rather than "", so every over-cap value does not read as
// the same one.
func (w *withholder) id(s string, limit int) string {
	if len(s) > limit {
		w.withheld = true
		return digestID(s)
	}
	return s
}

// ptr is str for a nullable field: over limit it is null, and its
// *_withheld key, when it has one, says oversize.
func (w *withholder) ptr(s *string, limit int, key **string) *string {
	if s != nil && len(*s) > limit {
		if key != nil {
			*key = w.oversize()
		} else {
			w.withheld = true
		}
		return nil
	}
	return s
}

// oversize marks a value withheld and is the reason its *_withheld key
// carries.
func (w *withholder) oversize() *string {
	w.withheld = true
	reason := withheldOversize
	return &reason
}

// command withholds a stored command, with its cut and its marker spans,
// when it is over MaxCommandBytes or when its spans cannot describe it: more
// than MaxMaskSpans of them, a rule id over MaxMaskRuleBytes, or offsets
// outside the command. A span is the evidence of what was masked, and a
// command stored beside spans that misdescribe it is a command whose
// masking nothing can check, so it is not stored at all.
func (w *withholder) command(cmd *string, cut *Cut, spans []MaskSpan, key **string) (*string, *Cut, []MaskSpan) {
	if cmd == nil {
		return cmd, cut, spans
	}
	fits := len(*cmd) <= MaxCommandBytes && len(spans) <= MaxMaskSpans
	for _, sp := range spans {
		if len(sp.Rule) > MaxMaskRuleBytes || sp.Start < 0 || sp.Start > sp.End || sp.End > len(*cmd) {
			fits = false
		}
	}
	if fits {
		return cmd, cut, spans
	}
	*key = w.oversize()
	return nil, nil, nil
}

// writtenPathsFit reports whether a written_paths list is within its caps:
// at most MaxWrittenPaths entries and MaxWrittenPathsBytes in all.
func (w *withholder) writtenPathsFit(list []string) bool {
	if len(list) > MaxWrittenPaths {
		return false
	}
	total := 0
	for _, p := range list {
		total += len(p)
	}
	return total <= MaxWrittenPathsBytes
}

// targetFits reports whether every value of a target is within
// MaxTargetValueBytes. A target is withheld whole rather than with one value
// dropped: a dropped value is a check's verdict, listed in Dropped, and a
// value cut by a cap must not read as one a check refused.
func (w *withholder) targetFits(t *Target) bool {
	if t == nil {
		return true
	}
	for _, v := range []*string{t.AWSProfile, t.AWSRegion, t.AWSAccount, t.EndpointHost, t.S3Bucket,
		t.KubeContext, t.KubeNamespace, t.GCPProject, t.AzureSubscription} {
		if v != nil && len(*v) > MaxTargetValueBytes {
			return false
		}
	}
	return true
}

// hosts withholds a host list whole -- null, never a shortened list -- when
// it has more than MaxHosts entries or any entry over MaxHostBytes. A list
// that dropped some hosts would read as a complete one, and a host the call
// named but the list lost would read as reached and never named.
func (w *withholder) hosts(list []string, key **string) []string {
	over := len(list) > MaxHosts
	for _, h := range list {
		if len(h) > MaxHostBytes {
			over = true
		}
	}
	if over {
		*key = w.oversize()
		return nil
	}
	return list
}
