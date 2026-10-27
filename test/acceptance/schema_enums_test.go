package acceptance

import (
	"maps"
	"slices"
	"strconv"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/report"
	"github.com/altrace-dev-role/rashomon/internal/store"
)

// The closed vocabularies docs/store-schema.json publishes on the records,
// each enum compared on its own against the code list that owns it, so a
// word added or dropped on one side fails a row that names the other. The
// reason check these rows replace compared one pooled set, which stays
// whole when one enum loses a word another still has (B40: coverage could
// lose lock_timeout while terminal kept it).

// terminalOnlyReasons are the store reasons a coverage record never
// carries: flush_failed marks the spill terminal of a call whose records did
// not sync (store.Terminal.IsFlushFailed), written after its coverage.
var terminalOnlyReasons = []string{store.ReasonFlushFailed}

// terminalReasons is every reason a terminal record carries, spelled out
// from its writers, since no code list names the subset. Close
// (internal/hook/handle.go:279) writes a terminal only for a declaration
// that landed, or for one the lock cost (its spill branch, handle.go:276),
// so of closeReason's answers (internal/hook/payload.go:89) the two that
// fail before the declaration lands, payload_too_large and record_oversize,
// never reach one.
var terminalReasons = []struct{ reason, writer string }{
	{store.ReasonTerminatedBySignal, "internal/hook/handle.go:265, Close on a delivered signal"},
	{store.ReasonLockTimeout, "internal/hook/payload.go:94 through handle.go:276, Close's spill for a declaration the lock cost"},
	{store.ReasonRecoveredPanic, "internal/hook/payload.go:102 through handle.go:279, a panic after the declaration landed"},
	{store.ReasonFlushFailed, "internal/hook/flush.go:51, flushOrMark's spill terminal"},
	// No live writer: once the declaration lands, capture returns nil and
	// only a panic can fail, which closes as recovered_panic. b9f0b256's
	// Close closed every capture error after the declaration as
	// internal_error, a recovered panic included, before recovered_panic
	// existed, and those terminals are on disk. closeReason's fallback still
	// names it.
	{store.ReasonInternalError, "b9f0b256's Close (internal/hook/handle.go:115 there); internal/hook/payload.go:104 today"},
}

// vocabulary is the rows for every property the schema publishes under one
// name, one row per enum.
type vocabulary struct {
	name string
	rows []enumRow
}

// schemaVocabularies is every record vocabulary 1.1k compares, from the code
// lists that own them. file_label, verb_class, status_masked and runner_kind
// have tests of their own (repo_test.go).
func schemaVocabularies(t *testing.T) []vocabulary {
	t.Helper()
	var terminal []string
	for _, r := range terminalReasons {
		terminal = append(terminal, r.reason)
	}
	coverage := slices.DeleteFunc(slices.Clone(store.Reasons()), func(r string) bool {
		return slices.Contains(terminalOnlyReasons, r)
	})
	const callRange, sessionRange = "store.AcceptsKind's 1..CallSchemaVersion", "store.AcceptsKind's 1..SessionSchemaVersion"
	return []vocabulary{
		{"reason", []enumRow{
			{"/$defs/coverage/properties/reason", "store.Reasons() less terminalOnlyReasons", coverage, true},
			// Every gap reason the store can write. GapForgetHost was once
			// missing from this exact list, which is why it is spelled out
			// rather than folded into a helper that could omit one again.
			{"/$defs/gap/properties/reason", "the store's gap reasons (store.Gap*)",
				[]string{store.GapForget, store.GapForgetHost, store.GapSizeCap, store.GapPaused}, false},
			{"/$defs/terminal/properties/reason", "terminalReasons (the terminal writers)", terminal, true},
		}},
		{"report_reason", []enumRow{
			{"/$defs/report_reason", "report.Reasons()", report.Reasons(), false},
		}},
		{"type", []enumRow{
			{"/$defs/coverage/properties/type", "store.TypeCoverage", []string{store.TypeCoverage}, false},
			{"/$defs/declaration/properties/type", "store.TypeDeclaration", []string{store.TypeDeclaration}, false},
			{"/$defs/execution/properties/type", "store.TypeExecution", []string{store.TypeExecution}, false},
			{"/$defs/forgotten_host/properties/type", "store.TypeForgottenHost", []string{store.TypeForgottenHost}, false},
			{"/$defs/gap/properties/type", "store.TypeGap", []string{store.TypeGap}, false},
			{"/$defs/terminal/properties/type", "store.TypeTerminal", []string{store.TypeTerminal}, false},
		}},
		{"outcome", []enumRow{
			{"/$defs/execution/properties/outcome", "store.Exec*",
				[]string{store.ExecOK, store.ExecFailed, store.ExecInterrupted}, false},
			{"/$defs/terminal/properties/outcome", "store.Outcome*",
				[]string{store.OutcomeOK, store.OutcomeError, store.OutcomeSignal}, false},
		}},
		{"phase", []enumRow{
			{"/$defs/coverage/properties/phase", "store.Phase*",
				[]string{store.PhaseStart, store.PhaseCall, store.PhasePost, store.PhaseEnd}, false},
			// flushOrMark's two callers (internal/hook/flush.go:14 and :19);
			// Close's terminal leaves phase null.
			{"/$defs/terminal/properties/phase", "the terminal writers' phases",
				[]string{store.PhaseCall, store.PhasePost}, true},
		}},
		{"schema_version", []enumRow{
			{"/$defs/coverage/properties/schema_version", sessionRange, acceptedVersions(t, store.TypeCoverage), false},
			{"/$defs/declaration/properties/schema_version", callRange, acceptedVersions(t, store.TypeDeclaration), false},
			{"/$defs/execution/properties/schema_version", callRange, acceptedVersions(t, store.TypeExecution), false},
			// Not read through AcceptsKind: its readers take any version
			// (internal/store/gaps.go:536 and :764), so its one writer's stamp
			// bounds it (gaps.go:507).
			{"/$defs/forgotten_host/properties/schema_version", "the forgotten_host writer's SessionSchemaVersion",
				[]string{strconv.Itoa(store.SessionSchemaVersion)}, false},
			{"/$defs/gap/properties/schema_version", sessionRange, acceptedVersions(t, store.TypeGap), false},
			{"/$defs/terminal/properties/schema_version", callRange, acceptedVersions(t, store.TypeTerminal), false},
		}},
		{"hook_entry", []enumRow{
			// EntryPresent has no live writer and stays for the version 2
			// records that carry it (record.go's comment on the block).
			{"/$defs/coverage/properties/hook_entry", "store.Entry*", []string{store.EntryPresentSettings,
				store.EntryPresentPlugin, store.EntryPresent, store.EntryAbsent, store.EntryUnknown}, false},
		}},
		{"probe", []enumRow{
			{"/$defs/coverage/properties/probe", "store.Probe*",
				[]string{store.ProbeFresh, store.ProbeAbsent, store.ProbeUnknown}, false},
		}},
		{"state", []enumRow{
			{"/$defs/coverage/properties/state", "store.State*", []string{store.StateVerified, store.StateUnverified}, false},
		}},
	}
}

