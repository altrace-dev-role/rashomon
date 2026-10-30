package verify

import (
	"path"
	"slices"
	"strings"

	"github.com/altrace-dev-role/rashomon/internal/shape"
	"github.com/altrace-dev-role/rashomon/internal/spec"
	"github.com/altrace-dev-role/rashomon/internal/store"
)

// tri is a rule condition read three ways, ordered so that min is "and" and
// max is "or" (Kleene's): it does not hold, the record cannot tell, it holds.
type tri uint8

const (
	triNo tri = iota
	triMaybe
	triYes
)

// triOf is triYes when all holds, triMaybe when only some does, else triNo.
func triOf(all, some bool) tri {
	switch {
	case all:
		return triYes
	case some:
		return triMaybe
	}
	return triNo
}

// ruleRead is one rule over the window: the calls that violated it, and
// whether a call it could not decide was seen. A violation beats an
// undecided call.
type ruleRead struct {
	seqs      []int64
	undecided bool
}

func (r ruleRead) verdict() RuleVerdictValue {
	switch {
	case len(r.seqs) > 0:
		return Violation
	case r.undecided:
		return RuleUnknown
	}
	return NoRecordedViolation
}

// ruleVerdicts is the spec's rules judged over the record up to the point
// (archive 20.2.8's evaluation table), one verdict per rule that applies, in
// spec order, each with its rule's own trust (TrustOf). forbid_command reads
// the command lines a Bash call recorded, each read once for every rule;
// protect_path the writes classify named; require_evidence the snapshot's
// gates, at a point its at list names, done read at Stop and only when the
// window holds a relevant write (wrote). A rule missing the field its kind
// reads is undecided (spec/file.go, Rule). forbid_code and order are not
// judged yet and have no entry. insufficient is the 30-day window's and
// never read at a point.
func ruleVerdicts(sp *spec.Compiled, run *store.Run, picked map[string]*store.Execution, c *classified, snap *Snapshot, wrote bool) []RuleVerdict {
	out := []RuleVerdict{}
	if sp == nil {
		return out
	}
	at := snap.Point.Seq
	var lines []ruleLine
	read := false
	for i := range sp.Rules {
		dr := &sp.Rules[i]
		kind := spec.RuleKind(dr.Rule.Kind)
		var r ruleRead
		switch kind {
		case spec.ForbidCommand:
			if !read {
				lines, read = ruleLines(run, picked, at), true
			}
			r = forbidOver(dr.Rule.Command, lines)
		case spec.ProtectPath:
			r = protectOver(&dr.Rule, c, at)
		case spec.RequireEvidence:
			var ok bool
			if r, ok = evidenceAt(&dr.Rule, snap, wrote); !ok {
				continue
			}
		default:
			continue
		}
		slices.Sort(r.seqs)
		seqs := append([]int64{}, slices.Compact(r.seqs)...)
		out = append(out, RuleVerdict{Rule: SlotID(dr.ID), Kind: kind, Verdict: r.verdict(), Seqs: seqs,
			HardControl: dr.Rule.HardControl, trust: sp.TrustOf(dr.Source)})
	}
	return out
}

// binding reports a trust whose rules bind the record: trusted or managed.
// An untrusted rule's verdict is reported and binds nothing; the implied
// task-file rule's (builtin) is integrity/store-touched's.
func binding(t spec.Trust) bool { return t == spec.Trusted || t == spec.Managed }

// bindingViolations counts the violations of binding rules: each holds
// record-green false (PF-PG8) and is one of the value line's k.
func bindingViolations(vs []RuleVerdict) int {
	n := 0
	for _, v := range vs {
		if v.Verdict == Violation && binding(v.trust) {
			n++
		}
	}
	return n
}

// wroteIn reports a named relevant write at or before at: WroteRelevant over
// writes classify already built.
func wroteIn(writes []write, at int64) bool {
	for i := range writes {
		if w := &writes[i]; w.Seq <= at && w.Path != nil && Relevant(w.Class) {
			return true
		}
	}
	return false
}

// ruleLine is one command line a Bash call recorded, as shape.Commands read
// it: the declaration's seq, its commands, and whether it was read whole. A
// line withheld (record scope v4, oversize), cut or masked is not whole; a
// cut one is not read at all, since its head and tail do not join.
type ruleLine struct {
	seq   int64
	cmds  []shape.Command
	whole bool
}

