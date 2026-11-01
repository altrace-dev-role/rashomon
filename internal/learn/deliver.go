package learn

import (
	"slices"
	"strings"
	"sync"

	"github.com/altrace-dev-role/rashomon/internal/jsonscan"
	"github.com/altrace-dev-role/rashomon/internal/steer"
	"github.com/altrace-dev-role/rashomon/internal/steer/pattern"
	"github.com/altrace-dev-role/rashomon/internal/steer/render"
	"github.com/altrace-dev-role/rashomon/internal/steer/render/slot"
)

// UncheckedKey is the unchecked flag's finding key: outside the id form
// (idForm), so no lesson's id can share the flag's key or its once a session
// (D171 LD11).
const UncheckedKey = "#unchecked"

// pickAll is the most approved lessons a project can hold for each to be
// given whatever the prompt names (L4).
const pickAll = 3

// Message is ls's learned-fact note: its id, and its text in quotes, through
// the renderer's slot checks and the host screen (slot.RefuseHosts).
// Approval and delivery both call it, so a person approves only text that
// can arrive (L8), and its error names the rule a text breaks.
func Message(ls Lesson) (render.Message, error) {
	if err := slot.RefuseHosts(ls.Text); err != nil {
		return render.Message{}, err
	}
	return render.New(pattern.LearnedFact, render.LearnedFactBinding{Lesson: slot.LessonID(ls.ID), Text: slot.Approved(ls.Text)})
}

// UncheckedMessage is the user's flag for a state that did not check: one
// sentence for the signature and another for the size, so a large ledger is
// never called tampered.
func UncheckedMessage(u Unchecked) (render.Message, error) {
	if u == UncheckedSize {
		return render.New(pattern.LearnedFact, render.LearnedFactOversizeBinding{})
	}
	return render.New(pattern.LearnedFact, render.LearnedFactUncheckedBinding{})
}

// Pick is the prompt's one lesson (L3, L4): the first of a's lessons, proven
// rows before the rest and then in a's order, that delivered does not name,
// that a person approved (ApprovedUnixMS is set: defence in depth, as the
// signature is tamper evidence and not a boundary), that is relevant, and
// whose Message builds. Every lesson is relevant when
// a holds pickAll or fewer; past that, one whose text names one of the
// prompt's paths whole (namesAny). An unchecked state gives none. Which tiers may
// arrive is ReadActive's to decide, once.
func Pick(a Active, delivered func(id string) bool, paths []string) (Lesson, render.Message, bool) {
	all := len(a.Lessons) <= pickAll
	return pick(a, delivered, paths, all)
}

// PickRide is the lesson the request's preflight note carries as its last
// line (LEARN-RIDE1, R13 D1): Pick's choice among the lessons whose text
// names one of the request's paths, however many a holds, so a lesson rides
// only the note of a prompt it is about.
func PickRide(a Active, delivered func(id string) bool, paths []string) (Lesson, bool) {
	ls, _, ok := pick(a, delivered, paths, false)
	return ls, ok
}

// pick is Pick with all, whether every lesson is relevant, given.
func pick(a Active, delivered func(id string) bool, paths []string, all bool) (Lesson, render.Message, bool) {
	if a.Unchecked != "" {
		return Lesson{}, render.Message{}, false
	}
	for _, proven := range [...]bool{true, false} {
		for _, ls := range a.Lessons {
			if (ls.Tier == TierProven) != proven || ls.ApprovedUnixMS == 0 || delivered(ls.ID) || !all && !namesAny(ls.Text, paths) {
				continue
			}
			if m, err := Message(ls); err == nil {
				return ls, m, true
			}
		}
	}
	return Lesson{}, render.Message{}, false
}

// namesAny reports whether text names one of paths whole: a token of text,
// read as intent's scan reads the request (a run of path bytes, trailing
// dots dropped), with a leading "./" dropped, is the path. The paths are
// root-relative (intent's resolvePaths), so a longer token that holds one
// names another file: "main.go" is not in "domain.go", "cmd/main.go" or
// "main.go.bak" (#197 F2).
func namesAny(text string, paths []string) bool {
	for i := 0; i < len(text); {
		if !pathByte(text[i]) {
			i++
			continue
		}
		j := i
		for j < len(text) && pathByte(text[j]) {
			j++
		}
		if tok := strings.TrimPrefix(strings.TrimRight(text[i:j], "."), "./"); tok != "" && slices.Contains(paths, tok) {
			return true
		}
		i = j
	}
	return false
}

// pathByte is a byte intent's scan reads inside a token: a letter, a digit,
// one of "._@+-/", or a byte at or above 0x80.
func pathByte(c byte) bool {
	return c >= 0x80 || 'a' <= c && c <= 'z' || 'A' <= c && c <= 'Z' || '0' <= c && c <= '9' || strings.IndexByte("._@+-/", c) >= 0
}

// Delivered is the session's given set from its steer lines: the key of
// every learned-fact line in state delivered, whatever its delivery
// (delivered, flag_only or holdout; PREFLIGHT1's P16 rule), and of every line
// arm off withheld (state suppressed, suppressed_by arm_off: the shadow
// decision is the steer arm's, which gave it), so a lesson, and the unchecked
// flag under its own key, comes once a session in either arm. A
// task-preflight line under the same rule gave the lesson its lesson slot
// names, one that rode the note (LEARN-RIDE1). A superseded line is not
// given. Lines are located with jsonscan, never decoded (RES-5).
func Delivered(lines [][]byte) map[string]bool {
	var out map[string]bool
	var sp [dkCount]jsonscan.Span
	for _, b := range lines {
		if jsonscan.Top(b, deliveredKeys(), sp[:]) != nil ||
			spanText(b, sp[dkState]) != string(steer.LineStateDelivered) && spanText(b, sp[dkSuppressedBy]) != string(steer.SuppressedByArmOff) {
			continue
		}
		id := ""
		switch spanText(b, sp[dkPattern]) {
		case string(pattern.LearnedFact):
			id = spanText(b, sp[dkKey])
		case string(pattern.TaskPreflight):
			if l, err := jsonscan.Lookup(b, sp[dkSlots], "lesson"); err == nil {
				id = spanText(b, l)
			}
		}
		if id == "" {
			continue
		}
		if out == nil {
			out = map[string]bool{}
		}
		out[id] = true
	}
	return out
}

// deliveredKeys is the one KeySet a steer line is read with here.
func deliveredKeys() *jsonscan.KeySet {
	deliveredKeysOnce.Do(func() {
		deliveredKeysOnce.v = jsonscan.NewKeySet("pattern", "state", "key", "suppressed_by", "slots")
	})
	return deliveredKeysOnce.v
}

// deliveredKeysOnce holds what deliveredKeys returns, built on its first
// call and never at init (RES-2).
var deliveredKeysOnce struct {
	sync.Once
	v *jsonscan.KeySet
}

const (
	dkPattern = iota
	dkState
	dkKey
	dkSuppressedBy
	dkSlots
	dkCount
)

// spanText is a string value's text, "" for any other kind or a bad escape.
func spanText(b []byte, sp jsonscan.Span) string {
	if sp.Kind != jsonscan.String {
		return ""
	}
	s, err := jsonscan.DecodeString(b, sp)
	if err != nil {
		return ""
	}
	return s
}
