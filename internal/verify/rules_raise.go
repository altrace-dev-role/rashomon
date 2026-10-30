package verify

import (
	"path"
	"strconv"
	"strings"

	"github.com/altrace-dev-role/rashomon/internal/shape"
	"github.com/altrace-dev-role/rashomon/internal/spec"
	"github.com/altrace-dev-role/rashomon/internal/steer"
	"github.com/altrace-dev-role/rashomon/internal/steer/pattern"
	"github.com/altrace-dev-role/rashomon/internal/steer/render"
	"github.com/altrace-dev-role/rashomon/internal/steer/render/slot"
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
}

// RulesAtCall is the spec's rules raised at a post from the call alone, no
// run read: at PostToolUse, each forbid_command rule one of the line's
// commands matches raises platform-rule/command, a nudge for a trusted or
// managed rule and a user flag otherwise (the card raises platform-rule at
// that event alone, so a call that failed is the record's, judged at Stop);
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
	var cmds []shape.Command
	read := false
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
			if event != string(steer.EventPostToolUse) || call.ToolName != "Bash" || call.Command == "" ||
				dr.Rule.Command == nil || len(dr.Rule.Command.Argv) == 0 {
				continue
			}
			if !read {
				cmds, _ = shape.Commands(call.Command)
				read = true
			}
			prog, ok := matchedProgram(dr.Rule.Command, cmds)
			if !ok {
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
