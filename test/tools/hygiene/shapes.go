// Package hygiene keeps captured payloads fit to commit. Scrub rewrites a
// captured hook payload or session transcript so that it carries no secret
// shape, no email address, no UUID, no home, temp or scratchpad path and no
// account inventory; Walk looks for what Scrub removes in the files this
// repository commits as fixtures, and H-113 fails CI on anything it finds.
//
// A secret scan alone is not this. The inventory in a session's init event,
// a home directory and a scratchpad id are not credentials, so a capture can
// pass a secret scan and still name the account, its employer and its
// clients. Secret shapes come from internal/secretshape, so the scan and any
// later reader that must not print a secret share one definition.
//
// It lives under test/ because nothing shipped needs it: ./cmd/rashomon does
// not import it, and test/tools/scrub is the command a contributor runs.
package hygiene

import (
	"fmt"
	"regexp"
	"strings"
)

// inventory names the lists of Claude Code's init event that enumerate an
// account: its tools, skills, plugins, MCP servers, slash commands and
// agents. A skill or plugin name can carry an employer's or a client's name
// and a plugin's path its owner's home, so a fixture keeps how many there
// were and nothing else.
var inventory = map[string]bool{
	"tools": true, "skills": true, "plugins": true,
	"mcp_servers": true, "slash_commands": true, "agents": true,
}

// inventoryInText finds an inventory list that is not a member of a JSON
// document Walk can parse: a payload logged inside a line of text, or one
// escaped inside a string. Scrub cannot rewrite it there, so it is reported.
var inventoryInText = regexp.MustCompile(`\\*"(tools|skills|plugins|mcp_servers|slash_commands|agents)\\*"\s*:\s*\[\s*[^\]\s]`)

// namedDigest finds a lowercase 40- or 64-hex value under a member whose
// name says it is a digest (leaf_hash, chain_hash, sha256, checksum), escaped
// or not. The entropy test flags every such run, because a GitHub token and a
// Datadog key have the same shape and only the location tells them apart;
// secretshape keeps one under sha256/ in a path, and a member named for a
// hash is the same kind of location. A value the file names a digest once
// is a digest wherever it repeats: nono's prev_chain is the previous
// record's chain_hash.
var namedDigest = regexp.MustCompile(`\\*"(?i:[a-z0-9_]*(?:hash|digest|checksum|sha|sha1|sha256))\\*"\s*:\s*\\*"([0-9a-f]{64}|[0-9a-f]{40})\\*"`)

func digests(text string) map[string]bool {
	set := map[string]bool{}
	for _, m := range namedDigest.FindAllStringSubmatch(text, -1) {
		set[m[1]] = true
	}
	return set
}

var uuidRE = regexp.MustCompile(`[0-9A-Fa-f]{8}-[0-9A-Fa-f]{4}-[0-9A-Fa-f]{4}-[0-9A-Fa-f]{4}-[0-9A-Fa-f]{12}`)

// placeholderUUID begins every UUID Scrub writes: the version and variant
// bits are set, so code that parses a session id still can, and the leading
// zeros are a run no generator produces, so the scan tells a placeholder
// from a capture's own. The nil UUID shares the prefix and is not an
// account's either.
const placeholderUUID = "00000000-0000-"

// accountUUIDs returns the spans of the UUIDs in s that are not placeholders
// and not part of a longer run of letters and digits.
func accountUUIDs(s string) [][]int {
	var out [][]int
	for _, sp := range uuidRE.FindAllStringIndex(s, -1) {
		if sp[0] > 0 && isAlnum(s[sp[0]-1]) || sp[1] < len(s) && isAlnum(s[sp[1]]) ||
			strings.HasPrefix(s[sp[0]:sp[1]], placeholderUUID) {
			continue
		}
		out = append(out, sp)
	}
	return out
}

