// Package episode finds the mistakes in a session: short spans of trace steps
// with a negative signal and, where one exists, what resolved it. Detectors
// are pure functions of a session's steps; nothing here stores, reads a file
// or knows an agent, a language, a test runner or an error string. What an
// episode matches (a program, a verb, a directory, the shape of an error) is
// read from the session itself, so the same code finds a pytest import error,
// a Jest regression and a Terraform plan loop.
//
// The kinds are a fixed list because each needs its own rule for what counts
// as an opportunity to make the mistake again (docs/autosteer/LEARNING.md,
// section 6). The patterns within a kind are data.
package episode

import (
	"time"

	"github.com/altrace-dev-role/rashomon/internal/trace"
)

// Kind is a kind of episode.
type Kind string

const (
	// ActFailure: an act failed and the agent ran the same kind of act
	// again later in the session.
	ActFailure Kind = "act_failure"
	// CheckRegression: a command the session ran as a check passed, files
	// were edited, and the same command then failed.
	CheckRegression Kind = "check_regression"
	// Correction: the person stopped the agent, refused a call, or a rule of
	// theirs refused one; or, with a pushback lexicon, they turned it back
	// in words.
	Correction Kind = "correction"
	// SelfRevert: an edit put back what an earlier edit in the session
	// replaced.
	SelfRevert Kind = "self_revert"
	// RetryLoop: the identical call was made again after an identical
	// failure, at least twice in a row.
	RetryLoop Kind = "retry_loop"
)

// KeyVersion is the version of Key's fields and of how they are derived.
const KeyVersion = 1

// Key is what makes two episodes the same mistake. A lesson carries the key
// of the episodes it was mined from.
type Key struct {
	V    int    `json:"v"`
	Kind Kind   `json:"kind"`
	Act  ActSig `json:"act"`
	// Scope is where the mistake happens.
	Scope Scope `json:"scope"`
	// Detail qualifies a Correction: "interrupt", "reject", "block" or
	// "pushback". Empty for other kinds.
	Detail string `json:"detail,omitempty"`
	// FP is the failure's fingerprint (Fingerprint), empty for a kind with
	// no output to read.
	FP  string `json:"fp,omitempty"`
	FPV int    `json:"fpv,omitempty"`
}

// ActSig is a kind of act: its class, and for a command its program and verb.
type ActSig struct {
	Class   trace.ActClass `json:"class"`
	Program string         `json:"program,omitempty"`
	Verb    string         `json:"verb,omitempty"`
}

// Scope is where in the repository an episode happens: the deepest directory
// common to its paths, at most three levels down, and their most common file
// extension. The zero Scope is the whole repository.
type Scope struct {
	Dir string `json:"dir,omitempty"`
	Ext string `json:"ext,omitempty"`
}

// Episode is one occurrence of a mistake.
type Episode struct {
	Key     Key
	Session string
	At      time.Time
	// From and To are indexes into the steps Detect read: the first step
	// with the negative signal and the last step of the span.
	From, To int
	// Evidence are the indexes of the steps that show the negative signal.
	Evidence []int
	// Template is the masked line the fingerprint was taken of. It is made
	// of the output's own words and is never part of the key.
	Template string
	// Resolution is what resolved the mistake, nil when nothing did in the
	// session.
	Resolution *Resolution
}

// Resolution is what came between a failure and the next success of the
// same act.
type Resolution struct {
	// Added and Removed are the command words the successful run has that
	// the failed one did not, and the reverse.
	Added, Removed []string
	// Before are the kinds of act run between the failure and the success
	// that the session had not run before the failure.
	Before []ActSig
	// Edited are the repository-relative paths changed in between.
	Edited []string
}

// Options configures Detect.
type Options struct {
	// Root is the directory paths are made relative to; "" takes the first
	// working directory the steps record.
	Root string
	// Baseline holds the templates of passing outputs, which a fingerprint
	// never picks (see Fingerprint). Nil is the first-run rule.
	Baseline Baseline
	// Pushback, when set, says whether a line the person typed turns the
	// agent back. It is read only for lines the source marks as typed by
	// the person; nil detects no pushback.
	Pushback func(text string) bool
}

// Detect finds the episodes in one session's steps, in step order.
func Detect(steps []trace.Step, o Options) []Episode {
	s := view(steps, o.Root)
	var out []Episode
	out = append(out, failures(s, o.Baseline)...)
	out = append(out, corrections(s, o.Pushback)...)
	out = append(out, reverts(s)...)
	out = append(out, loops(s, o.Baseline)...)
	sortEpisodes(out)
	return out
}
