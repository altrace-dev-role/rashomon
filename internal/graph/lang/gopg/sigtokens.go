package gopg

import (
	"go/ast"
	"go/scanner"
	"go/token"
	"slices"
	"strings"
)

// sigSpelling is a function's type spelled by its tokens, the overlay's sig
// attr: "func", then each token of the source from the type parameters'
// opening bracket (the parameters' when there are none) to the type's end,
// joined by one space. In a declaration the receiver and the name sit between
// the func keyword and that bracket, so neither is part of the spelling, as
// neither is part of the type go/printer printed before it.
//
// A change gofmt alone makes leaves the spelling as it was:
//   - whitespace and line breaks are not tokens, and comments are not scanned;
//   - every semicolon, written or inserted by the scanner at a line end, is
//     spelled ";", and a separator (a comma or a semicolon) directly before a
//     closing bracket or the end is dropped, as a multi-line list's trailing
//     comma is, so a field separator gofmt removes at a line end changes
//     nothing and struct{ T; U } still differs from struct{ T U };
//   - the parentheses gofmt removes are dropped: those around a single
//     unnamed result of any function type in the signature, and the outer
//     pair of a doubled pair. They are found by the syntax tree's positions
//     (gofmtParens), not by token shape.
//
// One rewrite stays visible: a number literal's prefix or exponent case in an
// array length.
//
// The spelling is coarser than go/printer's, which kept a multi-line list's
// line breaks (go/printer's parameters reads each field's line from the
// FileSet): a list reflowed onto one line keeps its spelling here and changed
// it there. Every pair go/printer printed equal spells equal here
// (sigtokens_test.go). It links nothing beyond the scanner the parser
// already uses, and returns "" when the type has no opening bracket.
func sigSpelling(fset *token.FileSet, content []byte, t *ast.FuncType) string {
	if t == nil || t.Params == nil {
		return ""
	}
	open := t.Params.Opening
	if t.TypeParams != nil {
		open = t.TypeParams.Opening
	}
	tf := fset.File(open)
	if tf == nil || !open.IsValid() {
		return ""
	}
	start, end := tf.Offset(open), tf.Offset(t.End())
	if start >= end || end > len(content) {
		return ""
	}
	skip := gofmtParens(tf, t)
	src := content[start:end]
	var s scanner.Scanner
	sf := token.NewFileSet().AddFile("", -1, len(src))
	s.Init(sf, src, nil, 0)
	var b strings.Builder
	b.WriteString("func")
	pending := ""
	for {
		pos, tok, lit := s.Scan()
		if tok == token.EOF {
			break
		}
		// A separator before a closing bracket is dropped before the skip:
		// a dropped closing paren still closes the list, as in (int,).
		if (pending == "," || pending == ";") && (tok == token.RPAREN || tok == token.RBRACK || tok == token.RBRACE) {
			pending = ""
		}
		if slices.Contains(skip, start+sf.Offset(pos)) {
			continue
		}
		if pending != "" {
			b.WriteByte(' ')
			b.WriteString(pending)
		}
		switch {
		case tok == token.SEMICOLON:
			pending = ";"
		case lit != "":
			pending = lit
		default:
			pending = tok.String()
		}
	}
	if pending != "" && pending != "," && pending != ";" {
		b.WriteByte(' ')
		b.WriteString(pending)
	}
	return b.String()
}

// gofmtParens is the offsets of the parentheses gofmt removes from a
// signature, as go/printer does: those around a single unnamed result of any
// function type in it (printer's signature and stripParensAlways), and the
// outer pair of a parenthesised expression whose operand is parenthesised
// again (printer's expr1).
func gofmtParens(tf *token.File, t *ast.FuncType) []int {
	var skip []int
	pair := func(lp, rp token.Pos) {
		skip = append(skip, tf.Offset(lp), tf.Offset(rp))
	}
	ast.Inspect(t, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.FuncType:
			r := n.Results
			if r == nil || len(r.List) != 1 || len(r.List[0].Names) != 0 || !r.Opening.IsValid() {
				return true
			}
			pair(r.Opening, r.Closing)
			for p, ok := r.List[0].Type.(*ast.ParenExpr); ok; p, ok = p.X.(*ast.ParenExpr) {
				pair(p.Lparen, p.Rparen)
			}
		case *ast.ParenExpr:
			if _, ok := n.X.(*ast.ParenExpr); ok {
				pair(n.Lparen, n.Rparen)
			}
		}
		return true
	})
	return skip
}
