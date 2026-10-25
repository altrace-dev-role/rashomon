package hook

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/altrace-dev-role/rashomon/internal/fault"
	"github.com/altrace-dev-role/rashomon/internal/hookio"
	"github.com/altrace-dev-role/rashomon/internal/shape"
	"github.com/altrace-dev-role/rashomon/internal/store"
)

// MaxPayloadBytes bounds how much of stdin is read.
//
// This is a panic-barrier concern, not a tidiness one. Go's response to
// exhausting memory is a runtime fatal error, which recover cannot catch and
// which exits 2 -- and exit 2 from a PreToolUse hook blocks the user's tool
// call. An unbounded read over an attacker-influenced tool_input is therefore
// a way to turn this recorder into an enforcer. hookio.Read enforces the
// limit; a refusal is recorded as payload_too_large.
//
// The limit bounds the bytes read, not the work done on them, and alone it
// does not close that path: 8 MiB of one-letter words is four million shell
// words, and lexed whole the shape of that one line allocated 4.5 GB and
// drove a hook to about 1.07 GB of RSS. Each reader of the line bounds its
// own work: the shape package stops lexing at its token bound (RES-9), keeps
// no more continuation offsets than that and reads ssh lines to the same
// bound, takes a plain word as a slice of the line, and decodes the command
// once per reader into a buffer of its size (RES-29); the leading-cd fold
// below stops at maxLeadingCd (RES-10). That is measured, not proved: on the
// sixteen 8 MiB line shapes TestRES29_ShapeOfEveryPathologicalLineIsBounded
// reads, no reader allocates more than five times the payload, and the hook
// entry stays within RES-29's 64 MiB of RSS on each, which
// cmd/rashomon's TestRES29_PathologicalInputsStayBounded pins (59.5 MiB at
// most over three runs, the here-document and the substitutions).
const MaxPayloadBytes = hookio.MaxPayloadBytes

// maxLeadingCd bounds the leading-cd fold in startDirectory: see
// shape.MaxLeadingCd, whose bound the masked-status walk shares.
const maxLeadingCd = shape.MaxLeadingCd

// LockWaitTotal is how long one invocation may wait for store locks, all of
// its appends together (store.Store.LimitLockWaits). The hook timeout is five
// seconds (install.Timeout), and three of them leave two for starting, the
// payload, the flush and exiting, which measure in milliseconds. It is at
// least one append's own two-second budget, so a declaration still waits its
// whole budget, and the terminal and coverage record after it share what is
// left rather than each waiting two more seconds, which could add up to six
// (perf-2). install.Timeout itself stays: an install written with any other
// value reads as edited by hand.
const LockWaitTotal = 3 * time.Second

// Handler captures one tool call and then closes it out.
//
// Capture and Close are separate because Close has to run on paths where
// Capture did not return normally. Once a declaration is on disk it must be
// followed by a terminal record on every exit this process controls, and a
// single function cannot promise that about its own panic.
//
// A Handler is for one invocation, used by one goroutine: Capture fills the
// fields Close reads, and its Store is deferred (Store.DeferDurability).
type Handler struct {
	st  *store.Store
	now func() time.Time

	// lockWaits is LockWaitTotal, which a test shortens.
	lockWaits time.Duration
	// capDecl is store.CapDeclaration, which a test replaces to hand the
	// store a record that skipped the caps, the one way to reach its degrade.
	capDecl func(*store.Declaration) bool

	// HarnessID, when non-empty, is stamped on the declaration as harness_id.
	// The caller sets it to "unknown" for an Unknown detection, which this
	// build records rather than stands down on (BRD-4 ruling, 2026-10-24: stand
	// down on positive foreign evidence only; Unknown is recorded and counted);
	// it stays null otherwise.
	HarnessID string

	sessionID string
	toolUseID string
	cwd       string
	opened    bool
	// withheld is set when a payload value was over its cap and left out of
	// the declaration (store.CapDeclaration); the call's coverage record
	// then carries record_oversize.
	withheld bool
	// labelRecovered is set when fileLabel recovered a panic. The label
	// then reads unknown, and Close names the panic in the coverage record.
	labelRecovered bool
}

// Writer is the version of the binary writing the records, stamped on every
// call record as writer, so a reader can tell which release wrote a record
// as well as which schema it follows. cmd/rashomon sets it from its
// build-time version before any hook runs; "dev" is an unreleased build.
var Writer = "dev"

