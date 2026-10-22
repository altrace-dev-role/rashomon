package acceptance

import (
	"go/token"
	"testing"
)

// TestRubricLAW10_TwoInitFunctionsBothCount holds functionSizes' own keying
// to the fact that Go allows more than one init() per file: the bare name
// "init" used as the whole key let a second one silently overwrite the
// first's measurement, so an over-limit init could hide behind an earlier,
// smaller one.
func TestRubricLAW10_TwoInitFunctionsBothCount(t *testing.T) {
	src := []byte(`package p

func init() {
	_ = 1
}

func init() {
	_ = 1
	_ = 2
	_ = 3
}
`)
	fset := token.NewFileSet()
	funcs, _, err := measureFile(fset, "p.go", src)
	if err != nil {
		t.Fatal(err)
	}
	if len(funcs) != 2 {
		t.Fatalf("funcs = %v; want two distinct init() measurements, not one overwriting the other", funcs)
	}
	var sizes []int
	for _, n := range funcs {
		sizes = append(sizes, n)
	}
	if !(sizes[0] == 1 && sizes[1] == 3 || sizes[0] == 3 && sizes[1] == 1) {
		t.Errorf("sizes = %v; want 1 and 3", sizes)
	}
}

// TestRubricLAW10_AGenericMethodWithTwoTypeParametersKeysOnItsOwn holds
// funcName to a receiver with two or more type parameters (*ast.
// IndexListExpr, one type parameter alone parses as *ast.IndexExpr): it
// used to key as the bare method name, colliding with a same-named method
// on a different, single-parameter generic type in the same file.
func TestRubricLAW10_AGenericMethodWithTwoTypeParametersKeysOnItsOwn(t *testing.T) {
	src := []byte(`package p

type One[K any] struct{}
func (o One[K]) Get() {
	_ = 1
}

type Two[K, V any] struct{}
func (t Two[K, V]) Get() {
	_ = 1
	_ = 2
}
`)
	fset := token.NewFileSet()
	funcs, _, err := measureFile(fset, "p.go", src)
	if err != nil {
		t.Fatal(err)
	}
	if n, ok := funcs["p.go:One.Get"]; !ok || n != 1 {
		t.Errorf("funcs[p.go:One.Get] = %v, %v; want 1, true", n, ok)
	}
	if n, ok := funcs["p.go:Two.Get"]; !ok || n != 2 {
		t.Errorf("funcs[p.go:Two.Get] = %v, %v; want 2, true", n, ok)
	}
}

// TestRubricLAW10_ABlockCommentIsNotCode holds codeLines to its own doc: a
// line inside a /* */ block comment, however many lines it spans, is not
// code, the same rule it already applies to a // line comment.
func TestRubricLAW10_ABlockCommentIsNotCode(t *testing.T) {
	src := []byte(`package p

func f() {
	/*
	   this whole block is a comment
	   spanning several lines
	*/
	_ = 1 // trailing comment, the line still counts
}
`)
	fset := token.NewFileSet()
	funcs, _, err := measureFile(fset, "p.go", src)
	if err != nil {
		t.Fatal(err)
	}
	if n := funcs["p.go:f"]; n != 1 {
		t.Errorf("funcs[p.go:f] = %d; want 1 (only the _ = 1 line is code)", n)
	}
}