// acceptedVersions is every schema_version store.AcceptsKind reads for kind,
// in decimal, asked of the function rather than copied from its constants.
// It also holds the constant the rows name as the bound: 1..CallSchemaVersion
// for a call record, 1..SessionSchemaVersion for coverage and gaps.
func acceptedVersions(t *testing.T, kind string) []string {
	t.Helper()
	bound := store.CallSchemaVersion
	if kind == store.TypeCoverage || kind == store.TypeGap {
		bound = store.SessionSchemaVersion
	}
	var got, want []string
	for v := 0; v <= 64; v++ {
		if store.AcceptsKind(kind, v) {
			got = append(got, strconv.Itoa(v))
		}
		if v >= 1 && v <= bound {
			want = append(want, strconv.Itoa(v))
		}
	}
	if !slices.Equal(got, want) {
		t.Errorf("store.AcceptsKind reads %s at %v, not 1..%d: the bound the schema_version row names is stale",
			kind, got, bound)
	}
	return got
}

// TestStoreSchemaEnumsAreTheirCodeLists: every record vocabulary, one
// subtest per name and one row per enum, each compared both ways against its
// own code list, and its null against the row and the store field behind it
// (assertEnumsPerPath). Break: drop lock_timeout from
// coverage.reason alone, and the reason row for coverage names its path
// while terminal's stays green.
func TestStoreSchemaEnumsAreTheirCodeLists(t *testing.T) {
	for _, v := range schemaVocabularies(t) {
		t.Run(v.name, func(t *testing.T) {
			assertEnumsPerPath(t, v.name, v.rows)
		})
	}
}

// TestStoreSchemaReasonEnumsAreTheirCodeLists holds what the pooled reason
// check held beyond the rows: every terminal reason is a store reason, every
// store reason is on coverage or terminal (coverage's row is store.Reasons()
// less terminalOnlyReasons, each of which must be a terminal reason), and the
// reason rows compare exactly the paths the reason tripwire
// (TestStoreSchemaHasNoUncheckedReasonVocabulary) lists, so a row deleted
// fails here rather than leaving its enum unread.
func TestStoreSchemaReasonEnumsAreTheirCodeLists(t *testing.T) {
	for _, r := range terminalReasons {
		if !slices.Contains(store.Reasons(), r.reason) {
			t.Errorf("terminalReasons names %q (%s), which store.Reasons() does not", r.reason, r.writer)
		}
	}
	for _, r := range terminalOnlyReasons {
		if !slices.ContainsFunc(terminalReasons, func(w struct{ reason, writer string }) bool { return w.reason == r }) {
			t.Errorf("terminalOnlyReasons names %q, which no terminal writer sets, so no record enum publishes it", r)
		}
	}
	compared := map[string]bool{}
	for _, v := range schemaVocabularies(t) {
		if v.name != "reason" && v.name != "report_reason" {
			continue
		}
		for _, r := range v.rows {
			compared[r.path] = true
		}
	}
	if !maps.Equal(compared, reasonVocabularyPaths) {
		t.Errorf("the reason rows compare %v, and the reason tripwire lists %v: a reason vocabulary is "+
			"listed without a row, or compared without being listed",
			slices.Sorted(maps.Keys(compared)), slices.Sorted(maps.Keys(reasonVocabularyPaths)))
	}
}
