package session

import "testing"

// TestKnow_DeliveredIsSentNotDemonstrated: contracts-v2 section 4.2.
// Delivery writes six records in state sent through the context channel
// and none demonstrated; the writes that complete a line's predicate
// demonstrate it by that act; a boundary moves every record to lost and
// keeps the demonstrating act. The 11-04a record had held|lost only, so
// delivery counted as knowledge.
func TestKnow_DeliveredIsSentNotDemonstrated(t *testing.T) {
	f := newFixture(t, retention{})
	if n := len(f.Knows); n != 6 {
		t.Fatalf("know after delivery = %d, want 6", n)
	}
	for _, k := range f.Knows {
		if k.Via != ViaContext || k.State != Sent || k.AtSeq != 5 || k.Rev != "C1" {
			t.Fatalf("after delivery: %+v", k)
		}
	}
	if n := len(f.KnowsIn(Demonstrated)); n != 0 {
		t.Fatalf("demonstrated after delivery = %d, want 0", n)
	}
	f.reads()
	if n := len(f.KnowsIn(Sent)); n != 9 {
		t.Fatalf("sent after three reads = %d, want 9", n)
	}
	f.edit(t, 12, sessionsPy, oldCreate, newCreate)
	if n := len(f.KnowsIn(Demonstrated)); n != 0 {
		t.Fatalf("one of three files written demonstrated a line: %d", n)
	}
	f.edit(t, 14, cachePy, "ex=3600)", "ex=ttl)")
	f.edit(t, 16, purgePy, oldPurge, newPurge)
	dem := f.KnowsIn(Demonstrated)
	if len(dem) != 2 || dem[0].Fact != line1 || dem[1].Fact != line2 || dem[0].By != 16 || dem[1].By != 16 {
		t.Fatalf("demonstrated after the purge edit = %+v, want lines 1 and 2 by act 16", dem)
	}
	f.MarkBoundary(17)
	if n := len(f.KnowsIn(Demonstrated)); n != 0 {
		t.Fatalf("demonstrated must not survive a boundary; got %d", n)
	}
	k := f.KnowFor(line1)
	if k.State != Lost || k.By != 16 {
		t.Fatalf("lost must keep the demonstrating act: %+v", k)
	}
	f.Deliver(18, line1)
	if k := f.Knows[len(f.Knows)-1]; k.Fact != line1 || k.State != Sent || k.AtSeq != 18 || k.Rev != "C1+ov16" {
		t.Fatalf("re-delivery after the boundary = %+v", k)
	}
}
