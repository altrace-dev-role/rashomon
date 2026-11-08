package knowledge

import (
	"fmt"
	"slices"
	"strings"
)

// A failure ledger is a text file whose rows are "id | when | who | what
// happened | pattern / layer | evidence | catcher | today | gap / card":
// nine fields separated by " | ", one row per line, the id one capital
// and digits (B227); or, in its prose form, "id when who: what happened",
// the when a date (11-03). Lines around the rows are prose.
const ledgerFields = 9

// ParseLedger is the rows of a ledger file as lessons: the row's id, what
// happened as the title (the failure), the whole row as the text, the
// gap field's rule (gapRule: the sentence its author marked as the rule,
// else the field's first sentence that states one) as the clause before
// the approved-text rules, and the file and row as the source; a prose
// row's first sentence is its title and the sentence its author marked as
// the rule is its clause. What happened is never the clause: a brief
// states the clause as the rule, and a narrative of the failure stated as
// the rule tells the agent to do what went wrong, so a row with no rule
// written is title only. The count is the lines that start with a row id
// and are neither form. A repeated id keys on its own (B227-2).
func ParseLedger(file string, text []byte) ([]Row, int) {
	var rows []Row
	unparsed := 0
	seen := map[string]int{}
	for _, line := range strings.Split(string(text), "\n") {
		id, rest, _ := strings.Cut(line, " ")
		if !rowID(id) {
			continue
		}
		f := strings.Split(line, " | ")
		if len(f) < ledgerFields {
			if r, ok := proseRow(file, id, rest); ok {
				r.ID, r.Text = unique(seen, id), line
				rows = append(rows, r)
			} else {
				unparsed++
			}
			continue
		}
		if len(f) > ledgerFields {
			f = append(f[:ledgerFields-1], strings.Join(f[ledgerFields-1:], " | "))
		}
		rows = append(rows, Row{ID: unique(seen, id), Kind: KindLesson, Title: strings.TrimSpace(f[3]), Clause: gapRule(strings.TrimSpace(f[8])), Text: line,
			Source: Source{File: file, Row: id}, Status: StatusCandidate, Tier: TierInferred})
	}
	return rows, unparsed
}

// proseRow reads "when who: what happened" after a row id: the when a date,
// the text after the first ": " non-empty.
func proseRow(file, id, rest string) (Row, bool) {
	fields := strings.Fields(rest)
	if len(fields) == 0 || !dateLike(fields[0]) {
		return Row{}, false
	}
	_, text, ok := strings.Cut(rest, ": ")
	if text = strings.TrimSpace(text); !ok || text == "" {
		return Row{}, false
	}
	return Row{ID: id, Kind: KindLesson, Title: firstSentence(text), Clause: markedRule(text), Source: Source{File: file, Row: id}, Status: StatusCandidate, Tier: TierInferred}, true
}

// gapRule is a gap field's rule: the sentence after a rule mark when its
// author wrote the repair first ("fixed at 94c2451e; rule: a green is
// cited only with its log's exit line"); else the first sentence of the
// first of the field's clauses ("; "), after a card label, that states a
// rule (genericRule: "EVIDENCE-AT-HEAD1: a claim whose last supporting
// read names a sha other than the current head is unbacked"); else ""
// (W-M1.30). A field that holds a repair, a card or a status states no
// rule, and stated as one it would tell the agent to apply one incident's
// fix ("cff9e14f routes audit --keep through Refusal"), so the row is
// title only and its failure still renders.
func gapRule(gap string) string {
	if r := markedRule(gap); r != "" {
		return r
	}
	for _, c := range strings.Split(gap, "; ") {
		if s := firstSentence(unlabelled(strings.TrimSpace(c))); genericRule(s) {
			return s
		}
	}
	return ""
}

// unlabelled is c after its card label, the text before its first ": "
// when that text opens with a card id ("EVIDENCE-AT-HEAD1", "SDK-GATE1
// (eng-1, stacked on #228)", "PROJECT-RESOLVE1 amended"); else c.
func unlabelled(c string) string {
	label, after, ok := strings.Cut(c, ": ")
	if w, _, _ := strings.Cut(label, " "); ok && upperID(w) {
		return after
	}
	return c
}

// genericOpeners are the words a sentence opens with when it states what
// holds for every case of a kind: an indefinite or universal determiner
// ("a claim ... is unbacked", "every judge reads ...", "no note when
// ..."), a universal adverb, or the precondition "before ...". A repair
// opens with the thing it changed ("train.sh v2 refuses ...", "cff9e14f
// routes ..."), a status with its state ("out of scope", "none yet"). An
// imperative opens neither way: "save first" is a rule and "add the
// plugin archive to checksums.txt" a repair, so neither is taken.
var genericOpeners = [...]string{"a", "an", "every", "each", "any", "no", "never", "always", "before"}

// deonticModals make a sentence a norm wherever they stand ("the nightly
// purge must change with session lifetime").
var deonticModals = [...]string{"must", "should", "shall"}

