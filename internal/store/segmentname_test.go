package store

import "testing"

// TestIsSegment_IsExactlyWhatSegmentReturns: every value Segment returns,
// plain or digested, satisfies IsSegment; a path or an empty name does
// not. A digest-shaped name with the wrong length or case is a plain
// segment (the allowlist holds "x", "-" and hex), so Segment returns it
// verbatim and IsSegment must accept it.
func TestIsSegment_IsExactlyWhatSegmentReturns(t *testing.T) {
	for _, id := range []string{"s1", "0b6a9c52-1111-4222-8333-444455556666", "", ".", "..", "/../x", "a/b", "a b", string(make([]byte, 200))} {
		if got := Segment(id); !IsSegment(got) {
			t.Errorf("Segment(%q) = %q fails IsSegment", id, got)
		}
	}
	for _, n := range []string{"", ".", "..", "a/b"} {
		if IsSegment(n) {
			t.Errorf("IsSegment(%q) = true", n)
		}
	}
}
