package spec

import (
	"crypto/sha256"
	"encoding/hex"
	"slices"
	"strconv"
	"unicode/utf8"
)

// Parse reads one spec.json (archive 20.2.8), checks every limit and closed
// list the archive names, and returns its Compiled form with the schema's
// defaults applied. It is all or nothing: on any error it returns a nil
// Compiled and an *Error naming the first error's JSON pointer and reason,
// and the digest to key that one flag by.
//
// Order, cheapest first (OWASP: caps before parsing): the byte cap, UTF-8,
// the lexical pass (syntax, duplicate keys, integers only, nesting and value
// caps), the digest over the canonical form, then the schema walk in schema
// order: version, unknown keys, counts, gates, ci_commands, rules, facts. A
// list's count is checked before its items, so an over-cap list costs
// nothing to refuse. "First error" means first in that order, which is
// deterministic: no map iteration reaches it.
//
// The caller reads at most MaxBytes+1 bytes of the file; Parse digests at
// most that many, so the digest of an oversize file is the same however much
// of it was read. path is carried into errors and into Sources, and the
// Compiled keeps no reference to data. Parse is a pure function: it opens no
// file and reads no clock (functional core).
func Parse(path string, data []byte, trust Trust) (*Compiled, error) {
	trust = Lowest(trust, Managed) // an unknown trust is untrusted
	if len(data) > MaxBytes {
		return nil, &Error{Path: path, Reason: ReasonTooLarge, Limit: MaxBytes, Digest: rawDigest(data[:MaxBytes+1])}
	}
	if !utf8.Valid(data) {
		return nil, &Error{Path: path, Reason: ReasonNotUTF8, Digest: rawDigest(data)}
	}
	root, lerr := lex(data)
	if lerr != nil {
		lerr.Path, lerr.Digest = path, rawDigest(data)
		return nil, lerr
	}
	sum := sha256.Sum256(appendJCS(nil, root))
	digest := DigestPrefix + hex.EncodeToString(sum[:])
	p := &parser{}
	c := p.file(root)
	if p.err != nil {
		p.err.Path, p.err.Digest, p.err.Canonical = path, digest, true
		return nil, p.err
	}
	c.Sources = []Source{{Path: path, Digest: digest, Trust: trust}}
	c.Digest, c.Trust = digest, trust
	return c, nil
}

// parser walks a document tree into typed values. Invariant: err is set at
// most once, by the first failure, and path is the pointer of the value
// being read; after err is set every reader returns a zero value, so no
// partial result is ever assembled into a Compiled.
type parser struct {
	err   *Error
	path  []string
	insts int // compiled regex instructions so far, against MaxSpecInstructions
}

func (p *parser) fail(r Reason, limit int) {
	if p.err == nil {
		p.err = &Error{Pointer: pointer(p.path), Reason: r, Limit: limit}
	}
}

func (p *parser) failAt(key string, r Reason, limit int) {
	p.path = append(p.path, key)
	p.fail(r, limit)
	p.path = p.path[:len(p.path)-1]
}

func (p *parser) push(seg string) { p.path = append(p.path, seg) }
func (p *parser) pop()            { p.path = p.path[:len(p.path)-1] }

// fields indexes an object's members by key. It is only ever looked up, never
// ranged over.
type fields map[string]*node

// index checks n is an object and indexes it.
func (p *parser) index(n *node) fields {
	if n.kind != kObject {
		p.fail(ReasonType, 0)
		return nil
	}
	f := make(fields, len(n.keys))
	for i, k := range n.keys {
		f[k] = &n.vals[i]
	}
	return f
}

// object indexes n and refuses its first key, in file order, outside allowed.
// Keys are exact: "ID" is not "id".
func (p *parser) object(n *node, allowed []string) fields {
	f := p.index(n)
	if f == nil {
		return nil
	}
	for _, k := range n.keys {
		if !slices.Contains(allowed, k) {
			p.failAt(k, ReasonUnknownKey, 0)
			return nil
		}
	}
	return f
}

