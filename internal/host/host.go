// Package host canonicalises a hostname to the one spelling every side of the
// product compares on.
//
// It exists because a hostname arrives here from unrelated places and has to
// be the same string in each: the declaration side reads it out of a tool call
// the agent wrote, the novelty baseline compares it against every earlier
// session's, and a sandbox's network record (internal/nono) names the target
// it saw. "PyPI.org", "pypi.org:443" and "pypi.org." are one destination, and
// so are "[2606:4700:0:0:0:0:0:1111]" and "[2606:4700::1111]", and "167838211"
// and "10.1.2.3" -- a client normalises an IP literal before it connects. A
// comparison that treats them as different answers wrongly: a host named in
// one spelling reads as new, or as never named, when it was.
//
// A hostname is the most sensitive field this product stores. It can carry a
// tenant, a bucket, or an internal topology. That is why this package returns a
// hostname and nothing else: no path, no query, no userinfo, no port. Anything
// that is not a hostname is dropped here rather than trusted to be dropped by
// every caller.
package host

import (
	"net/netip"
	"net/url"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

// maxName is the longest a hostname can be: 253 bytes in its text form
// (RFC 1035 2.3.4, less the root label's length byte and the trailing dot).
// Nothing longer is returned, so a fragment of a command line cannot be
// recorded as one host however long it is.
const maxName = 253

// Canonical reduces a raw authority or URL-ish string to a comparable
// hostname, reporting false when the input yields none.
//
// The transformations, and why each one is needed rather than defensive:
//
//   - lower-cased, because DNS is case-insensitive and every source spells it
//     however it likes;
//   - port stripped, because pypi.org:443 and pypi.org are one destination and
//     a network record carries the port while a tool call usually does not;
//   - userinfo REJECTED outright rather than stripped (see below);
//   - an IP literal written in its one spelling (address): an IPv6 address
//     compressed, lower-case and in brackets, because "::1" without them is
//     ambiguous against a host:port split; an IPv4-mapped one as the IPv4
//     address it maps; an IPv4 address in any form inet_aton reads --
//     167838211, 0xa010203, 012.1.2.3, 10.1.515 -- as a dotted quad. A
//     bracket holding anything but an IPv6 address, and a colon outside one,
//     is refused: neither is a host;
//   - one trailing dot dropped, because "pypi.org." is the fully-qualified
//     spelling of the same name;
//   - at most 253 bytes (maxName);
//   - loopback KEPT. Filtering it is the report's decision, not this
//     function's (Loopback): a canonicaliser that silently dropped it would
//     make "no loopback traffic" and "loopback traffic we discarded" the same
//     answer.
//
// Userinfo is rejected, not stripped, and that is a security decision
// (CWE-522). A string like "user:password@internal.example" carries a
// credential, and stripping it would leave this function returning a clean
// hostname for an input we then have to trust every caller never logged. There
// is no legitimate source of userinfo in this product's inputs: a network
// record's target is a bare authority, and a URL with embedded
// credentials in a tool call is a thing we specifically do not want to record
// the existence of. Refusing it means a credential cannot be carried even one
// function deeper.
func Canonical(raw string) (string, bool) {
	return canonical(raw, true)
}

// CanonicalName is Canonical without the inet_aton fold: every other
// transformation in Canonical's doc still applies -- case, port, the
// trailing dot, the length bound, an IPv6 literal's one spelling -- but a
// name made only of digits is returned as the name it is, not reinterpreted
// as the address curl or wget would read it as.
//
// Folding belongs to a URL authority, where it is a fact about what a real
// client does before it connects. It does not belong to an ssh, scp, sftp or
// rsync destination: ssh matches a Host alias in ~/.ssh/config against the
// exact text the command line named, before any resolution happens, and a
// numeric alias is ordinary there ("Host 1234" is ssh config nobody has to
// justify). Canonical("1234") -> "0.0.4.210", and Canonical("0") ->
// "0.0.0.0", which Loopback then reads as this machine -- right for a URL a
// client actually folds, wrong for an alias nothing resolved. CanonicalName
// is what the ssh-destination path (destinationHost) and this package's own
// Loopback use instead, so a destination this product deliberately never
// resolves (see destinationHost's doc) is not resolved here either.
func CanonicalName(raw string) (string, bool) {
	return canonical(raw, false)
}

func canonical(raw string, foldNumeric bool) (string, bool) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return "", false
	}

	// Reject before parsing. url.Parse would happily split userinfo off and
	// hand back a clean Host, which is exactly the outcome that makes the
	// credential's presence invisible to the caller.
	if strings.Contains(s, "@") {
		return "", false
	}

	// A scheme-ful string is parsed; a bare authority is not a valid URL on its
	// own, so it is parsed as the authority it is.
	h := s
	if i := strings.Index(s, "://"); i >= 0 {
		u, err := url.Parse(s)
		if err != nil {
			return "", false
		}
		// Hostname() removes the port and the brackets around an IPv6 literal.
		h = u.Hostname()
		if h == "" {
			return "", false
		}
		if strings.Contains(h, ":") {
			h = "[" + h + "]"
		}
	} else {
		h = stripPort(h)
	}

	h = strings.ToLower(strings.TrimSpace(h))
	// Exactly one trailing dot, and only when something precedes it: "." alone
	// is not a hostname and must not reduce to the empty string quietly. A
	// second is an empty label, which no hostname has, and accepting what is
	// left would return a name that a second pass changes again.
	if len(h) > 1 && strings.HasSuffix(h, ".") {
		h = h[:len(h)-1]
		if strings.HasSuffix(h, ".") {
			return "", false
		}
	}

	if !plausible(h) {
		return "", false
	}
	return address(h, foldNumeric)
}

