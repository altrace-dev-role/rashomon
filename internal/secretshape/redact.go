// Package secretshape is the one definition of a secret-shaped value: the
// secret deny list and the entropy test of the steer branch's slot layer
// (STR-13), lifted function by function, under a span walk that redacts
// rather than refuses. Redact replaces what fails them and keeps the rest;
// Secretish asks whether it would replace anything.
//
// It comes from the memory prototype's screen (internal/memory/screen on the
// experimental/memory-prototype branch) without the memory store around it,
// as the shared leaf the #84 split planned: one deny list and one entropy
// table, so that a fixture scan and any later reader that must not print a
// secret cannot disagree about what one looks like. Nothing in the shipped
// binary imports it yet.
//
// It imports nothing of ours and opens nothing, so every hook path can reach
// it. Every regular expression compiles on first use inside a sync.Once,
// never at package scope.
package secretshape

import (
	"strings"
	"sync"
)

// Redacted replaces every span the deny list or the entropy test refuses. A
// reader of the output sees where something was, and never what.
const Redacted = "[redacted]"

// Redact returns s with every secret-shaped span replaced by Redacted and
// how many replacements it made.
//
// Line by line, two passes per line. First every deny-list match is widened
// to the whitespace-delimited token it sits in and replaced: the deny list's
// last rule matches a secret-named key up to its `=` or `:`, and replacing
// the match span alone would blank the name of DB_PASSWORD=hunter2 and keep
// the value, the half that matters; widening to the token takes both. Then
// every remaining token the 40-character key test or the entropy test
// flags (secretToken), the two checks the slot layer ran after its deny
// list, is replaced whole. A token already replaced is Redacted itself,
// which neither check flags, so the two passes never double-count and a
// second Redact of the output changes nothing.
//
// Per line, and per rule, for a hook's budget: Go's regexp runs the whole
// twenty-rule alternation as one automaton and costs 1.4 microseconds a
// byte (measured: 11 ms for 8,000 bytes, where the spec allows 5), so each
// rule runs on its own, only on a line that holds one of the literals every
// match of that rule must contain (denyRules). A line of prose pays the
// literal searches and nothing else. What a match can span is one line: the
// two rules whose whitespace class could cross a newline (a bearer token on
// the line after its scheme word, a key with its `=` on the next line) are
// cut there, and the entropy pass still sees the token.
func Redact(s string) (out string, n int) {
	if !strings.Contains(s, "\n") {
		return redactLine(s)
	}
	lines := strings.Split(s, "\n")
	changed := false
	for i, line := range lines {
		l, k := redactLine(line)
		if k > 0 {
			lines[i] = l
			n += k
			changed = true
		}
	}
	if !changed {
		return s, 0
	}
	return strings.Join(lines, "\n"), n
}

// Secretish reports whether Redact would change s.
func Secretish(s string) bool {
	_, n := Redact(s)
	return n > 0
}

// SecretNamed reports whether key is a name the deny list's last rule calls
// secret (DB_PASSWORD, api_key, client_secret), with no value after it.
//
// The rule matches a name followed by its `=` or `:`, so in text a name and
// its value go together, and Redact takes a quoted member's value too
// (memberSpans). A reader that walks a decoded payload's members sees the
// name and the value apart, as two strings, and asks this of the name.
func SecretNamed(key string) bool {
	return len(keySpans(asciiLower(key)+"=")) > 0
}

// redactLine is Redact over one line.
func redactLine(s string) (string, int) {
	spans := denySpans(s)
	if len(spans) == 0 && !anySecretToken(s) {
		return s, 0
	}
	n := 0
	// Each deny span widens to the token holding its first byte and the
	// token holding its last byte; the widened spans, in order, are merged
	// where they overlap or touch.
	var widened [][2]int
	for _, sp := range spans {
		lo := tokenStart(s, sp[0])
		hi := tokenEnd(s, sp[1])
		if k := len(widened) - 1; k >= 0 && lo <= widened[k][1] {
			if hi > widened[k][1] {
				widened[k][1] = hi
			}
			continue
		}
		widened = append(widened, [2]int{lo, hi})
	}
	var b strings.Builder
	b.Grow(len(s))
	last := 0
	for _, sp := range widened {
		b.WriteString(s[last:sp[0]])
		b.WriteString(Redacted)
		n++
		last = sp[1]
	}
	b.WriteString(s[last:])
	first := b.String()

	// Second pass, token by token over the first pass's output.
	b.Reset()
	b.Grow(len(first))
	i := 0
	for i < len(first) {
		j := i
		for j < len(first) && isSpace(first[j]) {
			j++
		}
		b.WriteString(first[i:j])
		i = j
		for j < len(first) && !isSpace(first[j]) {
			j++
		}
		tok := first[i:j]
		if secretToken(tok) {
			b.WriteString(Redacted)
			n++
		} else {
			b.WriteString(tok)
		}
		i = j
	}
	return b.String(), n
}

