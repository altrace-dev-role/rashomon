package learn

import (
	"strings"
	"testing"
	"time"
)

// lessonOf is lesson id as the ledger holds it now.
func lessonOf(t *testing.T, l *Ledger, id string) Lesson {
	t.Helper()
	for _, ls := range l.Lessons() {
		if ls.ID == id {
			return ls
		}
	}
	t.Fatalf("no lesson %q", id)
	return Lesson{}
}

// TestLRN17s_AStampZeroApprovalApprovesAgain (#156 D2): a lesson a build
// before the stamp approved holds an approved tier and no stamp, so Pick
// never sends it. learn approve takes it again: the tier stays the one its
// tally gives, the stamp is set, and a second approval is refused as before.
func TestLRN17s_AStampZeroApprovalApprovesAgain(t *testing.T) {
	root := t.TempDir()
	l := open(t, root, t0)
	importText(t, l, twoLessons)
	if err := l.Approve("npm-ci", t0); err != nil {
		t.Fatal(err)
	}
	l.file.Lessons[l.index("npm-ci")].ApprovedUnixMS = 0 // as the older build wrote it
	if err := l.Save(); err != nil {
		t.Fatal(err)
	}
	l = open(t, root, t1)
	if ls := lessonOf(t, l, "npm-ci"); ls.Tier != TierProbation || ls.ApprovedUnixMS != 0 || l.Quarantined {
		t.Fatalf("premise: npm-ci %s at %d, quarantined %v; want probation with no stamp", ls.Tier, ls.ApprovedUnixMS, l.Quarantined)
	}
	at := t1.Add(5 * time.Millisecond)
	if err := l.Approve("npm-ci", at); err != nil {
		t.Fatalf("approving a lesson approved before the stamp: %v; want it stamped now", err)
	}
	if ls := lessonOf(t, l, "npm-ci"); ls.Tier != TierProbation || ls.ApprovedUnixMS != at.UnixMilli() {
		t.Errorf("after approving again: npm-ci %s at %d; want probation at %d", ls.Tier, ls.ApprovedUnixMS, at.UnixMilli())
	}
	if err := l.Approve("npm-ci", at); err == nil || !strings.Contains(err.Error(), "not proposed") {
		t.Errorf("a stamped lesson approved twice: %v; want refused as not proposed", err)
	}
}