// address writes an IP literal h in its one spelling (see Canonical) and
// returns any other host as it is. A bracket that does not hold an IPv6
// address, or a colon outside a bracket that does not make one, is refused.
// foldNumeric gates inet_aton only -- an IPv6 literal is still parsed to its
// one spelling either way, because a bracket or a bare double colon is
// unambiguously an address and carries none of the alias ambiguity inet_aton
// folding has (see CanonicalName).
func address(h string, foldNumeric bool) (string, bool) {
	inner, bracketed := h, strings.HasPrefix(h, "[")
	if bracketed {
		inner = h[1 : len(h)-1]
	}
	if bracketed || strings.IndexByte(h, ':') >= 0 {
		a, err := netip.ParseAddr(inner)
		if err != nil || !a.Is6() {
			return "", false
		}
		return spell(a), true
	}
	if foldNumeric {
		if a, ok := inetAton(h); ok {
			return a.String(), true
		}
	}
	return h, true
}

// spell is an address's one spelling: an IPv4-mapped IPv6 address as the
// IPv4 address it maps, any other IPv6 address compressed, lower-case and in
// brackets.
func spell(a netip.Addr) string {
	if a = a.Unmap(); a.Is4() {
		return a.String()
	}
	return "[" + a.String() + "]"
}

// inetAton reads h as an IPv4 address the way inet_aton(3) does, which is how
// curl, ssh and the resolver read a host that is all numbers: one to four
// parts separated by dots, each decimal, octal with a leading 0, or hex with
// 0x; the last part fills the bytes the others leave, so 10.1.515 is
// 10.1.2.3 and so is 167838211. Anything else is not an address and stays a
// name -- 1.2.3.4.5, 256.1.1.1 and 08.1.2.3 are resolved, not parsed.
func inetAton(h string) (netip.Addr, bool) {
	if !atonShaped(h) {
		return netip.Addr{}, false
	}
	parts := strings.SplitN(h, ".", 5)
	if len(parts) > 4 {
		return netip.Addr{}, false
	}
	var v uint64
	for i, p := range parts {
		n, ok := atonPart(p)
		if !ok {
			return netip.Addr{}, false
		}
		// The last part takes every byte left: four for a lone number, one
		// for the fourth of four. Every other part takes one.
		width := uint(8)
		if i == len(parts)-1 {
			width = 8 * uint(5-len(parts))
		}
		if n >= 1<<width {
			return netip.Addr{}, false
		}
		v = v<<width | n
	}
	return netip.AddrFrom4([4]byte{byte(v >> 24), byte(v >> 16), byte(v >> 8), byte(v)}), true
}

