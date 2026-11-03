package verify

import (
	"reflect"
	"slices"
	"strings"
	"testing"
)

// alpha1Keys are the keys v1.2.0-alpha.1 (949d6aca) names in the project
// index, at the top and in an entry. Its ReadIndex decodes with
// DisallowUnknownFields, and its Stop writes a fresh index over one it cannot
// read, so any other key this build writes costs a person who goes back to
// alpha.1 the project's learned gates.
var alpha1Keys = map[string][]string{
	"ProjectIndex": {"entries", "project_key"},
	"IndexEntry":   {"family", "first_seen_unix_ms", "kind", "last_seen_unix_ms", "last_session", "sessions"},
}

// TestIPF1b_TheIndexNamesOnlyAlpha1Keys (INDEX-POINTER-FILE1, R29): the
// index's two types encode alpha.1's keys and no other, omitempty or not, so
// no value of them can reach the file with a key alpha.1 refuses. Break: give
// ProjectIndex a key alpha.1 does not name.
func TestIPF1b_TheIndexNamesOnlyAlpha1Keys(t *testing.T) {
	for _, typ := range []reflect.Type{reflect.TypeOf(ProjectIndex{}), reflect.TypeOf(IndexEntry{})} {
		var keys []string
		for i := range typ.NumField() {
			name, _, _ := strings.Cut(typ.Field(i).Tag.Get("json"), ",")
			keys = append(keys, name)
		}
		slices.Sort(keys)
		if want := alpha1Keys[typ.Name()]; !slices.Equal(keys, want) {
			t.Errorf("%s encodes %q; alpha.1 reads %q and rebuilds the index on any other", typ.Name(), keys, want)
		}
	}
}
