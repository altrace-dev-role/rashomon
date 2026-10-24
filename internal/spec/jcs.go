package spec

import (
	"slices"
	"strconv"
	"unicode/utf16"
	"unicode/utf8"
)

// RFC 8785 (JSON Canonicalization Scheme) over a document tree, standard
// library only. It is not internal/shape's canonical form, which reproduces
// encoding/json: that one escapes <, > and & and sorts keys by byte, and
// this one does neither (RFC 8785 sections 3.2.2.2 and 3.2.3).
//
// Integers are the only numbers the tree holds, and they are within 2^53-1,
// so the ECMAScript number form is their decimal digits. Complexity: O(n log
// k) in the n bytes of the document and the k keys of its widest object.

// appendJCS appends the canonical form of n to dst: no whitespace, object
// keys in UTF-16 code unit order, strings in the RFC's escaping.
func appendJCS(dst []byte, n node) []byte {
	switch n.kind {
	case kObject:
		dst = append(dst, '{')
		for i, j := range sortedKeys(n.keys) {
			if i > 0 {
				dst = append(dst, ',')
			}
			dst = appendJCSString(dst, n.keys[j])
			dst = append(dst, ':')
			dst = appendJCS(dst, n.vals[j])
		}
		return append(dst, '}')
	case kArray:
		dst = append(dst, '[')
		for i, v := range n.vals {
			if i > 0 {
				dst = append(dst, ',')
			}
			dst = appendJCS(dst, v)
		}
		return append(dst, ']')
	case kString:
		return appendJCSString(dst, n.str)
	case kInt:
		return strconv.AppendInt(dst, n.num, 10)
	case kBool:
		return strconv.AppendBool(dst, n.flag)
	}
	return append(dst, "null"...)
}

// sortedKeys returns the indexes of keys ordered by UTF-16 code units, the
// order RFC 8785 fixes. A code point order differs for runes above U+FFFF,
// whose surrogates (D800 to DFFF) sort below U+E000 to U+FFFF.
func sortedKeys(keys []string) []int {
	units := make([][]uint16, len(keys))
	idx := make([]int, len(keys))
	for i, k := range keys {
		units[i] = utf16.Encode([]rune(k))
		idx[i] = i
	}
	slices.SortFunc(idx, func(a, b int) int { return slices.Compare(units[a], units[b]) })
	return idx
}

// appendJCSString appends s quoted. Only the quote, the backslash and the C0
// controls are escaped (the short forms where JSON has one, \u00xx in lower
// case otherwise); every other rune, DEL and U+2028 and the HTML characters
// included, is written as itself.
func appendJCSString(dst []byte, s string) []byte {
	const hexDigits = "0123456789abcdef"
	dst = append(dst, '"')
	for i := 0; i < len(s); {
		c := s[i]
		switch {
		case c == '"' || c == '\\':
			dst = append(dst, '\\', c)
		case c == '\b':
			dst = append(dst, '\\', 'b')
		case c == '\f':
			dst = append(dst, '\\', 'f')
		case c == '\n':
			dst = append(dst, '\\', 'n')
		case c == '\r':
			dst = append(dst, '\\', 'r')
		case c == '\t':
			dst = append(dst, '\\', 't')
		case c < 0x20:
			dst = append(dst, '\\', 'u', '0', '0', hexDigits[c>>4], hexDigits[c&0xf])
		case c < utf8.RuneSelf:
			dst = append(dst, c)
		default:
			_, size := utf8.DecodeRuneInString(s[i:])
			dst = append(dst, s[i:i+size]...)
			i += size
			continue
		}
		i++
	}
	return append(dst, '"')
}
