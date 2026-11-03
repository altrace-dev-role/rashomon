package intent

import (
	"bytes"
	"testing"
)

// TestCSL1_SaidDigestIsKeyedAndApartFromPaths: a quote's digest is 64
// lowercase hex under the install key, differs under another key, and
// differs from PathDigest of the same string under the same key (its own
// domain), so a steer line's digests of the person's words cannot be reversed
// by guessing the words without the key. Break: drop the key, share
// PathDigest's domain, or hash the words alone, and this fails.
func TestCSL1_SaidDigestIsKeyedAndApartFromPaths(t *testing.T) {
	const words = "no push without my word"
	k1, k2 := bytes.Repeat([]byte{1}, 32), bytes.Repeat([]byte{2}, 32)
	d := SaidDigest(k1, words)
	if len(d) != 64 || d != SaidDigest(k1, words) {
		t.Fatalf("SaidDigest = %q, want 64 hex, the same each call", d)
	}
	for i := 0; i < len(d); i++ {
		if c := d[i]; !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			t.Fatalf("SaidDigest = %q, want lowercase hex", d)
		}
	}
	if SaidDigest(k2, words) == d {
		t.Error("two install keys give one digest of the same words")
	}
	if SaidDigest(nil, words) == d {
		t.Error("the digest is the same with no key")
	}
	if PathDigest(k1, words) == d {
		t.Error("a quote and a path of the same string digest the same")
	}
	if SaidDigest(k1, words+".") == d {
		t.Error("two different quotes digest the same")
	}
}
