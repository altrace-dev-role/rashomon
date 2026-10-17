// Package disclose is the inventory of everything rashomon installs, writes,
// reads, prints and may say to the agent, and the one source every
// user-facing statement of it is rendered from: the consent screen, the
// README's machine section, `rashomon disclose` and `status`.
//
// It exists because a tool that installs hooks into an editor and adds text
// to an agent's context is the kind of tool people get warned about, and the
// defence is that nothing it does is undisclosed: a capability that is not in
// this inventory cannot ship (TRU-1, TRU-23), and a screen rendered from the
// inventory cannot drift from what the binary does. Technical spec 7.2.2;
// Archive 80.3.2.
//
// Contract only (P-07): the types and the scope version. Default() is filled
// by P-21, which also moves install.Events onto it.
package disclose

import (
	"errors"

	"github.com/altrace-dev-role/rashomon/internal/store"
)

// ScopeVersion is bumped on a new capability class. The consent a user gave
// is recorded against it, so a build that can do a new kind of thing shows
// the screen again rather than riding on an older yes. Pinned by TRU-8.
const ScopeVersion = 3

// ErrNotImplemented is returned by stubs that a later PR fills.
var ErrNotImplemented = errors.New("disclose: not implemented in this build")

// Inventory is the whole disclosure, as JSON and as the input to every
// rendering.
type Inventory struct {
	Version      string          `json:"version"`
	ScopeVersion int             `json:"scope_version"`
	Entries      []HookEntry     `json:"entries"`
	Files        []FileRule      `json:"files"`
	SettingsKeys []SettingsWrite `json:"settings_writes"`
	Outputs      []OutputRule    `json:"outputs"`
	NeverEmit    []Forbidden     `json:"never_emit"`
	NeverWrite   []string        `json:"never_write"`
	Switches     []Switch        `json:"switches"`
	EnvVars      []EnvVar        `json:"env_vars"`
	Paragraphs   []Paragraph     `json:"screen_paragraphs"`
}

// HookEntry is one hook rashomon installs into Claude Code's settings.
type HookEntry struct {
	Event      string `json:"event"`
	Matcher    string `json:"matcher,omitempty"`
	Subcommand string `json:"subcommand"`
	TimeoutS   int    `json:"timeout_s"`
	Why        string `json:"why"`
	// Prints is what the entry's process may write to stdout: "nothing",
	// "user" or "user+model". The output gate (7.2.5) enforces it.
	Prints string `json:"prints"`
	Owner  string `json:"owner"`
}

// OutputRule is one field one event may carry, with its audience and whether
// the user-facing mirror of a model-facing field has been verified live.
type OutputRule struct {
	Event          string   `json:"event"`
	Field          string   `json:"field"`
	Audience       string   `json:"audience"` // "user" | "model"
	MirrorVerified bool     `json:"mirror_verified"`
	MirrorSource   string   `json:"mirror_source"`
	Templates      []string `json:"template_ids"`
}

// FileRule is one path rashomon may create under its store or elsewhere,
// what it holds, what it never holds and who removes it.
type FileRule struct {
	Base       string `json:"base"`
	Pattern    string `json:"pattern"`
	Mode       string `json:"mode"`
	Purpose    string `json:"purpose"`
	Holds      string `json:"holds"`
	NeverHolds string `json:"never_holds"`
	RemovedBy  string `json:"removed_by"`
	Owner      string `json:"owner"`
}

// SettingsWrite is one JSON pointer in one settings file rashomon writes.
type SettingsWrite struct {
	File    string `json:"file"`
	JSONPtr string `json:"json_ptr"`
	Writer  string `json:"writer"`
}

// Forbidden is one output field rashomon never emits on one event. Except is
// set on one row only: the non-zero-exit row's scoped exception (7.2.5).
type Forbidden struct {
	Event  string `json:"event"`
	Field  string `json:"field"`
	Why    string `json:"why"`
	Except string `json:"except,omitempty"`
}

// Switch is one user-settable switch, with the command that sets it, where
// it is stored, and whether it ratchets (an expansion needs a terminal; a
// reduction does not, 7.2.4).
type Switch struct {
	Name    string `json:"name"`
	Default string `json:"default"`
	Command string `json:"command"`
	Stored  string `json:"stored"`
	Ratchet bool   `json:"ratchet"`
}

// EnvVar is one environment variable rashomon reads.
type EnvVar struct {
	Name     string `json:"name"`
	Effect   string `json:"effect"`
	TestOnly bool   `json:"test_only"`
}

// Paragraph is one paragraph of the consent screen, owned by the layer whose
// capability it discloses, citing the inventory rows it is rendered from.
type Paragraph struct {
	Owner string `json:"owner"`
	Text  string `json:"text"`
	Cite  string `json:"cite"`
}

// Sections of `rashomon disclose`, in print order (7.2.8).
const (
	SectionInstalled    = "INSTALLED"
	SectionConsent      = "CONSENT"
	SectionSwitches     = "SWITCHES"
	SectionStored       = "STORED"
	SectionHookCost     = "HOOK COST"
	SectionSaid         = "SAID TO THE AGENT"
	SectionNeverWritten = "NEVER WRITTEN BY RASHOMON"
	SectionRemove       = "REMOVE"
)