// ruleLines is every Bash call's line declared at or before at, and the line
// it ran as where another hook rewrote it (ExecutedCommand), both under the
// declaration's seq.
func ruleLines(run *store.Run, picked map[string]*store.Execution, at int64) []ruleLine {
	var out []ruleLine
	for i := range run.Declarations {
		d := &run.Declarations[i]
		if d.Seq > at || d.ToolName != "Bash" {
			continue
		}
		out = append(out, lineOf(d.Seq, d.Command, d.CommandCut != nil, len(d.Masked) > 0))
		e := picked[d.ToolUseID]
		switch {
		case e == nil:
		case e.ExecutedCommand != nil:
			out = append(out, lineOf(d.Seq, e.ExecutedCommand, e.ExecutedCommandCut != nil, len(e.ExecutedMasked) > 0))
		case e.ExecutedCommandWithheld != nil:
			out = append(out, ruleLine{seq: d.Seq})
		}
	}
	return out
}

func lineOf(seq int64, line *string, cut, masked bool) ruleLine {
	if line == nil || cut {
		return ruleLine{seq: seq}
	}
	cmds, ok := shape.Commands(*line)
	return ruleLine{seq: seq, cmds: cmds, whole: ok && !masked}
}

// forbidOver is a forbid_command rule over the lines: a violation at each
// line one of whose commands it matches, undecided where a line holds only
// commands it cannot decide or was not read whole.
func forbidOver(m *spec.CommandMatch, lines []ruleLine) ruleRead {
	var r ruleRead
	if m == nil || len(m.Argv) == 0 {
		r.undecided = true
		return r
	}
	for i := range lines {
		l := &lines[i]
		t := triOf(false, !l.whole)
		for _, c := range l.cmds {
			if t = max(t, commandMatch(m, c)); t == triYes {
				break
			}
		}
		switch t {
		case triYes:
			r.seqs = append(r.seqs, l.seq)
		case triMaybe:
			r.undecided = true
		}
	}
	return r
}

// commandMatch is archive 20.2.8's forbid_command test over one command:
// the argv a prefix of its words, the program by its base name, and then
// any flags token present, a flag_values value matched, or every
// require_flags token absent; a rule with no condition is its argv alone.
// An expanding word where an argv word or a condition token could stand
// leaves it undecided, and so does a shell's -c string, eval, a wrapper
// that runs the rule's program, or a here-document feeding a shell. sql is
// undecided: no statement reader yet. Options between the argv words are
// not stepped over (`git -C repo push`).
func commandMatch(m *spec.CommandMatch, c shape.Command) tri {
	prog, want := path.Base(c.Words[0]), path.Base(m.Argv[0])
	if prog != want {
		return hiddenRun(prog, want, c)
	}
	t := triYes
	for k := 1; k < len(m.Argv); k++ {
		switch {
		case k >= len(c.Words):
			return triNo
		case c.Expands[k]:
			t = triMaybe
		case c.Words[k] != m.Argv[k]:
			return triNo
		}
	}
	return min(t, conditions(m, c.Words[len(m.Argv):], c.Expands[len(m.Argv):]))
}

// hiddenRun is whether a command whose program is not the rule's may run
// it where the line does not show it: a shell given a -c string or a
// here-document, eval, or a wrapper, with the rule's program or an expanding
// word after it.
func hiddenRun(prog, want string, c shape.Command) tri {
	sh := isShell(prog)
	switch {
	case sh && c.Heredoc:
		return triMaybe
	case sh && dashC(c.Words[1:]), prog == "eval", isWrapper(prog):
	default:
		return triNo
	}
	for j := 1; j < len(c.Words); j++ {
		if c.Expands[j] || mentions(c.Words[j], want) {
			return triMaybe
		}
	}
	return triNo
}

func isShell(p string) bool {
	switch p {
	case "sh", "bash", "zsh", "dash", "ksh", "mksh", "ash", "fish":
		return true
	}
	return false
}

func isWrapper(p string) bool {
	switch p {
	case "sudo", "doas", "env", "nohup", "time", "nice", "exec", "command", "xargs", "timeout", "stdbuf":
		return true
	}
	return false
}

