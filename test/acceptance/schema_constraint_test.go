package acceptance

import (
	"strconv"
	"strings"
)

// reasonConstraintPaths are reason-named keys that constrain a compared
// vocabulary without declaring one: the coverage record's if/then/else, which
// makes reason non-null beside state unverified and null beside any other
// state. Each may hold a type, or pin the value to null, and nothing else.
// Unlike reasonVocabularyPaths, a path here need not exist; one that does and
// declares values (enum, a non-null const, $ref, items, a combinator) is
// reported unchecked like any other new reason key.
var reasonConstraintPaths = map[string]bool{
	"/$defs/coverage/allOf/0/then/properties/reason": true,
	"/$defs/coverage/allOf/0/else/properties/reason": true,
}

// declaresNoReason reports whether a schema node constrains nothing but type,
// or pins the value to null, and so publishes no vocabulary of its own: null
// is the absence of a reason, not one of the codes.
func declaresNoReason(node any) bool {
	m, ok := node.(map[string]any)
	if !ok {
		return false
	}
	for k, v := range m {
		switch {
		case k == "type", k == "description":
		case k == "const" && v == nil:
		default:
			return false
		}
	}
	return true
}

// pointerNode resolves a JSON pointer built by collectReasonPaths.
func pointerNode(root any, pointer string) any {
	node := root
	for _, raw := range strings.Split(strings.TrimPrefix(pointer, "/"), "/") {
		key := strings.NewReplacer("~1", "/", "~0", "~").Replace(raw)
		switch v := node.(type) {
		case map[string]any:
			node = v[key]
		case []any:
			i, err := strconv.Atoi(key)
			if err != nil || i < 0 || i >= len(v) {
				return nil
			}
			node = v[i]
		default:
			return nil
		}
	}
	return node
}