// atonShaped reports whether h could be an inet_aton address at all: a digit
// first, and nothing but hex digits, x and dots. A name is turned away here
// without being split or parsed: each part strconv refused cost an error
// value, and an 8 MiB line of `https://a.b/` allocated 50 MiB of them.
func atonShaped(h string) bool {
	if h == "" || h[0] < '0' || h[0] > '9' {
		return false
	}
	for i := 0; i < len(h); i++ {
		switch c := h[i]; {
		case c >= '0' && c <= '9', c >= 'a' && c <= 'f', c >= 'A' && c <= 'F', c == 'x', c == 'X', c == '.':
		default:
			return false
		}
	}
	return true
}

// atonPart reads one part of an inet_aton address: hex after 0x, octal after
// a leading 0, decimal otherwise, at most 32 bits.
func atonPart(p string) (uint64, bool) {
	base := 10
	switch {
	case len(p) > 2 && (p[:2] == "0x" || p[:2] == "0X"):
		p, base = p[2:], 16
	case len(p) > 1 && p[0] == '0':
		p, base = p[1:], 8
	}
	if p == "" || p[0] == '+' || p[0] == '-' {
		return 0, false
	}
	n, err := strconv.ParseUint(p, base, 32)
	return n, err == nil
}

// Shaped reports whether s has the shape of a host as Canonical returns one:
// a bracketed IPv6 address with no zone, or a name of at most 253 bytes made
// of ASCII letters, digits, '-', '_' and '.', or non-ASCII letters, marks and
// digits for an internationalised name, with no empty label, no '-' or '.'
// at its start and no '-' at its end. One trailing dot is allowed: it is the
// fully-qualified spelling, which Canonical drops.
//
// Canonical is permissive about the bytes a name may hold on purpose: it
// serves every caller, and an over-strict rule there would silently drop
// real internal hostnames. Shaped is the declaration side's question, asked
// where a host is cut out of a command line: is this a host at all? A URL's
// authority is cut where a command line's punctuation begins, which is not
// where a host ends -- `=` and `!` are neither -- so without it
// `https://api.example.com=ghp_...!x/` named a "host" carrying a token, and a
// field documented as the hostnames a call named held text that was not one.
func Shaped(s string) bool {
	if strings.HasPrefix(s, "[") {
		if !strings.HasSuffix(s, "]") {
			return false
		}
		a, err := netip.ParseAddr(s[1 : len(s)-1])
		return err == nil && a.Is6() && a.Zone() == ""
	}
	if s == "" || len(s) > maxName || !utf8.ValidString(s) {
		return false
	}
	if s[0] == '-' || s[0] == '.' || s[len(s)-1] == '-' || strings.Contains(s, "..") {
		return false
	}
	alnum := false
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			alnum = true
		case r == '-' || r == '.' || r == '_':
		case r >= utf8.RuneSelf && (unicode.IsLetter(r) || unicode.IsMark(r) || unicode.IsDigit(r)):
			alnum = true
		default:
			return false
		}
	}
	return alnum
}

// Same reports whether a stored host and a canonical one name the same
// destination: equal as stored, or equal once the stored one is
// canonicalised. A stored host is canonical when this build wrote it, but a
// record or a baseline written before an IP literal had one spelling holds it
// as the call wrote it -- 167838211, [2606:4700:0:0:0:0:0:1111] -- and a
// forget that compared it as stored would leave it on disk.
func Same(stored, canonical string) bool {
	if stored == canonical {
		return true
	}
	c, ok := Canonical(stored)
	return ok && c == canonical
}

