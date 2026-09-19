package shape

import (
	"encoding/json"
	"path"
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
	ssh = collect(text, sshSchemes)
	if verbForTool(toolName) == VerbExecute {
		ssh = mergeHosts(ssh, sshCommandHosts(text))
	}
	return collect(text, wireSchemes), ssh
}

// mergeHosts unions two host lists, sorted and de-duplicated, preserving the
// nil-for-empty convention the record depends on: null means the call named
// none, and an empty array would be a different claim.
func mergeHosts(a, b []string) []string {
	if len(b) == 0 {
		return a
	}
	seen := make(map[string]struct{}, len(a)+len(b))
	for _, h := range a {
		seen[h] = struct{}{}
	}
	for _, h := range b {
		seen[h] = struct{}{}
	}
	out := make([]string, 0, len(seen))
	for h := range seen {
		out = append(out, h)
	}
	sort.Strings(out)
	return out
}

// sshDestPrograms are the programs whose ARGUMENTS name a host reached over
// ssh, as opposed to the URL forms sshSchemes already finds.
//
// Without this the report is quietly incomplete about the one transport it
// advertises as its blind spot. Measured before this existed: `ssh
// git@github.com` extracted github.com only by the coincidence of the user
// being named git, and `ssh deploy@git.example.com`, `ssh git.example.com`,
// `scp f deploy@host:/tmp/` and `rsync -a ./ deploy@host:/srv/` named a
// destination that appeared nowhere in the report -- not as a host, and not
// under "not observable" either.
var sshDestPrograms = map[string]bool{"ssh": true, "scp": true, "rsync": true, "sftp": true}

// valueFlags are the short flags that consume the FOLLOWING token, one set
// per program, taken from each program's own usage line.
//
// One shared table was wrong in both directions and wrong in the way that
// matters: it treated rsync's -P and -i and scp's -p, which are booleans, as
// though they took a value, so `rsync -avP deploy@host:/srv/ ./` swallowed
// the destination and recorded nothing; and it was missing most of ssh's real
// value flags, so `ssh -L 8080:localhost:80 deploy@bastion` recorded "8080"
// and missed the host entirely. A false host is worse than a missing one, and
// one table for four programs produces both.
//
// -J is in ssh's set to be SKIPPED, not collected. Its value is a jump host,
// which is a destination the command named, but recording it would mean this
// function returns hosts from two different positions with no way for a
// reader to tell them apart. It is left for a version that can say which.
var valueFlags = map[string]string{
	"ssh":   "BbcDEeFIiJLlmOoPpQRSWw",
	"scp":   "cDFiJloPSX",
	"sftp":  "BbcDFiJloPRSsX",
	"rsync": "eBTfM@",
}

func takesValue(prog string, flag byte) bool {
	return strings.IndexByte(valueFlags[prog], flag) >= 0
}

// sshCommandHosts extracts the ssh destinations named by a command line.
//
// It reads the tokens, not the raw text, because a destination is positional:
// it is an argument of a particular program, and which token that is depends
// on which flags came before it. A regular expression over the whole line
// cannot know that `key` in `ssh -i key host` is not a host.
//
// A tokenizer error is not fatal here. tokenize returns the tokens it
// completed along with the error, and a destination that appeared before an
// unterminated quote is still a destination the command named.
func sshCommandHosts(text string) []string {
	toks, _ := tokenize(text)

	var out []string
	atCommand := true
	for i := 0; i < len(toks); i++ {
		tok := toks[i]
		if isMetaToken(tok) {
			atCommand = true
			continue
		}
		if !atCommand {
			continue
		}
		if isAssignment(tok) {
			// Still at a command position: `FOO=bar ssh host` runs ssh.
			continue
		}
		atCommand = false

		prog := path.Base(tok)
		if !sshDestPrograms[prog] {
			continue
		}
		// Arguments run to the end of this command, which the next
		// metacharacter token ends.
		end := i + 1
		for end < len(toks) && !isMetaToken(toks[end]) {
			end++
		}
		out = append(out, destinations(prog, toks[i+1:end])...)
		i = end - 1
	}
	return out
}

// isMetaToken reports whether a token is one the tokenizer emitted for a shell
// metacharacter, which is where one command ends and the next begins.
func isMetaToken(tok string) bool {
	return tok != "" && len(tok) <= 2 && isMeta(tok[0])
}

