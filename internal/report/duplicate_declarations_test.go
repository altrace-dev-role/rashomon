package report

import (
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/store"
)

// TestHasDuplicateToolUseID is the rule report.build and the turn digest
// both apply for ReasonDuplicateDeclarations, tested once where it lives so
// neither caller's test is the only thing standing behind it.
//
// The empty-id case is the one a simpler tally gets wrong: a declaration
// that carries no tool_use_id is not evidence that it shares an identity
// with another that carries none, only that neither carries one.
func TestHasDuplicateToolUseID(t *testing.T) {
	d := func(id string) store.Declaration { return store.Declaration{ToolUseID: id, ToolName: "Bash"} }
	for _, tc := range []struct {
		name  string
		decls []store.Declaration
		want  bool
	}{
		{"none", nil, false},
		{"distinct", []store.Declaration{d("a"), d("b")}, false},
		{"two without an id", []store.Declaration{d(""), d("")}, false},
		{"one id twice", []store.Declaration{d("a"), d("b"), d("a")}, true},
	} {
		if got := HasDuplicateToolUseID(tc.decls); got != tc.want {
			t.Errorf("%s: HasDuplicateToolUseID = %v, want %v", tc.name, got, tc.want)
		}
	}
}
