package verify

import (
	"path"
	"slices"
	"strconv"
	"strings"

	"github.com/altrace-dev-role/rashomon/internal/shape"
	"github.com/altrace-dev-role/rashomon/internal/spec"
	"github.com/altrace-dev-role/rashomon/internal/steer"
	"github.com/altrace-dev-role/rashomon/internal/steer/pattern"
	"github.com/altrace-dev-role/rashomon/internal/steer/render"
	"github.com/altrace-dev-role/rashomon/internal/steer/render/slot"
	"github.com/altrace-dev-role/rashomon/internal/store"
)

// RuleCall is the call a post judges the spec's rules against, from its
// payload, in memory: Seq is the record seq its execution landed at;
// Command a Bash call's line; FilePath a file tool's path, which Project,
// else CWD, is stripped from as relativise strips a recorded one.
type RuleCall struct {
	Seq      int64
	ToolName string
	Command  string
	FilePath string
	Project  string
	CWD      string
	// ToolUseID and Run are an order rule's reads: the call's own id, and
	// the run as read at this post, nil where none was (the rule then raises
	// nothing: a rule over earlier calls needs the record).
	ToolUseID string
	Run       *store.Run
	// Exit and Foreground are how the call ended, for the after_hit an After
	// call leaves: the status (0 at PostToolUse, the failure's parsed code,
	// nil where none was stated) and whether it ended in the foreground.
	Exit       *int
	Foreground bool
	// Cmds and CmdsRead are the line as shape.Commands read it, for a
	// caller that already read it (the hook, to decide whether to read the
	// run); CmdsOK is its second result.
	Cmds     []shape.Command
	CmdsRead bool
	CmdsOK   bool
}