// secretToken is the second pass's question for one token: hasKeyRun over
// the whole token and highEntropy over each of its path segments of 20
// characters or more, a digest under the directory that names its
// algorithm excepted (isDigest) -- the way the branch's Path ran the two
// checks (slot/path.go checkSecret), so that an OCI blob path or a
// content-addressed store path is kept, a path whose words the slash would
// otherwise join into one high-entropy run is kept, and the same hex run
// anywhere else is redacted.
func secretToken(tok string) bool {
	if tok == "" {
		return false
	}
	if hasKeyRun(tok) {
		return true
	}
	if !strings.Contains(tok, "/") {
		return highEntropy(tok)
	}
	parent := ""
	for _, seg := range strings.Split(tok, "/") {
		if len(seg) >= entropyMinRun && !isDigest(parent, seg) && highEntropy(seg) {
			return true
		}
		parent = seg
	}
	return false
}

// anySecretToken reports a whitespace-delimited token of s that secretToken
// flags: the fast path's second question, asked the same way the second
// pass asks it, so the fast path cannot say "unchanged" about a line the
// slow path would change.
func anySecretToken(s string) bool {
	i := 0
	for i < len(s) {
		for i < len(s) && isSpace(s[i]) {
			i++
		}
		j := i
		for j < len(s) && !isSpace(s[j]) {
			j++
		}
		if secretToken(s[i:j]) {
			return true
		}
		i = j
	}
	return false
}

// isDigest reports seg as the digest its parent directory names: exactly
// 40 lowercase hex characters under "sha1", or 64 under "sha256", the
// layout content-addressed stores use (an OCI image's blobs/sha256/). A
// bare 40- or 64-hex segment anywhere else is tested like any other: a
// legacy GitHub token and a Datadog application key are 40 lowercase hex,
// a Linode token and a hex-encoded 32-byte key 64, and only the location
// tells a digest from them.
func isDigest(parent, seg string) bool {
	n := 0
	switch parent {
	case "sha1":
		n = 40
	case "sha256":
		n = 64
	}
	if n == 0 || len(seg) != n {
		return false
	}
	for i := 0; i < len(seg); i++ {
		if c := seg[i]; !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return false
		}
	}
	return true
}

// tokenStart returns the index of the first byte of the whitespace-delimited
// token holding s[i], or of the token after i when s[i] is whitespace: a
// deny match that begins on the blank before a word (the rules anchored on
// `(?:^|[^A-Za-z0-9])`) widens forward into the word, not backward into the
// one before it, and the blank itself is kept so the line's spacing reads
// as it was.
func tokenStart(s string, i int) int {
	for i < len(s) && isSpace(s[i]) {
		i++
	}
	for i > 0 && !isSpace(s[i-1]) {
		i--
	}
	return i
}

// tokenEnd returns the index just past the whitespace-delimited token holding
// s[end-1], or end itself when the match ended on whitespace.
func tokenEnd(s string, end int) int {
	if end <= 0 || isSpace(s[end-1]) {
		return end
	}
	for end < len(s) && !isSpace(s[end]) {
		end++
	}
	return end
}

// isSpace is the token separator of both passes: the ASCII blanks a shell, a
// diff or a prose line is split on. Other bytes, punctuation included, stay
// inside a token so that a key and its value are one token.
func isSpace(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '\f' || c == '\v'
}

// ---- the deny list, one rule at a time ------------------------------------

// denyRule is one alternative of denyRE with the literals every match of it
// must contain: a line holding none of them cannot match the rule, so the
// rule's automaton does not run on it. Folded literals are searched in a
// lower-cased copy of the line, for the two rules written under (?i).
type denyRule struct {
	re     *lazyRegexp
	any    []string // case-sensitive: one of these must occur
	folded []string // case-insensitive: one of these must occur in the lower-cased line
}

