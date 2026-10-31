package spec

import (
	"strings"
	"unicode/utf8"
)

// MatchPath reports whether pattern covers rel, a project-relative,
// slash-separated, cleaned path: the one matcher for Rule.Paths,
// Rule.ExcludePaths, Gate.AppliesTo and Rule.InvalidatedBy.
//
//   - `*` matches any run within one segment, `?` one character, and a
//     segment that is `**` zero or more whole segments; `**` inside a
//     segment is two stars. There are no character classes and no negation:
//     `[`, `]` and `!` are plain characters. Matching is case-sensitive.
//   - A pattern matches a path when it matches the path or one of its parent
//     directories, gitignore's rule: a pattern that names a directory covers
//     what is under it.
//   - A pattern with no slash, or whose only slash is trailing, matches any
//     one segment: the basename or any parent directory ("docs" and "docs/"
//     cover x/docs/b.md, "*.md" covers it by its basename).
//   - A leading slash, or a slash inside the pattern, anchors it to the
//     project root ("internal/*.go" does not cover x/internal/a.go).
//
// An empty pattern, a pattern with an empty, "." or ".." segment, and a rel
// that is empty, absolute or not cleaned match nothing.
func MatchPath(pattern, rel string) bool {
	if !cleanSegments(rel) {
		return false
	}
	anchored := strings.HasPrefix(pattern, "/")
	pat := strings.TrimSuffix(strings.TrimPrefix(pattern, "/"), "/")
	if !cleanSegments(pat) {
		return false
	}
	if !anchored && !strings.Contains(pat, "/") {
		for rest, more := rel, true; more; {
			var seg string
			seg, rest, more = strings.Cut(rest, "/")
			if matchSegment(pat, seg) {
				return true
			}
		}
		return false
	}
	// Each prefix of rel that ends at a segment boundary, shortest first.
	for end := 0; end < len(rel); {
		if i := strings.IndexByte(rel[end:], '/'); i >= 0 {
			end += i
		} else {
			end = len(rel)
		}
		if matchSegments(pat, rel[:end]) {
			return true
		}
		end++
	}
	return false
}

// Covers reports whether one of the rule's Paths covers path and none of its
// ExcludePaths does; a rule with no Paths covers no path.
func (r Rule) Covers(path string) bool {
	for _, p := range r.ExcludePaths {
		if MatchPath(p, path) {
			return false
		}
	}
	for _, p := range r.Paths {
		if MatchPath(p, path) {
			return true
		}
	}
	return false
}

// cleanSegments reports whether p is a non-empty, relative path whose every
// segment is non-empty and neither "." nor ".."; it walks p with
// strings.Cut and allocates nothing (card SPEC-RULES1, D210).
func cleanSegments(p string) bool {
	if p == "" || strings.HasPrefix(p, "/") {
		return false
	}
	for rest, more := p, true; more; {
		var seg string
		seg, rest, more = strings.Cut(rest, "/")
		if seg == "" || seg == "." || seg == ".." {
			return false
		}
	}
	return true
}

// matchSegments matches whole segments, `**` taking zero or more of them. Both
// arguments are slash-joined clean segment strings, walked with strings.Cut.
func matchSegments(pat, segs string) bool {
	for pat != "" {
		head, rest, _ := strings.Cut(pat, "/")
		if head != "**" {
			if segs == "" {
				return false
			}
			seg, segRest, _ := strings.Cut(segs, "/")
			if !matchSegment(head, seg) {
				return false
			}
			pat, segs = rest, segRest
			continue
		}
		pat = rest
		for pat != "" {
			h, r, _ := strings.Cut(pat, "/")
			if h != "**" {
				break
			}
			pat = r
		}
		if pat == "" {
			return true
		}
		for s := segs; s != ""; {
			if matchSegments(pat, s) {
				return true
			}
			_, s, _ = strings.Cut(s, "/")
		}
		return false
	}
	return segs == ""
}

// matchSegment matches one segment against one pattern segment: `*` any run
// of characters, `?` exactly one, every other character itself. It is the
// linear wildcard match, backtracking only to the last star.
func matchSegment(pat, seg string) bool {
	p, s := 0, 0
	star, mark := -1, 0
	for s < len(seg) {
		r, n := utf8.DecodeRuneInString(seg[s:])
		switch {
		case p < len(pat) && pat[p] == '*':
			star, mark = p, s
			p++
		case p < len(pat) && pat[p] == '?':
			p, s = p+1, s+n
		case p < len(pat) && strings.HasPrefix(pat[p:], string(r)):
			p, s = p+len(string(r)), s+n
		case star >= 0:
			_, m := utf8.DecodeRuneInString(seg[mark:])
			mark += m
			p, s = star+1, mark
		default:
			return false
		}
	}
	for p < len(pat) && pat[p] == '*' {
		p++
	}
	return p == len(pat)
}
