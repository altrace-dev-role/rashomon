package main

import (
	"encoding/json"
	"fmt"

	"github.com/altrace-dev-role/rashomon/internal/settings"
)

// renderSettings is the bytes editSettings writes for doc; a test swaps it to
// reach the parse guard in writeChecked.
var renderSettings = (*settings.Document).Bytes

// writeChecked writes what doc renders to over original, and returns it. Bytes
// that are not JSON are refused and nothing is written: an editor bug must not
// reach the user's Claude Code configuration.
func writeChecked(path string, original []byte, doc *settings.Document) ([]byte, error) {
	out := renderSettings(doc)
	if !json.Valid(out) {
		return nil, fmt.Errorf("the edited %s would not be valid JSON; settings.json was left unchanged", path)
	}
	return out, settings.Write(path, original, out)
}
