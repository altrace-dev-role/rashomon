package knowledge

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/regfile"
)

// capRows is one row whose file, as WriteFile writes it, is exactly n
// bytes: its Text is padded by what a probe write of the bare row, under a
// root of its own, falls short of n. WriteFile measures it, so the fixture
// follows the format wherever it goes and copies none of it; the bytes
// depend on the rows alone, not the file's name.
func capRows(t *testing.T, n int) []Row {
	t.Helper()
	row := Row{ID: "k1", Kind: KindDecision, Title: "cap"}
	p, err := WriteFile(t.TempDir(), "probe", []Row{row})
	if err != nil {
		t.Fatal(err)
	}
	fi, err := os.Stat(p)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Size() > int64(n) {
		t.Fatalf("the bare row's file is %d bytes, over the %d asked for", fi.Size(), n)
	}
	row.Text = strings.Repeat("x", n-int(fi.Size()))
	return []Row{row}
}

// TestWS1_WC2_WriteFileRefusesARowFileOverMaxKnowledgeFileBytes: the
// writer holds the reader's cap by its name (card WRITE-SAFE1), so what an
// import writes ReadDir reads. A row file one byte over
// MaxKnowledgeFileBytes is refused with one line naming the file, an error
// matching regfile.ErrOverCap whose *regfile.OverCapError holds the cap
// and a size past it; the file written before is byte-equal after, and no
// temporary file is left beside it. A file of exactly the cap is written,
// and ReadDir reads its row. Break: write without the cap, and the
// over-cap file is written whole, where ReadDir then refuses it.
func TestWS1_WC2_WriteFileRefusesARowFileOverMaxKnowledgeFileBytes(t *testing.T) {
	t.Run("over the cap is refused", func(t *testing.T) {
		root := t.TempDir()
		p, err := WriteFile(root, "a", []Row{{ID: "old", Kind: KindDecision, Title: "old"}})
		if err != nil {
			t.Fatal(err)
		}
		old, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		_, err = WriteFile(root, "a", capRows(t, MaxKnowledgeFileBytes+1))
		if err == nil {
			size := int64(-1)
			if fi, err := os.Stat(p); err == nil {
				size = fi.Size()
			}
			t.Fatalf("WriteFile wrote a row file over the %d byte cap with no error (the file is now %d bytes); want it refused", MaxKnowledgeFileBytes, size)
		}
		if msg := err.Error(); !strings.Contains(msg, p) || strings.Contains(msg, "\n") {
			t.Errorf("WriteFile = %q; want one line naming %s", msg, p)
		}
		var oc *regfile.OverCapError
		switch {
		case !errors.Is(err, regfile.ErrOverCap):
			t.Errorf("WriteFile = %v; want an error matching regfile.ErrOverCap", err)
		case !errors.As(err, &oc):
			t.Errorf("WriteFile = %v; want a *regfile.OverCapError in its chain", err)
		case oc.Max != MaxKnowledgeFileBytes || oc.Size <= oc.Max:
			t.Errorf("the refusal holds size %d and cap %d; want the cap %d and a size past it", oc.Size, oc.Max, MaxKnowledgeFileBytes)
		}
		if got, err := os.ReadFile(p); err != nil || !bytes.Equal(got, old) {
			t.Errorf("the file written before now reads %d bytes (err %v); want its %d bytes unchanged", len(got), err, len(old))
		}
		ents, err := os.ReadDir(filepath.Dir(p))
		if err != nil {
			t.Fatal(err)
		}
		if len(ents) != 1 || ents[0].Name() != filepath.Base(p) {
			var names []string
			for _, e := range ents {
				names = append(names, e.Name())
			}
			t.Errorf("%s holds %v after the refusal; want only %s", filepath.Dir(p), names, filepath.Base(p))
		}
	})
	t.Run("at the cap is written and read", func(t *testing.T) {
		root := t.TempDir()
		p, err := WriteFile(root, "a", capRows(t, MaxKnowledgeFileBytes))
		if err != nil {
			t.Fatalf("WriteFile of a row file of exactly %d bytes: %v; want it written", MaxKnowledgeFileBytes, err)
		}
		if fi, err := os.Stat(p); err != nil || fi.Size() != MaxKnowledgeFileBytes {
			t.Fatalf("the written file: %v (err %v); want exactly %d bytes", fi, err, MaxKnowledgeFileBytes)
		}
		rows, err := ReadDir(root)
		if err != nil || len(rows) != 1 || rows[0].ID != "k1" {
			t.Fatalf("ReadDir of the file WriteFile wrote at the cap: %d rows, error %v; want the one row k1", len(rows), err)
		}
	})
}