// genericRule reports whether s states a rule: it names no incident, and
// it opens with one of genericOpeners or holds one of deonticModals.
func genericRule(s string) bool {
	if s == "" || incident(s) {
		return false
	}
	w, _, _ := strings.Cut(s, " ")
	if slices.Contains(genericOpeners[:], strings.ToLower(w)) {
		return true
	}
	for _, w := range strings.FieldsFunc(strings.ToLower(s), func(r rune) bool { return r < 'a' || r > 'z' }) {
		if slices.Contains(deonticModals[:], w) {
			return true
		}
	}
	return false
}

// incident reports whether s names one incident: a row, card or lane id
// (B259, QA-3, CI-GATES-ALL1, eng-4), a commit id, a clock time (08:5x),
// a date (10-31, 10/24) or an issue number (#216). A rule stated to an
// agent carries none of them; a line range (preflight.go:69-71) is no
// date.
func incident(s string) bool {
	for _, w := range strings.FieldsFunc(s, func(r rune) bool { return strings.ContainsRune(" ,;()\"`", r) }) {
		w = strings.TrimRight(w, ".:")
		if len(w) > 1 && w[0] == '#' && digits(w[1:]) || clockTime(w) || monthDay(w) || commitID(w) {
			return true
		}
		for _, p := range strings.Split(w, "/") {
			if upperID(p) || laneID(p) {
				return true
			}
		}
	}
	return false
}

// upperID is a capital, then capitals, digits and hyphens holding at least
// one digit: a row, card or lane id (B259, QA-3, CI-GATES-ALL1, K1-K4).
func upperID(w string) bool {
	if w == "" || w[0] < 'A' || w[0] > 'Z' {
		return false
	}
	digit := false
	for i := 1; i < len(w); i++ {
		switch c := w[i]; {
		case isDigit(c):
			digit = true
		case c != '-' && (c < 'A' || c > 'Z'):
			return false
		}
	}
	return digit
}

// laneID is lower-case letters, a hyphen and digits (eng-4).
func laneID(w string) bool {
	name, n, ok := strings.Cut(w, "-")
	if !ok || name == "" || !digits(n) {
		return false
	}
	for i := 0; i < len(name); i++ {
		if name[i] < 'a' || name[i] > 'z' {
			return false
		}
	}
	return true
}

// clockTime is one or two digits, a colon, a digit and a digit or x (08:5x).
func clockTime(w string) bool {
	h, m, ok := strings.Cut(w, ":")
	return ok && len(h) >= 1 && len(h) <= 2 && digits(h) && len(m) == 2 && isDigit(m[0]) && (isDigit(m[1]) || m[1] == 'x')
}

// monthDay is a month 01 to 12, a hyphen or slash, and a day 01 to 31,
// alone or after a year and a hyphen (2026-11-07).
func monthDay(w string) bool {
	if len(w) == 10 && digits(w[:4]) && w[4] == '-' {
		w = w[5:]
	}
	if len(w) != 5 || w[2] != '-' && w[2] != '/' || !digits(w[:2]) || !digits(w[3:]) {
		return false
	}
	m, d := int(w[0]-'0')*10+int(w[1]-'0'), int(w[3]-'0')*10+int(w[4]-'0')
	return m >= 1 && m <= 12 && d >= 1 && d <= 31
}

// commitID is 7 to 40 lower-case hex digits holding a digit and a letter.
func commitID(w string) bool {
	if len(w) < 7 || len(w) > 40 {
		return false
	}
	digit, letter := false, false
	for i := 0; i < len(w); i++ {
		switch c := w[i]; {
		case isDigit(c):
			digit = true
		case c >= 'a' && c <= 'f':
			letter = true
		default:
			return false
		}
	}
	return digit && letter
}

// digits is one or more decimal digits.
func digits(s string) bool {
	for i := 0; i < len(s); i++ {
		if !isDigit(s[i]) {
			return false
		}
	}
	return s != ""
}

// ruleMarks are the labels a ledger author puts before the rule the
// failure broke, in the order they are read: the intent the session
// missed, then a rule or a lesson stated as such (a gap field writes them
// lower case after the repair). "Fix:" is not one: it names the repair of
// that incident, not the rule.
var ruleMarks = [...]string{"Intent missed: ", "Rule: ", "rule: ", "Lesson: ", "lesson: "}

// markedRule is the first sentence after the first mark of ruleMarks the
// text holds, or "" when it holds none.
func markedRule(text string) string {
	for _, m := range ruleMarks {
		if _, after, ok := strings.Cut(text, m); ok {
			return firstSentence(strings.TrimSpace(after))
		}
	}
	return ""
}

// dateLike is a token starting with two digits, a hyphen and two digits.
func dateLike(s string) bool {
	return len(s) >= 5 && isDigit(s[0]) && isDigit(s[1]) && s[2] == '-' && isDigit(s[3]) && isDigit(s[4])
}

func isDigit(c byte) bool { return c >= '0' && c <= '9' }

// rowID is one capital and one to six digits.
func rowID(s string) bool {
	if len(s) < 2 || len(s) > 7 || s[0] < 'A' || s[0] > 'Z' {
		return false
	}
	for i := 1; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

// unique is id the first time and id-n after, so one source's rows never
// share a key.
func unique(seen map[string]int, id string) string {
	seen[id]++
	if seen[id] == 1 {
		return id
	}
	return fmt.Sprintf("%s-%d", id, seen[id])
}
