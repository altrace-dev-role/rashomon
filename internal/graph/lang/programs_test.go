package lang

import "testing"

// TestProgramKind_TheRegisteredSeventeen (W-M1.46): the set is the
// seventeen kinds the registration names, no more and no fewer. The table's
// length and each member together pin it as a set; a data file, a kind a
// Resolver reads, a document and no extension at all are not program kinds.
func TestProgramKind_TheRegisteredSeventeen(t *testing.T) {
	want := []string{".sh", ".bash", ".zsh", ".fish", ".ps1", ".psm1", ".bat", ".cmd", ".py", ".rb", ".pl",
		".exs", ".js", ".mjs", ".cjs", ".ts", ".lua"}
	if len(want) != 17 {
		t.Fatalf("want lists %d kinds; the registration names 17", len(want))
	}
	if len(programKinds) != len(want) {
		t.Fatalf("table holds %d kinds, want %d", len(programKinds), len(want))
	}
	for _, k := range want {
		if !ProgramKind(k) {
			t.Errorf("ProgramKind(%q) = false, want a program kind", k)
		}
	}
	for _, k := range []string{".json", ".go", ".md", ""} {
		if ProgramKind(k) {
			t.Errorf("ProgramKind(%q) = true, want no program kind", k)
		}
	}
}
