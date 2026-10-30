package shape

import "encoding/json"

// declPathFields are the tools whose declaration carries the path the call
// names, and the ONE field read for each: file_path for Edit, Write and
// MultiEdit, notebook_path for NotebookEdit. These are the tools that change
// the file their input names, which is what a reader of the record needs the
// path for: which file a call may have changed.
//
// A second table and not pathFields, on purpose. pathFields is the label
// contract's, and the two answer different questions: Read earns a label and
// changes no file, so it is there and not here; MultiEdit edits a file as
// surely as Edit does and the label contract does not name it, so it is here
// and not there. TestDeclaredPath_DiffersFromLabelTableExactly pins that
// difference, so folding the two into one table fails loudly.
//
// Bash is absent for the reason it is absent from pathFields: a command
// line's write targets are a derivation of their own, not one field.
var declPathFields = map[string]string{
	"Edit":         "file_path",
	"Write":        "file_path",
	"MultiEdit":    "file_path",
	"NotebookEdit": "notebook_path",
}

// DeclaredPath is the path a file-changing tool's input names, as written,
// for the declaration to carry: nil for every tool outside declPathFields,
// and for an input whose field is absent, null, empty or not a string.
//
// As written means exactly that. A relative path stays relative, and nothing
// is cleaned, resolved or case-folded: the record carries what the harness
// sent, and resolving it against the call's cwd is the reader's. The length
// is not bounded here either; the declaration's cap is
// (store.CapDeclaration).
//
// This is one of the four places a substring of a tool input leaves this
// package, Hosts, WrittenPaths and CommandLine being the others (see the
// package comment). It reads one named field with stringField, the reader
// Label and Hosts share, so no field whose name is not written in the table
// above can be read by accident.
func DeclaredPath(toolName string, toolInput json.RawMessage) *string {
	field, ok := declPathFields[toolName]
	if !ok {
		return nil
	}
	p, ok := stringField(toolInput, field)
	if !ok || p == "" {
		return nil
	}
	return &p
}
