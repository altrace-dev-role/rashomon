package graph

import (
	"bytes"
	"reflect"
	"strings"
	"testing"
)

// TestSave_TheHeaderCarriesItsLeftOutDirs (L2): the count and the named
// directories are written in the header line after the refused fields and
// read back whole; a header with none is TestSave_AHeaderWithNoRefusalIsAsBefore's
// line, byte for byte. Break: drop either from the header.
func TestSave_TheHeaderCarriesItsLeftOutDirs(t *testing.T) {
	h := Header{Repo: "r", Rev: HeaderRev{Commit: "c1"}, LeftOut: 21,
		LeftOutDirs: []LeftDir{{Path: "py/a-b", Why: "w1"}, {Path: "py/venv", Why: "w2"}}}
	var buf bytes.Buffer
	if err := Save(&buf, NewStore(), h); err != nil {
		t.Fatal(err)
	}
	first, _, _ := strings.Cut(buf.String(), "\n")
	if want := `"records":0,"left_out":21,"left_out_dirs":[{"path":"py/a-b","why":"w1"},{"path":"py/venv","why":"w2"}]}`; !strings.HasSuffix(first, want) {
		t.Errorf("header line = %s; want it to end %s", first, want)
	}
	_, got, err := Load(&buf)
	if err != nil {
		t.Fatal(err)
	}
	if got.LeftOut != 21 || !reflect.DeepEqual(got.LeftOutDirs, h.LeftOutDirs) {
		t.Errorf("read back left out %d %+v; want 21 %+v", got.LeftOut, got.LeftOutDirs, h.LeftOutDirs)
	}
}