// The key sets of each object, in schema order.
var (
	topKeys    = []string{"rashomon_spec", "gates", "ci_commands", "rules", "facts"}
	gateKeys   = []string{"id", "kind", "argv", "match", "aliases", "dir", "applies_to", "required", "partial_flags", "success_line", "failure_line", "empty_run_line", "exit_codes"}
	cmdKeys    = []string{"argv", "flags", "require_flags", "flag_values", "sql"}
	factKeys   = []string{"id", "kind", "programs", "target", "classes", "value", "mechanism", "why"}
	targetKeys = []string{"kind", "id"}
	ruleCommon = []string{"id", "kind", "why", "hard_control", "alternative"}
	// ruleOwn is the fields each rule kind may carry beyond ruleCommon.
	ruleOwn = map[RuleKind][]string{
		ForbidCode:      {"pattern", "languages", "paths", "exclude_paths"},
		ForbidCommand:   {"command"},
		ProtectPath:     {"paths", "modes"},
		RequireEvidence: {"at", "before", "evidence"},
		Order:           {"then", "after", "after_gate", "same_run_key", "invalidated_by"},
	}
)

// ruleKeys is every key any rule may carry: the unknown-key set, before the
// kind narrows it.
func ruleKeys() []string {
	all := slices.Clone(ruleCommon)
	for _, k := range RuleKinds {
		all = append(all, ruleOwn[k]...)
	}
	return all
}

// file walks the root. It returns a Compiled without its Sources and digest,
// which Parse fills.
func (p *parser) file(root node) *Compiled {
	top := p.index(&root)
	if top == nil {
		return nil
	}
	// The version first: a version-2 file is refused as a version, not as its
	// first key this version does not know.
	v, ok := top["rashomon_spec"]
	switch {
	case !ok:
		p.failAt("rashomon_spec", ReasonRequired, 0)
	case v.kind != kInt:
		p.failAt("rashomon_spec", ReasonType, 0)
	case v.num != 1:
		p.failAt("rashomon_spec", ReasonVersion, 0)
	}
	if p.err != nil || p.object(&root, topKeys) == nil {
		return nil
	}
	gates, ci, rules, facts := p.listAt(top, "gates"), p.listAt(top, "ci_commands"), p.listAt(top, "rules"), p.listAt(top, "facts")
	switch {
	case p.err != nil:
	case len(gates) > MaxGates:
		p.failAt("gates", ReasonTooMany, MaxGates)
	case len(gates)+len(ci) > MaxGates:
		p.failAt("ci_commands", ReasonTooMany, MaxGates)
	case len(rules) > MaxRules:
		p.failAt("rules", ReasonTooMany, MaxRules)
	case len(facts) > MaxFacts:
		p.failAt("facts", ReasonTooMany, MaxFacts)
	}
	if p.err != nil {
		return nil
	}
	c := &Compiled{}
	seen := map[string]struct{}{}
	p.push("gates")
	for i := range gates {
		p.push(strconv.Itoa(i))
		c.Gates = append(c.Gates, p.gate(&gates[i], seen))
		p.pop()
	}
	p.pop()
	p.push("ci_commands")
	for i := range ci {
		p.push(strconv.Itoa(i))
		c.CICommands = append(c.CICommands, p.strList(&ci[i], MaxArgvWords, true, wordRule))
		p.pop()
	}
	p.pop()
	seen = map[string]struct{}{}
	p.push("rules")
	for i := range rules {
		p.push(strconv.Itoa(i))
		c.Rules = append(c.Rules, p.rule(&rules[i], seen))
		p.pop()
	}
	p.pop()
	seen = map[string]struct{}{}
	p.push("facts")
	for i := range facts {
		p.push(strconv.Itoa(i))
		c.Facts = append(c.Facts, p.fact(&facts[i], seen))
		p.pop()
	}
	p.pop()
	if p.err != nil {
		return nil
	}
	return c
}

// listAt returns the elements of the array at key, nil when the key is
// absent, and sets a type error when it is not an array.
func (p *parser) listAt(f fields, key string) []node {
	n, ok := f[key]
	if !ok {
		return nil
	}
	if n.kind != kArray {
		p.failAt(key, ReasonType, 0)
		return nil
	}
	return n.vals
}