// dashC reports a short-option word holding c: -c, -lc, -ec.
func dashC(words []string) bool {
	for _, w := range words {
		if len(w) > 1 && w[0] == '-' && w[1] != '-' && strings.IndexByte(w, 'c') > 0 {
			return true
		}
	}
	return false
}

// mentions reports a program name in w whose base name is want: w split at
// every byte a program name cannot hold, so `cd x&&git push` names git.
func mentions(w, want string) bool {
	from := -1
	for i := 0; i <= len(w); i++ {
		if i < len(w) && nameByte(w[i]) {
			if from < 0 {
				from = i
			}
			continue
		}
		if from >= 0 && path.Base(w[from:i]) == want {
			return true
		}
		from = -1
	}
	return false
}

func nameByte(c byte) bool {
	return 'a' <= c && c <= 'z' || 'A' <= c && c <= 'Z' || '0' <= c && c <= '9' || strings.IndexByte("._+-/", c) >= 0
}

// conditions is the rule's condition over the words after its argv: true
// when it has none; otherwise any of flags, flag_values and require_flags
// holding, sql undecided.
func conditions(m *spec.CommandMatch, rest []string, exp []bool) tri {
	if len(m.Flags) == 0 && len(m.FlagValues) == 0 && len(m.RequireFlags) == 0 && len(m.SQL) == 0 {
		return triYes
	}
	opaque := slices.Contains(exp, true)
	t := triOf(false, len(m.SQL) > 0)
	if len(m.Flags) > 0 {
		t = max(t, triOf(anyFlag(m.Flags, rest, exp), opaque))
	}
	if len(m.FlagValues) > 0 {
		t = max(t, flagValue(m.FlagValues, rest, exp, opaque))
	}
	if len(m.RequireFlags) > 0 && !anyFlag(m.RequireFlags, rest, exp) {
		t = max(t, triOf(!opaque, true))
	}
	return t
}

// anyFlag reports a literal word that is one of flags, as itself or in its
// flag= form.
func anyFlag(flags, words []string, exp []bool) bool {
	for j, w := range words {
		if exp[j] {
			continue
		}
		for _, f := range flags {
			if w == f || len(w) > len(f) && w[len(f)] == '=' && strings.HasPrefix(w, f) {
				return true
			}
		}
	}
	return false
}

// flagValue is a flag_values entry matched by a literal flag and its value,
// as two words or in the flag=value form, the value compared without case.
func flagValue(values map[string][]string, words []string, exp []bool, opaque bool) tri {
	for j, w := range words {
		if exp[j] {
			continue
		}
		flag, val, eq := strings.Cut(w, "=")
		vals, ok := values[flag]
		switch {
		case !ok:
		case eq && foldIn(vals, val):
			return triYes
		case !eq && j+1 < len(words) && !exp[j+1] && foldIn(vals, words[j+1]):
			return triYes
		}
	}
	return triOf(false, opaque)
}

func foldIn(vals []string, s string) bool {
	for _, v := range vals {
		if strings.EqualFold(v, s) {
			return true
		}
	}
	return false
}

// protectOver is a protect_path rule over the writes at or before at: a
// violation at each write to a path its paths match and its exclude_paths
// do not, in a mode it lists. Edit, MultiEdit and NotebookEdit modify; a
// Write creates or modifies, which the record does not tell apart; a Bash
// target may create, modify or delete. A write the record cannot name, a
// Bash target holding a glob, a Bash line whose targets were not all read,
// and a mode the record cannot tell are undecided; an MCP call is no write
// a path rule reads. A path outside the project matches nothing; a symlink
// is matched by the name written, not the file it reaches.
func protectOver(r *spec.Rule, c *classified, at int64) ruleRead {
	var out ruleRead
	if len(r.Paths) == 0 {
		out.undecided = true
		return out
	}
	blind, unnamed := 0, 0
	for i := range c.writes {
		w := &c.writes[i]
		if w.Seq > at {
			continue
		}
		if w.Path == nil {
			blind++
			out.undecided = out.undecided || !strings.HasPrefix(w.Tool, "mcp__")
			continue
		}
		rel, inside := projectPath(*w.Path)
		switch {
		case !inside:
		case w.Tool == "Bash" && strings.ContainsAny(rel, "*?[{"):
			out.undecided = true
		case !protects(r, rel):
		default:
			switch modeTri(w.Tool, r.Modes) {
			case triYes:
				out.seqs = append(out.seqs, w.Seq)
			case triMaybe:
				out.undecided = true
			}
		}
	}
	// Every blind write is in unnamed once; one more is a Bash line whose
	// targets were not all read (appendWrites).
	for _, s := range c.unnamed {
		if s <= at {
			unnamed++
		}
	}
	out.undecided = out.undecided || unnamed > blind
	return out
}

