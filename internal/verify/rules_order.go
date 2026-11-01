package verify

import (
	"strings"

	"github.com/altrace-dev-role/rashomon/internal/spec"
	"github.com/altrace-dev-role/rashomon/internal/store"
)

// The order and forbid_code kinds of SPEC-RULES1 1b, read over what
// ruleVerdicts classified: the Bash lines (ruleLines) and the writes.

// codeOver is a forbid_code rule over the writes at or before at. The record
// holds no edit content yet (EditMarks' pre-read, EDITMARKS-WIRE1; card
// SPEC-RULES1), so a write the rule's paths match, or that may be one, is
// undecided with reason ReasonContentNotRead and the rule is never
// violated; with no such write it holds. A rule with no pattern or no paths
// is undecided, as a missing field is.
func codeOver(r *spec.Rule, c *classified, rels []projectRel, at int64) ruleRead {
	if r.Pattern == "" || len(r.Paths) == 0 {
		return ruleRead{undecided: true}
	}
	w := protectOver(&spec.Rule{Paths: r.Paths, ExcludePaths: r.ExcludePaths}, c, rels, at)
	if len(w.seqs) == 0 && !w.undecided {
		return ruleRead{}
	}
	return ruleRead{undecided: true, reason: ReasonContentNotRead}
}

// lineMatch is the rule's command over one line: yes where one of its
// commands matches for certain, maybe where one may or the line was not read
// whole, else no.
func lineMatch(m *spec.CommandMatch, l *ruleLine) tri {
	t := triOf(false, !l.whole)
	for _, c := range l.cmds {
		if t = max(t, commandMatch(m, c)); t == triYes {
			break
		}
	}
	return t
}

// orderOver is an order rule over the lines at or before at: a violation at
// each line its then command matches for certain with no qualifying earlier
// after run (exit 0, foreground, the same directory, no write its
// invalidated_by paths match between them). A then or an after the line does
// not show, both in one line (their order is the shell's), a glob or an
// unnamed write between, a line not read whole, and a rule with no then,
// after, or with an after_gate or a same_run_key (not read yet) are
// undecided.
func orderOver(r *spec.Rule, lines []ruleLine, picked map[string]*store.Execution, c *classified, rels []projectRel, at int64) ruleRead {
	var out ruleRead
	if r.Then == nil || len(r.Then.Argv) == 0 || r.After == nil || len(r.After.Argv) == 0 || len(r.SameRunKey) > 0 {
		out.undecided = true
		return out
	}
	for i := range lines {
		l := &lines[i]
		then := lineMatch(r.Then, l)
		if then == triNo {
			continue
		}
		best := triNo
		if lineMatch(r.After, l) != triNo {
			best = triMaybe
		}
		for j := range lines {
			a := &lines[j]
			if a.seq >= l.seq {
				continue
			}
			if m := lineMatch(r.After, a); m != triNo {
				best = max(best, min(m, afterRan(a, l, picked), untouched(r, c, rels, a.seq, l.seq, at)))
			}
		}
		switch {
		case best == triYes:
		case best == triMaybe || then == triMaybe:
			out.undecided = true
		default:
			out.seqs = append(out.seqs, l.seq)
		}
	}
	return out
}

// afterRan is whether the after line's call qualifies for the then line's:
// it landed with outcome ok in the foreground, in the same directory.
func afterRan(a, then *ruleLine, picked map[string]*store.Execution) tri {
	e := picked[a.id]
	if e == nil || e.Outcome != store.ExecOK || e.Backgrounded || a.cwd != then.cwd {
		return triNo
	}
	return triYes
}

// untouched is whether no write the rule's invalidated_by paths match lies
// strictly between the seqs: no where one does, maybe where a write between
// may (a glob, a target the record could not name), else yes. A rule with no
// invalidated_by is never invalidated.
func untouched(r *spec.Rule, c *classified, rels []projectRel, from, to, at int64) tri {
	if len(r.InvalidatedBy) == 0 {
		return triYes
	}
	t := triYes
	for i := range c.writes {
		w := &c.writes[i]
		if w.Seq <= from || w.Seq >= to || w.Seq > at {
			continue
		}
		switch rel := rels[i].rel; {
		case w.Path == nil:
			if !strings.HasPrefix(w.Tool, "mcp__") {
				t = triMaybe
			}
		case !rels[i].inside:
		case w.Tool == "Bash" && strings.ContainsAny(rel, "*?[{"):
			t = triMaybe
		case anyPath(r.InvalidatedBy, rel):
			return triNo
		}
	}
	return t
}
