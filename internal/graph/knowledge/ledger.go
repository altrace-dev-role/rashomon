package knowledge

import (
	"fmt"
	"strings"
)

// A failure ledger is a text file whose rows are "id | when | who | what
// happened | pattern / layer | evidence | catcher | today | gap / card":
// nine fields separated by " | ", one row per line, the id one capital
// and digits (B227); or, in its prose form, "id when who: what happened",
// the when a date (11-03). Lines around the rows are prose.
const ledgerFields = 9

// ParseLedger is the rows of a ledger file as lessons: the row's id, what
// happened as the title (the failure), the whole row as the text, the gap
// field as the clause (the rule) before the approved-text rules, and the
// file and row as the source; a prose row's first sentence is its title
// and the sentence its author marked as the rule is its clause. What
// happened is never the clause: a brief states the clause as the rule, and
// a narrative of the failure stated as the rule tells the agent to do what
// went wrong, so a row with no rule written is title only. The count is
// the lines that start with a row id and are neither form. A repeated id
// keys on its own (B227-2).
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
		rows = append(rows, Row{ID: unique(seen, id), Kind: KindLesson, Title: strings.TrimSpace(f[3]), Clause: strings.TrimSpace(f[8]), Text: line,
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
	return Row{ID: id, Kind: KindLesson, Title: firstSentence(text), Clause: proseRule(text), Source: Source{File: file, Row: id}, Status: StatusCandidate, Tier: TierInferred}, true
}

// ruleMarks are the labels a prose row's author puts before the rule the
// failure broke, in the order they are read: the intent the session
// missed, then a rule or a lesson stated as such. "Fix:" is not one: it
// names the repair of that incident, not the rule.
var ruleMarks = [...]string{"Intent missed: ", "Rule: ", "Lesson: "}

// proseRule is the first sentence after the first mark of ruleMarks the
// text holds, or "" when it holds none.
func proseRule(text string) string {
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
