package render

// fill and fillInto, the placeholder pass every template goes through, kept
// apart from render.go so each file stays inside Law 10's 500 code lines.

import (
	"fmt"
	"slices"
	"strings"
)

// fill replaces each {name} in format with the named slot's text. Every
// placeholder must name a bound slot, every bound slot must be used, and a
// brace outside a placeholder is refused. Slot text is written, never
// scanned again, so a value cannot add a placeholder.
func fill(format string, slots []Slot) (string, error) {
	var b strings.Builder
	used := make(map[string]bool, len(slots))
	if _, err := fillInto(&b, format, slots, used); err != nil {
		return "", err
	}
	if len(used) != len(slots) {
		return "", fmt.Errorf("%w: a bound slot has no placeholder", ErrTemplate)
	}
	return b.String(), nil
}

// fillInto writes format to b with each {name} replaced by the named slot's
// text, marks each name used and returns the names in the order format holds
// them: fill's pass, and fillLines' for one line.
func fillInto(b *strings.Builder, format string, slots []Slot, used map[string]bool) ([]string, error) {
	var names []string
	for i := 0; i < len(format); {
		switch c := format[i]; c {
		case '{':
			end := strings.IndexAny(format[i+1:], "{}")
			if end < 0 || format[i+1+end] != '}' {
				return nil, fmt.Errorf("%w: an unclosed placeholder", ErrTemplate)
			}
			name := format[i+1 : i+1+end]
			at := slices.IndexFunc(slots, func(s Slot) bool { return s.Name == name })
			if at < 0 {
				return nil, fmt.Errorf("%w: placeholder %q has no slot", ErrTemplate, name)
			}
			b.WriteString(slots[at].Value.Text())
			used[name] = true
			names = append(names, name)
			i += end + 2
		case '}':
			return nil, fmt.Errorf("%w: a stray '}'", ErrTemplate)
		default:
			b.WriteByte(c)
			i++
		}
	}
	return names, nil
}
