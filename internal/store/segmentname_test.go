package store

import "testing"

// TestIsSegment_IsExactlyWhatSegmentReturns: every value Segment returns,
// plain or digested, satisfies IsSegment; a path, a short digest and an
// upper-case digest do not.
func TestIsSegment_IsExactlyWhatSegmentReturns(t *testing.T) {
	for _, id := range []string{"s1", "0b6a9c52-1111-4222-8333-444455556666", "", ".", "..", "/../x", "a/b", "a b", string(make([]byte, 200))} {
		if got := Segment(id); !IsSegment(got) {
			t.Errorf("Segment(%q) = %q fails IsSegment", id, got)
		}
	}
	for _, n := range []string{"", ".", "..", "a/b", "x-", "x-0123", "x-0123456789ABCDEF0123456789ABCDEF", "x-0123456789abcdef0123456789abcdeg", "x-0123456789abcdef0123456789abcdef0"} {
		if IsSegment(n) {
			t.Errorf("IsSegment(%q) = true", n)
		}
	}
}
