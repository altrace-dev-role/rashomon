package store

import (
	"fmt"
	"os"
	"sync"
	"testing"
)

func kindStore(t *testing.T) *Store {
	t.Helper()
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(s.RunDir("s1"), 0o700); err != nil {
		t.Fatal(err)
	}
	return s
}

// TestLANGREG1_AKindNoticeIsOncePerKindUpToSixteen: the unclaimed kind's
// marker (card LANG-REG1) is taken once per session and kind for 16
// kinds; the 17th kind takes the one marker that names no kind, and the 18th
// takes nothing. Break: take without O_EXCL, drop the limit, or count the
// other-kinds marker among the kinds, and this fails.
func TestLANGREG1_AKindNoticeIsOncePerKindUpToSixteen(t *testing.T) {
	s := kindStore(t)
	for i := 0; i < kindNoticeLimit; i++ {
		key := fmt.Sprintf(".k%d", i)
		if got := s.TakeKindNotice("s1", key); got != KindNoticeName(key) {
			t.Fatalf("kind %d's first take = %q, want its own marker %q", i, got, KindNoticeName(key))
		}
		if got := s.TakeKindNotice("s1", key); got != "" {
			t.Errorf("kind %d's second take = %q, want none: once per kind", i, got)
		}
	}
	if got := s.TakeKindNotice("s1", ".k16"); got != FileNoticeOtherKinds {
		t.Errorf("the 17th kind's take = %q, want %q", got, FileNoticeOtherKinds)
	}
	if got := s.TakeKindNotice("s1", ".k17"); got != "" {
		t.Errorf("the 18th kind's take = %q, want none: the other kinds are said once", got)
	}
	if got := s.TakeKindNotice("s1", ".k16"); got != "" {
		t.Errorf("the 17th kind's second take = %q, want none", got)
	}
	if got := s.TakeKindNotice("s1", ".k0"); got != "" {
		t.Errorf("the first kind's take after the limit = %q, want none", got)
	}
	if n, ok := s.kindNotices("s1"); !ok || n != kindNoticeLimit {
		t.Errorf("kind markers = %d (read %v), want %d", n, ok, kindNoticeLimit)
	}
}

// TestLANGREG1_AReturnedKindNoticeIsTakenAgain: a post that took a marker
// and could not print its line gives it back, and the kind's next edit takes
// it again; the other-kinds marker returns the same way, and a kind under
// the limit takes its own marker though a key the hook would not print took
// the other-kinds one first. Break: leave either name out of isNotice, so
// ReturnNotice removes nothing, or let the other-kinds marker silence every
// later kind, and this fails.
func TestLANGREG1_AReturnedKindNoticeIsTakenAgain(t *testing.T) {
	s := kindStore(t)
	name := s.TakeKindNotice("s1", ".rs")
	if name != KindNoticeName(".rs") {
		t.Fatalf("the first take = %q, want %q", name, KindNoticeName(".rs"))
	}
	s.ReturnNotice("s1", name)
	if got := s.TakeKindNotice("s1", ".rs"); got != name {
		t.Errorf("a take after the return = %q, want %q", got, name)
	}
	if !s.TakeNotice("s1", FileNoticeOtherKinds) {
		t.Fatal("the other-kinds marker's first take = false, want true")
	}
	if got := s.TakeKindNotice("s1", ".go2"); got != KindNoticeName(".go2") {
		t.Errorf("a kind under the limit after the other-kinds take = %q, want its own marker", got)
	}
	s.ReturnNotice("s1", FileNoticeOtherKinds)
	if !s.TakeNotice("s1", FileNoticeOtherKinds) {
		t.Error("the other-kinds marker's take after the return = false, want true")
	}
}

// TestLANGREG1_AKindMarkerNameIsADigestOfTheKey: a marker's name is the
// prefix and the first 16 hex digits of the key's sha256, whatever bytes the
// key holds, and the other-kinds marker is outside the family. Break: put
// the key in the name, change the digest, or let the other-kinds name match
// the family, and this fails.
func TestLANGREG1_AKindMarkerNameIsADigestOfTheKey(t *testing.T) {
	if got, want := KindNoticeName(".py"), FileNoticeKindPrefix+"8a702c2572f9aa05"; got != want {
		t.Errorf("KindNoticeName(.py) = %q, want %q", got, want)
	}
	if got, want := KindNoticeName("Makefile"), FileNoticeKindPrefix+"76ed074a9305c040"; got != want {
		t.Errorf("KindNoticeName(Makefile) = %q, want %q", got, want)
	}
	for _, key := range []string{".py", "../../x", ".a\x1b[31m", "a/b", ""} {
		if name := KindNoticeName(key); !IsKindNoticeFile(name) {
			t.Errorf("KindNoticeName(%q) = %q, not a marker name", key, name)
		}
	}
	for _, bad := range []string{FileNoticeOtherKinds, FileNoticeKindPrefix + "0123456789ABCDEF", FileNoticeKindPrefix + "0123",
		FileNoticeKindPrefix + "0123456789abcdef0", FileBlastReadPrefix + "0123456789abcdef", "../" + FileNoticeKindPrefix + "0123456789abcdef"} {
		if IsKindNoticeFile(bad) {
			t.Errorf("%q reads as a kind marker", bad)
		}
	}
}

// TestLANGREG1_RacingKindsNeverPassTheLimit: posts that take new kinds at
// once leave at most 16 kind markers, each one a kind that took its own, and
// the line naming no kind is taken once. Break: drop the count after the
// create, or the removal of a marker past the limit, and this fails.
func TestLANGREG1_RacingKindsNeverPassTheLimit(t *testing.T) {
	s := kindStore(t)
	const kinds = 2 * kindNoticeLimit
	got := make([]string, kinds)
	var wg sync.WaitGroup
	for i := 0; i < kinds; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			got[i] = s.TakeKindNotice("s1", fmt.Sprintf(".k%d", i))
		}(i)
	}
	wg.Wait()
	own, other := 0, 0
	for i, g := range got {
		switch g {
		case "":
		case FileNoticeOtherKinds:
			other++
		case KindNoticeName(fmt.Sprintf(".k%d", i)):
			own++
		default:
			t.Errorf("kind %d took %q", i, g)
		}
	}
	n, ok := s.kindNotices("s1")
	if !ok || n != own || own > kindNoticeLimit || other != 1 {
		t.Errorf("markers %d (read %v), own takes %d, other-kinds takes %d; want markers equal to own takes, at most %d, and one other-kinds take",
			n, ok, own, other, kindNoticeLimit)
	}
}
