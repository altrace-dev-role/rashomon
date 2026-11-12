package recap

import (
	"strconv"
	"strings"

	"github.com/altrace-dev-role/rashomon/internal/steer/render/slot"
	"github.com/altrace-dev-role/rashomon/internal/verify"
)

// stopMessageChars is Claude Code's cap on a hook's systemMessage, in
// characters (cmd/rashomon/recap_turn.go, the human channel's comment). Past
// it the tail of the message is cut, which is the clause and the pointer, so
// the conflict lines, the one kind with no count cap of its own to bound the
// message, take only what every other line leaves (conflictLines).
const stopMessageChars = 10000

// maxConflictLines is how many unresolved conflicts the Stop names; the rest
// are one line that counts them.
const maxConflictLines = 2

// conflictPhrases is each kind's phrase, one per verify.ConflictRowKinds
// entry and no more: a withheld kind has none and so says nothing. Each rests
// on what that kind's producer in internal/graph/engine checks (boundaryConflicts,
// decisionConflicts, signatureConflicts, protectedConflicts) and claims no more.
var conflictPhrases = []struct{ kind, phrase string }{
	// boundary.go: an edge the diff adds, a call, possible call, import or
	// reference, from a unit the rule's From names into a unit its To names.
	{"boundary_cross", "adds a call, import or reference across a boundary a rule sets"},
	// decision.go: paths_untouched (a path or unit the diff touches) and
	// symbol_present (an added callable that reaches no symbol the rule names).
	{"decision_contradicted", "goes against a decision a rule records"},
	// signature.go: a callable whose sig changed between base and now, with a
	// caller whose argument count the new signature does not admit.
	{"signature_mismatch", "changes a signature that a caller no longer fits"},
	// protected.go: a file in the diff that the rule applies to.
	{"protected_path", "edits a file a rule protects"},
}

// ConflictPhraseKinds is the kinds conflictPhrases holds, in its order, for
// the pin that they equal verify.ConflictRowKinds.
func ConflictPhraseKinds() []string {
	out := make([]string, 0, len(conflictPhrases))
	for _, p := range conflictPhrases {
		out = append(out, p.kind)
	}
	return out
}

func conflictPhrase(kind string) (string, bool) {
	for _, p := range conflictPhrases {
		if p.kind == kind {
			return p.phrase, true
		}
	}
	return "", false
}

// liveConflicts is v.Conflicts less any row whose kind has no phrase, so a
// withheld kind in a hand-built view still says nothing.
func liveConflicts(v verify.StopView) []verify.ConflictRow {
	var out []verify.ConflictRow
	for _, r := range v.Conflicts {
		if _, ok := conflictPhrase(r.Kind); ok {
			out = append(out, r)
		}
	}
	return out
}

// conflictLine is one unresolved conflict's line: the rule id as ruleLine
// names it (a rule-less kind names none) and the unit as the brief does,
// "unit " and the path slot's text, else "a unit": less, never wrong. Past
// lineColumns the detail goes on a line of its own, and past that it prints
// whole (D86).
func conflictLine(r verify.ConflictRow, phrase string) string {
	unit := "a unit"
	if s := slot.Path(r.UnitPath); r.UnitPath != "" && s.Validate() == nil {
		unit = "unit " + s.Text()
	}
	name, what := unit, phrase
	if r.RuleID != "" {
		name = "a spec rule"
		if id := slot.RuleID(strings.TrimPrefix(r.RuleID, specPrefix)); id.Validate() == nil {
			name = "spec rule " + id.Text()
		}
		what = unit + " " + phrase
	}
	one := continuation() + name + ": " + what
	if columns(one) <= lineColumns {
		return one + "\n"
	}
	return continuation() + name + ":\n" + continuation() + "  " + what + "\n"
}

// conflictLines is a line for each unresolved conflict (verify.StopView.
// Conflicts, W-M2.4), at most maxConflictLines, then one line counting the
// rest. rest is every other character the message carries, head, lines,
// clause and pointer included: the lines take what is left of
// stopMessageChars after it. If they would pass it, the counting line goes
// first, then the lines are replaced by a line that counts them all, then
// nothing; a new line never cuts the tail (W-M1.29). The catch-up prints the
// message through PreviousTurn, which only lengthens it, so the budget is
// measured on that form of both rest and the lines.
func conflictLines(v verify.StopView, fromPlugin bool, rest string) string {
	rows := liveConflicts(v)
	if len(rows) == 0 {
		return ""
	}
	var b strings.Builder
	for _, r := range rows[:min(len(rows), maxConflictLines)] {
		phrase, _ := conflictPhrase(r.Kind)
		b.WriteString(conflictLine(r, phrase))
	}
	lines, where := b.String(), ""
	if !fromPlugin {
		where = ": rashomon report --ledger"
	}
	n := len(rows) - maxConflictLines
	alone := continuation() + strconv.Itoa(len(rows)) + " unresolved " + noun(len(rows), "conflict", "conflicts") + where + "\n"
	var candidates []string
	if n > 0 {
		counted := lines + continuation() + "and " + strconv.Itoa(n) + " more unresolved " + noun(n, "conflict", "conflicts") + where + "\n"
		candidates = []string{counted, lines, alone, ""}
	} else {
		candidates = []string{lines, alone, ""}
	}
	left := stopMessageChars - columns(PreviousTurn(rest))
	for _, c := range candidates {
		// c sits after a newline in the message, so its first line widens too.
		if c == "" || columns(PreviousTurn("\n"+c))-1 <= left {
			return c
		}
	}
	return ""
}

// ConflictLines is conflictLines for a caller outside this package:
// recapTurn hands them to LineWith for recap.Line's sentences to carry. rest
// is the message without them, as LineWith composed it.
func ConflictLines(v verify.StopView, fromPlugin bool, rest string) string {
	return conflictLines(v, fromPlugin, rest)
}

// ConflictPhrase is kind's phrase as the Stop's line says it, for the post's
// conflict note to say the same (cmd/rashomon conflictMessages); false for a
// kind with none, a withheld kind among them.
func ConflictPhrase(kind string) (string, bool) {
	return conflictPhrase(kind)
}