// RulesAtCall is the spec's rules raised at a post from the call alone, no
// run read: at either post event, each forbid_command rule one of the line's
// commands matches is a hit (view.Hits), which the hook keeps in the record
// for the Stop to count (card SPEC-RULES-STOP1); at PostToolUse it also
// raises platform-rule/command, a nudge for a trusted or managed rule and a
// user flag otherwise (the card raises platform-rule at that event alone, so
// a call that failed is the record's, judged at Stop from its hit);
// at either post event, each protect_path rule whose paths the file tool's
// path falls in, in a mode the rule lists, raises integrity/frozen-path, a
// user flag. One matcher reads the line and the path as ruleVerdicts reads
// the record, so what is undecided there raises nothing here; a Bash
// call's write targets are judged in the record only. The implied task-file
// rule raises nothing: integrity/store-touched raises its writes. Rewritten
// is false: the line as it ran is known only once the record is read. The
// view names no project root: a steer line's project follows the record,
// which this view never reads.
func RulesAtCall(sp *spec.Compiled, call RuleCall, pt Point, event string) SteerView {
	if sp == nil {
		return SteerView{}
	}
	view := SteerView{Spec: specInfo(sp)}
	agent := ""
	if pt.AgentID != nil {
		agent = *pt.AgentID
	}
	cmds, ok := call.Cmds, call.CmdsOK
	read := call.CmdsRead
	var ord orderView
	var ordLine ruleLine
	judged, ordRead := false, false
	rel, inside := "", false
	if call.FilePath != "" {
		rel, inside = projectPath(relativise(call.FilePath, &call.Project, &call.CWD))
	}
	for i := range sp.Rules {
		dr := &sp.Rules[i]
		if dr.Source == spec.SourceImplied {
			continue
		}
		fact := RuleFact{RuleID: SlotID(dr.ID), Kind: spec.RuleKind(dr.Rule.Kind), Trust: sp.TrustOf(dr.Source), Seq: call.Seq}
		f := Finding{Event: event, UserOnly: !binding(fact.Trust), Rule: &fact}
		switch fact.Kind {
		case spec.ForbidCommand:
			if call.ToolName != "Bash" || call.Command == "" || dr.Rule.Command == nil || len(dr.Rule.Command.Argv) == 0 {
				continue
			}
			if !read {
				cmds, ok = shape.Commands(call.Command)
				read = true
			}
			prog, ok := matchedProgram(dr.Rule.Command, cmds)
			if !ok {
				continue
			}
			view.Hits = append(view.Hits, RuleHit{RuleID: dr.ID, SourceDigest: sourceDigest(sp, dr.Source)})
			if event != string(steer.EventPostToolUse) {
				continue
			}
			if p := Program(prog); p.Valid() {
				fact.Program = &p
			}
			f.Pattern, f.Class = pattern.PlatformRule, Literal
		case spec.ProtectPath:
			if !fileTool(call.ToolName) || !inside || len(dr.Rule.Paths) == 0 || !protects(&dr.Rule, rel) || modeTri(call.ToolName, dr.Rule.Modes) != triYes {
				continue
			}
			p := PathSlot(rel)
			fact.Path = &p
			f.Pattern, f.Class, f.UserOnly = pattern.Integrity, UserLabelled, true
		case spec.Order:
			if call.ToolName != "Bash" || dr.Rule.Then == nil || len(dr.Rule.Then.Argv) == 0 || dr.Rule.After == nil || len(dr.Rule.After.Argv) == 0 {
				continue
			}
			if !read && call.Command != "" {
				cmds, ok = shape.Commands(call.Command)
				read = true
			}
			if !ordRead {
				ordLine, ordRead = ruleLine{cmds: cmds, whole: read && ok}, true
			}
			// The call as an After: a hit the Stop and a later Then read, from
			// the call alone, whatever the run holds (#216 ruling 1).
			if t := lineMatch(dr.Rule.After, &ordLine); t != triNo && read {
				view.AfterHits = append(view.AfterHits, AfterHit{RuleID: dr.ID, SourceDigest: sourceDigest(sp, dr.Source),
					CWDDigest: call.cwdDigest(), Exit: call.Exit, Foreground: call.Foreground, Maybe: t == triMaybe})
			}
			if call.Run == nil || !read {
				continue
			}
			if !judged {
				ord, judged = orderRead(call, ordLine), true
			}
			if !ord.violates(&dr.Rule, sp, dr, call.ToolUseID) {
				continue
			}
			if p := Program(path.Base(dr.Rule.Then.Argv[0])); p.Valid() {
				fact.Program = &p
			}
			f.Pattern, f.Class = pattern.OrderingRule, Literal
		default:
			continue
		}
		f.DedupeKey = string(f.Pattern) + "|" + dr.ID + "|" + strconv.FormatInt(call.Seq, 10)
		sf, err := ToSteer(f, pt, agent)
		if err != nil {
			view.Dropped++
			continue
		}
		sf.Message = renderRule(&fact, &dr.Rule)
		view.Findings = append(view.Findings, sf)
	}
	return view
}

// OrderTouches reports a Bash line, read as cmds (ok: read whole), that may
// run the then or the after of an order rule of sp's own: the calls that
// read the run at a post (the Then to be judged, the After for its hit's
// directory). Every other call is one parse of the line.
func OrderTouches(sp *spec.Compiled, cmds []shape.Command, ok bool) bool {
	l := ruleLine{cmds: cmds, whole: ok}
	for i := range sp.Rules {
		dr := &sp.Rules[i]
		if dr.Source == spec.SourceImplied || spec.RuleKind(dr.Rule.Kind) != spec.Order {
			continue
		}
		if dr.Rule.Then != nil && len(dr.Rule.Then.Argv) > 0 && lineMatch(dr.Rule.Then, &l) != triNo ||
			dr.Rule.After != nil && len(dr.Rule.After.Argv) > 0 && lineMatch(dr.Rule.After, &l) != triNo {
			return true
		}
	}
	return false
}

// cwdDigest is the keyed directory digest of the call's own declaration, ""
// where the run was not read or holds none.
func (c RuleCall) cwdDigest() string {
	if c.Run == nil {
		return ""
	}
	for i := range c.Run.Declarations {
		if d := &c.Run.Declarations[i]; d.ToolUseID == c.ToolUseID {
			return d.CWDDigest
		}
	}
	return ""
}

