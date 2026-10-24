package spec

import (
	"slices"
	"strconv"
)

// The schema walk below the file level: one method per object of 20.2.8, each
// reading its fields in schema order and leaving the first error in p.err.

func (p *parser) gate(n *node, seen map[string]struct{}) Gate {
	var g Gate
	f := p.object(n, gateKeys)
	if f == nil {
		return g
	}
	g.ID = p.idAt(f, seen)
	g.Kind, _ = p.strAt(f, "kind", true, in(GateKinds))
	g.Argv = p.strsAt(f, "argv", true, MaxArgvWords, wordRule)
	var ok bool
	if g.Match, ok = p.strAt(f, "match", false, in(GateMatches)); !ok {
		g.Match = GateMatches[0]
	}
	g.Aliases = p.aliases(f)
	if g.Dir, ok = p.strAt(f, "dir", false, pathRule); !ok {
		g.Dir = "."
	}
	g.AppliesTo = p.strsAt(f, "applies_to", false, MaxListItems, pathRule)
	yes := true
	g.Required = &yes
	if rn, ok := f["required"]; ok {
		if rn.kind != kBool {
			p.failAt("required", ReasonType, 0)
		}
		yes = rn.flag
	}
	g.PartialFlags = p.strsAt(f, "partial_flags", false, MaxListItems, flagRule)
	g.SuccessLine = p.regexesAt(f, "success_line")
	g.FailureLine = p.regexesAt(f, "failure_line")
	g.EmptyRunLine = p.regexesAt(f, "empty_run_line")
	g.ExitCodes = p.exitCodes(f)
	return g
}

// aliases reads Gate.Aliases: at most MaxAliases argv lists.
func (p *parser) aliases(f fields) [][]string {
	n, ok := f["aliases"]
	if !ok {
		return nil
	}
	p.push("aliases")
	defer p.pop()
	switch {
	case n.kind != kArray:
		p.fail(ReasonType, 0)
	case len(n.vals) > MaxAliases:
		p.fail(ReasonTooMany, MaxAliases)
	}
	var out [][]string
	for i := range n.vals {
		if p.err != nil {
			return nil
		}
		p.push(strconv.Itoa(i))
		out = append(out, p.strList(&n.vals[i], MaxArgvWords, true, wordRule))
		p.pop()
	}
	return out
}

// exitCodes reads {"<code>": "<state>"}: codes are canonical decimals 0 to
// 255, states one of ExitStates.
func (p *parser) exitCodes(f fields) map[string]string {
	n, ok := f["exit_codes"]
	if !ok {
		return nil
	}
	p.push("exit_codes")
	defer p.pop()
	switch {
	case n.kind != kObject:
		p.fail(ReasonType, 0)
	case len(n.keys) > MaxExitCodes:
		p.fail(ReasonTooMany, MaxExitCodes)
	}
	if p.err != nil || len(n.keys) == 0 {
		return nil
	}
	out := make(map[string]string, len(n.keys))
	for i, k := range n.keys {
		p.push(k)
		if code, err := strconv.Atoi(k); err != nil || code < 0 || code > 255 || strconv.Itoa(code) != k {
			p.fail(ReasonShape, 0)
		}
		out[k] = p.str(&n.vals[i], in(ExitStates))
		p.pop()
		if p.err != nil {
			return nil
		}
	}
	return out
}

// regexesAt reads a list of regular expressions and checks each against the
// limits, adding its instructions to the spec's total.
func (p *parser) regexesAt(f fields, key string) []string {
	n, ok := f[key]
	if !ok {
		return nil
	}
	p.push(key)
	defer p.pop()
	switch {
	case n.kind != kArray:
		p.fail(ReasonType, 0)
	case len(n.vals) > MaxListItems:
		p.fail(ReasonTooMany, MaxListItems)
	}
	var out []string
	for i := range n.vals {
		if p.err != nil {
			return nil
		}
		p.push(strconv.Itoa(i))
		out = append(out, p.regex(&n.vals[i]))
		p.pop()
	}
	return out
}

