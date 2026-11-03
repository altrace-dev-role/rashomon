package recap

import (
	"strconv"
	"strings"

	"github.com/altrace-dev-role/rashomon/internal/spec"
	"github.com/altrace-dev-role/rashomon/internal/steer/render/slot"
	"github.com/altrace-dev-role/rashomon/internal/verify"
)

// maxRuleLines is how many rules the Stop names; the rest are one line that
// counts them.
const maxRuleLines = 2

// ruleLines is a line for each trusted or managed spec rule the turn's calls
// matched (verify.StopView.Violations, card RECALL-15): the rule's id and the
// turn's calls, so the person reads at the Stop the rule the post note named
// to the agent at the call. The value line's k counts the record's findings
// and names none (1.8 Gate B F1); these lines sit beside it and name only
// the rules the person declared. At most maxRuleLines, then one line counting
// the rest.
func ruleLines(v verify.StopView) string {
	var b strings.Builder
	for _, r := range v.Violations[:min(len(v.Violations), maxRuleLines)] {
		b.WriteString(ruleLine(r))
	}
	if n := len(v.Violations) - maxRuleLines; n > 0 {
		b.WriteString(continuation() + "and " + strconv.Itoa(n) + " more spec " + noun(n, "rule", "rules") + " matched\n")
	}
	return b.String()
}

// RuleLines is ruleLines for a caller outside this package: recapTurn hands
// them to LineWith for recap.Line's sentences to carry.
func RuleLines(v verify.StopView) string { return ruleLines(v) }

// RuleLine is the line that names the turn's rules where no other line
// speaks: a turn that matched a rule and wrote nothing and ran no gate has
// no value line (RowNone), and a turn under a summary switch set off prints
// none (card RECALL-15, B237). "" for a turn that matched none.
func RuleLine(v verify.StopView, sessionID string, fromPlugin bool) string {
	n := len(v.Violations)
	if n == 0 {
		return ""
	}
	head := "a spec rule matched in this turn"
	if n > 1 {
		head = strconv.Itoa(n) + " spec rules matched in this turn"
	}
	return prefix + head + ".\n" + ruleLines(v) + pointer(sessionID, fromPlugin)
}

// ruleLine is one rule's line: "spec rule <id> <what>", the id without its
// specPrefix as the rule id slot admits it, as the post note names it
// (verify.renderRule), else "a spec rule". Past lineColumns, what goes on a
// line of its own, and past that it prints whole (D86).
func ruleLine(r verify.RuleVerdict) string {
	name := "a spec rule"
	if id := slot.RuleID(strings.TrimPrefix(string(r.Rule), specPrefix)); id.Validate() == nil {
		name = "spec rule " + id.Text()
	}
	what := ruleWhat(r)
	one := continuation() + name + " " + what
	if columns(one) <= lineColumns {
		return one + "\n"
	}
	return continuation() + name + ":\n" + continuation() + "  " + what + "\n"
}

// ruleWhat is what the record holds of a rule's kind: the calls a command
// matched, the calls that wrote a path it protects, or evidence it requires
// not green at the Stop, whose seq is the Stop's and no call's.
func ruleWhat(r verify.RuleVerdict) string {
	switch r.Kind {
	case spec.ProtectPath:
		return "protects a path " + calls(r.Seqs) + " wrote"
	case spec.RequireEvidence:
		return "has evidence not green at the Stop"
	}
	return "matched " + calls(r.Seqs)
}

// calls names the seqs: "call 3", "calls 3 and 7", "calls 3, 7 and 2 more".
func calls(seqs []int64) string {
	switch len(seqs) {
	case 0:
		return "a call"
	case 1:
		return "call " + strconv.FormatInt(seqs[0], 10)
	case 2:
		return "calls " + strconv.FormatInt(seqs[0], 10) + " and " + strconv.FormatInt(seqs[1], 10)
	}
	return "calls " + strconv.FormatInt(seqs[0], 10) + ", " + strconv.FormatInt(seqs[1], 10) + " and " +
		strconv.Itoa(len(seqs)-2) + " more"
}