// orderView is the record an order rule reads at a post: the Then lines and
// writes at or before the call, read once for every order rule, and the
// after_hits its rules read from the run (afterHits).
type orderView struct {
	lines []ruleLine
	c     classified
	rels  []projectRel
	at    int64
	run   *store.Run
}

// orderRead reads the run for the call at a post. The call's own line is the
// payload's, read in memory (own), set under its declaration's seq and
// directory, since a record scope that withholds command lines holds none:
// no other call's line is needed for an After, whose record is its hit.
func orderRead(call RuleCall, own ruleLine) orderView {
	picked := pickExecutions(call.Run)
	v := orderView{run: call.Run, at: call.Seq}
	runs, writes, unnamed, _ := classifyWith(call.Run, picked)
	v.c = classified{runs: runs, writes: writes, unnamed: unnamed}
	for _, l := range ruleLines(call.Run, picked, call.Seq) {
		if l.id == call.ToolUseID {
			own.seq, own.id, own.cwd = l.seq, l.id, l.cwd
			continue
		}
		v.lines = append(v.lines, l)
	}
	if own.id != "" {
		v.lines = append(v.lines, own)
	}
	v.rels = projectRels(writes)
	return v
}

// violates reports the rule violated at the call whose tool_use_id is id: a
// violation the record names at that call's declaration, never an undecided
// one.
func (v *orderView) violates(r *spec.Rule, sp *spec.Compiled, dr *spec.DeclaredRule, id string) bool {
	for i := range v.run.Declarations {
		if d := &v.run.Declarations[i]; d.ToolUseID == id {
			hits := afterHits(v.run, dr.ID, sourceDigest(sp, dr.Source), v.at)
			return slices.Contains(orderOver(r, v.lines, hits, &v.c, v.rels, v.at).seqs, d.Seq)
		}
	}
	return false
}

// matchedProgram is the base name of the first command m matches for
// certain, and whether one does.
func matchedProgram(m *spec.CommandMatch, cmds []shape.Command) (string, bool) {
	for _, c := range cmds {
		if commandMatch(m, c) == triYes {
			return path.Base(c.Words[0]), true
		}
	}
	return "", false
}

// fileTool reports the tools whose input names the file they write.
func fileTool(name string) bool {
	switch name {
	case "Edit", "MultiEdit", "NotebookEdit", "Write":
		return true
	}
	return false
}

// renderRule is a rule fact's line: platform-rule/command names the program
// and the rule, and adds the rule's alternative where the rule binds and
// the call was not rewritten; integrity/frozen-path names the path and the
// rule. Where a slot refuses a value, the kind's row without its named
// slots.
func renderRule(f *RuleFact, r *spec.Rule) render.Message {
	id := slot.RuleID(strings.TrimPrefix(string(f.RuleID), "spec:"))
	var p pattern.ID
	var approved, full, unnamed any
	switch f.Kind {
	case spec.ForbidCommand:
		p, unnamed = pattern.PlatformRule, render.PlatformRuleCommandUnnamedBinding{}
		if f.Program != nil {
			prog := slot.Program(*f.Program)
			full = render.PlatformRuleCommandBinding{Program: prog, Rule: id}
			if r.Alternative != "" && binding(f.Trust) && !f.Rewritten {
				approved = render.PlatformRuleCommandApprovedBinding{Program: prog, Rule: id, Approved: slot.Approved(r.Alternative)}
			}
		}
	case spec.ProtectPath:
		p, unnamed = pattern.Integrity, render.IntegrityFrozenPathUnnamedBinding{}
		if f.Path != nil {
			full = render.IntegrityFrozenPathBinding{Path: slot.Path(*f.Path), Rule: id}
		}
	case spec.Order:
		p, unnamed = pattern.OrderingRule, render.OrderingRuleThenUnnamedBinding{}
		if f.Program != nil {
			full = render.OrderingRuleThenBinding{Program: slot.Program(*f.Program), Rule: id}
		}
	default:
		return render.Message{}
	}
	for _, b := range []any{approved, full, unnamed} {
		if b == nil {
			continue
		}
		if m, err := render.New(p, b); err == nil {
			return m
		}
	}
	return render.Message{}
}