// destinations returns the hosts named by one ssh-family invocation's
// arguments.
//
// ssh and sftp take exactly one destination and everything after it is the
// remote command, which must not be scanned: `ssh host ls /etc` names one
// host, not a host and a directory.
//
// scp and rsync are different, and deliberately not "the first non-flag
// argument": their first argument is usually the LOCAL side. In `scp f
// deploy@host:/tmp/` the destination is the second. So every non-flag
// argument is examined and the ones shaped like a remote spec are taken,
// which also records both ends of `rsync a@h1:/x b@h2:/y`.
func destinations(prog string, args []string) []string {
	var out []string
	endOfFlags := false

	for i := 0; i < len(args); i++ {
		a := args[i]
		if !endOfFlags && strings.HasPrefix(a, "-") && a != "-" {
			if a == "--" {
				endOfFlags = true
				continue
			}
			if strings.HasPrefix(a, "--") {
				// A long option. Its value is attached with '=' when it has
				// one; a form that separates them is not modelled, and the
				// cost of guessing wrong is reading a value as a host.
				continue
			}
			// A short flag or a cluster of them. Only the last letter can
			// take the following token as its value: in -ave the value
			// belongs to -e, and in -p2222 it is already attached.
			if takesValue(prog, a[len(a)-1]) {
				i++
			}
			continue
		}

		if prog == "ssh" || prog == "sftp" {
			if h, ok := destinationHost(a, false); ok {
				out = append(out, h)
			}
			return out
		}
		if h, ok := destinationHost(a, true); ok {
			out = append(out, h)
		}
	}
	return out
}

// destinationHost parses one [user@]host[:path] token.
//
// The user part is stripped here rather than left to host.Canonical, which
// REFUSES an authority carrying userinfo and refuses it for a good reason: in
// a URL, user:password@host is credential-bearing and dropping the credential
// silently is worse than dropping the host. In an ssh destination user@host is
// the documented syntax and carries no password, so the two cases need
// opposite handling, and this is the only place that may do the stripping --
// the URL scan in collect still hands the whole authority to Canonical.
//
// requireRemote is set for scp and rsync, where an argument is only a
// destination if it looks like one: it carries a user, or a colon that comes
// before any slash. Without that test the LOCAL side of every copy would be
// read as a host.
func destinationHost(tok string, requireRemote bool) (string, bool) {
	if tok == "" {
		return "", false
	}

	// A bracketed IPv6 literal, with an optional user in front of it. Its own
	// colons are not host:path separators, so it is recognised before any
	// colon is looked for.
	rest, hadUser := tok, false
	if at := strings.IndexByte(tok, '@'); at >= 0 && strings.HasPrefix(tok[at+1:], "[") {
		rest, hadUser = tok[at+1:], true
	}
	if strings.HasPrefix(rest, "[") {
		end := strings.Index(rest, "]")
		if end < 0 {
			return "", false
		}
		if requireRemote && !hadUser && !strings.HasPrefix(rest[end+1:], ":") {
			return "", false
		}
		return host.Canonical(rest[:end+1])
	}

	// The PATH is split off before the user, and the order is the whole
	// point: a path may contain an '@' of its own. Splitting on the last '@'
	// in the token turned `deploy@host:/srv/app@1.2.3/` into "1.2.3/" and the
	// destination was lost.
	colon := strings.IndexByte(tok, ':')
	slash := strings.IndexByte(tok, '/')
	hostPart := tok
	remoteByColon := false
	switch {
	case colon >= 0 && (slash < 0 || colon < slash):
		hostPart = tok[:colon]
		remoteByColon = true
	case slash >= 0:
		hostPart = tok[:slash]
	}

	// Only now is the user stripped, and only from within the host part.
	//
	// It is stripped here rather than left to host.Canonical, which REFUSES
	// an authority carrying userinfo and refuses it for a good reason: in a
	// URL, user:password@host is credential-bearing and dropping the
	// credential silently is worse than dropping the host. In an ssh
	// destination user@host is the documented syntax and carries no password,
	// so the two cases need opposite handling, and this is the only place
	// that may do the stripping -- the URL scan in collect still hands the
	// whole authority to Canonical.
	if at := strings.LastIndexByte(hostPart, '@'); at >= 0 {
		hadUser = true
		hostPart = hostPart[at+1:]
	}

	// requireRemote is set for scp and rsync, where an argument is only a
	// destination if it looks like one: it carries a user, or a colon that
	// comes before any slash. Without that test the LOCAL side of every copy
	// would be read as a host.
	if requireRemote && !hadUser && !remoteByColon {
		return "", false
	}
	// A single-character host before a colon is a Windows drive letter far
	// more often than a hostname, and `rsync C:/src dst` must not record a
	// host called "c".
	if remoteByColon && !hadUser && len(hostPart) < 2 {
		return "", false
	}

	// An ssh-config alias has no dots and resolves to something this function
	// cannot see. It is recorded as the command named it and never resolved:
	// resolving would mean reading ~/.ssh/config, which is a file this package
	// is not allowed to open, and guessing would put a name in the store that
	// the user never typed.
	return host.Canonical(hostPart)
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
