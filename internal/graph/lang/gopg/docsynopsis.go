package gopg

import (
	"go/doc/comment"
	"strings"
	"unicode"
)

// docSynopsis returns what new(doc.Package).Synopsis(text) returns, without
// importing go/doc. go/doc's package variables allocate at init (496 B on
// darwin/arm64 with go1.27.4: its lazyregexp patterns and an ast.Ident), and
// every process that links gopg pays that before main, against RES-2's exact
// darwin budget; go/doc/comment does no init work. The body mirrors go1.27.4's
// src/go/doc/synopsis.go (Package.Synopsis, firstSentence, IllegalPrefixes)
// and doc.go (the Parser and Printer of an empty Package: it knows no symbol,
// and the only package name it resolves itself is its own, ""; any other name
// falls to go/doc/comment's standard library lookup). TestDocSynopsis_EqualsGoDoc
// holds the two equal.
func docSynopsis(text string) string {
	text = firstSentence(text)
	lower := strings.ToLower(text)
	if strings.HasPrefix(lower, "copyright") || strings.HasPrefix(lower, "all rights") || strings.HasPrefix(lower, "author") {
		return ""
	}
	p := comment.Parser{
		LookupPackage: func(name string) (string, bool) { return "", name == "" },
		LookupSym:     func(string, string) bool { return false },
	}
	d := p.Parse(text)
	if len(d.Content) == 0 {
		return ""
	}
	if _, ok := d.Content[0].(*comment.Paragraph); !ok {
		return ""
	}
	d.Content = d.Content[:1] // the first sentence may hold blank lines or a code block after it
	pr := comment.Printer{TextWidth: -1}
	return strings.TrimSpace(string(pr.Text(d)))
}

// firstSentence is go/doc's: s up to the first period that is followed by a
// space (a newline, carriage return or tab counts as one) and does not end a
// single capital letter, or up to the first ideographic or fullwidth full
// stop; all of s when there is neither.
func firstSentence(s string) string {
	var ppp, pp, p rune
	for i, q := range s {
		if q == '\n' || q == '\r' || q == '\t' {
			q = ' '
		}
		if q == ' ' && p == '.' && (!unicode.IsUpper(pp) || unicode.IsUpper(ppp)) {
			return s[:i]
		}
		if p == '。' || p == '．' {
			return s[:i]
		}
		ppp, pp, p = pp, p, q
	}
	return s
}
