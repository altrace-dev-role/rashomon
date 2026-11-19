package knowledge

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

// knowledgeBoundR21 is the bound R21 holds a knowledge file to, the
// package's MaxKnowledgeFileBytes (16 MiB). Base had no such constant, and
// its run held the row to a local one of the same value.
const knowledgeBoundR21 = MaxKnowledgeFileBytes

// padKnowledge writes one row as root/Dir/a.json with WriteFile, the
// import's own writer, then pads the file with JSON whitespace to size
// bytes, so the file is valid and the parse would keep its row.
func padKnowledge(t *testing.T, root string, size int) {
	t.Helper()
	p, err := WriteFile(root, "a", []Row{{Schema: Schema, ID: "R21", Kind: KindDecision, Title: "bound"}})
	if err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if len(b) > size {
		t.Fatalf("the one-row file is %d bytes, over the %d asked for", len(b), size)
	}
	b = append(b, bytes.Repeat([]byte(" "), size-len(b))...)
	if err := os.WriteFile(p, b, 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestRS1a_R21_TheKnowledgeCapIs16MiB pins the value R21's rows read
// through the constant, so a change of the cap fails here and not only by
// moving both rows with it. A change of the value changes how large an
// import may be read, and is made here on purpose.
func TestRS1a_R21_TheKnowledgeCapIs16MiB(t *testing.T) {
	if MaxKnowledgeFileBytes != 16<<20 {
		t.Fatalf("MaxKnowledgeFileBytes = %d, want %d (16 MiB)", MaxKnowledgeFileBytes, 16<<20)
	}
}

// TestRS1a_R21_ReadDirOverTheBoundIsRefused: a knowledge file over the
// bound is refused before its parse, the error naming the file by its base
// name only; a file at exactly the bound still reads its row. Break: read
// the file whole, and the over-bound file returns its row with no error.
func TestRS1a_R21_ReadDirOverTheBoundIsRefused(t *testing.T) {
	t.Run("at the bound reads its row", func(t *testing.T) {
		root := t.TempDir()
		padKnowledge(t, root, knowledgeBoundR21)
		rows, err := ReadDir(root)
		if err != nil || len(rows) != 1 || rows[0].ID != "R21" {
			t.Fatalf("a file of exactly %d bytes: %d rows, error %v; want the one row and no error", knowledgeBoundR21, len(rows), err)
		}
	})
	t.Run("over the bound is refused", func(t *testing.T) {
		root := t.TempDir()
		padKnowledge(t, root, knowledgeBoundR21+1)
		rows, err := ReadDir(root)
		if err == nil {
			t.Fatalf("a file of %d bytes read %d rows with no error; want a refusal over the %d byte bound", knowledgeBoundR21+1, len(rows), knowledgeBoundR21)
		}
		if msg := err.Error(); !strings.HasPrefix(msg, "knowledge: a.json: ") || !strings.Contains(msg, "over") || strings.Contains(msg, root) {
			t.Fatalf("refusal %q; want it to start \"knowledge: a.json: \", say over, and name no directory", msg)
		}
		if rows != nil {
			t.Fatalf("a refusal returned %d rows; want none", len(rows))
		}
	})
}
