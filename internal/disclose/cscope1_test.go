package disclose

import (
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/store"
)

// TestCSCOPE1a_StoredSaysWhatRecordScopeV5Keeps (CONSENT-SCOPE1, absorbs
// CONSENT-TEXT2): every consent this build records is at record scope v5
// (watch.go's record), and no command records v4, so the STORED paragraph
// says what v5 keeps: the working directory, the project root, the file an
// edit names and the files a shell line may write, in the clear, and the
// SHA-256 of the file an edit leaves; that `rashomon status` shows the
// scope; and that no setting leaves them out. The inventory's call-record
// row names the same digest, and neither calls v4 the default. The premise
// is the gate itself: v5 keeps content_sha256 and v4 drops it, so a change
// to what v5 keeps has to change this text. Break: the screen says paths are
// keyed digests at v4, the default, which a yes here never records, and is
// silent on the file digest a yes here does record.
func TestCSCOPE1a_StoredSaysWhatRecordScopeV5Keeps(t *testing.T) {
	sum := strings.Repeat("d", 64)
	var x store.Execution
	x.ContentSHA256 = &sum
	store.GateExecution(&x, store.RecordScopeV5)
	if x.ContentSHA256 == nil {
		t.Fatal("premise: record scope v5 drops content_sha256")
	}
	store.GateExecution(&x, store.RecordScopeV4)
	if x.ContentSHA256 != nil {
		t.Fatal("premise: record scope v4 keeps content_sha256")
	}

	inv, err := Default()
	if err != nil {
		t.Fatal(err)
	}
	var stored string
	for _, p := range inv.Paragraphs {
		if strings.HasPrefix(p.Text, "STORED: for each tool call") {
			stored = p.Text
		}
	}
	for _, want := range []string{
		"at record scope v5", "the working directory", "the project root", "the file an edit names",
		"the files a shell line may write", "in the clear", "the SHA-256 of the file an edit names",
		"`rashomon status` shows the scope", "no setting that leaves these out",
	} {
		if !strings.Contains(stored, want) {
			t.Errorf("the STORED paragraph does not say %q:\n%s", want, stored)
		}
	}
	if strings.Contains(stored, "v4, the default") {
		t.Errorf("the STORED paragraph calls v4 the default, which no consent from this build records:\n%s", stored)
	}

	var row string
	for _, f := range inv.Files {
		if strings.Contains(f.Holds, "(written_paths)") {
			row = f.Holds
		}
	}
	if !strings.Contains(row, "(content_sha256)") || strings.Contains(row, "v4, the default") {
		t.Errorf("the call record's inventory row does not name content_sha256, or calls v4 the default: %q", row)
	}
}
