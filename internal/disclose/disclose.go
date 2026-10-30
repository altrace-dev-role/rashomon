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
// Contract only (P-07): the types and the scope version. P-21 fills Files.
// Entries -- moving install.Events onto the inventory -- is NOT part of
// P-21: an Entries row needs Why, Prints and Owner text per event, and
// Prints names the output gate's own vocabulary ("nothing" | "user" |
// "user+model", 7.2.5), which does not exist in this tree yet. Entries stays
// owned by the PR that builds the output gate; see Default's doc.
package disclose

import (
	"errors"

	"github.com/altrace-dev-role/rashomon/internal/baseline"
	"github.com/altrace-dev-role/rashomon/internal/recap"
	"github.com/altrace-dev-role/rashomon/internal/store"
	"github.com/altrace-dev-role/rashomon/internal/verify"
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
	Units        []UnitParagraph `json:"units"`
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
// P-21 fills Files: every path ANY package that writes under a store root
// creates -- not only internal/store's own, but internal/recap's and
// internal/baseline's, which write beside it -- read from each package's
// own exported names rather than copied as literal strings, so a renamed or
// added file shows up here by construction instead of by someone
// remembering to update a second list (TRU-23).
//
// Paragraphs is the consent screen's text (paragraphs.go). The other
// sections -- Entries, Outputs, SettingsKeys, NeverEmit, NeverWrite,
// Switches, EnvVars -- are owned by later PRs (output gate, disclose/status
// CLI) and are empty until each lands.
//
// An empty section here is "not yet described", not "this build does
// nothing" -- and that distinction currently lives only in this comment, not
// in the type: Default returns a nil error whether a section is genuinely
// empty or not yet owned, so a future consumer (a consent screen, `watch`)
// cannot tell "fully described, zero rows" from "undescribed" by looking at
// the Inventory alone. No caller does that today, so it is not yet a
// behaviour gap, only a sharp edge the next caller needs to know about.
func Default() (Inventory, error) {
	return Inventory{
		ScopeVersion: ScopeVersion,
		Files:        append(defaultFiles(), learnFiles()...),
		Paragraphs:   defaultParagraphs(),
		Units:        defaultUnits(),
	}, nil
}

// defaultFiles is the FileRule for every path any package that writes under
// a store root creates -- internal/store's own, plus internal/recap's and
// internal/baseline's, which write beside it rather than through it. Base is
// a path relative to the store root; Pattern is the file name within it, or
// a glob when the base itself varies per session (runs/<session>/...) or per
// project (baseline/<key>.json).
//
// Three files hold a destination's identity and nothing else about it: a
// host named in records.ndjson travels in the clear because the report has
// to show it to be useful, but forgetting a host must make the hostname
// LEAVE the store's bytes while still letting later forgets and the report's
// suppression recognise it again -- which is what the keyed digest in
// gaps.ndjson and forgotten-hosts.ndjson is for (store/gaps.go, HostDigest).
// baseline/<key>.json is the exception that proves the rule rather than a
// fourth instance of it: it is the one file here that holds hostnames in the
// clear, by design, because a novelty baseline has no later-forget path to
// protect against yet (ForgetFile exists but nothing calls it from a user
// command in this tree) and a keyed digest cannot answer "which host is new"
// without the plaintext to compare.
func defaultFiles() []FileRule {
	const runSegment = store.DirRuns + "/<session>"
	return append([]FileRule{
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
			Purpose:    "the install's identity and the schema version it writes, and what watch found and changed, which detach and purge put back",
			Holds:      "install_id, created_at_unix_ms, schema_version; once watch has run, settings_existed, the sha256 of settings.json before and after watch, the directories watch created, and the work tree a confirmed store root sits in",
			NeverHolds: "a hostname or any call's content",
			RemovedBy:  "purge",
			Owner:      "store",
		},
		{
			// SetRootInWorkTree writes the new record under this name and
			// renames it over install.json (internal/store).
			Base:       "",
			Pattern:    "." + store.FileInstallMeta + ".tmp-*",
			Mode:       "0600",
			Purpose:    "install.json's replacement; present only for the instant between the write and the rename, or after a crash in between",
			Holds:      "what install.json is about to hold",
			NeverHolds: "a hostname or any call's content",
			RemovedBy:  "the write that made it (renamed over install.json); purge",
			Owner:      "store",
		},
		{
			Base:       runSegment,
			Pattern:    store.FileRecords,
			Mode:       "0600",
			Purpose:    "the append-only, totally ordered record of one session's declared and executed tool calls",
			Holds:      "tool name, program, verb class, argc, hosts and ssh_hosts the call named, transcript path, permission mode, agent type, file label, rule match, outcome, exit code and timing, its working directory, its project root and the file path an edit names, all in the clear, plus keyed digests of its working directory and its shape",
			NeverHolds: "file contents, prompts, any part of a tool response or other tool output, a call's other arguments in the clear, or a Bash command's text without consent",
			RemovedBy:  "forget --since, --before or --host; size-cap eviction; purge",
			Owner:      "store",
		},
		{
			Base:       runSegment,
			Pattern:    store.FileSpill,
			Mode:       "0600",
			Purpose:    "an execution or terminal record the ordered append lock could not place in time",
			Holds:      "the same fields records.ndjson holds, unordered",
			NeverHolds: "file contents, prompts, any part of a tool response or other tool output, a call's other arguments in the clear, or a Bash command's text without consent",
			RemovedBy:  "forget --since, --before or --host; size-cap eviction; purge",
			Owner:      "store",
		},
		{
			Base:       runSegment,
			Pattern:    store.FileSteer,
			Mode:       "0600",
			Purpose:    "one line per steer decision in the session, delivered or not, appended under the run's lock",
			Holds:      "the pattern, its level, channel and arm, the hook event, the seqs of the records it rests on, the slot values its message bound, and on a delivered or flagged line the message text shown, each capped at 2,048 bytes",
			NeverHolds: "a tool call's arguments or response beyond the slot values a message names",
			RemovedBy:  "size-cap eviction, with its run; purge. forget --since, --before and --host do not rewrite it",
			Owner:      "store",
		},
		{
			Base:       runSegment,
			Pattern:    store.FileCoverage,
			Mode:       "0600",
			Purpose:    "the configuration state resolved for the session",
			Holds:      "phase, hook outcome, and the session's own working directory",
			NeverHolds: "a tool call's content",
			// Coverage records stay through a forget on purpose (store/gaps.go:
			// "Coverage records stay: they describe whether the run could be
			// trusted"): ForgetWindow never opens coverage.ndjson. What does
			// remove it is a whole run directory going, which eviction and
			// purge both do.
			RemovedBy: "size-cap eviction; purge",
			Owner:     "store",
		},
		{
			Base:       runSegment,
			Pattern:    store.FileProbe,
			Mode:       "0600",
			Purpose:    "a liveness marker for the session's hook entries",
			Holds:      "one Unix-millisecond timestamp",
			NeverHolds: "anything else",
			RemovedBy:  "size-cap eviction; purge",
			Owner:      "store",
		},
		{
			Base:       runSegment,
			Pattern:    store.FileSeq,
			Mode:       "0600",
			Purpose:    "the session's last allocated record sequence number",
			Holds:      "one integer",
			NeverHolds: "anything else",
			RemovedBy:  "size-cap eviction; purge",
			Owner:      "store",
		},
		{
			Base:       runSegment,
			Pattern:    store.FileLockWait,
			Mode:       "0600",
			Purpose:    "the longest any one hook call of the session waited for the store's locks",
			Holds:      "one integer, in milliseconds, rounded up; absent until a call has waited",
			NeverHolds: "anything else",
			RemovedBy:  "size-cap eviction; purge",
			Owner:      "store",
		},
		{
			Base:       runSegment,
			Pattern:    store.FileStaleArmed,
			Mode:       "0600",
			Purpose:    "a hint that a gate ran since the last edit the session's steer step judged for stale evidence",
			Holds:      "nothing; present after a gate run, removed by the next judged edit",
			NeverHolds: "anything",
			RemovedBy:  "the steer step; size-cap eviction; purge",
			Owner:      "store",
		},
		{
			Base:       "",
			Pattern:    store.FileRecordingPaused,
			Mode:       "0600",
			Purpose:    "the marker that recording is paused machine-wide, and since when",
			Holds:      "one Unix-millisecond timestamp",
			NeverHolds: "anything else",
			RemovedBy:  "rashomon resume; purge",
			Owner:      "store",
		},
		{
			Base:       "",
			Pattern:    store.FileResolvedCap,
			Mode:       "0600",
			Purpose:    "the size cap the last hook call resolved, so status shows the cap eviction runs with rather than the one its own environment would resolve",
			Holds:      "the cap in bytes, and, when the cap variable was not a byte count, one line quoting the value it held and naming the default that applied",
			NeverHolds: "a hostname or any call's content",
			RemovedBy:  "purge",
			Owner:      "store",
		},
		{
			Base:       "",
			Pattern:    recap.StateFileName,
			Mode:       "0600",
			Purpose:    "recap's own bookkeeping: the last turn it evaluated, whether that evaluation was healthy, and which (session, prompt) pairs it has already spoken or checked, so a replayed turn is not announced twice",
			Holds:      "a Unix-millisecond timestamp, a health flag, and session/prompt ids -- never a tool call's content",
			NeverHolds: "a tool call's content",
			RemovedBy:  "purge",
			Owner:      "recap",
		},
		{
			// writeState writes recap.json by temp-then-rename beside it
			// (internal/recap/state.go), one uniquely named temp file per
			// write, not under the store's runs/ tree, so it is a
			// root-level sibling like recap.json rather than a run-segment
			// pattern.
			Base:       "",
			Pattern:    recap.StateTempPrefix + "*",
			Mode:       "0600",
			Purpose:    "recap.json's own temp-then-rename write target; present only for the instant between the write and the rename, or after a crash in between",
			Holds:      "the same fields recap.json holds",
			NeverHolds: "a tool call's content",
			RemovedBy:  "the write that made it (renamed over recap.json); the next recap write, once it is a minute old; purge",
			Owner:      "recap",
		},
		{
			// recap.StateTempName: the one shared temp name builds through
			// v1.1.0 wrote, which a store written by one of them may still
			// hold. Nothing writes it now.
			Base:       "",
			Pattern:    recap.StateTempName,
			Mode:       "0600",
			Purpose:    "recap.json's temp-then-rename write target in builds through v1.1.0; present only after such a build crashed between the write and the rename",
			Holds:      "the same fields recap.json holds",
			NeverHolds: "a tool call's content",
			RemovedBy:  "purge",
			Owner:      "recap",
		},
		{
			Base:       baseline.DirName,
			Pattern:    baseline.FilePattern,
			Mode:       "0600",
			Purpose:    "one project's novelty baseline: every host any of its sessions has named, kept outside the evictable run store so eviction cannot make an old host look new again",
			Holds:      "every host this project's sessions have named, IN THE CLEAR, with the session and timestamp that first named each",
			NeverHolds: "a tool call's content, or anything about a host beyond its name and when it was first named",
			RemovedBy:  "purge",
			Owner:      "baseline",
		},
		{
			Base:       baseline.DirName,
			Pattern:    baseline.LockName,
			Mode:       "0600",
			Purpose:    "the lock every load-modify-save of a baseline takes, so a render and a forget cannot each save a copy loaded before the other's change",
			Holds:      "nothing: it is empty, and only the lock on it matters",
			NeverHolds: "anything",
			RemovedBy:  "purge",
			Owner:      "baseline",
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
			Pattern:    store.FileConsent,
			Mode:       "0600",
			Purpose:    "every consent you gave: one record per line, appended when you answer y to the consent screen or a notice counts as shown, and read by every layer before it records or adds a note",
			Holds:      "the scope and disclosure versions agreed to, the record scope, whether notes were on, a SHA-256 of the exact text shown, this binary's version and SHA-256, how the consent was given, and when",
			NeverHolds: "the text shown, or anything about a session",
			RemovedBy:  "compaction at 64 KiB to the latest record per way of giving it and per scope version; purge",
			Owner:      "consent",
		},
		{
			// compact writes the compacted file under this name beside
			// consent.json and renames it over it (internal/consent).
			Base:       "",
			Pattern:    "." + store.FileConsent + ".tmp-*",
			Mode:       "0600",
			Purpose:    "consent.json's compaction target; present only for the instant between the write and the rename, or after a crash in between",
			Holds:      "the records consent.json keeps",
			NeverHolds: "the text shown, or anything about a session",
			RemovedBy:  "the compaction that wrote it (renamed over consent.json); purge",
			Owner:      "consent",
		},
		{
			Base:       "",
			Pattern:    store.FileHarnesses,
			Mode:       "0600",
			Purpose:    "per harness and surface switches; watch writes whether Agent SDK programs are recorded, from the consent screen's second question",
			Holds:      "a version, and per surface whether it records (on or paused) and whether it steers",
			NeverHolds: "anything about a session or a call",
			RemovedBy:  "purge",
			Owner:      "consent",
		},
		{
			// WriteSDKAnswer writes the new file under this name and
			// renames it over harnesses.json (internal/consent).
			Base:       "",
			Pattern:    "." + store.FileHarnesses + ".tmp-*",
			Mode:       "0600",
			Purpose:    "harnesses.json's replacement; present only for the instant between the write and the rename, or after a crash in between",
			Holds:      "what harnesses.json is about to hold",
			NeverHolds: "anything about a session or a call",
			RemovedBy:  "the write that made it (renamed over harnesses.json); purge",
			Owner:      "consent",
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
		{
			Base:       "",
			Pattern:    store.FileHarnessCounts,
			Mode:       "0600",
			Purpose:    "how often a hook entry stood down because the agent harness calling it was not the one this install is for, written only by the entry that stood down",
			Holds:      "a version; per harness pair, a count per day for at most 30 days, when it last stood down and the evidence kinds that decided it; a keyed digest of each counted session, at most 256, so a session counts once",
			NeverHolds: "a session id in the clear, a path, or anything about a call",
			RemovedBy:  "purge",
			Owner:      "store",
		},
		{
			// RecordHarnessMismatch locks this sidecar, never the data file,
			// so the data file can be replaced by rename under the lock.
			Base:       "",
			Pattern:    store.FileHarnessCountsLock,
			Mode:       "0600",
			Purpose:    "the lock harness_counts.json's writer takes; its content is never read",
			Holds:      "nothing",
			NeverHolds: "anything",
			RemovedBy:  "purge",
			Owner:      "store",
		},
		{
			Base:       "",
			Pattern:    "." + store.FileHarnessCounts + ".rashomon-*",
			Mode:       "0600",
			Purpose:    "harness_counts.json's replacement; present only for the instant between the write and the rename, or after a crash in between",
			Holds:      "what harness_counts.json is about to hold",
			NeverHolds: "a session id in the clear, a path, or anything about a call",
			RemovedBy:  "the write that made it (renamed over harness_counts.json); purge",
			Owner:      "store",
		},
	}, append(recordScopeFiles(), append(indexFiles(), bodiesFiles()...)...)...)
}

// indexFiles is the inventory's rows for the project index the Stop hook's
// recap reads and writes (verify.IndexPath, card 2.4), and its replacement.
// Like recordScopeFiles, they sit apart from defaultFiles for Law 10.
func indexFiles() []FileRule {
	const base = verify.DirName + "/" + verify.ProjectsDir
	return []FileRule{
		{
			Base:       base,
			Pattern:    baseline.FilePattern,
			Mode:       "0600",
			Purpose:    "one project's index of the test and build commands its sessions have run, so a command two sessions ran in thirty days is required before a turn reads verified",
			Holds:      "the project's directory in the clear and, per command family, its runner, a keyed digest of the directory it ran in, its kind, how many sessions ran it, the last of them by session id, and when it was first and last seen",
			NeverHolds: "a command's arguments or output, or anything about a call beyond its family",
			RemovedBy:  "eviction of the least recently seen families at 64 KiB; purge",
			Owner:      "verify",
		},
		{
			Base:       base,
			Pattern:    "." + baseline.FilePattern + ".tmp-*",
			Mode:       "0600",
			Purpose:    "a project index's replacement; present only for the instant between the write and the rename, or after a crash in between",
			Holds:      "what the project index is about to hold",
			NeverHolds: "a command's arguments or output, or anything about a call beyond its family",
			RemovedBy:  "the write that made it (renamed over the index); purge",
			Owner:      "verify",
		},
	}
}

// recordScopeFiles is the inventory's rows for the record_scope marker and
// its replacement, which internal/consent writes beside consent.json for
// the hook to read. They sit apart from defaultFiles, whose length Law 10
// holds where it is (test/acceptance's ratchet).
func recordScopeFiles() []FileRule {
	return []FileRule{
		{
			// Every consent write removes it, appends to consent.json and
			// writes it again from consent.json's answer (internal/consent);
			// the hook reads it on every call (internal/store).
			Base:       "",
			Pattern:    store.FileRecordScope,
			Mode:       "0600",
			Purpose:    "the record scope consent.json names, kept beside it so a hook call reads one word instead of parsing consent.json; no marker means the v4 shape, without paths or command text",
			Holds:      "v4 or v5",
			NeverHolds: "anything else",
			RemovedBy:  "every consent write, which writes it again after; purge",
			Owner:      "consent",
		},
		{
			Base:       "",
			Pattern:    "." + store.FileRecordScope + ".tmp-*",
			Mode:       "0600",
			Purpose:    "record_scope's replacement; present only for the instant between the write and the rename, or after a crash in between",
			Holds:      "v4 or v5",
			NeverHolds: "anything else",
			RemovedBy:  "the write that made it (renamed over record_scope); purge",
			Owner:      "consent",
		},
		{
			// Every consent write removes it, appends to consent.json and
			// writes it again from consent.json's answer, and every session
			// start puts it right (internal/consent, internal/hook).
			Base:       "",
			Pattern:    store.FileSteerDisclosure,
			Mode:       "0600",
			Purpose:    "the highest steering disclosure consent.json holds, kept beside it so a hook call reads one number instead of parsing consent.json; no marker means no consent to steering, so notes reach the agent as flags",
			Holds:      "a non-negative integer, the highest steer disclosure version consented to",
			NeverHolds: "anything else",
			RemovedBy:  "every consent write, which writes it again after; purge",
			Owner:      "consent",
		},
		{
			Base:       "",
			Pattern:    "." + store.FileSteerDisclosure + ".tmp-*",
			Mode:       "0600",
			Purpose:    "steer_disclosure's replacement; present only for the instant between the write and the rename, or after a crash in between",
			Holds:      "a non-negative integer",
			NeverHolds: "anything else",
			RemovedBy:  "the write that made it (renamed over steer_disclosure); purge",
			Owner:      "consent",
		},
	}
}
