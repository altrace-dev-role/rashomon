package claudecode

import (
	"encoding/json"

	"github.com/altrace-dev-role/rashomon/internal/trace"
)

// input is the part of a Claude Code tool's input the content fields read:
// Bash's command; the path Read, Edit, Write, MultiEdit (file_path),
// NotebookEdit and NotebookRead (notebook_path), Glob and Grep (path) name;
// an edit's strings; a write's content.
type input struct {
	Command      string `json:"command"`
	FilePath     string `json:"file_path"`
	NotebookPath string `json:"notebook_path"`
	Path         string `json:"path"`
	OldString    string `json:"old_string"`
	NewString    string `json:"new_string"`
	Content      string `json:"content"`
	NewSource    string `json:"new_source"`
	Edits        []struct {
		OldString string `json:"old_string"`
		NewString string `json:"new_string"`
	} `json:"edits"`
}

// fill sets a call's Command, Paths and Changes from its input; an input
// that does not parse leaves them empty.
func fill(c *trace.CallStep) {
	var in input
	if json.Unmarshal(c.Input, &in) != nil {
		return
	}
	if c.Class == trace.ActExec {
		c.Command = in.Command
	}
	path := in.FilePath
	if path == "" {
		path = in.NotebookPath
	}
	if path == "" {
		path = in.Path
	}
	if path != "" {
		c.Paths = []string{path}
	}
	switch c.Tool {
	case "Edit":
		c.Changes = []trace.Change{{Path: path, Old: in.OldString, New: in.NewString}}
	case "MultiEdit":
		for _, e := range in.Edits {
			c.Changes = append(c.Changes, trace.Change{Path: path, Old: e.OldString, New: e.NewString})
		}
	case "Write":
		c.Changes = []trace.Change{{Path: path, New: in.Content}}
	case "NotebookEdit":
		c.Changes = []trace.Change{{Path: path, New: in.NewSource}}
	}
}
