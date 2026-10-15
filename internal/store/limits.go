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
// hands a record verbatim (REC-52). A value over its cap is withheld at
// capture: emptied, or null where the field is nullable, and the call's
// coverage record carries ReasonRecordOversize, because a v4 record has no
// key of its own to say a field was cut. A session id over its cap is not
// withheld but bucketed, by the hook, under its unattributed session.
const (
	// MaxIDBytes caps session_id, tool_use_id, prompt_id, agent_id,
	// agent_type, permission_mode and tool_name.
	MaxIDBytes = 256
	// MaxPathBytes caps transcript_path and the coverage record's cwd:
	// PATH_MAX on Linux, so no path a file can be opened at is cut.
	MaxPathBytes = 4096
	// MaxProgramBytes caps shape.program, a path element (NAME_MAX is 255).
	MaxProgramBytes = 256
	// MaxHosts and MaxHostBytes cap hosts and ssh_hosts: 32 entries, of at
	// most a DNS name's 253 bytes each.
	MaxHosts     = 32
	MaxHostBytes = 253
)

// cappedIDs is how many fields MaxIDBytes caps on a declaration, the widest
// record: the seven named on it.
const cappedIDs = 7

// jsonEscapeFactor is the most bytes json.Marshal writes for one input
// byte: `<`, `>`, `&` and control bytes go out as six, `<` as \u003c.
const jsonEscapeFactor = 6

// fixedRecordBytes bounds everything in a declaration no cap governs: the
// keys, the punctuation, the integers, three hex digests and the closed
// vocabularies. A declaration's keys and fixed values come to under 1.2 KB.
const fixedRecordBytes = 4096

// ErrRecordOversize is a record over MaxRecordBytes, refused before any of it
// was written. The caller records ReasonRecordOversize.
var ErrRecordOversize = errors.New("store: record is over MaxRecordBytes")

// worstCaseRecordBytes is the longest line a record can marshal to once the
// caps hold: every capped field at its cap, every byte of it escaped to six,
// plus the fixed part. It is computed from the constants capture enforces,
// so it moves with them.
func worstCaseRecordBytes() int {
	capped := cappedIDs*MaxIDBytes + MaxPathBytes + MaxProgramBytes + 2*MaxHosts*MaxHostBytes
	return jsonEscapeFactor*capped + fixedRecordBytes
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
func CapDeclaration(d *Declaration) bool {
	var w withholder
	d.ToolUseID = w.id(d.ToolUseID, MaxIDBytes)
	d.PromptID = w.ptr(d.PromptID, MaxIDBytes)
	d.AgentID = w.ptr(d.AgentID, MaxIDBytes)
	d.AgentType = w.ptr(d.AgentType, MaxIDBytes)
	d.TranscriptPath = w.str(d.TranscriptPath, MaxPathBytes)
	d.PermissionMode = w.str(d.PermissionMode, MaxIDBytes)
	d.ToolName = w.str(d.ToolName, MaxIDBytes)
	d.Shape.Program = w.ptr(d.Shape.Program, MaxProgramBytes)
	d.Hosts = w.hosts(d.Hosts)
	d.SSHHosts = w.hosts(d.SSHHosts)
	return w.withheld
}

// CapExecution is CapDeclaration for an execution record.
func CapExecution(x *Execution) bool {
	var w withholder
	x.ToolUseID = w.id(x.ToolUseID, MaxIDBytes)
	x.ToolName = w.str(x.ToolName, MaxIDBytes)
	return w.withheld
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

func (w *withholder) ptr(s *string, limit int) *string {
	if s != nil && len(*s) > limit {
		w.withheld = true
		return nil
	}
	return s
}

// hosts withholds a host list whole -- null, never a shortened list -- when
// it has more than MaxHosts entries or any entry over MaxHostBytes. A list
// that dropped some hosts would read as a complete one, and a host the call
// named but the list lost would read as reached and never named.
func (w *withholder) hosts(list []string) []string {
	if len(list) > MaxHosts {
		w.withheld = true
		return nil
	}
	for _, h := range list {
		if len(h) > MaxHostBytes {
			w.withheld = true
			return nil
		}
	}
	return list
}