// denyTriggers are the literals of each rule of denyRE, in the alternation's
// order. Each entry is derived from its rule's text: a required prefix, or
// for the last rule every word of its key list reduced to a part that each
// alternative holds (password, passwd, _pass, passphrase and pgpassword all
// hold "pass"; api_key, access_key, secret_key, private_key, session_key
// and _key hold "_key"). TestDenyRulesSplitAsWritten pins the count and
// that each rule's own example reaches it.
var denyTriggers = [][2][]string{
	{{"AKIA", "ASIA", "ABIA", "ACCA"}, nil},
	{{"ghp_", "gho_", "ghu_", "ghs_", "ghr_"}, nil},
	{{"github_pat_"}, nil},
	{{"glpat-"}, nil},
	{{"xoxa-", "xoxb-", "xoxp-", "xoxo-", "xoxs-", "xoxr-"}, nil},
	{{"sk-ant-"}, nil},
	{{"sk-"}, nil},
	{{"sk_live_", "sk_test_", "rk_live_", "rk_test_"}, nil},
	{{"AIza"}, nil},
	{{"eyJ"}, nil},
	{{"npm_"}, nil},
	{{"pypi-AgEIcHlwaS5vcmc"}, nil},
	{{"hvs."}, nil},
	{{"dop_v1_", "shpat_", "shpca_", "shppa_", "shpss_", "ATATT", "xapp-", "dp.st."}, nil},
	{{"hf_", "SG.", "dapi"}, nil},
	{{"key-", "SK", "PMAK-"}, nil},
	{{"-----BEGIN"}, nil},
	{{"@"}, nil},
	{nil, {"bearer"}},
	{nil, {"pass", "_pwd", "secret", "token", "_key", "apikey", "accountkey", "credential", "_auth", "cookie", "_pat", "_dsn", "database_url", "connection_string"}},
}

var (
	denyRulesOnce sync.Once
	denyRulesList []denyRule
)

// denyRules splits denyRE's pattern at its top-level `|` into one lazily
// compiled regexp per rule, paired with denyTriggers, so the lifted
// alternation stays the one copy of the rules. It panics when the split
// does not give one rule per trigger entry, which TestDenyRulesSplitAsWritten
// turns into a failure before any hook runs it.
func denyRules() []denyRule {
	denyRulesOnce.Do(func() {
		parts := splitAlternation(denyRE.pattern)
		if len(parts) != len(denyTriggers) {
			panic("secretshape: denyRE has a different number of rules than denyTriggers")
		}
		denyRulesList = make([]denyRule, len(parts))
		for i, p := range parts {
			denyRulesList[i] = denyRule{re: &lazyRegexp{pattern: p}, any: denyTriggers[i][0], folded: denyTriggers[i][1]}
		}
	})
	return denyRulesList
}

// splitAlternation cuts a RE2 pattern at every `|` outside a group and
// outside a character class, honouring backslash escapes.
func splitAlternation(pattern string) []string {
	var parts []string
	depth, inClass, start := 0, false, 0
	for i := 0; i < len(pattern); i++ {
		switch c := pattern[i]; {
		case c == '\\':
			i++
		case inClass:
			if c == ']' {
				inClass = false
			}
		case c == '[':
			inClass = true
		case c == '(':
			depth++
		case c == ')':
			depth--
		case c == '|' && depth == 0:
			parts = append(parts, pattern[start:i])
			start = i + 1
		}
	}
	return append(parts, pattern[start:])
}

// denySpans returns the deny list's match spans in line, ascending by
// start, from every rule whose triggers the line holds. The last rule, the
// secret-named key, is matched by keySpans rather than by its regexp: it is
// the one rule with an unbounded prefix under (?i), and it cost 30
// microseconds a line (470 under the race detector) where the other
// nineteen together cost under three.
func denySpans(line string) [][]int {
	var spans [][]int
	lower := ""
	rules := denyRules()
	last := len(rules) - 1
	for i, r := range rules {
		if len(r.folded) > 0 && lower == "" {
			lower = asciiLower(line)
		}
		if !r.triggered(line, lower) {
			continue
		}
		if i == last {
			for _, sp := range keySpans(lower) {
				sp[1] = valueEnd(line, sp[1])
				spans = append(spans, sp)
			}
			spans = append(spans, memberSpans(lower, line)...)
			continue
		}
		spans = append(spans, r.re.get().FindAllStringIndex(line, -1)...)
	}
	if len(spans) > 1 {
		sortSpans(spans)
	}
	return spans
}

// keyWords are the alternatives of denyRE's last rule, the secret-named key
// followed by `:` or `=`: the eight names it lists whole and the twenty-one
// suffixes it allows after `[a-z0-9_]*`, as written there.
var keyWords = []string{
	"pgpassword", "mysql_pwd", "database_url", "connection_string", "client_secret",
	"aws_secret_access_key", "aws_session_token", "_authtoken",
	"password", "passwd", "_pass", "_pwd", "secret", "token", "api_key", "apikey",
	"access_key", "secret_key", "private_key", "credentials", "credential", "_auth",
	"session_key", "cookie", "_pat", "_dsn", "_key", "passphrase", "accountkey",
}

