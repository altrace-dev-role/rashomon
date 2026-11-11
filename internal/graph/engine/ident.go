package engine

import (
	"strconv"
	"strings"

	"github.com/altrace-dev-role/rashomon/internal/graph"
)

// Identifier names (W-M1.13, R0): the form test for a prompt word in
// identifier form and its resolution against the declared names, with
// the kind-word preference. Split from names.go at LAW10's file ceiling.

// identForm is a word in identifier form. An underscore inside it or an
// upper-case letter after a lower-case one (platformRow, EvaluateStop) is
// the form itself; a word in code font is one whatever its case; a word
// in capitals alone (RSS, JSON, CI) is an acronym, not a name; a
// capitalised word off the sentence's start (Impact) is a name only
// after a kind word ("the function Impact"): alone it is as often a
// proper noun or an emphasis as a symbol, and a claim needs more.
func identForm(w string, at wordAt) bool {
	if !identChars(w) || len(w) < 2 {
		return false
	}
	if at.quoted {
		return true
	}
	lower := false
	for i := 0; i < len(w); i++ {
		c := w[i]
		switch {
		case c == '_':
			if i > 0 && i+1 < len(w) {
				return true
			}
		case c >= 'a' && c <= 'z':
			lower = true
		case c >= 'A' && c <= 'Z':
			if lower {
				return true
			}
		}
	}
	if at.initial || !lower || w[0] < 'A' || w[0] > 'Z' {
		return false
	}
	return kindWords.get()[at.prev]
}

// kindWords say what kind of thing the next word names.
var kindWords = &lazySet[bool]{f: func() map[string]bool {
	return map[string]bool{
		"function": true, "func": true, "method": true, "type": true, "struct": true, "interface": true, "const": true,
		"constant": true, "var": true, "variable": true, "field": true, "class": true, "helper": true, "handler": true,
		"symbol": true, "routine": true, "def": true, "enum": true, "trait": true, "property": true, "attribute": true,
		"member": true, "identifier": true, "command": true, "subcommand": true, "flag": true, "option": true,
	}
}}

func identChars(w string) bool {
	for i := 0; i < len(w); i++ {
		c := w[i]
		if !(c == '_' || c >= '0' && c <= '9' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z') {
			return false
		}
	}
	return w != ""
}

// resolveIdent is the word as a declared name: its one unit, or among
// several the one whose symbol's path the rest of the prompt's words name
// most (the identifier's own tokens excluded, so the symbol's name is no
// evidence for its unit), else the one of the kind the word before it
// says ("the function Impact" is the func, not the type), else disclosed
// and unclaimed.
func (c corpus) resolveIdent(w string, at wordAt, toks []string, weights map[string]float64) (NamedWord, bool) {
	syms := c.syms[w]
	if len(syms) == 0 {
		return NamedWord{}, false
	}
	nw := NamedWord{Word: w, Kind: NamedIdent}
	rest := except(toks, Tokens(w))
	best := -1.0
	var tied []graph.ID
	units := map[string]bool{}
	for _, id := range syms {
		units[c.units[c.index[id]]] = true
		switch s := pathHits(rest, c.fields[c.index[id]], weights); {
		case s > best:
			best, tied = s, []graph.ID{id}
		case s == best:
			tied = append(tied, id)
		}
	}
	if len(tied) > 1 && len(units) > 1 {
		tied = ofKind(tied, at.prev)
	}
	if len(tied) == 1 {
		nw.Node, nw.Unit = tied[0], c.units[c.index[tied[0]]]
		return nw, true
	}
	nw.Why = strconv.Itoa(len(units)) + " units declare it"
	return nw, true
}

// ofKind is the tied symbols of the kind a kind word names: the callable
// ones after function, method and their like; the type ones after type,
// struct, class and their like; all of them after any other word.
func ofKind(ids []graph.ID, kindWord string) []graph.ID {
	want := map[string]bool{}
	switch {
	case callKinds.get()[kindWord]:
		want = map[string]bool{"func": true, "method": true}
	case typeKinds.get()[kindWord]:
		want = map[string]bool{"type": true, "class": true, "struct": true, "interface": true, "enum": true, "trait": true}
	default:
		return ids
	}
	var out []graph.ID
	for _, id := range ids {
		if parts := strings.Split(string(id), ":"); len(parts) > 3 && want[parts[3]] {
			out = append(out, id)
		}
	}
	if len(out) == 0 {
		return ids
	}
	return out
}

var callKinds = &lazySet[bool]{f: func() map[string]bool {
	return map[string]bool{"function": true, "func": true, "method": true, "helper": true, "handler": true, "routine": true, "def": true, "command": true, "subcommand": true}
}}

var typeKinds = &lazySet[bool]{f: func() map[string]bool {
	return map[string]bool{"type": true, "struct": true, "interface": true, "class": true, "enum": true, "trait": true}
}}
