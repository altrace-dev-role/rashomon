package verify

import (
	"slices"
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

// afterHit is an order rule's After call as its post recorded it (the
// after_hit record, #216 ruling 1): seq is the call's declaration seq, the rest the
// record's own fields. The judge reads these and never an After call's line.
type afterHit struct {
	seq   int64
	exit  *int
	fg    bool
	cwd   string
	maybe bool
}

// afterHits is the after_hit records of the rule id recorded while its declaring
// file had digest, whose call landed at or before at, each at its call's
// declaration seq (hitSeq), as ruleHitSeqs reads a forbid_command rule's
// hits: an edit of the file drops its earlier hits, which then qualify
// nothing. Cost: O(H) per order rule.
func afterHits(run *store.Run, id, digest string, at int64) []afterHit {
	if digest == "" {
		return nil
	}
	var out []afterHit
	for i := range run.AfterHits {
		h := &run.AfterHits[i]
		if h.RuleID == id && h.SourceDigest == digest && h.LandedSeq > 0 && h.LandedSeq <= at {
			out = append(out, afterHit{seq: hitSeq(run, h.LandedSeq), exit: h.ExitCode, fg: h.Foreground, cwd: h.CWDDigest, maybe: h.Maybe})
		}
	}
	return out
}

// thenChecks is the then_check verdicts of the rule id recorded while its
// declaring file had digest, whose call landed at or before at, by their
// call's declaration seq, as afterHits reads the rule's hits: an edit of the
// file drops them, and a word other than yes or no reads maybe. Two checks
// for one call that disagree read maybe. Cost: O(N + C) per order rule, the
// landed-to-declaration index built once (hitSeq per check would be O(N*C),
// and every Bash call under the rule leaves one); nothing for a run with no
// checks.
func thenChecks(run *store.Run, id, digest string, at int64) map[int64]tri {
	if digest == "" || len(run.ThenChecks) == 0 {
		return nil
	}
	decl := make(map[string]int64, len(run.Declarations))
	for i := range run.Declarations {
		if d := &run.Declarations[i]; decl[d.ToolUseID] == 0 {
			decl[d.ToolUseID] = d.Seq
		}
	}
	landed := make(map[int64]int64, len(run.Executions))
	for i := range run.Executions {
		if x := &run.Executions[i]; x.Seq != nil && landed[*x.Seq] == 0 && decl[x.ToolUseID] != 0 {
			landed[*x.Seq] = decl[x.ToolUseID]
		}
	}
	out := map[int64]tri{}
	for i := range run.ThenChecks {
		c := &run.ThenChecks[i]
		if c.RuleID != id || c.SourceDigest != digest || c.LandedSeq <= 0 || c.LandedSeq > at {
			continue
		}
		t := triMaybe
		switch c.Then {
		case store.ThenYes:
			t = triYes
		case store.ThenNo:
			t = triNo
		}
		seq := c.LandedSeq
		if d := landed[seq]; d != 0 {
			seq = d
		}
		if old, ok := out[seq]; ok && old != t {
			t = triMaybe
		}
		out[seq] = t
	}
	return out
}

// thenWord is a then_check's verdict for t.
func thenWord(t tri) string {
	switch t {
	case triYes:
		return store.ThenYes
	case triNo:
		return store.ThenNo
	}
	return store.ThenMaybe
}

// orderOver is an order rule over the Then lines at or before at and the
// after_hits before each: a violation at each line its then command matches
// for certain with no qualifying earlier hit (exit 0, foreground, the same
// directory, no write its invalidated_by paths match between them). A then
// the line does not show, a then and an after in one line (their order is
// the shell's), an earlier line shown to the reader that the after matches
// and that left no hit (a lost record, never a violation; a line the record
// withheld shows nothing, so it adds no doubt), a hit that could not say it ran the after
// (maybe), a hit or then with no known directory, a glob or an unnamed write
// between, a line not read whole, and a rule with no then, after, or with an
// after_gate or a same_run_key (not read yet) are undecided. A line not read
// whole takes its then from the post's then_check where one landed (checks,
// card ORDER-LIVE1), and an after in the same line from a hit at that call;
// with no check it stays undecided.
func orderOver(r *spec.Rule, lines []ruleLine, hits []afterHit, checks map[int64]tri, c *classified, rels []projectRel, at int64) ruleRead {
	var out ruleRead
	if r.Then == nil || len(r.Then.Argv) == 0 || r.After == nil || len(r.After.Argv) == 0 || len(r.SameRunKey) > 0 {
		out.undecided = true
		return out
	}
	for i := range lines {
		l := &lines[i]
		then, after := lineMatch(r.Then, l), lineMatch(r.After, l)
		if t, ok := checks[l.seq]; ok && !l.whole {
			then, after = t, triOf(false, hasHit(hits, l.seq))
		}
		if then == triNo {
			continue
		}
		best := triNo
		if after != triNo {
			best = triMaybe
		}
		for j := range hits {
			h := &hits[j]
			if h.seq >= l.seq {
				continue
			}
			best = max(best, min(afterRan(h, l), untouched(r, c, rels, h.seq, l.seq, at)))
		}
		for j := range lines {
			a := &lines[j]
			if a.seq < l.seq && len(a.cmds) > 0 && lineMatch(r.After, a) != triNo && !hasHit(hits, a.seq) {
				best = max(best, triMaybe)
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

// hasHit reports a hit recorded for the call declared at seq.
func hasHit(hits []afterHit, seq int64) bool {
	return slices.ContainsFunc(hits, func(h afterHit) bool { return h.seq == seq })
}

// afterRan is whether the after hit qualifies for the then line's: the call
// ended with exit 0 in the foreground, in the same directory. A hit that
// said it may not be the after, or either call's directory not known, is
// maybe.
func afterRan(h *afterHit, then *ruleLine) tri {
	switch {
	case h.maybe:
		return triMaybe
	case h.exit == nil || *h.exit != 0 || !h.fg:
		return triNo
	case h.cwd == "" || then.cwd == "":
		return triMaybe
	case h.cwd != then.cwd:
		return triNo
	}
	return triYes
}

// untouched is whether no write the rule's invalidated_by paths match lies
// strictly between the seqs: no where one does, maybe where a write between
// may (a glob, a target the record could not name, a call that may have
// written and did not end ok: heldBlocks), else yes. A rule with no
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
	if heldBlocks(c.held, r.InvalidatedBy, nil, nil, from, min(to, at+1)) {
		t = triMaybe
	}
	return t
}