// regex reads one pattern (any length: checkRegex refuses a long one by its
// own reason, before parsing it) and counts its instructions into the
// spec's total.
func (p *parser) regex(n *node) string {
	s := p.str(n, strRule{})
	if p.err != nil {
		return ""
	}
	insts, reason := checkRegex(s)
	switch reason {
	case "":
	case ReasonPatternTooLong:
		p.fail(reason, MaxPatternBytes)
	case ReasonPatternComplex:
		p.fail(reason, MaxPatternInstructions)
	case ReasonRepeatTooLarge:
		p.fail(reason, MaxRepeat)
	default:
		p.fail(reason, 0)
	}
	if p.err == nil {
		if p.insts += insts; p.insts > MaxSpecInstructions {
			p.fail(ReasonSpecComplex, MaxSpecInstructions)
		}
	}
	if p.err != nil {
		return ""
	}
	return s
}

func (p *parser) command(n *node) *CommandMatch {
	f := p.object(n, cmdKeys)
	if f == nil {
		return nil
	}
	c := &CommandMatch{}
	c.Argv = p.strsAt(f, "argv", true, MaxArgvWords, wordRule)
	c.Flags = p.strsAt(f, "flags", false, MaxListItems, flagRule)
	c.RequireFlags = p.strsAt(f, "require_flags", false, MaxListItems, flagRule)
	if fv, ok := f["flag_values"]; ok {
		p.push("flag_values")
		c.FlagValues = p.flagValues(fv)
		p.pop()
	}
	c.SQL = p.strsAt(f, "sql", false, MaxListItems, sqlRule)
	if len(c.SQL) > 0 && p.err == nil && !slices.Contains(SQLPrograms, c.Argv[0]) {
		p.failAt("sql", ReasonShape, 0)
	}
	return c
}

func (p *parser) flagValues(n *node) map[string][]string {
	switch {
	case n.kind != kObject:
		p.fail(ReasonType, 0)
	case len(n.keys) > MaxFlagValueKeys:
		p.fail(ReasonTooMany, MaxFlagValueKeys)
	}
	if p.err != nil || len(n.keys) == 0 {
		return nil
	}
	out := make(map[string][]string, len(n.keys))
	for i, k := range n.keys {
		p.push(k)
		switch {
		case len(k) > MaxWordBytes:
			p.fail(ReasonTooLong, MaxWordBytes)
		case !clean(k):
			p.fail(ReasonChars, 0)
		case vetFlag(k) != "":
			p.fail(ReasonShape, 0)
		}
		out[k] = p.strList(&n.vals[i], MaxListItems, true, textRule)
		p.pop()
		if p.err != nil {
			return nil
		}
	}
	return out
}

// commandAt reads an optional or required command match at key.
func (p *parser) commandAt(f fields, key string, required bool) *CommandMatch {
	n, ok := f[key]
	if !ok {
		if required {
			p.failAt(key, ReasonRequired, 0)
		}
		return nil
	}
	p.push(key)
	defer p.pop()
	return p.command(n)
}

