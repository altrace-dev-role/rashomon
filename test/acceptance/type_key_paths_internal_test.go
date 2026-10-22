package acceptance

import (
	"reflect"
	"testing"
)

type typeKeyPathsLeaf struct {
	A string `json:"a"`
	B *int   `json:"b"`
}

type typeKeyPathsDoc struct {
	M      map[string]typeKeyPathsLeaf `json:"m"`
	S      []string                    `json:"s"`
	Hidden string                      `json:"-"`
	Named  int
	skip   string //lint:ignore U1000 unexported, never marshalled; the field under test
}

// TestRubricNAMING3_TypeKeyPathsWalkTheTypeNotAnInstance proves what docKeyPaths cannot:
// a map key renders as the wildcard "{}" rather than a key an instance
// happened to carry, a nested branch no instance needs to fill still shows
// up, an unexported or json:"-" field is excluded, a field with no tag uses
// its Go name, and every leaf carries its kind.
func TestRubricNAMING3_TypeKeyPathsWalkTheTypeNotAnInstance(t *testing.T) {
	got := string(typeKeyPaths(reflect.TypeOf(typeKeyPathsDoc{})))
	want := "Named: int\nm.{}.a: string\nm.{}.b: int\ns[]: string\n"
	if got != want {
		t.Errorf("typeKeyPaths = %q, want %q", got, want)
	}
}
