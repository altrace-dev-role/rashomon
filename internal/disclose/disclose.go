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

import "errors"

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

// Default is the build's inventory. Empty until P-21 fills it; a build whose
// inventory is empty discloses nothing and so may install nothing, which is
// the right failure.
func Default() (Inventory, error) {
	return Inventory{}, ErrNotImplemented
}
