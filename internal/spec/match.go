package spec

import (
	"slices"
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
	segs, ok := segments(rel)
	if !ok {
		return false
	}
	anchored := strings.HasPrefix(pattern, "/")
	pat, ok := segments(strings.TrimSuffix(strings.TrimPrefix(pattern, "/"), "/"))
	if !ok {
		return false
	}
	if !anchored && len(pat) == 1 {
		return slices.ContainsFunc(segs, func(s string) bool { return matchSegment(pat[0], s) })
	}
	for n := 1; n <= len(segs); n++ {
		if matchSegments(pat, segs[:n]) {
			return true
		}
	}
	return false
}

// Covers reports whether one of the rule's Paths covers path and none of its
// ExcludePaths does; a rule with no Paths covers no path.
func (r Rule) Covers(path string) bool {
	match := func(p string) bool { return MatchPath(p, path) }
	return slices.ContainsFunc(r.Paths, match) && !slices.ContainsFunc(r.ExcludePaths, match)
}

// segments splits a relative path into its segments, refusing an empty or
// absolute one and one with an empty, "." or ".." segment.
func segments(p string) ([]string, bool) {
	if p == "" || strings.HasPrefix(p, "/") {
		return nil, false
	}
	s := strings.Split(p, "/")
	for _, seg := range s {
		if seg == "" || seg == "." || seg == ".." {
			return nil, false
		}
	}
	return s, true
}

// matchSegments matches whole segments, `**` taking zero or more of them.
func matchSegments(pat, segs []string) bool {
	for len(pat) > 0 {
		if pat[0] != "**" {
			if len(segs) == 0 || !matchSegment(pat[0], segs[0]) {
				return false
			}
			pat, segs = pat[1:], segs[1:]
			continue
		}
		for len(pat) > 0 && pat[0] == "**" {
			pat = pat[1:]
		}
		if len(pat) == 0 {
			return true
		}
		for i := range segs {
			if matchSegments(pat, segs[i:]) {
				return true
			}
		}
		return false
	}
	return len(segs) == 0
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
