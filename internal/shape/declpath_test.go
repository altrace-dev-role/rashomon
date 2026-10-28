package shape

import (
	"encoding/json"
	"maps"
	"slices"
	"testing"
)

// TestDeclaredPath_TableIsClosed: the four tools that edit a file carry the
// path their input names, as written, and nothing else does. A tool outside
// the table gets nil even when its input names a path in both fields, and a
// tool inside it reads its own field and never the other one.
func TestDeclaredPath_TableIsClosed(t *testing.T) {
	want := map[string]string{
		"Edit":         "file_path",
		"Write":        "file_path",
		"MultiEdit":    "file_path",
		"NotebookEdit": "notebook_path",
	}
	if !maps.Equal(declPathFields, want) {
		t.Fatalf("declPathFields = %v, want exactly %v", declPathFields, want)
	}

	for tool, field := range want {
		for _, p := range []string{
			"/home/u/proj/main.go",
			"relative/stays/relative.go",    // stored as sent; the reader resolves it
			"/home/u/proj/../proj/./x.go",   // not normalised
			"/home/u/proj/dir with space/q", // as written, blanks and all
			`C:\Users\u\proj\x.go`,          // backslashes kept
			"/home/u/proj/caf\u00e9.ipynb",
		} {
			if got := DeclaredPath(tool, pathInput(t, field, p)); got == nil || *got != p {
				t.Errorf("DeclaredPath(%s, {%s: %q}) = %v, want the path as written", tool, field, p, deref(got))
			}
		}
		// The other table field is not read: NotebookEdit's file_path and
		// Edit's notebook_path name nothing.
		other := "notebook_path"
		if field == "notebook_path" {
			other = "file_path"
		}
		if got := DeclaredPath(tool, pathInput(t, other, "/home/u/proj/x")); got != nil {
			t.Errorf("DeclaredPath(%s, {%s: ...}) = %q, want nil: %s reads %s alone", tool, other, *got, tool, field)
		}
		for name, raw := range map[string]string{
			"no key":       `{"description":"irrelevant prose"}`,
			"null":         `{"` + field + `":null}`,
			"empty":        `{"` + field + `":""}`,
			"number":       `{"` + field + `":7}`,
			"object":       `{"` + field + `":{"path":"/x"}}`,
			"array":        `{"` + field + `":["/x"]}`,
			"bool":         `{"` + field + `":true}`,
			"not object":   `"/home/u/proj/x"`,
			"empty input":  ``,
			"nested only":  `{"edits":[{"` + field + `":"/x"}]}`,
			"escaped slug": `{"` + field + `\u0000":"/x"}`,
		} {
			if got := DeclaredPath(tool, json.RawMessage(raw)); got != nil {
				t.Errorf("DeclaredPath(%s, %s: %s) = %q, want nil", tool, name, raw, *got)
			}
		}
	}

	both := json.RawMessage(`{"file_path":"/home/u/a","notebook_path":"/home/u/b.ipynb","command":"cat /home/u/a"}`)
	for _, tool := range []string{
		"Read", "Bash", "Glob", "Grep", "WebFetch", "Task", "NotebookRead", "TodoWrite",
		"", "edit", "write", "EDIT", "mcp__fs__Write", "Write ", " Edit",
	} {
		if got := DeclaredPath(tool, both); got != nil {
			t.Errorf("DeclaredPath(%q, ...) = %q, want nil: %q is not a tool that edits a file", tool, *got, tool)
		}
	}
}

// TestDeclaredPath_DiffersFromLabelTableExactly: the declaration's table and
// the label contract's differ by Read, which only the label reads, and
// MultiEdit, which only the declaration carries, and in nothing else. A change
// that folds the two into one table moves one of these and fails here.
func TestDeclaredPath_DiffersFromLabelTableExactly(t *testing.T) {
	var onlyLabel, onlyDecl []string
	for tool, field := range pathFields {
		declField, ok := declPathFields[tool]
		if !ok {
			onlyLabel = append(onlyLabel, tool)
			continue
		}
		if declField != field {
			t.Errorf("%s reads %s for its label and %s for its declared path; a tool's path is one field", tool, field, declField)
		}
	}
	for tool := range declPathFields {
		if _, ok := pathFields[tool]; !ok {
			onlyDecl = append(onlyDecl, tool)
		}
	}
	slices.Sort(onlyLabel)
	slices.Sort(onlyDecl)
	if !slices.Equal(onlyLabel, []string{"Read"}) || !slices.Equal(onlyDecl, []string{"MultiEdit"}) {
		t.Errorf("only in pathFields %v, only in declPathFields %v; want [Read] and [MultiEdit]", onlyLabel, onlyDecl)
	}
}

func deref(p *string) any {
	if p == nil {
		return nil
	}
	return *p
}