// projectPath is a written path cleaned for the matcher, and whether it lies
// in the project: relativise leaves a path no project or cwd prefix strips
// as written, absolute or climbing out.
func projectPath(p string) (string, bool) {
	p = path.Clean(p)
	if path.IsAbs(p) || p == ".." || strings.HasPrefix(p, "../") || len(p) > 1 && p[1] == ':' {
		return "", false
	}
	return p, true
}

// protects reports rel matched by one of the rule's paths and none of its
// exclude_paths (spec.MatchPath, the one matcher).
func protects(r *spec.Rule, rel string) bool {
	return anyPath(r.Paths, rel) && !anyPath(r.ExcludePaths, rel)
}

func anyPath(patterns []string, rel string) bool {
	for _, p := range patterns {
		if spec.MatchPath(p, rel) {
			return true
		}
	}
	return false
}

// modeTri is whether a write by tool is in one of modes; no modes is every
// mode, as Parse defaults protect_path's.
func modeTri(tool string, modes []string) tri {
	if len(modes) == 0 {
		return triYes
	}
	create, modify, del := slices.Contains(modes, "create"), slices.Contains(modes, "modify"), slices.Contains(modes, "delete")
	switch tool {
	case "Edit", "MultiEdit", "NotebookEdit":
		return triOf(modify, modify)
	case "Write":
		return triOf(create && modify, create || modify)
	}
	return triOf(create && modify && del, create || modify || del)
}

// evidenceAt is a require_evidence rule at the snapshot's point, and whether
// it applies there: at a point its at list names (done is Stop's, and only
// with a relevant write in the window), every evidence entry green. An entry
// is a declared gate's id, read as the snapshot gate spec:<id>, or kind:<k>,
// met by any snapshot gate of kind k green. An entry not green is a
// violation citing the point; one whose gate reads unknown, or that no
// snapshot gate carries, is undecided, never a violation.
func evidenceAt(r *spec.Rule, snap *Snapshot, wrote bool) (ruleRead, bool) {
	word, ok := atWord(snap.Point.Kind)
	switch {
	case len(r.At) == 0:
		return ruleRead{undecided: true}, true
	case !ok || !slices.Contains(r.At, word) || word == "done" && !wrote:
		return ruleRead{}, false
	case len(r.Evidence) == 0:
		return ruleRead{undecided: true}, true
	}
	t := triYes
	for _, ev := range r.Evidence {
		t = min(t, evidenceTri(ev, snap.Gates))
	}
	switch t {
	case triNo:
		return ruleRead{seqs: []int64{snap.Point.Seq}}, true
	case triMaybe:
		return ruleRead{undecided: true}, true
	}
	return ruleRead{}, true
}

// atWord is the at list's word for a point kind.
func atWord(kind string) (string, bool) {
	switch kind {
	case PointStop:
		return "done", true
	case PointCommit, PointPush, PointPR, PointSubtask:
		return kind, true
	}
	return "", false
}

// evidenceTri is one evidence entry over the gates: yes green, no not green,
// maybe unknown or not there.
func evidenceTri(ev string, gates []GateState) tri {
	if k, ok := strings.CutPrefix(ev, "kind:"); ok {
		t, seen := triNo, false
		for i := range gates {
			if gates[i].Kind == k {
				t, seen = max(t, gateTri(gates[i].State)), true
			}
		}
		return triOf(seen && t == triYes, !seen || t != triNo)
	}
	for i := range gates {
		if id, ok := strings.CutPrefix(gates[i].Gate, "spec:"); ok && id == ev {
			return gateTri(gates[i].State)
		}
	}
	return triMaybe
}

func gateTri(s State) tri {
	switch s {
	case Green:
		return triYes
	case Unknown:
		return triMaybe
	}
	return triNo
}