// Loopback reports whether h names this machine: localhost or a name under
// .localhost (RFC 6761 6.3), an address in 127.0.0.0/8 or ::1, or the
// unspecified address, a connection to which reaches this machine. It is
// decided on the parsed address, so every spelling of one --
// [0:0:0:0:0:0:0:1], 2130706433, 127.1 -- answers alike, where a list of
// spellings answered only for the spellings on it. The fold (Canonical, not
// CanonicalName) is wanted here: a URL authority of "2130706433" really does
// reach this machine, obfuscated decimal form or not, and the whole point is
// to catch that spelling, not just the dotted one.
func Loopback(h string) bool {
	return loopback(Canonical(h))
}

// LoopbackName is Loopback without the inet_aton fold (host.CanonicalName,
// not Canonical): for an ssh destination, where "0" is as plausible a config
// alias as any other name and is never resolved by anything in this
// package. Loopback("0") folds to 0.0.0.0 and reads as this machine, which
// is right for a URL a client actually resolves and wrong for an alias
// nothing here resolved; LoopbackName("0") does not fold and is false.
func LoopbackName(h string) bool {
	return loopback(CanonicalName(h))
}

// loopback is Loopback and LoopbackName's shared tail, given the result of
// whichever canonicalisation its caller already applied.
func loopback(c string, ok bool) bool {
	if !ok {
		return false
	}
	if c == "localhost" || strings.HasSuffix(c, ".localhost") {
		return true
	}
	a, err := netip.ParseAddr(strings.TrimSuffix(strings.TrimPrefix(c, "["), "]"))
	return err == nil && (a.IsLoopback() || a.IsUnspecified())
}

// stripPort removes a :port suffix from a bare authority, leaving a bracketed
// IPv6 literal intact.
//
// It does not use net.SplitHostPort: that lives in package net, which the
// recorder's dependency graph must not contain (H-17), and the only thing
// needed here is the last colon outside the brackets. net/netip, which
// address uses, is not package net: it opens nothing and imports no socket
// code, and net/url already brings it into the graph.
func stripPort(h string) string {
	if strings.HasPrefix(h, "[") {
		end := strings.LastIndex(h, "]")
		if end < 0 {
			return h
		}
		// Keep the brackets; drop anything after them.
		return h[:end+1]
	}
	// A bare IPv6 literal has several colons and no port. Only a single colon
	// can be a port separator on an unbracketed authority.
	if strings.Count(h, ":") != 1 {
		return h
	}
	return h[:strings.IndexByte(h, ':')]
}

// plausible rejects strings that cannot be a hostname.
//
// It is intentionally permissive about the characters a name may contain: this
// is not a validator and an over-strict rule here would silently drop real
// internal hostnames, which are the most interesting ones in the report. It
// refuses only what would corrupt a record or a comparison -- whitespace, a
// path or query that survived parsing, control bytes, a bracketed form that
// is not closed, and more than maxName bytes.
//
// Non-ASCII names pass through lower-cased rather than being converted to
// punycode, because IDNA-to-ASCII is not implemented: it needs
// golang.org/x/net/idna, a new module dependency this package has not taken.
// So a Unicode name and its xn-- spelling compare unequal: bücher.example
// named in one call and xn--bcher-kva.example in another are two hosts to
// the baseline, and a sandbox record, which carries the xn-- form a client
// resolved, does not match the Unicode name a command line held. Every such
// mismatch errs toward a host read as new, never toward one hidden.
func plausible(h string) bool {
	if h == "" || h == "." || len(h) > maxName {
		return false
	}
	if strings.HasPrefix(h, "[") != strings.HasSuffix(h, "]") {
		return false
	}
	for i := 0; i < len(h); i++ {
		c := h[i]
		if c <= ' ' || c == 0x7f {
			return false
		}
		switch c {
		case '/', '?', '#', '\\', '@', '"', '\'', '<', '>', '%', ',':
			return false
		}
	}
	return true
}
