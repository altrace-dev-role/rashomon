package episode

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"unicode"

	"github.com/altrace-dev-role/rashomon/internal/trace"
)

// FingerprintVersion is the version of the masking rule and of how a line is
// chosen. A change to either is a new version, and keys of two versions never
// compare equal.
const FingerprintVersion = 1

// Baseline is the templates seen in passing outputs, per kind of act. A
// failure's fingerprint never picks a line a passing run of the same act also
// prints (a banner, a progress line, a summary heading).
type Baseline map[ActSig]map[string]struct{}

// AddPassing adds a passing output's templates under sig.
func (b Baseline) AddPassing(sig ActSig, output string) {
	m := b[sig]
	if m == nil {
		m = map[string]struct{}{}
		b[sig] = m
	}
	for _, l := range strings.Split(output, "\n") {
		if t, n := mask(l); n >= minFixed {
			m[t] = struct{}{}
		}
	}
}

// Learn adds every passing output in a session's steps to b. Read sessions
// oldest first, the order live sessions arrive in, so a baseline built from
// history is the one a live reader would hold.
func (b Baseline) Learn(steps []trace.Step, root string) {
	s := view(steps, root)
	for _, a := range s.acts {
		if a.result != nil && !a.result.Failed && a.result.Output != "" {
			b.AddPassing(a.sig, a.result.Output)
		}
	}
}

// minFixed is how many fixed words a line needs to be a candidate: a line of
// one word ("FAIL", "Error") says too little to tell failures apart.
const minFixed = 2

// Fingerprint reduces a failed act's output to its key: the digest of the
// fixed words of one line, under a masking rule that replaces every word
// that varies between occurrences of the same failure (numbers, paths, quoted
// text, hex ids). It returns the digest and the masked line.
//
// The line is the candidate with the most fixed words among those no passing
// run of the same act printed; a tie goes to the later line. The most fixed
// words is the most specific line: an error's own sentence ("No module named
// <*>", "Cannot read properties of undefined reading <*>") outranks a count
// summary ("<*> error in <*>"), which would join unrelated failures. With no
// baseline for the act (its first run, or a reader that has none) no line is
// excluded, and the rule is the same for every reader, so two readers of one
// output agree.
//
// A tool error the adapter named is fingerprinted by that name alone.
func Fingerprint(sig ActSig, r *trace.ResultStep, b Baseline) (fp, template string) {
	if r.Error != trace.ErrorNone {
		t := "tool error " + toolErrorWord(r.Error)
		return digest(t), t
	}
	out := r.Output
	if out == "" {
		out = r.Text
	}
	seen := b[sig]
	pick, most := "", 0
	for _, l := range strings.Split(out, "\n") {
		t, n := mask(l)
		if n < minFixed || n < most {
			continue
		}
		if _, ok := seen[t]; ok {
			continue
		}
		pick, most = t, n
	}
	if pick == "" {
		return "", ""
	}
	return digest(pick), pick
}

func digest(template string) string {
	h := sha256.Sum256([]byte("episode-fp/1\x00" + template))
	return hex.EncodeToString(h[:8])
}

// mask is a line's template and how many fixed words it kept. A word is
// masked when it holds a digit, a path separator, a quote, or is a long run
// of hex; runs of masks fold into one. Surrounding punctuation is kept apart
// from the word it wraps, so "(line 42)" and "(line 7)" mask alike, and a
// token of symbols alone (a rule of "=" or "-") is decoration and dropped.
func mask(line string) (string, int) {
	var parts []string
	fixed := 0
	masked := false
	for _, w := range strings.Fields(line) {
		core := strings.TrimFunc(w, func(r rune) bool {
			return unicode.IsPunct(r) && r != '/' && r != '\\' && r != '\'' && r != '"' && r != '`'
		})
		if core == "" || !strings.ContainsFunc(core, func(r rune) bool { return unicode.IsLetter(r) || unicode.IsDigit(r) }) {
			continue // punctuation or a rule of symbols: decoration, not a word
		}
		if varies(core) {
			if !masked {
				parts = append(parts, "<*>")
				masked = true
			}
			continue
		}
		parts = append(parts, core)
		fixed++
		masked = false
	}
	return strings.Join(parts, " "), fixed
}

func varies(w string) bool {
	if strings.ContainsAny(w, "0123456789/\\'\"`") {
		return true
	}
	if len(w) >= 8 && strings.Trim(strings.ToLower(w), "0123456789abcdef") == "" {
		return true
	}
	return false
}

func toolErrorWord(e trace.ToolError) string {
	switch e {
	case trace.ErrorEditNotFound:
		return "edit-not-found"
	case trace.ErrorEditAmbiguous:
		return "edit-ambiguous"
	case trace.ErrorEditNoChange:
		return "edit-no-change"
	case trace.ErrorStaleWrite:
		return "stale-write"
	case trace.ErrorUnreadWrite:
		return "unread-write"
	case trace.ErrorTooLarge:
		return "too-large"
	case trace.ErrorNoSuchFile:
		return "no-such-file"
	case trace.ErrorBadInput:
		return "bad-input"
	}
	return "other"
}
