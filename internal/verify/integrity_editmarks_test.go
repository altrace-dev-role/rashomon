package verify

import (
	"encoding/json"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/editmarks"
	"github.com/altrace-dev-role/rashomon/internal/store"
)

// editInput is an Edit's tool_input, from which the post hook would compute
// the execution's marks (editmarks.Of).
func editInput(t *testing.T, path, from, to string) json.RawMessage {
	t.Helper()
	b, err := json.Marshal(map[string]string{"file_path": path, "old_string": from, "new_string": to})
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// marked is an Edit call of path whose execution carries the marks the
// producer writes for the edit from -> to.
func marked(t *testing.T, seq int64, path, from, to string) call {
	t.Helper()
	m := editmarks.Of("Edit", editInput(t, path, from, to))
	if m == nil {
		t.Fatalf("no marks for the edit %q -> %q of %s", from, to, path)
	}
	return edit(seq, path).marks(m.Added, m.Removed)
}

// AC-M3.6: edit marks written for test removal, skip insertion and assertion
// edits, read through the fold. Each fixture is a failing run followed by one
// edit of a test file, and the integrity view names the check and the marker.
func TestACM36_EditMarksSpeakThroughTheFold(t *testing.T) {
	const goTest, pyTest = "pkg/calc_test.go", "tests/test_calc.py"
	red := gateCall(1, "go-test", store.ExecFailed)
	for _, c := range []struct {
		name   string
		call   call
		counts IntegrityCounts
		entry  IntegrityEntry
	}{
		{name: "a Go test removed", call: marked(t, 2, goTest, "func TestAdd(t *testing.T) {\n\tt.Parallel()\n}\n", ""),
			counts: IntegrityCounts{TestsDelta: -1}, entry: IntegrityEntry{Check: TestRemoved, Marker: "go.test_def", Seq: 3}},
		{name: "a Python test removed", call: marked(t, 2, pyTest, "def test_add():\n    pass\n", ""),
			counts: IntegrityCounts{TestsDelta: -1}, entry: IntegrityEntry{Check: TestRemoved, Marker: "py.test_def", Seq: 3}},
		{name: "a Go skip inserted", call: marked(t, 2, goTest, "\tbody()\n", "\tt.Skip(\"flaky\")\n\tbody()\n"),
			counts: IntegrityCounts{SkipsAdded: 1}, entry: IntegrityEntry{Check: SkipAdded, Marker: "go.t_skip", Seq: 3}},
		{name: "a pytest skip inserted", call: marked(t, 2, pyTest, "def test_add():\n", "@pytest.mark.skip(reason='x')\ndef test_add():\n"),
			counts: IntegrityCounts{SkipsAdded: 1}, entry: IntegrityEntry{Check: SkipAdded, Marker: "py.pytest_skip", Seq: 3}},
		{name: "a Go assertion removed", call: marked(t, 2, goTest, "\tt.Errorf(\"got %d\", got)\n\tbody()\n", "\tbody()\n"),
			counts: IntegrityCounts{}, entry: IntegrityEntry{Check: AssertionsRemoved, Marker: "go.assert", Seq: 3}},
		{name: "a Go assertion commented out", call: marked(t, 2, goTest, "\tt.Errorf(\"got %d\", got)\n", "\t// t.Errorf(\"got %d\", got)\n"),
			counts: IntegrityCounts{}, entry: IntegrityEntry{Check: AssertionsRemoved, Marker: "go.assert", Seq: 3}},
		{name: "a Python assertion removed", call: marked(t, 2, pyTest, "    assert add(1, 2) == 3\n    body()\n", "    body()\n"),
			counts: IntegrityCounts{}, entry: IntegrityEntry{Check: AssertionsRemoved, Marker: "py.assert", Seq: 3}},
	} {
		t.Run(c.name, func(t *testing.T) {
			r := runOf(t, red, c.call)
			wantIntegrity(t, integrityOf(r, 0, 4), c.counts, c.entry)
			// Before the edit's declaration the edit is not in the window.
			wantIntegrity(t, integrityOf(r, 0, 2), IntegrityCounts{})
		})
	}
	// A skip that sits in a comment is not a skip: the producer writes no
	// marks for it, and so the fold has nothing to read.
	if m := editmarks.Of("Edit", editInput(t, goTest, "\tbody()\n", "\t// t.Skip(\"flaky\")\n\tbody()\n")); m != nil {
		t.Errorf("marks %+v for a skip in a comment", m)
	}
	// A restored skip resolves with the edit that takes it out again.
	r := runOf(t, red,
		marked(t, 2, goTest, "\tbody()\n", "\tt.Skip(\"flaky\")\n\tbody()\n"),
		marked(t, 3, goTest, "\tt.Skip(\"flaky\")\n\tbody()\n", "\tbody()\n"))
	wantIntegrity(t, integrityOf(r, 0, 6), IntegrityCounts{})
	// A bare comparison swapped in for an assertion removes it: one removal.
	r = runOf(t, red, marked(t, 2, goTest, "\tt.Errorf(\"got %d\", got)\n", "\tif got != want {\n\t\treturn\n\t}\n"))
	wantIntegrity(t, integrityOf(r, 0, 4), IntegrityCounts{}, IntegrityEntry{Check: AssertionsRemoved, Marker: "go.assert", Seq: 3})
	// A test removed with its assertions is test_removed alone (D40: the
	// assertion removal is said once, by the check that explains it).
	r = runOf(t, red, marked(t, 2, goTest, "func TestAdd(t *testing.T) {\n\tt.Errorf(\"x\")\n}\n", ""))
	wantIntegrity(t, integrityOf(r, 0, 4), IntegrityCounts{TestsDelta: -1}, IntegrityEntry{Check: TestRemoved, Marker: "go.test_def", Seq: 3})
}

// Every id the producer mints is one the fold's table holds, of the class its
// check needs: the table's ids are the producer's contract (integrity.go).
func TestACM36_EveryMintedIDIsInTheMarkerTable(t *testing.T) {
	for _, c := range []struct {
		path, from, to, id string
		class              markClass
	}{
		{"a_test.go", "func TestX(t *testing.T) {\n", "", "go.test_def", markTestDef},
		{"a_test.go", "t.Skip()\n", "", "go.t_skip", markSkip},
		{"a_test.go", "t.Error()\n", "", "go.assert", markAssert},
		{"test_a.py", "def test_x():\n", "", "py.test_def", markTestDef},
		{"test_a.py", "pytest.skip()\n", "", "py.pytest_skip", markSkip},
		{"test_a.py", "@unittest.skip('x')\n", "", "py.unittest_skip", markSkip},
		{"test_a.py", "@pytest.mark.xfail\n", "", "py.xfail", markSkip},
		{"test_a.py", "assert x\n", "", "py.assert", markAssert},
	} {
		m := editmarks.Of("Edit", editInput(t, c.path, c.from, c.to))
		if m == nil || m.Removed[c.id] != 1 || len(m.Removed) != 1 {
			t.Fatalf("%s: marks %+v, want removed %s once", c.path, m, c.id)
		}
		if got, ok := markerTable()[c.id]; !ok || got != c.class {
			t.Errorf("%s: the marker table holds class %d (present %v), want %d", c.id, got, ok, c.class)
		}
	}
}
