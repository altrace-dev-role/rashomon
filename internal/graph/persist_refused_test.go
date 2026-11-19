package graph

import (
	"bytes"
	"errors"
	"reflect"
	"strings"
	"testing"
)

// TestSave_AHeaderWithNoRefusalIsAsBefore (e2's Go control): a header
// with nothing refused is written byte for byte as before the refused
// fields existed, so an index of a tree no resolver refused (every Go
// index) does not change. Break: write the refused fields when empty.
func TestSave_AHeaderWithNoRefusalIsAsBefore(t *testing.T) {
	var buf bytes.Buffer
	if err := Save(&buf, NewStore(), Header{Repo: "r", Rev: HeaderRev{Commit: "c1"}}); err != nil {
		t.Fatal(err)
	}
	if got, want := buf.String(), `{"schema":1,"repo":"r","rev":{"commit":"c1"},"producers":{},"records":0}`+"\n"; got != want {
		t.Errorf("header line = %q; want %q", got, want)
	}
}

// TestSave_TheHeaderCarriesItsRefusals (e2): the count and the named files
// are written in the header line and read back whole. Break: drop either
// from the header.
func TestSave_TheHeaderCarriesItsRefusals(t *testing.T) {
	h := Header{Repo: "r", Rev: HeaderRev{Commit: "c1"}, Refused: 2,
		RefusedFiles: []Refusal{{Path: "py/bad.py", Line: 4, Reason: "a bracket opened at this line is not closed"}, {Path: "py/gone.py", Reason: "no such file or directory"}}}
	var buf bytes.Buffer
	if err := Save(&buf, NewStore(), h); err != nil {
		t.Fatal(err)
	}
	first, _, _ := strings.Cut(buf.String(), "\n")
	if want := `"refused":2,"refused_files":[{"path":"py/bad.py","line":4,"reason":"a bracket opened at this line is not closed"},{"path":"py/gone.py","reason":"no such file or directory"}]}`; !strings.HasSuffix(first, want) {
		t.Errorf("header line = %s; want it to end %s", first, want)
	}
	_, got, err := Load(&buf)
	if err != nil {
		t.Fatal(err)
	}
	if got.Refused != 2 || !reflect.DeepEqual(got.RefusedFiles, h.RefusedFiles) {
		t.Errorf("read back refused %d %+v; want 2 %+v", got.Refused, got.RefusedFiles, h.RefusedFiles)
	}
}

// TestLoadFile_NoIndexSaysWhereAndWhatToRun (e5): with no index the
// error names the file looked for and the command that writes it, and is
// still ErrNoGraph. Break: the old words "graph: no index".
func TestLoadFile_NoIndexSaysWhereAndWhatToRun(t *testing.T) {
	_, _, err := LoadFile(t.TempDir())
	if !errors.Is(err, ErrNoGraph) {
		t.Fatalf("LoadFile = %v; want ErrNoGraph", err)
	}
	if want := "graph: no index at .rashomon/graph.jsonl; run rashomon graph index at the repository root"; err.Error() != want {
		t.Errorf("error = %q; want %q", err.Error(), want)
	}
}