// New builds a Handler. now is injectable so a test can pin the clock and
// compare two records byte for byte.
func New(st *store.Store, now func() time.Time) *Handler {
	return &Handler{st: st, now: now, lockWaits: LockWaitTotal, capDecl: store.CapDeclaration,
		sessionID: store.UnattributedSession}
}

// Capture reads one payload and writes the declaration.
func (h *Handler) Capture(in io.Reader) error {
	h.prologue()
	payload, err := hookio.Intake(in)
	if err != nil {
		if s, cwd, ok := refusedSession(err); ok {
			h.sessionID, h.cwd = s, cwd
		}
		return err
	}
	return h.capture(payload)
}

// CapturePayload is Capture for a caller that has already read and parsed
// its payload into a *hookio.Payload -- cmd/rashomon's host-guard check,
// which hands this the exact value internal/hostguard.EvaluateRaw built,
// so the already-decided bytes are never read, scanned or copied a second
// time. It shares the same lock-wait and fault-injection prologue Capture
// does; only the read in between is skipped.
func (h *Handler) CapturePayload(payload *hookio.Payload) error {
	h.prologue()
	return h.capture(payload)
}

// prologue is Capture and CapturePayload's shared setup, run first so that
// Close's appends share the deadline too: there is exactly one of this
// pair's lock-wait deadline and PointHookStart fault injection, which is
// what perf-2's and H-17's own mutants pin against -- a caller that reaches
// capture by any other route must still have run this exactly once.
func (h *Handler) prologue() {
	h.st.LimitLockWaits(h.lockWaits)
	fault.Inject(fault.PointHookStart)
}

// capture is Capture and CapturePayload's shared body, once each has its
// own *hookio.Payload in hand.
func (h *Handler) capture(payload *hookio.Payload) error {
	var p Payload
	if err := p.from(payload); err != nil {
		return err
	}

	// Attribute before anything else can fail. A fault after this point is
	// still recorded against the session that produced it; a payload too
	// malformed to name its session, or one that names none, goes to the
	// unattributed bucket. The second kind still gets its declaration: it is
	// the evidence that something other than a Claude Code session is running
	// these hooks, and the report counts it. Its post, by contrast, records
	// no execution -- see Post.Capture for why an outcome is the one thing a
	// sessionless payload cannot be trusted to carry.
	h.sessionID = attribute(p.SessionID)
	h.withheld = len(p.SessionID) > store.MaxIDBytes
	h.toolUseID = store.CapID(p.ToolUseID)
	h.cwd = p.CWD

	fault.Inject(fault.PointHookParsed)

	decl := store.Declaration{
		Type:           store.TypeDeclaration,
		SchemaVersion:  store.SchemaVersion,
		RecordedAtMS:   h.now().UnixMilli(),
		ToolUseID:      p.ToolUseID,
		SessionID:      h.sessionID,
		PromptID:       nilIfEmpty(p.PromptID),
		AgentID:        nilIfEmpty(p.AgentID),
		AgentType:      nilIfEmpty(p.AgentType),
		TranscriptPath: p.TranscriptPath,
		PermissionMode: p.PermissionMode,
		ToolName:       p.ToolName,
		RulesVersion:   shape.RulesVersion,
	}
	decl.Shape, decl.CWDDigest = Locate(h.st.Key(), p.ToolName, p.ToolInput, p.CWD)
	decl.Writer = Writer
	decl.HarnessID = nilIfEmpty(h.HarnessID)
	// Hostnames are metadata and are stored verbatim, because the report
	// lists them and forget --host matches them by the name as written.
	// Nothing else from tool_input is persisted, and shape is still the only
	// package that reads inside it.
	decl.Hosts, decl.SSHHosts = shape.Hosts(p.ToolName, p.ToolInput)

	// The label is computed on every declaration, and attached only to a
	// record whose schema carries the field.
	//
	// Computed unconditionally on purpose: the recover inside fileLabel runs
	// on every call, so the path H-46 exercises is the path that runs in the
	// field, not one that only wakes up when the schema moves.
	//
	// Written unconditionally now. The Phase B reservation declares
	// file_label in the record and in docs/store-schema.json, where it is
	// permitted at every version and required only at schema 3, so there is
	// no longer a version to gate on: the field is always present, and this
	// decides whether it is null or a label.
	label, recovered := fileLabel(p.ToolName, p.ToolInput)
	if label != "" {
		decl.FileLabel = &label
	}
	h.labelRecovered = recovered
	// The caps, after every field is filled: a value over its cap is left
	// out of the record rather than cut, and said so on the coverage record
	// (REC-52), so no payload inside the intake limit can make a record the
	// store would refuse.
	if h.capDecl(&decl) {
		h.withheld = true
	}

	fault.Inject(fault.PointStoreWrite)

	if err := landed(h.st.AppendDeclaration(decl), &h.withheld); err != nil {
		return err
	}
	h.opened = true

	fault.Inject(fault.PointHookAfterDeclaration)
	return nil
}

