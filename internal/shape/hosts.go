package shape

import (
	"encoding/json"
	"sort"
	"strings"

	"github.com/altrace-dev-role/rashomon/internal/host"
)

// Host extraction lives in this package for the reason stated at the top of
// shape.go: it is the only place allowed to look inside tool_input, so it is
// the only file that has to be audited to believe the no-content guarantee.
// Putting it anywhere else would mean two files could read a command string,
// and the guarantee would cost twice as much to check.
//
// What crosses the boundary out of here is a list of hostnames and nothing
// else. Not the URL, not the path, not the query, not the surrounding command.

// Scheme prefixes that introduce a network host we can observe on the wire.
//
// Plain http:// is included in the SEARCH even though v0.1 does not observe
// plain HTTP through the proxy. A host the agent named is a declaration
// whatever scheme it named it under, and the report's "declared but not
// observed" line is the honest place for it -- dropping it here would instead
// make that host silently absent from both sides.
var wireSchemes = []string{"https://", "http://", "wss://", "ws://"}

// Scheme prefixes that introduce a host reached over ssh, which the proxy
// cannot see. These render under coverage as "not observable", never as a
// finding, so they are collected separately rather than mixed in and then
// filtered by guesswork downstream.
var sshSchemes = []string{"ssh://", "git+ssh://", "git@"}

// Hosts returns the wire-observable and ssh hostnames named by a tool call.
//
// Both lists are sorted and de-duplicated: the record has to be byte-identical
// across two runs of the same call for the determinism property, and map
// iteration order is not.
//
// Only two tools are read, and only one field of each. WebFetch declares its
// destination in `url`. Bash carries a command line whose text may name any
// number of hosts. Every other tool -- Read, Write, Edit, Task, every mcp__*
// call -- names no network destination in a field whose meaning we know, and
// guessing at an arbitrary tool's arbitrary field is how a "url" belonging to
// some MCP server's unrelated schema ends up recorded as a destination this
// session reached.
func Hosts(toolName string, toolInput json.RawMessage) (wire []string, ssh []string) {
	var text string
	switch {
	case toolName == "WebFetch":
		if u, ok := stringField(toolInput, "url"); ok {
			text = u
		}
	case verbForTool(toolName) == VerbExecute:
		if cmd, ok := commandField(toolInput); ok {
			text = cmd
		}
	}
	if text == "" {
		return nil, nil
	}
	return collect(text, wireSchemes), collect(text, sshSchemes)
}

// collect finds every hostname in text introduced by one of the given prefixes.
//
// The scan and the slice both read `lower`, and they have to be the same
// string. strings.ToLower is NOT length-preserving in UTF-8 -- U+212A KELVIN
// SIGN is three bytes and lowercases to one, U+212B and U+2126 are three and
// lowercase to two -- so searching the lowered copy and slicing the original
// put the slice 8-k bytes from the true host for k bytes of shrink earlier in
// the command. Under 8, the slice began inside "https://" and the host was
// silently dropped, so a host the command NAMED came back as "reached but
// never named". Over 8, it began before the scheme and a fragment of the
// command line was accepted as a hostname and written to the store, which
// nothing else in this program permits (CWE-176).
//
// Reading the lowered copy costs nothing: host.Canonical lower-cases what it
// returns, and DNS is case-insensitive, so the original case was never wanted.
func collect(text string, prefixes []string) []string {
	seen := map[string]struct{}{}
	lower := strings.ToLower(text)

	for _, p := range prefixes {
		from := 0
		for {
			i := strings.Index(lower[from:], p)
			if i < 0 {
				break
			}
			at := from + i + len(p)
			from = at
			if h, ok := host.Canonical(authority(lower[at:])); ok {
				seen[h] = struct{}{}
			}
		}
	}
	if len(seen) == 0 {
		return nil
	}
	out := make([]string, 0, len(seen))
	for h := range seen {
		out = append(out, h)
	}
	sort.Strings(out)
	return out
}

// authority returns the leading authority of s: everything up to the first
// character that cannot be part of a host:port, with a bracketed IPv6 literal
// kept whole.
//
// A command line is not a URL, so the terminator set is deliberately wide. It
// includes the shell's own punctuation (quotes, backticks, $, |, &, ;,
// parentheses, braces) because a URL inside a command is surrounded by it, and
// a host that swallowed a trailing quote would never match the same host seen
// on the wire.
//
// It deliberately does NOT stop at ':', and that is a security property rather
// than a convenience. Stopping there splits "u:p@internal.example" at the
// first colon and returns "u" -- a plausible-looking hostname that is actually
// the USERNAME out of a credential-bearing URL, which host.Canonical would
// then happily accept and the store would record. Returning the whole
// authority instead lets Canonical see the '@' and refuse the input outright
// (CWE-522). Port stripping is Canonical's job, and it does it after that
// check, which is the only order in which both answers are right.
func authority(s string) string {
	if strings.HasPrefix(s, "[") {
		if end := strings.Index(s, "]"); end >= 0 {
			// Keep any :port after the bracket; Canonical strips it.
			return s[:end+1] + portSuffix(s[end+1:])
		}
		return ""
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c <= ' ' || c == 0x7f {
			return s[:i]
		}
		switch c {
		case '/', '?', '#', '\\', '"', '\'', '`', '$', '|', '&', ';', '(', ')', '{', '}', '<', '>', ',', '*':
			return s[:i]
		}
	}
	return s
}

// portSuffix returns a leading ":digits" run, and nothing else. It exists so a
// bracketed IPv6 authority keeps its port for Canonical to strip, without
// swallowing the path that follows.
func portSuffix(s string) string {
	if !strings.HasPrefix(s, ":") {
		return ""
	}
	i := 1
	for i < len(s) && s[i] >= '0' && s[i] <= '9' {
		i++
	}
	if i == 1 {
		return ""
	}
	return s[:i]
}

// stringField reads one named string field out of a tool input.
//
// It is the WebFetch counterpart to commandField and is kept just as narrow:
// one named key, a string or nothing. It never walks the object, so no field
// whose name we did not write down can be read by accident.
func stringField(raw json.RawMessage, name string) (string, bool) {
	if len(raw) == 0 {
		return "", false
	}
	var obj map[string]json.RawMessage
	if json.Unmarshal(raw, &obj) != nil {
		return "", false
	}
	v, ok := obj[name]
	if !ok {
		return "", false
	}
	var s string
	if json.Unmarshal(v, &s) != nil {
		return "", false
	}
	return s, true
}
