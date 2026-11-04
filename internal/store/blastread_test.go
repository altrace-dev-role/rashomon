package store

import (
	"os"
	"testing"
)

// TestAI1_TakeBlastReadIsOncePerNameUntilReturned: the blast-radius read's
// marker (card ARCH-INLINE1, A2's read gate) is taken once per session and
// name, and taken again after ReturnBlastRead; a name that is not the
// prefix and 16 lower-case hex digits is never a file. Break: take the
// marker without O_EXCL, or let any name through, and this fails.
func TestAI1_TakeBlastReadIsOncePerNameUntilReturned(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(s.RunDir("s1"), 0o700); err != nil {
		t.Fatal(err)
	}
	const name = FileBlastReadPrefix + "0123456789abcdef"
	if !s.TakeBlastRead("s1", name) {
		t.Fatal("the first take = false, want true")
	}
	if s.TakeBlastRead("s1", name) {
		t.Error("the second take = true, want false: one read per package and log")
	}
	if !s.TakeBlastRead("s1", FileBlastReadPrefix+"fedcba9876543210") {
		t.Error("another name's first take = false, want true")
	}
	s.ReturnBlastRead("s1", name)
	if !s.TakeBlastRead("s1", name) {
		t.Error("a take after the return = false, want true")
	}
	for _, bad := range []string{FileBlastReadPrefix + "0123456789ABCDEF", FileBlastReadPrefix + "0123", "../" + FileBlastReadPrefix + "0123456789abcdef",
		FileBlastReadPrefix + "0123456789abcdef0", "records.ndjson"} {
		if IsBlastReadFile(bad) || s.TakeBlastRead("s1", bad) {
			t.Errorf("%q was taken as a marker", bad)
		}
	}
}