// Close writes the terminal record for a captured declaration and the coverage
// record for this invocation. It is called on every path Capture can leave by,
// including a recovered panic.
//
// sig reports signal delivery. SIGTERM is what a hook timeout cancellation
// delivers and it is catchable, so it is a controlled exit and gets a terminal
// record. SIGKILL is not catchable: nothing runs, no terminal record appears,
// and that absence is what the end-of-run probe reads as an unterminated entry.
func (h *Handler) Close(sig *Signals, captureErr error) {
	outcome, reason := store.OutcomeOK, ""
	switch {
	case sig.Delivered():
		outcome, reason = store.OutcomeSignal, store.ReasonTerminatedBySignal
	case captureErr != nil:
		outcome, reason = store.OutcomeError, closeReason(captureErr)
	}

	// A terminal closes a declaration that landed, whatever its id. It is also
	// written when the declaration itself could not be written for want of the
	// lock: it then carries the only trace of that tool_use_id, and goes
	// straight to the spill file -- the ordered stream's lock has already cost
	// its full budget once, and a second wait would push the handler past the
	// hook timeout.
	if h.opened || (h.toolUseID != "" && reason == store.ReasonLockTimeout) {
		// A call record, at its declaration's version: a reader that skips
		// the one must skip the other (store.SessionSchemaVersion).
		term := store.Terminal{
			Type:          store.TypeTerminal,
			SchemaVersion: store.SchemaVersion,
			RecordedAtMS:  h.now().UnixMilli(),
			ToolUseID:     h.toolUseID,
			SessionID:     h.sessionID,
			Outcome:       outcome,
			Reason:        nilIfEmpty(reason),
			TerminalV5:    store.TerminalV5{Writer: Writer},
		}
		var err error
		if reason == store.ReasonLockTimeout {
			err = h.st.SpillTerminal(term)
		} else {
			err = h.st.AppendTerminal(term)
		}
		// A call whose terminal did not land must not be covered by a
		// verified record: the reason was fixed before this write, and the
		// end probe finds the declaration unterminated only if the session
		// ever ends. A reason already set says unverified on its own.
		if err != nil && reason == "" {
			reason = store.ReasonInternalError
		}
	}

	if reason == "" && h.withheld {
		reason = store.ReasonRecordOversize
	}
	// A panic recovered inside the label derivation cost the label and not
	// the call, so the terminal above stays ok; the coverage record is where
	// it is named, unless something that cost more already is. BuildCoverage
	// is given reason, not a pre-substituted one, so it gets the chance to
	// resolve hook_entry_absent and the rest first; recovered_panic only
	// takes the record's empty slot, never one BuildCoverage already filled.
	cov := BuildCoverage(h.st, h.sessionID, store.PhaseCall, reason, h.cwd, h.now())
	if cov.Reason == nil && h.labelRecovered {
		cov.Reason = nilIfEmpty(store.ReasonRecoveredPanic)
		cov.State = store.StateUnverified
	}
	// A call-phase coverage record that cannot be written is read as its
	// absence: the report counts calls against their coverage (H-10), and a
	// call with none never reads as covered.
	_ = h.st.AppendCoverage(cov)
}

