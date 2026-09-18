// Package host canonicalises a hostname to the one spelling every side of the
// product compares on.
//
// It exists because a hostname arrives here from two unrelated places and has
// to be the same string in both: the declaration side reads it out of a tool
// call the agent wrote, and the wire side reads it out of the proxy's CONNECT
// record. "PyPI.org", "pypi.org:443" and "pypi.org." are one destination, and a
// report that treats them as three answers its own central question wrongly --
// "reached but never named" fires on a host that WAS named, in different case.
//
// A hostname is the most sensitive field this product stores. It can carry a
// tenant, a bucket, or an internal topology. That is why this package returns a
// hostname and nothing else: no path, no query, no userinfo, no port. Anything
// that is not a hostname is dropped here rather than trusted to be dropped by
// every caller.
package host

import (
	"net/url"
	"strings"
)

// Canonical reduces a raw authority or URL-ish string to a comparable
// hostname, reporting false when the input yields none.
//
// The transformations, and why each one is needed rather than defensive:
//
//   - lower-cased, because DNS is case-insensitive and both sides spell it
//     however their source did;
//   - port stripped, because pypi.org:443 and pypi.org are one destination and
//     the proxy records the port while a tool call usually does not;
//   - userinfo REJECTED outright rather than stripped (see below);
//   - IPv6 brackets kept, because "::1" without them is ambiguous against a
//     host:port split and every consumer here expects the bracketed form;
//   - one trailing dot dropped, because "pypi.org." is the fully-qualified
//     spelling of the same name and only one of the two sides ever emits it;
//   - loopback KEPT. Filtering it is the report's decision, not this
//     function's: the wire side needs to see loopback rows in order to exclude
//     them knowingly, and a canonicaliser that silently dropped them would make
//     "no loopback traffic" and "loopback traffic we discarded" the same
//     answer.
//
// Userinfo is rejected, not stripped, and that is a security decision
// (CWE-522). A string like "user:password@internal.example" carries a
// credential, and stripping it would leave this function returning a clean
// hostname for an input we then have to trust every caller never logged. There
// is no legitimate source of userinfo in either of this product's two inputs:
// the proxy's CONNECT target is a bare authority, and a URL with embedded
// credentials in a tool call is a thing we specifically do not want to record
// the existence of. Refusing it means a credential cannot be carried even one
// function deeper.
func Canonical(raw string) (string, bool) {
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
	// is not a hostname and must not reduce to the empty string quietly.
	if len(h) > 1 && strings.HasSuffix(h, ".") {
		h = h[:len(h)-1]
	}

	if !plausible(h) {
		return "", false
	}
	return h, true
}

// stripPort removes a :port suffix from a bare authority, leaving a bracketed
// IPv6 literal intact.
//
// It does not use net.SplitHostPort: that lives in package net, which the
// recorder's dependency graph must not contain (H-17), and the only thing
// needed here is the last colon outside the brackets.
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
// path or query that survived parsing, control bytes, and a bracketed form that
// is not closed.
//
// Non-ASCII names pass through lower-cased rather than being converted to
// punycode. IDNA-to-ASCII is NOT implemented (see the package note in the
// commit): the two sides of the comparison receive the same bytes from the same
// session, so they still match each other; what is lost is that a Unicode name
// and its xn-- spelling would compare unequal.
func plausible(h string) bool {
	if h == "" || h == "." {
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
