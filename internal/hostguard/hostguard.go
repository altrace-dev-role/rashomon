// Package hostguard decides, for one hook invocation, whether the harness
// that is actually calling matches the harness its entry was installed for
// (Technical spec 6.2.3, "H1 stand-down"; BRD-4). It is the one caller of
// harness.Detect outside harness's own tests (Technical spec 4.2.9: "H1 is
// BREADTH's harness.Detect(raw, env), called through internal/hostguard"),
// so every entry point asks the SAME question the same way; none of them
// re-derives a mismatch from Detect's fields itself.
//
// This package decides. It does not read stdin, open a store, or write
// anything: internal/store's harness-counts file is a separate concern a
// caller reaches only after Evaluate says to stand down, and cmd/rashomon
// is the only caller that touches an actual OS pipe. That split is what
// lets Evaluate be tested with plain byte slices, the same way
// harness.Detect itself is tested.
package hostguard

import (
	"github.com/altrace-dev-role/rashomon/internal/harness"
	"github.com/altrace-dev-role/rashomon/internal/hookio"
)

// Family is the installer identity: which of rashomon's own install paths
// placed an entry (6.2.3: "Cursor entries run '<exe>' cursor --install
// <id>', Codex '<exe>' codex --install <id>'; Claude entries are
// unchanged"). It is the "installed_for" half of a harness_counts.json key
// (6.2.1). Exactly three values exist; only FamilyClaude currently names
// any entry this tree installs, because no Cursor or Codex adapter exists
// yet (6.2.5 is future work) -- see internal/harness/caps.go's own note on
// the same gap.
type Family string

const (
	// FamilyClaude is "claude --install <id>": every entry this tree
	// installs today.
	FamilyClaude Family = "claude"
	// FamilyCursor is "cursor --install <id>": no adapter exists yet to
	// install one (6.2.5 is future work).
	FamilyCursor Family = "cursor"
	// FamilyCodex is "codex --install <id>": no adapter exists yet to
	// install one (6.2.5 is future work).
	FamilyCodex Family = "codex"
)

// familyOf maps a Detect'd Identity.ID to the family StandsDown compares
// against. harness.ID is a closed, five-value set (ClaudeCode,
// ClaudeCodePresumed, Cursor, Codex, Unknown); the two Claude values share
// one family (a positive detection and a presumed one are both "this is
// Claude Code", just with different confidence), and Unknown belongs to no
// family that was ever installed-for, so it can never equal an
// installedFor value -- an Unknown detection always mismatches, which is
// exactly BRD-4's "foreign OR UNKNOWN harnesses" rule.
func familyOf(id harness.ID) string {
	switch id {
	case harness.ClaudeCode, harness.ClaudeCodePresumed:
		return string(FamilyClaude)
	case harness.Cursor:
		return string(FamilyCursor)
	case harness.Codex:
		return string(FamilyCodex)
	default:
		return "unknown"
	}
}

// StandsDown reports whether an entry installed for installedFor must stand
// down given what Detect decided about this invocation (6.2.3, BRD-4): true
// on any mismatch, Unknown included. It never reads id.Evidence or
// id.Surface -- only id.ID, the one field a family is defined over -- so a
// caller cannot accidentally widen or narrow the comparison by passing a
// differently-shaped Identity than Detect itself produces.
func StandsDown(installedFor Family, id harness.Identity) bool {
	return string(installedFor) != familyOf(id.ID)
}

// CounterKey is the harness_counts.json key a stand-down moves:
// "<installed_for>@<detected>" (6.2.1), where <detected> is Detect's own ID
// string verbatim (not the family) -- so a Claude entry under an unknown
// host keys as "claude@unknown", matching the spec's own example, and a
// future Cursor adapter under Codex keys as "cursor@codex" without this
// function changing.
func CounterKey(installedFor Family, id harness.Identity) string {
	return string(installedFor) + "@" + string(id.ID)
}

// Evaluate is what every Claude entry point that reads a buffered payload
// (hook, post, probe) calls: it runs harness.Detect on raw -- the exact
// bytes hookio.Read already produced from stdin -- and decides whether an
// entry installed for installedFor must stand down.
//
// readErr is hookio.Read's own error reading those same bytes, if reading
// stdin failed before raw could be produced (e.g. hookio.ErrTooLarge). A
// non-nil readErr always reports standsDown=false: a payload this process
// could not even buffer carries no POSITIVE evidence of a foreign harness,
// only the absence of any evidence, and treating "unreadable" the same as
// "foreign" would turn a merely oversized Claude Code payload into a silent
// stand-down -- discarding the payload_too_large coverage reason the
// downstream Capture call already writes for exactly this case today. The
// caller is expected to go on to call the SAME downstream function with a
// reader that reproduces readErr, so that function's own, already-tested
// error handling is the only thing that ever runs on this path -- see
// cmd/rashomon's replay helper.
//
// A payload hookio.Intake itself refuses (malformed JSON, a key of the
// wrong type) gets the same treatment, for the same reason: Detect was
// never reached, so there is nothing to compare, and the caller's
// downstream re-scan of the identical bytes will refuse it identically.
func Evaluate(raw []byte, readErr error, env harness.Env, installedFor Family) (harness.Identity, bool) {
	_, id, down := EvaluateRaw(raw, readErr, env, installedFor)
	return id, down
}

// EvaluateRaw is Evaluate's payload-returning twin: the same decision, plus
// the *hookio.Payload it was decided from (nil whenever Evaluate would have
// returned the zero Identity). cmd/rashomon's hook/post/probe entries use
// this instead of Evaluate for two reasons: a field they also need from the
// SAME payload -- session_id, for harness_counts.json's dedup key -- costs
// no second scan of raw, AND the returned *Payload is the one they hand
// straight to Handler.CapturePayload / Post.CapturePayload / hook.
// RunProbeFromPayload on the non-stand-down path, instead of reading raw a
// second time through hookio.Intake. That second read is what used to cost
// a full extra copy of raw -- not just extra CPU, a second live allocation
// the same size as the first, held at the same time as it on every hook,
// post and probe invocation -- which is why TestRES28_MaxRSS budgets this
// package's callers at the one-copy figure, not two.
//
// "no second scan" is this function's own job to keep true, not just a
// claim: raw is already hookio.Read's output (the caller's doc says so), so
// building the Payload is hookio.IntakeBytes(raw) -- the same scan hookio.
// Intake does internally minus its own Read -- never hookio.Intake(bytes.
// NewReader(raw)), which would run hookio.Read a second time on bytes
// already read once.
func EvaluateRaw(raw []byte, readErr error, env harness.Env, installedFor Family) (*hookio.Payload, harness.Identity, bool) {
	if readErr != nil {
		return nil, harness.Identity{}, false
	}
	payload, err := hookio.IntakeBytes(raw)
	if err != nil {
		return nil, harness.Identity{}, false
	}
	id, down := EvaluateFromPayload(payload, env, installedFor)
	return payload, id, down
}

// EvaluateFromPayload is Evaluate's payload-level half, for a caller that
// has already built a *hookio.Payload some other way than a single
// hookio.Intake call over a fully buffered reader -- cmd/rashomon's recap
// entry, whose payload is built from a streamed, size-bounded capture of
// only the harness-relevant fields (never the full Stop/StopFailure
// message), reuses this rather than re-implementing the comparison Evaluate
// makes.
func EvaluateFromPayload(payload *hookio.Payload, env harness.Env, installedFor Family) (harness.Identity, bool) {
	if payload == nil {
		return harness.Identity{}, false
	}
	id := harness.Detect(payload, env)
	return id, StandsDown(installedFor, id)
}
