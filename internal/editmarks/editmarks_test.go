package editmarks

import (
	"encoding/json"
	"maps"
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/store"
)

// input is an Edit's tool_input as the harness sends it.
func input(t testing.TB, path, from, to string) json.RawMessage {
	t.Helper()
	b, err := json.Marshal(map[string]any{"file_path": path, "old_string": from, "new_string": to})
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// multi is a MultiEdit's tool_input: one old/new pair per element.
func multi(t testing.TB, path string, pairs ...[2]string) json.RawMessage {
	t.Helper()
	edits := make([]map[string]string, len(pairs))
	for i, p := range pairs {
		edits[i] = map[string]string{"old_string": p[0], "new_string": p[1]}
	}
	b, err := json.Marshal(map[string]any{"file_path": path, "edits": edits})
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func want(t *testing.T, got *store.EditMarks, added, removed map[string]int) {
	t.Helper()
	if added == nil && removed == nil {
		if got != nil {
			t.Fatalf("marks = %+v, want none", got)
		}
		return
	}
	if got == nil {
		t.Fatalf("no marks, want added %v removed %v", added, removed)
	}
	if !maps.Equal(got.Added, added) || !maps.Equal(got.Removed, removed) {
		t.Errorf("added %v removed %v, want added %v removed %v", got.Added, got.Removed, added, removed)
	}
	if got.Basis != Basis || got.Version != Version {
		t.Errorf("basis %q version %d, want %q %d", got.Basis, got.Version, Basis, Version)
	}
}

type ids = map[string]int

// Test removal: an edit that takes a test definition out is a removal of its
// id, in each language of the table.
func TestEM1_ADeletedTestIsARemovedTestDef(t *testing.T) {
	for _, c := range []struct {
		path, old, id string
	}{
		{"pkg/calc_test.go", "func TestAdd(t *testing.T) {\n\tt.Parallel()\n}\n", "go.test_def"},
		{"tests/test_calc.py", "def test_add():\n    pass\n", "py.test_def"},
		{"tests/test_calc.py", "class T:\n    async def test_add(self):\n        pass\n", "py.test_def"},
	} {
		want(t, Of("Edit", input(t, c.path, c.old, "")), nil, ids{c.id: 1})
	}
	// Renamed, not removed: the same definition on both sides is no move.
	want(t, Of("Edit", input(t, "pkg/calc_test.go", "func TestAdd(t *testing.T) {", "func TestAddTwo(t *testing.T) {")), nil, nil)
	// Not a test: TestMain, and a name a Go test cannot have.
	want(t, Of("Edit", input(t, "pkg/calc_test.go", "func TestMain(m *testing.M) {\nfunc Testing(t *testing.T) {\n", "")), nil, nil)
}

// Skip insertion.
func TestEM2_AnInsertedSkipIsAnAddedSkip(t *testing.T) {
	for _, c := range []struct {
		path, line, id string
	}{
		{"pkg/calc_test.go", "t.Skip(\"later\")", "go.t_skip"},
		{"pkg/calc_test.go", "t.Skipf(\"later %d\", 1)", "go.t_skip"},
		{"pkg/calc_test.go", "t.SkipNow()", "go.t_skip"},
		{"tests/test_calc.py", "@pytest.mark.skip(reason=\"later\")", "py.pytest_skip"},
		{"tests/test_calc.py", "@pytest.mark.skipif(sys.platform == 'win32', reason='x')", "py.pytest_skip"},
		{"tests/test_calc.py", "pytest.skip('later')", "py.pytest_skip"},
		{"tests/test_calc.py", "pytest.importorskip('numpy')", "py.pytest_skip"},
		{"tests/test_calc.py", "@unittest.skip('later')", "py.unittest_skip"},
		{"tests/test_calc.py", "@unittest.skipIf(True, 'x')", "py.unittest_skip"},
		{"tests/test_calc.py", "@unittest.skipUnless(False, 'x')", "py.unittest_skip"},
		{"tests/test_calc.py", "self.skipTest('later')", "py.unittest_skip"},
		{"tests/test_calc.py", "@pytest.mark.xfail", "py.xfail"},
		{"tests/test_calc.py", "pytest.xfail('later')", "py.xfail"},
	} {
		got := Of("Edit", input(t, c.path, "body()\n", "\t"+c.line+"\nbody()\n"))
		want(t, got, ids{c.id: 1}, nil)
	}
	// A longer word is not the mark: a skipped_thing is no skip.
	want(t, Of("Edit", input(t, "tests/test_calc.py", "body()\n", "@pytest.mark.skipped_thing\nbody()\n")), nil, nil)
	want(t, Of("Edit", input(t, "tests/test_calc.py", "body()\n", "@unittest.skipper\n@pytest.mark.xfailing\nbody()\n")), nil, nil)
}

// Assertion edits: removing one, and replacing it with a bare comparison.
func TestEM3_AnEditedAssertionIsACountedAssertion(t *testing.T) {
	for _, c := range []struct {
		path, line, id string
	}{
		{"pkg/calc_test.go", "t.Error(\"no\")", "go.assert"},
		{"pkg/calc_test.go", "t.Errorf(\"got %d\", got)", "go.assert"},
		{"pkg/calc_test.go", "t.Fatal(err)", "go.assert"},
		{"pkg/calc_test.go", "t.Fatalf(\"got %d\", got)", "go.assert"},
		{"pkg/calc_test.go", "assert.Equal(t, want, got)", "go.assert"},
		{"pkg/calc_test.go", "require.NoError(t, err)", "go.assert"},
		{"tests/test_calc.py", "assert add(1, 2) == 3", "py.assert"},
		{"tests/test_calc.py", "self.assertEqual(add(1, 2), 3)", "py.assert"},
	} {
		want(t, Of("Edit", input(t, c.path, "\t"+c.line+"\n", "")), nil, ids{c.id: 1})
		want(t, Of("Edit", input(t, c.path, "", "\t"+c.line+"\n")), ids{c.id: 1}, nil)
	}
	// A bare comparison is not an assertion (the ruling): removing it, or
	// replacing the assertion with it, leaves the one the assertion was.
	want(t, Of("Edit", input(t, "pkg/calc_test.go", "if got != want {\n\treturn\n}\n", "")), nil, nil)
	want(t, Of("Edit", input(t, "pkg/calc_test.go", "\tt.Errorf(\"got %d\", got)\n", "\tif got != want {\n\t\treturn\n\t}\n")), nil, ids{"go.assert": 1})
}

// A commented-out line is not present: an assertion an edit comments out is a
// removal, and a skip that sits in a comment is not a skip.
func TestEM4_ACommentedOutLineIsNotPresent(t *testing.T) {
	want(t, Of("Edit", input(t, "pkg/calc_test.go", "\tt.Errorf(\"got %d\", got)\n", "\t// t.Errorf(\"got %d\", got)\n")), nil, ids{"go.assert": 1})
	want(t, Of("Edit", input(t, "tests/test_calc.py", "    assert add(1, 2) == 3\n", "    # assert add(1, 2) == 3\n")), nil, ids{"py.assert": 1})
	want(t, Of("Edit", input(t, "pkg/calc_test.go", "body()\n", "\t// t.Skip(\"later\")\nbody()\n")), nil, nil)
	want(t, Of("Edit", input(t, "tests/test_calc.py", "body()\n", "# @pytest.mark.skip\nbody()\n")), nil, nil)
	// Uncommenting is the other way round.
	want(t, Of("Edit", input(t, "pkg/calc_test.go", "\t// t.Skip(\"later\")\n", "\tt.Skip(\"later\")\n")), ids{"go.t_skip": 1}, nil)
}

// MultiEdit sums its edits; an edit moved back by a later one nets out.
func TestEM5_AMultiEditSumsItsEdits(t *testing.T) {
	got := Of("MultiEdit", multi(t, "pkg/calc_test.go",
		[2]string{"\tt.Errorf(\"a\")\n", ""},
		[2]string{"body()\n", "\tt.Skip(\"x\")\nbody()\n"},
		[2]string{"\tt.Skip(\"x\")\n", ""},
		[2]string{"func TestA(t *testing.T) {\n", ""},
	))
	want(t, got, nil, ids{"go.assert": 1, "go.test_def": 1})
}

// Only a test-like path in a language of the table is read; the fold decides
// the class, so this is the cheap filter and may say yes too often, never no
// to a test path.
func TestEM6_OnlyTestPathsInTheTableLanguages(t *testing.T) {
	line := "\tt.Skip(\"x\")\n"
	for _, p := range []string{"pkg/calc.go", "src/app.py", "README.md", "pkg/calc_test.rs", "pkg/calc_test.js"} {
		want(t, Of("Edit", input(t, p, "", line)), nil, nil)
	}
	for _, p := range []string{"pkg/calc_test.go", "test/helper.go", "e2e/run.go", "tests/x.go", "spec/x.go", "PKG/CALC_TEST.GO"} {
		want(t, Of("Edit", input(t, p, "", line)), ids{"go.t_skip": 1}, nil)
	}
	// Another tool, or no input, has no marks.
	want(t, Of("Write", input(t, "pkg/calc_test.go", "", line)), nil, nil)
	want(t, Of("Edit", nil), nil, nil)
	want(t, Of("Edit", json.RawMessage(`{"file_path":"pkg/calc_test.go","old_string":7,"new_string":"t.Skip()"}`)), nil, nil)
}

// A line is read through its JSON escapes: tabs, escaped quotes and unicode
// escapes are decoded in the first bytes of the line, and a very long line is
// read by its head.
func TestEM7_EscapesAndLongLines(t *testing.T) {
	raw := json.RawMessage(`{"file_path":"pkg/calc_test.go","old_string":"","new_string":"\t\tt.Skip(\"\u00e9\")\r\nx := \"\\nt.Skip()\"\n\u00e9 t.Skip()\n"}`)
	want(t, Of("Edit", raw), ids{"go.t_skip": 1}, nil)
	// An escape of an ASCII byte is that byte: \u0074 is t.
	want(t, Of("Edit", json.RawMessage(`{"file_path":"pkg/calc_test.go","new_string":"\u0074.Skip()\n"}`)), ids{"go.t_skip": 1}, nil)
	long := "\tt.Errorf(\"" + strings.Repeat("x", 100000) + "\")\n"
	want(t, Of("Edit", input(t, "pkg/calc_test.go", "", long)), ids{"go.assert": 1}, nil)
	// The head window never reads a prefix across its end.
	want(t, Of("Edit", input(t, "pkg/calc_test.go", "", strings.Repeat(" ", 200)+"t.Skip()\n")), ids{"go.t_skip": 1}, nil)
}

// Every id the table mints is a closed one: lower case words and dots, short.
func TestEM8_TheTableIsClosedAndVersioned(t *testing.T) {
	if Version != 1 {
		t.Errorf("Version = %d, want 1", Version)
	}
	seen := map[string]bool{}
	for _, r := range table {
		if seen[r.id] {
			t.Errorf("id %s is in the table twice", r.id)
		}
		seen[r.id] = true
		if len(r.prefixes) == 0 {
			t.Errorf("row %s has no prefixes", r.id)
		}
		for _, p := range r.prefixes {
			if len(p) >= headBytes {
				t.Errorf("row %s: prefix %q does not fit the %d-byte head window", r.id, p, headBytes)
			}
		}
	}
	for _, id := range []string{"go.test_def", "go.t_skip", "go.assert", "py.test_def", "py.pytest_skip", "py.unittest_skip", "py.xfail", "py.assert"} {
		if !seen[id] {
			t.Errorf("table lacks %s", id)
		}
	}
	// The unminted ids of the other languages stay unminted (issue 207 Q4).
	for _, id := range []string{"jvm.assert", "ruby.assert", "php.assert", "dotnet.assert", "js.assert", "rust.assert"} {
		if seen[id] {
			t.Errorf("table mints %s, which this card leaves unminted", id)
		}
	}
}

// The consent pin (issue 207): a mark holds ids and counts from the fixed
// table and never the edited text. Each marker line here carries a canary the
// edit's own text would leak, as do the lines around it; the marks, however
// they are encoded, hold none of it, and every key is a table id.
const (
	canaryOld = "CANARY-OLD-5f3a91"
	canaryNew = "CANARY-NEW-c07d42"
)

func TestEM9_AMarkCarriesNoBytesOfTheEdit(t *testing.T) {
	from := "func TestSecret(t *testing.T) {\n\tt.Errorf(\"" + canaryOld + "\")\n\tassert.Equal(t, \"" + canaryOld + "\", got)\n\ttoken := \"" + canaryOld + "\"\n}\n"
	to := "func TestSecret(t *testing.T) {\n\tt.Skip(\"" + canaryNew + "\")\n\t// t.Errorf(\"" + canaryNew + "\")\n\ttoken := \"" + canaryNew + "\"\n}\n"
	m := Of("Edit", input(t, "pkg/secret_test.go", from, to))
	if m == nil {
		t.Fatal("no marks for an edit that skips a test and drops its assertions")
	}
	b, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []string{canaryOld, canaryNew, "CANARY", "TestSecret", "token", "Errorf", "Equal"} {
		if strings.Contains(string(b), c) {
			t.Errorf("the marks %s carry %q from the edit", b, c)
		}
	}
	closed := map[string]bool{}
	for _, r := range table {
		closed[r.id] = true
	}
	for _, side := range []map[string]int{m.Added, m.Removed} {
		for id, n := range side {
			if !closed[id] || n <= 0 || n > strings.Count(from+to, "\n") {
				t.Errorf("mark %q: %d is not a table id with a count of lines", id, n)
			}
		}
	}
	if m.Basis != Basis {
		t.Errorf("basis %q, want %q", m.Basis, Basis)
	}
	var keys map[string]json.RawMessage
	if err := json.Unmarshal(b, &keys); err != nil || len(keys) != 4 {
		t.Errorf("marks encode %d keys (%v), want added, removed, basis and version", len(keys), err)
	}
}

// The producer reads the input in place: an edit with no marker line, however
// large, allocates nothing, which the post's per-call allocation rows depend
// on.
func TestEM10_NoMarkersMeansNoAllocation(t *testing.T) {
	if raceEnabled {
		t.Skip("the race detector adds allocations")
	}
	body := strings.Repeat("\tx := compute(a, b)\n", 4000)
	in := input(t, "pkg/calc_test.go", "// old\n", body)
	if n := testing.AllocsPerRun(20, func() {
		if Of("Edit", in) != nil {
			t.Fatal("marks for an edit with no marker line")
		}
	}); n != 0 {
		t.Errorf("%v allocations for an edit with no marker line, want 0", n)
	}
	multiIn := multi(t, "pkg/calc_test.go", [2]string{"a\n", body}, [2]string{"b\n", "c\n"})
	if n := testing.AllocsPerRun(20, func() { Of("MultiEdit", multiIn) }); n != 0 {
		t.Errorf("%v allocations for a MultiEdit with no marker line, want 0", n)
	}
}