// uuidInPath reports an account UUID inside a path: a Claude Code project's
// transcript, a session's scratchpad and a task's output file are all named
// by one, and the path is what a fixture copies from the machine.
func uuidInPath(s string) bool {
	for _, sp := range accountUUIDs(s) {
		lo, hi := sp[0], sp[1]
		for lo > 0 && !isBlank(s[lo-1]) {
			lo--
		}
		for hi < len(s) && !isBlank(s[hi]) {
			hi++
		}
		if strings.ContainsAny(s[lo:hi], `/\`) {
			return true
		}
	}
	return false
}

var emailRE = regexp.MustCompile(`[A-Za-z0-9._%+-]+@([A-Za-z0-9-]+(?:\.[A-Za-z0-9-]+)*\.[A-Za-z]{2,})`)

// emailSpans returns the spans of the email addresses in s that can reach a
// person: not one at a domain reserved for examples (RFC 2606, RFC 6761),
// where Scrub's placeholders live, and not git@, the user every forge's SSH
// remote names.
func emailSpans(s string) [][]int {
	var out [][]int
	for _, m := range emailRE.FindAllStringSubmatchIndex(s, -1) {
		if s[m[0]:m[2]-1] == "git" || reservedDomain(strings.ToLower(s[m[2]:m[3]])) {
			continue
		}
		out = append(out, m[:2])
	}
	return out
}

func reservedDomain(d string) bool {
	for _, r := range []string{"example.com", "example.net", "example.org"} {
		if d == r || strings.HasSuffix(d, "."+r) {
			return true
		}
	}
	switch d[strings.LastIndexByte(d, '.')+1:] {
	case "example", "invalid", "test", "localhost":
		return true
	}
	return false
}

// pathStart is what may come before a path: the start, or a byte no file
// name holds there. "/home/x" after a host name, as in https://host/home/x,
// is part of a URL and not a home directory.
const pathStart = `(^|[^A-Za-z0-9_.~$-])`

// homeRE is a home directory that names its user: /home/<name>,
// /Users/<name>, /root, and C:\Users\<name> however its separators are
// written or escaped.
var homeRE = regexp.MustCompile(pathStart + `(/home/[^/\s"'\\:;,]+|/Users/[^/\s"'\\:;,]+|/root\b|(?i:[a-z]:[\\/]+users[\\/]+)[^\\/\s"':;,]+)`)

// slugRE is a Claude Code project directory, the session's working directory
// with each separator turned into '-': -home-alice-src-app names alice as
// plainly as /home/alice does.
var slugRE = regexp.MustCompile(`(^|[/\\])(-home-[^/\\\s"']+|-Users-[^/\\\s"']+|[A-Za-z]--Users-[^/\\\s"']+|-root(?:-[^/\\\s"']+)?)($|[/\\\s"'])`)

// scratchpadRE is a Claude Code session's scratchpad, which is named by the
// project directory and the session id; tempRE is a temp root.
var (
	scratchpadRE = regexp.MustCompile(`(?:/private)?/tmp/claude-[0-9]+/[^/\s"'\\]+/[^/\s"'\\]+/scratchpad`)
	tempRE       = regexp.MustCompile(pathStart + `((?:/private)?/var/folders/[^/\s"'\\]+/[^/\s"'\\]+/T|(?:/private)?/tmp|/var/tmp|(?i:[a-z]:[\\/]+users[\\/]+[^\\/\s"']+[\\/]+appdata[\\/]+local[\\/]+temp))\b`)
)

// homeIn reports a home directory, or a project directory named after one.
func homeIn(s string) bool { return homeRE.MatchString(s) || slugRE.MatchString(s) }

// replaceDir replaces dir wherever it is a whole path prefix in s: followed
// by a separator or the end, never by more of a longer name.
func replaceDir(s, dir, with string) string {
	if len(dir) < 2 || !strings.Contains(s, dir) {
		return s
	}
	var b strings.Builder
	for {
		i := strings.Index(s, dir)
		if i < 0 {
			break
		}
		end := i + len(dir)
		if end < len(s) && (isAlnum(s[end]) || strings.IndexByte("_.-", s[end]) >= 0) {
			b.WriteString(s[:end])
		} else {
			b.WriteString(s[:i] + with)
		}
		s = s[end:]
	}
	b.WriteString(s)
	return b.String()
}

// assign replaces each span of s with the placeholder its value already has
// in seen, or the next one in order of first appearance, so the same id
// wherever it recurs reads as the same placeholder and a join survives.
func assign(s string, spans [][]int, seen map[string]string, form string) string {
	if len(spans) == 0 {
		return s
	}
	var b strings.Builder
	last := 0
	for _, sp := range spans {
		key := strings.ToLower(s[sp[0]:sp[1]])
		p, ok := seen[key]
		if !ok {
			p = fmt.Sprintf(form, len(seen)+1)
			seen[key] = p
		}
		b.WriteString(s[last:sp[0]] + p)
		last = sp[1]
	}
	return b.String() + s[last:]
}

func isAlnum(c byte) bool {
	return c >= '0' && c <= '9' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z'
}

func isBlank(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '"' || c == '\''
}
