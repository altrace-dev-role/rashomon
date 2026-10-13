package acceptance

import (
	"strconv"
	"strings"
)

// reasonConstraintPaths are reason-named keys that constrain a compared
// vocabulary without declaring one: the coverage record's if/then that makes
// reason non-null beside state unverified. Each may hold a type and nothing
// else. Unlike reasonVocabularyPaths, a path here need not exist; one that
// does and declares values (enum, const, $ref, items, a combinator) is
// reported unchecked like any other new reason key.
var reasonConstraintPaths = map[string]bool{
	"/$defs/coverage/allOf/0/then/properties/reason": true,
}

// typeOnly reports whether a schema node constrains nothing but type, and so
// publishes no vocabulary of its own.
func typeOnly(node any) bool {
	m, ok := node.(map[string]any)
	if !ok {
		return false
	}
	for k := range m {
		if k != "type" && k != "description" {
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
