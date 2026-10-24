package spec

import (
	"slices"
	"strings"
	"unicode"
)

// The per-field checks: charsets, shapes and the facts rules. Each returns the
// reason a value is refused, or "".

// mechanismReason is "" when mech is the mechanism word of kind. The order is
// the closed list, then the kind's own word, then the no-spaces rule: the two
// words of the closed list that carry a space are therefore refused for it
// (see factMechanisms).
func mechanismReason(kind, mech string) Reason {
	i := slices.Index(factMechanisms, mech)
	switch {
	case i < 0:
		return ReasonEnum
	case FactKinds[i] != kind:
		return ReasonMechanism
	case strings.Contains(mech, " "):
		return ReasonChars
	}
	return ""
}

// classReason is "" when class is an authentication failure class: one of
// the names in 20.2.8 (a "*" there is one or more of a class's characters),
// and never an authorization one. An authorization class is recognised by
// "authz", "forbidden" or "403" anywhere in it, before the allow list is
// read, so "authz.token_expired" is refused although "*_expired" would admit
// it.
func classReason(class string) Reason {
	for _, bad := range []string{"authz", "forbidden", "403"} {
		if strings.Contains(class, bad) {
			return ReasonAuthzClass
		}
	}
	switch class {
	case "cred.registry", "cred.kube_context", "mcp.auth", "mcp.token_expired":
		return ""
	}
	wild := func(prefix, suffix string) bool {
		return strings.HasPrefix(class, prefix) && strings.HasSuffix(class, suffix) && len(class) > len(prefix)+len(suffix)
	}
	if wild("cred.", "_missing") || wild("", "_profile_missing") || wild("", "_expired") || wild("", "_sso_needs_human") {
		return ""
	}
	return ReasonClassNotAllowed
}

// clean says a string holds no control character and no bidirectional
// override: nothing that can forge a line or reorder text in the flag a user
// reads (OWASP: reject, do not sanitise).
func clean(s string) bool {
	for _, r := range s {
		if unicode.IsControl(r) || r >= 0x202A && r <= 0x202E || r >= 0x2066 && r <= 0x2069 {
			return false
		}
	}
	return true
}

// vetID is the id charset of 20.2.8: ^[a-z0-9][a-z0-9._-]{0,47}$.
func vetID(s string) Reason {
	if len(s) > MaxIDBytes {
		return ReasonID
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if !('a' <= c && c <= 'z' || '0' <= c && c <= '9' || i > 0 && (c == '.' || c == '_' || c == '-')) {
			return ReasonID
		}
	}
	return ""
}

// vetWord refuses whitespace: argv words, languages and the like are single
// tokens.
func vetWord(s string) Reason {
	if strings.IndexFunc(s, unicode.IsSpace) >= 0 {
		return ReasonChars
	}
	return ""
}

// vetFlag is a flag name: a dash and at least one more character, no space.
func vetFlag(s string) Reason {
	if len(s) < 2 || s[0] != '-' || vetWord(s) != "" {
		return ReasonShape
	}
	return ""
}

// vetPath is a project-relative glob or directory: not rooted, and no ".."
// segment, so no spec path leaves the project.
func vetPath(s string) Reason {
	if strings.HasPrefix(s, "/") || slices.Contains(strings.Split(s, "/"), "..") {
		return ReasonShape
	}
	return ""
}

// vetSQL is a statement keyword: letters only.
func vetSQL(s string) Reason {
	for i := 0; i < len(s); i++ {
		if !('a' <= s[i] && s[i] <= 'z' || 'A' <= s[i] && s[i] <= 'Z') {
			return ReasonShape
		}
	}
	return ""
}

// vetEvidence is "kind:<gate kind>" or a gate id.
func vetEvidence(s string) Reason {
	if k, ok := strings.CutPrefix(s, "kind:"); ok {
		if !slices.Contains(GateKinds, k) {
			return ReasonEnum
		}
		return ""
	}
	return vetID(s)
}

// vetClass is a failure class name: lower case, digits, "." and "_", at most
// 64 bytes, starting with a letter or digit.
func vetClass(s string) Reason {
	if len(s) > 64 {
		return ReasonChars
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if !('a' <= c && c <= 'z' || '0' <= c && c <= '9' || i > 0 && (c == '.' || c == '_')) {
			return ReasonChars
		}
	}
	return ""
}

// vetFactValue is STEER's FactValue charset, [A-Za-z0-9._:/@+=-], without a
// URL: a value names a mechanism, and "://" would carry a host and perhaps a
// credential. STEER's own check (entropy, deny list, per-kind shape) is the
// second line, at render time.
func vetFactValue(s string) Reason {
	for i := 0; i < len(s); i++ {
		c := s[i]
		if !('a' <= c && c <= 'z' || 'A' <= c && c <= 'Z' || '0' <= c && c <= '9' || strings.IndexByte("._:/@+=-", c) >= 0) {
			return ReasonChars
		}
	}
	if strings.Contains(s, "://") {
		return ReasonShape
	}
	return ""
}