// Default is the build's inventory.
//
// P-21 fills Files: every path rashomon's own store package creates, read
// from that package's own exported names rather than copied as literal
// strings, so a renamed or added file shows up here by construction instead
// of by someone remembering to update a second list (TRU-23). The other
// sections -- Entries, Outputs, SettingsKeys, NeverEmit, NeverWrite,
// Switches, EnvVars, Paragraphs -- are owned by later PRs (consent screen,
// output gate, disclose/status CLI) and are empty until each lands; an empty
// section here is "not yet described", not "this build does nothing".
func Default() (Inventory, error) {
	return Inventory{
		ScopeVersion: ScopeVersion,
		Files:        defaultFiles(),
	}, nil
}

// defaultFiles is the FileRule for every path the store package creates,
// under the store root. Base is a path relative to the store root; Pattern is
// the file name within it, or a glob when the base itself varies per session
// (runs/<session>/...).
//
// Two files hold a destination's identity and nothing else about it: a host
// named in records.ndjson travels in the clear because the report has to
// show it to be useful, but forgetting a host must make the hostname LEAVE
// the store's bytes while still letting later forgets and the report's
// suppression recognise it again -- which is what the keyed digest in
// gaps.ndjson and forgotten-hosts.ndjson is for (store/gaps.go, HostDigest).
func defaultFiles() []FileRule {
	const runSegment = store.DirRuns + "/<session>"
	return []FileRule{
		{
			Base:       "",
			Pattern:    store.FileInstallKey,
			Mode:       "0600",
			Purpose:    "the per-install key everything else here digests keyed with",
			Holds:      "32 bytes from crypto/rand, written once",
			NeverHolds: "a hostname, a path, or any call's content",
			RemovedBy:  "purge",
			Owner:      "store",
		},
		{
			Base:       "",
			Pattern:    store.FileInstallMeta,
			Mode:       "0600",
			Purpose:    "the install's identity and the schema version it writes",
			Holds:      "install_id, created_at_unix_ms, schema_version",
			NeverHolds: "a hostname or any call's content",
			RemovedBy:  "purge",
			Owner:      "store",
		},
		{
			Base:       runSegment,
			Pattern:    store.FileRecords,
			Mode:       "0600",
			Purpose:    "the append-only, totally ordered record of one session's declared and executed tool calls",
			Holds:      "tool name, hosts and ssh_hosts the call named, timing, and keyed digests of its working directory and shape",
			NeverHolds: "a tool call's arguments in the clear, or any part of a tool response",
			RemovedBy:  "forget --since, --before or --host; purge",
			Owner:      "store",
		},
		{
			Base:       runSegment,
			Pattern:    store.FileSpill,
			Mode:       "0600",
			Purpose:    "an execution or terminal record the ordered append lock could not place in time",
			Holds:      "the same fields records.ndjson holds, unordered",
			NeverHolds: "a tool call's arguments in the clear, or any part of a tool response",
			RemovedBy:  "forget --since, --before or --host; purge",
			Owner:      "store",
		},
		{
			Base:       runSegment,
			Pattern:    store.FileCoverage,
			Mode:       "0600",
			Purpose:    "the configuration state resolved for the session",
			Holds:      "phase, hook outcome, and the session's own working directory",
			NeverHolds: "a tool call's content",
			RemovedBy:  "forget --since or --before; purge",
			Owner:      "store",
		},
		{
			Base:       runSegment,
			Pattern:    store.FileProbe,
			Mode:       "0600",
			Purpose:    "a liveness marker for the session's hook entries",
			Holds:      "one Unix-millisecond timestamp",
			NeverHolds: "anything else",
			RemovedBy:  "purge",
			Owner:      "store",
		},
		{
			Base:       runSegment,
			Pattern:    store.FileSeq,
			Mode:       "0600",
			Purpose:    "the session's last allocated record sequence number",
			Holds:      "one integer",
			NeverHolds: "anything else",
			RemovedBy:  "purge",
			Owner:      "store",
		},
		{
			Base:       "",
			Pattern:    store.FileGaps,
			Mode:       "0600",
			Purpose:    "one record per removal a forget made",
			Holds:      "the reason, the time window, the count removed, and -- for a host-scoped forget -- the host's keyed digest",
			NeverHolds: "the removed content, or the host name a host-scoped forget removed",
			RemovedBy:  "purge",
			Owner:      "store",
		},
		{
			Base:       "",
			Pattern:    store.FileForgottenHosts,
			Mode:       "0600",
			Purpose:    "every host a `forget --host` has ever named, kept so a later report still suppresses it and a repeat forget recognises it",
			Holds:      "the host's keyed digest (HMAC-SHA256 under the install key)",
			NeverHolds: "the host name",
			RemovedBy:  "purge",
			Owner:      "store",
		},
	}
}