// Locate is a call's shape and its location, the cwd digest the hook
// records: Capture's, and what a test that builds records the way the hook
// does calls. One reading of the command, shape.DeriveWithDirs, gives the
// shape and the leading cd steps, so the line is not decoded and tokenized a
// second time for them. The location is one of three:
//
//   - the leading steps LeadingDirectory reads, folded onto the payload's cwd
//     (startDirectory), `~` expanded with the hook's own home;
//   - for steps the program search steps over and LeadingDirectory does not
//     read, shape.UnreadCWDDigest of the payload's cwd and the steps, a
//     location no read directory's digest equals;
//   - "", unknown, where there is no payload cwd to resolve a relative step
//     against or to key unread steps on (keyed on the steps alone, one line
//     run in two projects would be one location), or past maxLeadingCd read
//     steps.
//
// With no steps it is the payload's cwd.
func Locate(key []byte, toolName string, toolInput json.RawMessage, cwd string) (shape.Shape, string) {
	s, _, dirs, unread := shape.DeriveWithDirs(toolName, toolInput, key)
	if unread != "" {
		if cwd == "" {
			return s, ""
		}
		return s, shape.UnreadCWDDigest(key, cwd, unread)
	}
	return s, shape.CWDDigest(key, startDirectory(cwd, dirs))
}

// startDirectory is the directory a call's command starts in: the payload's
// cwd, which is the shell's directory before the line runs, with the targets
// of the line's leading plain `cd DIR &&` steps (shape.LeadingDirectory's
// own answer, dirs) folded onto it. An absolute target replaces the
// directory and a relative one joins it. Without it the first
// `cd /repo/web && go test ./...` is keyed on /repo and every repeat on
// /repo/web, although all of them ran in web. A relative target with no
// directory to resolve it against is not known, and the directory stays
// empty until an absolute target names one.
//
// `~` and `~/...` are the home, an absolute target: the hook's own home
// (os.UserHomeDir), never the line's environment, which the hook does not
// see. Claude Code's Bash tool keeps only the working directory from one call
// to the next, not the environment, so no earlier call's export reaches a
// later `cd ~`; on the same line, `HOME=x cd ~` is a step LeadingDirectory
// refuses, and an export before the cd ends the leading run. A sandboxed tool
// whose environment differs from the hook process's is the case this does
// not see. The home is only an input to CWDDigest: no path is written. With
// no home to expand it with, the directory is not known.
//
// dirs, not toolName and toolInput: the caller already has
// shape.DeriveWithDirs's dirs, from the SAME read of the program search
// Derive makes for every other field of the shape. Decoding the command and
// tokenizing it again here, as a toolName/toolInput signature would need to
// call shape.LeadingDirectory on its own, was a second and a third reading
// of a line Hosts then reads once more -- the budget this package derives
// (RES-9) assumes three readings, not at least five.
//
// Past maxLeadingCd steps the directory is not known, and is returned empty,
// which CWDDigest records as unknown: not the payload's cwd, which the cds
// moved away from, and not a fold stopped short of where the line ran.
func startDirectory(cwd string, dirs []string) string {
	if len(dirs) > maxLeadingCd {
		return ""
	}
	for _, dir := range dirs {
		switch {
		case dir == "~" || strings.HasPrefix(dir, "~/"):
			home, err := os.UserHomeDir()
			if err != nil || !filepath.IsAbs(home) {
				return ""
			}
			cwd = filepath.Join(home, dir[1:])
		case filepath.IsAbs(dir):
			cwd = filepath.Clean(dir)
		case cwd != "":
			cwd = filepath.Join(cwd, dir)
		}
	}
	return cwd
}

func nilIfEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// fileLabel derives the label under its own recover.
//
// The recover is inside this function, and therefore before the declaration is
// appended, for the reason H-46 names: a panic in labelling must cost the label
// and nothing else. Recovering further out -- in the barrier that wraps Capture
// -- would lose the record itself, and a missing declaration is a far worse
// answer than an unknown label.
//
// The empty string means the tool names no file and the record's field stays
// null. A panic means the label could not be derived: the label is then
// LabelUnknown and recovered is true. LabelUnknown alone cannot say so,
// because shape.Label also returns it for a path field that is absent,
// non-string or empty; recovered is what lets Close name the panic in the
// coverage record. No new label value carries it, because the report clamps any
// label outside shape.Labels() to unknown.
func fileLabel(toolName string, toolInput json.RawMessage) (label string, recovered bool) {
	defer func() {
		if v := recover(); v != nil {
			label, recovered = shape.LabelUnknown, true
		}
	}()
	fault.Inject(fault.PointLabel)
	return shape.Label(toolName, toolInput), false
}
