package spec

import (
	"crypto/sha256"
	"encoding/hex"
	"slices"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Parse reads one spec.json strictly (archive 20.2.8, Schema) and returns its
// File and its digest, DigestPrefix and the hex SHA-256 of the RFC 8785 form of
// the parsed document (archive 20.2.8, Digest).
//
// It is all or nothing: on any error the File is nil and the error is the
// *Error of the first fault, in this order (cheapest first, caps before
// parsing): the byte cap, the lexical rules of readTree, then the schema in
// the order of its types, with the version, unknown keys and counts of a
// list before its items. The first error is therefore deterministic; no map
// is ranged over. The digest is returned whenever the document read, even
// when the schema refused it, so a caller can key one flag per digest; it is
// "" when the bytes are not a document.
//
// Parse opens no file and reads no clock (functional core), keeps no
// reference to data, and costs O(n) in its length.
func Parse(data []byte) (*File, string, error) {
	root, terr := readTree(data)
	if terr != nil {
		return nil, "", terr
	}
	sum := sha256.Sum256(appendJCS(nil, root))
	digest := DigestPrefix + hex.EncodeToString(sum[:])
	r := &reader{}
	f := r.file(&root)
	if r.err != nil {
		return nil, digest, r.err
	}
	return f, digest, nil
}

// reader walks a node tree into typed values. Invariant: err is set by the
// first failure only and path is the pointer of the value being read; once err
// is set every method returns a zero value, so no partial File is assembled.
type reader struct {
	err   *Error
	path  []string
	insts int // compiled regex instructions so far, against MaxSpecInstructions
}

func (r *reader) fail(reason Reason, limit int) {
	if r.err == nil {
		r.err = &Error{Pointer: pointer(r.path), Reason: reason, Limit: limit}
	}
}

func (r *reader) failAt(seg string, reason Reason, limit int) {
	r.path = append(r.path, seg)
	r.fail(reason, limit)
	r.path = r.path[:len(r.path)-1]
}

func (r *reader) at(seg string) func() {
	r.path = append(r.path, seg)
	return func() { r.path = r.path[:len(r.path)-1] }
}

// fields indexes an object's members by key. It is looked up, never ranged
// over.
type fields map[string]*node

func (r *reader) index(n *node) fields {
	if n.kind != kObject {
		r.fail(ReasonType, 0)
		return nil
	}
	f := make(fields, len(n.keys))
	for i, k := range n.keys {
		f[k] = &n.vals[i]
	}
	return f
}

// object indexes n and refuses its first key, in file order, that is not in
// allowed. Keys are exact: "ID" is not "id".
func (r *reader) object(n *node, allowed ...string) fields {
	f := r.index(n)
	for _, k := range n.keys {
		if f != nil && !slices.Contains(allowed, k) {
			r.failAt(k, ReasonUnknownKey, 0)
			return nil
		}
	}
	return f
}

// field reads the member key of f with read, at its pointer. A missing member
// is an error when required, else the zero value.
func field[T any](r *reader, f fields, key string, required bool, read func(*node) T) (v T) {
	if r.err != nil {
		return v
	}
	n, ok := f[key]
	if !ok {
		if required {
			r.failAt(key, ReasonRequired, 0)
		}
		return v
	}
	defer r.at(key)()
	return read(n)
}

// list reads an array of at least one element, each with one. An empty list
// says nothing, so it is refused: the key is simply left out.
func list[T any](r *reader, n *node, one func(*node) T) []T {
	switch {
	case n.kind != kArray:
		r.fail(ReasonType, 0)
		return nil
	case len(n.vals) == 0:
		r.fail(ReasonEmpty, 0)
		return nil
	}
	out := make([]T, 0, len(n.vals))
	for i := range n.vals {
		done := r.at(strconv.Itoa(i))
		out = append(out, one(&n.vals[i]))
		done()
		if r.err != nil {
			return nil
		}
	}
	return out
}

// The closed lists of archive 20.2.8, Schema, in the archive's order.
var (
	gateKinds   = []string{"test", "build", "lint", "typecheck", "scan", "custom"}
	matchModes  = []string{"prefix", "exact"}
	ruleKinds   = []string{"require_evidence", "forbid_code", "forbid_command", "protect_path", "order"}
	pathModes   = []string{"modify", "delete", "create"}
	atPoints    = []string{"commit", "push", "pr", "done", "subtask"}
	factKinds   = []string{"aws_profile", "aws_region", "kube_context", "kube_namespace", "package_index_host", "env_var_name", "sign_in_step"}
	mechanisms  = []string{"profile", "region", "context", "namespace", "index", "env var name", "sign-in step"}
	targetKinds = []string{"aws_account", "aws_region", "endpoint_host", "kube_context", "gcp_project", "azure_subscription"}
	gateKeys    = []string{"id", "kind", "argv", "match", "aliases", "dir", "applies_to", "required", "partial_flags", "success_line", "failure_line", "empty_run_line", "exit_codes"}
	ruleKeys    = []string{"id", "kind", "why", "hard_control", "alternative", "pattern", "languages", "paths", "exclude_paths", "command", "modes", "at", "before", "evidence", "then", "after", "after_gate", "same_run_key", "invalidated_by"}
	commandKeys = []string{"argv", "flags", "require_flags", "flag_values", "sql"}
	factKeys    = []string{"id", "kind", "programs", "target", "classes", "value", "mechanism", "why"}
	targetKeys  = []string{"kind", "id"}
)

// text is a non-empty string without control characters (OWASP: reject, do
// not sanitise) of at most maxRunes runes, or any length when maxRunes is 0.
func (r *reader) text(n *node, maxRunes int) string {
	if n.kind != kString {
		r.fail(ReasonType, 0)
		return ""
	}
	return r.checkText(n.str, maxRunes)
}

func (r *reader) checkText(s string, maxRunes int) string {
	switch {
	case s == "":
		r.fail(ReasonEmpty, 0)
	case slices.ContainsFunc([]rune(s), unicode.IsControl):
		r.fail(ReasonControl, 0)
	case maxRunes > 0 && utf8.RuneCountInString(s) > maxRunes:
		r.fail(ReasonTooLong, maxRunes)
	default:
		return s
	}
	return ""
}

func (r *reader) plain(n *node) string { return r.text(n, 0) }
func (r *reader) why(n *node) string   { return r.text(n, MaxWhyRunes) }
func (r *reader) alt(n *node) string   { return r.text(n, MaxAlternativeRunes) }

// words is a list of plain strings.
func (r *reader) words(n *node) []string { return list(r, n, r.plain) }

// oneOf reads a string that is a member of set.
func (r *reader) oneOf(set []string) func(*node) string {
	return func(n *node) string {
		if n.kind != kString {
			r.fail(ReasonType, 0)
		} else if !slices.Contains(set, n.str) {
			r.fail(ReasonEnum, 0)
		} else {
			return n.str
		}
		return ""
	}
}

// idString is the id pattern of archive 20.2.8, ^[a-z0-9][a-z0-9._-]{0,47}$,
// written out so no regexp is compiled to read a spec.
func idString(s string) bool {
	if s == "" || len(s) > MaxIDBytes {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if !('a' <= c && c <= 'z' || '0' <= c && c <= '9' || i > 0 && (c == '.' || c == '_' || c == '-')) {
			return false
		}
	}
	return true
}

// id reads an id and, when seen is not nil, refuses one seen before in the
// same list.
func (r *reader) id(seen map[string]struct{}) func(*node) string {
	return func(n *node) string {
		switch {
		case n.kind != kString:
			r.fail(ReasonType, 0)
		case !idString(n.str):
			r.fail(ReasonID, 0)
		case seen == nil:
			return n.str
		default:
			if _, dup := seen[n.str]; dup {
				r.fail(ReasonDuplicateID, 0)
				return ""
			}
			seen[n.str] = struct{}{}
			return n.str
		}
		return ""
	}
}

// evidence is a gate id or "kind:<gate kind>".
func (r *reader) evidence(n *node) string {
	if n.kind == kString {
		if k, ok := strings.CutPrefix(n.str, "kind:"); ok {
			if !slices.Contains(gateKinds, k) {
				r.fail(ReasonEnum, 0)
				return ""
			}
			return n.str
		}
	}
	return r.id(nil)(n)
}

// regex reads an RE2 expression within the bounds of checkRegex and counts
// its instructions against the spec's total.
func (r *reader) regex(n *node) string {
	s := r.text(n, 0)
	if r.err != nil {
		return ""
	}
	insts, reason, limit := checkRegex(s)
	if reason != "" {
		r.fail(reason, limit)
		return ""
	}
	if r.insts += insts; r.insts > MaxSpecInstructions {
		r.fail(ReasonSpecComplex, MaxSpecInstructions)
		return ""
	}
	return s
}

func (r *reader) regexes(n *node) []string { return list(r, n, r.regex) }

// strMap reads an object of non-empty keys to values read by one.
func strMap[T any](r *reader, n *node, one func(*node) T) map[string]T {
	if n.kind != kObject {
		r.fail(ReasonType, 0)
		return nil
	} else if len(n.keys) == 0 {
		r.fail(ReasonEmpty, 0)
		return nil
	}
	out := make(map[string]T, len(n.keys))
	for i, k := range n.keys {
		done := r.at(k)
		if r.checkText(k, 0); r.err == nil {
			out[k] = one(&n.vals[i])
		}
		done()
		if r.err != nil {
			return nil
		}
	}
	return out
}

func (r *reader) boolean(n *node) *bool {
	if n.kind != kBool {
		r.fail(ReasonType, 0)
		return nil
	}
	return &n.flag
}

// file reads the root. The version comes first, so a version-2 file is refused
// as a version and not as the first key this version does not know.
func (r *reader) file(root *node) *File {
	top := r.index(root)
	if top == nil {
		return nil
	}
	switch v := top["rashomon_spec"]; {
	case v == nil:
		r.failAt("rashomon_spec", ReasonRequired, 0)
	case v.kind != kInt:
		r.failAt("rashomon_spec", ReasonType, 0)
	case v.num != 1:
		r.failAt("rashomon_spec", ReasonVersion, 0)
	}
	if r.err != nil || r.object(root, "rashomon_spec", "gates", "ci_commands", "rules", "facts") == nil {
		return nil
	}
	items := func(key string) []node {
		n := top[key]
		if n == nil || r.err != nil {
			return nil
		}
		if n.kind != kArray {
			r.failAt(key, ReasonType, 0)
			return nil
		}
		return n.vals
	}
	gates, ci, rules, facts := items("gates"), items("ci_commands"), items("rules"), items("facts")
	switch {
	case r.err != nil:
	case len(gates) > MaxGates:
		r.failAt("gates", ReasonTooMany, MaxGates)
	case len(gates)+len(ci) > MaxGates:
		r.failAt("ci_commands", ReasonTooMany, MaxGates)
	case len(rules) > MaxRules:
		r.failAt("rules", ReasonTooMany, MaxRules)
	case len(facts) > MaxFacts:
		r.failAt("facts", ReasonTooMany, MaxFacts)
	}
	f := &File{RashomonSpec: 1}
	each(r, "gates", gates, func(n *node, seen map[string]struct{}) { f.Gates = append(f.Gates, r.gate(n, seen)) })
	each(r, "ci_commands", ci, func(n *node, _ map[string]struct{}) { f.CICommands = append(f.CICommands, r.words(n)) })
	each(r, "rules", rules, func(n *node, seen map[string]struct{}) { f.Rules = append(f.Rules, r.rule(n, seen)) })
	each(r, "facts", facts, func(n *node, seen map[string]struct{}) { f.Facts = append(f.Facts, r.fact(n, seen)) })
	return f
}

// each reads the elements of one top-level list, in order, with the set of ids
// seen so far in that list.
func each(r *reader, key string, items []node, one func(*node, map[string]struct{})) {
	defer r.at(key)()
	seen := map[string]struct{}{}
	for i := range items {
		if r.err != nil {
			return
		}
		done := r.at(strconv.Itoa(i))
		one(&items[i], seen)
		done()
	}
}

func (r *reader) gate(n *node, seen map[string]struct{}) (g Gate) {
	f := r.object(n, gateKeys...)
	g.ID = field(r, f, "id", true, r.id(seen))
	g.Kind = field(r, f, "kind", true, r.oneOf(gateKinds))
	g.Argv = field(r, f, "argv", true, r.words)
	g.Match = field(r, f, "match", false, r.oneOf(matchModes))
	g.Aliases = field(r, f, "aliases", false, func(n *node) [][]string { return list(r, n, r.words) })
	g.Dir = field(r, f, "dir", false, r.plain)
	g.AppliesTo = field(r, f, "applies_to", false, r.words)
	g.Required = field(r, f, "required", false, r.boolean)
	g.PartialFlags = field(r, f, "partial_flags", false, r.words)
	g.SuccessLine = field(r, f, "success_line", false, r.regexes)
	g.FailureLine = field(r, f, "failure_line", false, r.regexes)
	g.EmptyRunLine = field(r, f, "empty_run_line", false, r.regexes)
	g.ExitCodes = field(r, f, "exit_codes", false, func(n *node) map[string]string { return strMap(r, n, r.plain) })
	if g.Match == "" {
		g.Match = "prefix"
	}
	if g.Required == nil {
		yes := true
		g.Required = &yes
	}
	return g
}

func (r *reader) command(n *node) (c *CommandMatch) {
	f := r.object(n, commandKeys...)
	c = &CommandMatch{}
	c.Argv = field(r, f, "argv", true, r.words)
	c.Flags = field(r, f, "flags", false, r.words)
	c.RequireFlags = field(r, f, "require_flags", false, r.words)
	c.FlagValues = field(r, f, "flag_values", false, func(n *node) map[string][]string { return strMap(r, n, r.words) })
	c.SQL = field(r, f, "sql", false, r.words)
	return c
}

func (r *reader) rule(n *node, seen map[string]struct{}) (x Rule) {
	f := r.object(n, ruleKeys...)
	x.ID = field(r, f, "id", true, r.id(seen))
	x.Kind = field(r, f, "kind", true, r.oneOf(ruleKinds))
	x.Why = field(r, f, "why", true, r.why)
	x.HardControl = field(r, f, "hard_control", true, r.why)
	x.Alternative = field(r, f, "alternative", false, r.alt)
	x.Pattern = field(r, f, "pattern", false, r.regex)
	x.Languages = field(r, f, "languages", false, r.words)
	x.Paths = field(r, f, "paths", false, r.words)
	x.ExcludePaths = field(r, f, "exclude_paths", false, r.words)
	x.Command = field(r, f, "command", false, r.command)
	x.Modes = field(r, f, "modes", false, func(n *node) []string { return list(r, n, r.oneOf(pathModes)) })
	x.At = field(r, f, "at", false, func(n *node) []string { return list(r, n, r.oneOf(atPoints)) })
	x.Before = field(r, f, "before", false, r.command)
	x.Evidence = field(r, f, "evidence", false, func(n *node) []string { return list(r, n, r.evidence) })
	x.Then = field(r, f, "then", false, r.command)
	x.After = field(r, f, "after", false, r.command)
	x.AfterGate = field(r, f, "after_gate", false, r.id(nil))
	x.SameRunKey = field(r, f, "same_run_key", false, r.words)
	x.InvalidatedBy = field(r, f, "invalidated_by", false, r.words)
	if x.Kind == "protect_path" && x.Modes == nil {
		x.Modes = slices.Clone(pathModes)
	}
	return x
}

func (r *reader) fact(n *node, seen map[string]struct{}) (x Fact) {
	f := r.object(n, factKeys...)
	x.ID = field(r, f, "id", true, r.id(seen))
	x.Kind = field(r, f, "kind", true, r.oneOf(factKinds))
	x.Programs = field(r, f, "programs", true, r.words)
	x.Target = field(r, f, "target", false, r.target)
	x.Classes = field(r, f, "classes", true, r.words)
	if slices.Contains(identityFactKinds, x.Kind) {
		r.authnClasses(x.Classes)
	}
	x.Value = field(r, f, "value", true, r.identifier)
	x.Mechanism = field(r, f, "mechanism", true, r.oneOf(mechanisms))
	x.Why = field(r, f, "why", false, r.why)
	return x
}

func (r *reader) target(n *node) (t *FactTarget) {
	f := r.object(n, targetKeys...)
	t = &FactTarget{}
	t.Kind = field(r, f, "kind", true, r.oneOf(targetKinds))
	t.ID = field(r, f, "id", true, r.plain)
	return t
}

// identityFactKinds are the fact kinds whose classes are restricted to
// authentication failures (archive 20.2.8, Facts). env_var_name and
// sign_in_step are not among them.
var identityFactKinds = []string{"aws_profile", "aws_region", "kube_context", "kube_namespace", "package_index_host"}

// authnClasses refuses the first class of an identity fact that is not an
// authentication class: only cred.*_missing, *_profile_missing, *_expired,
// *_sso_needs_human, cred.registry, cred.kube_context, mcp.auth and
// mcp.token_expired. Never authz.*, *.forbidden or a 403: a note after a
// permission denial that names a more privileged identity points the agent up
// the privilege ladder. The denial test comes first because "*_expired" also
// matches "authz.token_expired".
func (r *reader) authnClasses(classes []string) {
	defer r.at("classes")()
	for i, c := range classes {
		if !authnClass(c) {
			r.failAt(strconv.Itoa(i), ReasonClassNotAuthn, 0)
			return
		}
	}
}

func authnClass(c string) bool {
	if strings.HasPrefix(c, "authz.") || strings.Contains(c, "forbidden") || strings.Contains(c, "403") {
		return false
	}
	switch c {
	case "cred.registry", "cred.kube_context", "mcp.auth", "mcp.token_expired":
		return true
	}
	return strings.HasPrefix(c, "cred.") && strings.HasSuffix(c, "_missing") ||
		strings.HasSuffix(c, "_profile_missing") || strings.HasSuffix(c, "_expired") ||
		strings.HasSuffix(c, "_sso_needs_human")
}

// identifier is plain text without any space: "value ... identifiers (no
// spaces)" (archive 20.2.8, Facts). The rest of STEER's FactValue allowlist
// is that package's, applied where a value is rendered.
func (r *reader) identifier(n *node) string {
	s := r.plain(n)
	if r.err == nil && strings.IndexFunc(s, unicode.IsSpace) >= 0 {
		r.fail(ReasonNotIdentifier, 0)
		return ""
	}
	return s
}