// keySpans is denyRE's last rule by hand, over the lower-cased line: a match
// ends at each `:` or `=` that, past any RE2 blanks (\s is tab, newline,
// form feed, carriage return and space), follows text ending in one of
// keyWords; it begins where the regexp's would, at the start of the run of
// word bytes the key sits in. TestKeyRuleByHandMatchesTheRegexp holds the
// two to the same match ends on the branch's corpus and on random lines;
// the starts can only differ inside one token, which widening erases.
func keySpans(lower string) [][]int {
	var spans [][]int
	for q := 0; q < len(lower); q++ {
		if lower[q] != ':' && lower[q] != '=' {
			continue
		}
		j := q
		for j > 0 && isRE2Space(lower[j-1]) {
			j--
		}
		for _, w := range keyWords {
			if !strings.HasSuffix(lower[:j], w) {
				continue
			}
			start := j - len(w)
			for start > 0 && isWordByte(lower[start-1]) {
				start--
			}
			spans = append(spans, []int{start, q + 1})
			break
		}
	}
	return spans
}

// valueEnd extends a secret-named key's match, which ends at its `:` or `=`,
// over the value after it on the same line: past blanks, to the closing
// quote of a quoted value, or one byte into a bare one, which widening then
// takes to the end of its token. Without it the match widened to the token
// holding the `:` alone, and `password: hunter2` kept hunter2.
func valueEnd(line string, end int) int {
	i := end
	for i < len(line) && isRE2Space(line[i]) {
		i++
	}
	if i == len(line) {
		return end
	}
	if q := line[i]; q == '"' || q == '\'' {
		for j := i + 1; j < len(line); j++ {
			if line[j] == '\\' && q == '"' {
				j++
				continue
			}
			if line[j] == q {
				return j + 1
			}
		}
		return len(line)
	}
	return i + 1
}

// memberSpans is the key rule for a quoted name, as a JSON member or a quoted
// assignment writes it: `"password": "hunter2"`. The rule's regexp cannot
// match one, since the closing quote stands between the name and its `:`,
// so a structured payload's secret passed Redact whole. Each span runs from
// the name's word to the end of its value (valueEnd).
func memberSpans(lower, line string) [][]int {
	var spans [][]int
	for q := 0; q < len(lower); q++ {
		if lower[q] != ':' && lower[q] != '=' {
			continue
		}
		j := q
		for j > 0 && isRE2Space(lower[j-1]) {
			j--
		}
		if j == 0 || lower[j-1] != '"' && lower[j-1] != '\'' {
			continue
		}
		k := j - 1
		for _, w := range keyWords {
			if !strings.HasSuffix(lower[:k], w) {
				continue
			}
			start := k - len(w)
			for start > 0 && isWordByte(lower[start-1]) {
				start--
			}
			spans = append(spans, []int{start, valueEnd(line, q+1)})
			break
		}
	}
	return spans
}

// isRE2Space is RE2's \s.
func isRE2Space(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\f' || c == '\r'
}

// isWordByte is the rule's `[a-z0-9_]` over a lower-cased line.
func isWordByte(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '_'
}

// asciiLower lowers A-Z and nothing else, so an index into the result is
// the same index into the line: strings.ToLower can change a byte count
// for a non-ASCII letter, and a span found in one string is applied to the
// other.
func asciiLower(s string) string {
	for i := 0; i < len(s); i++ {
		if c := s[i]; c >= 'A' && c <= 'Z' {
			b := []byte(s)
			for ; i < len(b); i++ {
				if c := b[i]; c >= 'A' && c <= 'Z' {
					b[i] = c + 'a' - 'A'
				}
			}
			return string(b)
		}
	}
	return s
}

// triggered reports whether line can match the rule at all: it holds one of
// the rule's literals, or lower (the lower-cased line) holds one of its
// folded literals.
func (r denyRule) triggered(line, lower string) bool {
	for _, t := range r.any {
		if strings.Contains(line, t) {
			return true
		}
	}
	for _, t := range r.folded {
		if strings.Contains(lower, t) {
			return true
		}
	}
	return false
}

// sortSpans orders spans by start; the lists are short (one per match on
// one line), so an insertion sort is the whole of it.
func sortSpans(spans [][]int) {
	for i := 1; i < len(spans); i++ {
		for j := i; j > 0 && spans[j][0] < spans[j-1][0]; j-- {
			spans[j], spans[j-1] = spans[j-1], spans[j]
		}
	}
}