func (p *parser) rule(n *node, seen map[string]struct{}) Rule {
	var r Rule
	f := p.object(n, ruleKeys())
	if f == nil {
		return r
	}
	r.ID = p.idAt(f, seen)
	kind, _ := p.strAt(f, "kind", true, in(ruleKindNames()))
	r.Kind = RuleKind(kind)
	if p.err != nil {
		return r
	}
	// A key that belongs to another kind is refused, in file order.
	for _, k := range n.keys {
		if !slices.Contains(ruleCommon, k) && !slices.Contains(ruleOwn[r.Kind], k) {
			p.failAt(k, ReasonFieldForKind, 0)
			return r
		}
	}
	r.Why, _ = p.strAt(f, "why", true, whyRule)
	r.HardControl, _ = p.strAt(f, "hard_control", true, whyRule)
	r.Alternative, _ = p.strAt(f, "alternative", false, altRule)
	switch r.Kind {
	case ForbidCode:
		if pn, ok := f["pattern"]; ok {
			p.push("pattern")
			r.Pattern = p.regex(pn)
			p.pop()
		} else {
			p.failAt("pattern", ReasonRequired, 0)
		}
		r.Languages = p.strsAt(f, "languages", false, MaxListItems, wordRule)
		r.Paths = p.strsAt(f, "paths", false, MaxListItems, pathRule)
		r.ExcludePaths = p.strsAt(f, "exclude_paths", false, MaxListItems, pathRule)
	case ForbidCommand:
		r.Command = p.commandAt(f, "command", true)
	case ProtectPath:
		r.Paths = p.strsAt(f, "paths", true, MaxListItems, pathRule)
		r.Modes = p.strsAt(f, "modes", false, len(Modes), in(Modes))
		if r.Modes == nil {
			r.Modes = slices.Clone(Modes)
		}
	case RequireEvidence:
		r.At = p.strsAt(f, "at", false, len(Points), in(Points))
		r.Before = p.commandAt(f, "before", false)
		if p.err == nil && r.At == nil && r.Before == nil {
			p.failAt("at", ReasonOneOfRequired, 0)
		}
		r.Evidence = p.strsAt(f, "evidence", true, MaxListItems, evidRule)
	case Order:
		r.Then = p.commandAt(f, "then", true)
		r.After = p.commandAt(f, "after", false)
		r.AfterGate, _ = p.strAt(f, "after_gate", false, idRule)
		switch _, hasGate := f["after_gate"]; {
		case p.err != nil:
		case r.After != nil && hasGate:
			p.failAt("after_gate", ReasonExclusive, 0)
		case r.After == nil && !hasGate:
			p.failAt("after", ReasonOneOfRequired, 0)
		}
		r.SameRunKey = p.strsAt(f, "same_run_key", false, MaxListItems, flagRule)
		r.InvalidatedBy = p.strsAt(f, "invalidated_by", false, MaxListItems, pathRule)
	}
	return r
}

func ruleKindNames() []string {
	out := make([]string, len(RuleKinds))
	for i, k := range RuleKinds {
		out[i] = string(k)
	}
	return out
}

func (p *parser) fact(n *node, seen map[string]struct{}) Fact {
	var ft Fact
	f := p.object(n, factKeys)
	if f == nil {
		return ft
	}
	ft.ID = p.idAt(f, seen)
	ft.Kind, _ = p.strAt(f, "kind", true, in(FactKinds))
	ft.Programs = p.strsAt(f, "programs", true, MaxListItems, idRule)
	if tn, ok := f["target"]; ok {
		p.push("target")
		ft.Target = p.target(tn)
		p.pop()
	}
	if cn, ok := f["classes"]; ok {
		p.push("classes")
		ft.Classes = p.classes(cn)
		p.pop()
	} else {
		p.failAt("classes", ReasonRequired, 0)
	}
	ft.Value, _ = p.strAt(f, "value", true, valueRule)
	if mech, _ := p.strAt(f, "mechanism", true, strRule{cap: MaxFactValueBytes}); p.err == nil {
		if r := mechanismReason(ft.Kind, mech); r != "" {
			p.failAt("mechanism", r, 0)
		}
		ft.Mechanism = mech
	}
	ft.Why, _ = p.strAt(f, "why", false, whyRule)
	return ft
}

func (p *parser) target(n *node) *FactTarget {
	f := p.object(n, targetKeys)
	if f == nil {
		return nil
	}
	t := &FactTarget{}
	t.Kind, _ = p.strAt(f, "kind", true, in(TargetKinds))
	t.ID, _ = p.strAt(f, "id", true, valueRule)
	return t
}

// classes reads a fact's failure classes and applies the identity-fact rule
// (20.2.8, Facts): authentication classes only.
func (p *parser) classes(n *node) []string {
	out := p.strList(n, MaxListItems, true, classRule)
	for i := 0; i < len(out) && p.err == nil; i++ {
		if r := classReason(out[i]); r != "" {
			p.failAt(strconv.Itoa(i), r, 0)
		}
	}
	if p.err != nil {
		return nil
	}
	return out
}