// strRule says what a string must be: a byte or rune cap, then a charset or
// shape check. Every string also passes clean.
type strRule struct {
	cap   int
	runes bool // cap counts runes, not bytes
	vet   func(string) Reason
}

var (
	idRule    = strRule{vet: vetID}
	wordRule  = strRule{cap: MaxWordBytes, vet: vetWord}
	flagRule  = strRule{cap: MaxWordBytes, vet: vetFlag}
	pathRule  = strRule{cap: MaxWordBytes, vet: vetPath}
	textRule  = strRule{cap: MaxWordBytes}
	valueRule = strRule{cap: MaxFactValueBytes, vet: vetFactValue}
	classRule = strRule{cap: MaxWordBytes, vet: vetClass}
	sqlRule   = strRule{cap: MaxWordBytes, vet: vetSQL}
	whyRule   = strRule{cap: MaxWhyRunes, runes: true}
	altRule   = strRule{cap: MaxAlternativeRunes, runes: true}
	evidRule  = strRule{cap: MaxWordBytes, vet: vetEvidence}
)

// in is the rule for a closed list.
func in(set []string) strRule {
	return strRule{cap: MaxWordBytes, vet: func(s string) Reason {
		if !slices.Contains(set, s) {
			return ReasonEnum
		}
		return ""
	}}
}

// str reads a non-empty string: cap, then clean, then the rule's own check.
func (p *parser) str(n *node, r strRule) string {
	if p.err != nil {
		return ""
	}
	switch {
	case n.kind != kString:
		p.fail(ReasonType, 0)
	case n.str == "":
		p.fail(ReasonEmpty, 0)
	case r.cap > 0 && !r.runes && len(n.str) > r.cap, r.cap > 0 && r.runes && utf8.RuneCountInString(n.str) > r.cap:
		p.fail(ReasonTooLong, r.cap)
	case !clean(n.str):
		p.fail(ReasonChars, 0)
	case r.vet != nil:
		if reason := r.vet(n.str); reason != "" {
			p.fail(reason, 0)
		}
	}
	if p.err != nil {
		return ""
	}
	return n.str
}

// strAt reads the string at key: absent is an error only when required.
func (p *parser) strAt(f fields, key string, required bool, r strRule) (string, bool) {
	n, ok := f[key]
	if !ok {
		if required {
			p.failAt(key, ReasonRequired, 0)
		}
		return "", false
	}
	p.push(key)
	s := p.str(n, r)
	p.pop()
	return s, true
}

// strList reads an array of strings under the rule, at most max items, and at
// least one when nonEmpty. The count is checked before any item.
func (p *parser) strList(n *node, max int, nonEmpty bool, r strRule) []string {
	if p.err != nil {
		return nil
	}
	switch {
	case n.kind != kArray:
		p.fail(ReasonType, 0)
	case len(n.vals) > max:
		p.fail(ReasonTooMany, max)
	case len(n.vals) == 0 && nonEmpty:
		p.fail(ReasonEmpty, 0)
	}
	if p.err != nil || len(n.vals) == 0 {
		return nil
	}
	out := make([]string, 0, len(n.vals))
	for i := range n.vals {
		p.push(strconv.Itoa(i))
		out = append(out, p.str(&n.vals[i], r))
		p.pop()
		if p.err != nil {
			return nil
		}
	}
	return out
}

// strsAt reads the string list at key.
func (p *parser) strsAt(f fields, key string, required bool, max int, r strRule) []string {
	n, ok := f[key]
	if !ok {
		if required {
			p.failAt(key, ReasonRequired, 0)
		}
		return nil
	}
	p.push(key)
	out := p.strList(n, max, required, r)
	p.pop()
	return out
}

// idAt reads an id and refuses a repeat within seen.
func (p *parser) idAt(f fields, seen map[string]struct{}) string {
	id, _ := p.strAt(f, "id", true, idRule)
	if p.err == nil {
		if _, dup := seen[id]; dup {
			p.failAt("id", ReasonDuplicateID, 0)
		}
		seen[id] = struct{}{}
	}
	return id
}
