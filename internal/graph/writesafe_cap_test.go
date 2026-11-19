package graph

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/regfile"
)

// capPadBytes is the length of the pad string every full node of a cap
// fixture shares. The index reaches its size by many records over that
// shared string, so the two stores hold three pads between them, the
// shared pad and each last node's, and the encoder holds a record at a
// time. The test's peak is LoadFile's decode of the index written at the
// cap, which holds each node's pad apart, near the cap itself; a single
// attribute of 64 MiB would hold several times that under -race.
const capPadBytes = 1 << 20

// capHeader is the header every cap fixture saves under.
var capHeader = Header{Repo: "r", Rev: HeaderRev{Commit: "C1"}}

// capCounter counts the bytes written to it.
type capCounter int64

func (c *capCounter) Write(p []byte) (int, error) {
	*c += capCounter(len(p))
	return len(p), nil
}

// capSavedStores are two stores whose Save under capHeader is exactly
// MaxGraphFileBytes and exactly one byte more: full nodes sharing one pad,
// and a last node whose pad takes the rest. The rest is measured by Save
// itself, with the last pad empty, so the fixture follows the encoding
// wherever it goes and copies none of it.
func capSavedStores(t *testing.T) (atCap, over *Store) {
	t.Helper()
	pad := strings.Repeat("x", capPadBytes)
	full := max(int(MaxGraphFileBytes/capPadBytes)-1, 0)
	build := func(rest int) *Store {
		s := NewStore()
		for i := range full {
			s.AssertNode(Node{ID: ID("unit:go:p" + strconv.Itoa(i)), Kind: "unit", Tier: Proven, Conf: 1, Rev: RevCommit("C1"), Attrs: map[string]string{"pad": pad}})
		}
		s.AssertNode(Node{ID: "unit:go:z", Kind: "unit", Tier: Proven, Conf: 1, Rev: RevCommit("C1"), Attrs: map[string]string{"pad": strings.Repeat("x", rest)}})
		return s
	}
	var c capCounter
	if err := Save(&c, build(0), capHeader); err != nil {
		t.Fatal(err)
	}
	rest := MaxGraphFileBytes - int64(c)
	if rest < 0 {
		t.Fatalf("the fixture with its last pad empty saves %d bytes, over the %d byte cap already", int64(c), MaxGraphFileBytes)
	}
	return build(int(rest)), build(int(rest) + 1)
}

// TestWS1_WC1_SaveFileRefusesAnIndexOverMaxGraphFileBytes: the writer holds
// the reader's cap by its name (card WRITE-SAFE1), so what SaveFile writes
// LoadFile reads. An index one byte over MaxGraphFileBytes is refused with
// one line naming the index, an error matching regfile.ErrOverCap whose
// *regfile.OverCapError holds the cap and a size past it; the index saved
// before is byte-equal after, and no temporary file is left beside it. An
// index of exactly the cap is written, and LoadFile reads every record its
// header counts. Break: save without the cap, and the over-cap index is
// written whole, where every prompt's read then refuses it.
func TestWS1_WC1_SaveFileRefusesAnIndexOverMaxGraphFileBytes(t *testing.T) {
	atCap, over := capSavedStores(t)
	t.Run("over the cap is refused", func(t *testing.T) {
		root := t.TempDir()
		small := NewStore()
		small.AssertNode(Node{ID: "unit:go:a", Kind: "unit", Tier: Proven, Conf: 1, Rev: RevCommit("C1")})
		if err := SaveFile(root, small, capHeader); err != nil {
			t.Fatal(err)
		}
		p := filepath.Join(root, filepath.FromSlash(GraphFile))
		old, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		err = SaveFile(root, over, capHeader)
		if err == nil {
			size := int64(-1)
			if fi, err := os.Stat(p); err == nil {
				size = fi.Size()
			}
			t.Fatalf("SaveFile wrote an index over the %d byte cap with no error (the file is now %d bytes); want it refused", MaxGraphFileBytes, size)
		}
		if msg := err.Error(); !strings.Contains(msg, p) || strings.Contains(msg, "\n") {
			t.Errorf("SaveFile = %q; want one line naming %s", msg, p)
		}
		var oc *regfile.OverCapError
		switch {
		case !errors.Is(err, regfile.ErrOverCap):
			t.Errorf("SaveFile = %v; want an error matching regfile.ErrOverCap", err)
		case !errors.As(err, &oc):
			t.Errorf("SaveFile = %v; want a *regfile.OverCapError in its chain", err)
		case oc.Max != MaxGraphFileBytes || oc.Size <= oc.Max:
			t.Errorf("the refusal holds size %d and cap %d; want the cap %d and a size past it", oc.Size, oc.Max, MaxGraphFileBytes)
		}
		if got, err := os.ReadFile(p); err != nil || !bytes.Equal(got, old) {
			t.Errorf("the index saved before now reads %d bytes (err %v); want its %d bytes unchanged", len(got), err, len(old))
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
		if err := SaveFile(root, atCap, capHeader); err != nil {
			t.Fatalf("SaveFile of an index of exactly %d bytes: %v; want it written", MaxGraphFileBytes, err)
		}
		p := filepath.Join(root, filepath.FromSlash(GraphFile))
		if fi, err := os.Stat(p); err != nil || fi.Size() != MaxGraphFileBytes {
			t.Fatalf("the saved index: %v (err %v); want exactly %d bytes", fi, err, MaxGraphFileBytes)
		}
		s, h, err := LoadFile(root)
		if err != nil {
			t.Fatalf("LoadFile of the index SaveFile wrote at the cap: %v", err)
		}
		if n := len(s.Nodes(RevCommit("C1"))); h.Records == 0 || n != h.Records {
			t.Errorf("LoadFile read %d nodes of the %d records its header counts", n, h.Records)
		}
	})
}
