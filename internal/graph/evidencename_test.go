package graph

import (
	"testing"
	"time"
)

// TestGraph_EvidenceNameRoundTrip (W-M2.11 1d): every name the writer can
// build satisfies the predicate purge reads, a plain UUID pair keeps
// today's name byte for byte, and the near misses a person's files make do
// not satisfy it. IsSessionLogName reads sessionLogFile the same way.
func TestGraph_EvidenceNameRoundTrip(t *testing.T) {
	now := time.Date(2026, 11, 11, 17, 5, 32, 123e6, time.UTC)
	const s, p = "0b6a9c52-1111-4222-8333-444455556666", "7c1d2e3f-aaaa-4bbb-8ccc-ddddeeeeffff"
	if got := EvidenceFileName(s, p, now); got != s+"-"+p+".txt" {
		t.Errorf("a plain pair changed its name: %q", got)
	}
	if got := EvidenceFileName(s, "", now); got != s+"-20261111T170532.123.txt" {
		t.Errorf("the stamp name = %q", got)
	}
	for _, id := range []string{s, "s1", "", "/../../escaped", "../../escaped", "a/b", ".", "..", ".hidden", "-dash", "a b", "x-0123456789abcdef0123456789abcdef", string(make([]byte, 300))} {
		for _, pid := range []string{p, "p1", "", "/../..", "a/b", ".x", "-"} {
			if n := EvidenceFileName(id, pid, now); !IsEvidenceFileName(n) {
				t.Errorf("EvidenceFileName(%q, %q) = %q fails IsEvidenceFileName", id, pid, n)
			}
		}
	}
	for _, n := range []string{"notes.txt", "my-notes.md", ".x-y.txt", "a/b-c.txt", "-.txt", "x-.txt", "", ".txt", "a-b.TXT", "a-b.txt.bak", "a-.txt", "-b.txt", "a b-c.txt", "a-20261111T170532.12.txt/"} {
		if IsEvidenceFileName(n) {
			t.Errorf("IsEvidenceFileName(%q) = true", n)
		}
	}
	for _, rid := range []string{"", "github.com/o/r", "r"} {
		if n := sessionLogFile(rid); !IsSessionLogName(n) {
			t.Errorf("sessionLogFile(%q) = %q fails IsSessionLogName", rid, n)
		}
	}
	for _, n := range []string{"", ".jsonl", "0123456789abcdef.json", "0123456789ABCDEF.jsonl", "0123456789abcde.jsonl", "0123456789abcdef0.jsonl", "0123456789abcdeg.jsonl", "notes.jsonl", "a/0123456789abcdef.jsonl"} {
		if IsSessionLogName(n) {
			t.Errorf("IsSessionLogName(%q) = true", n)
		}
	}
}
