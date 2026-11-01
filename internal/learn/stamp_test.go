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

// TestLRN17w_WithdrawSendsALessonBackToProposed (#156 D3): Withdraw takes an
// approved lesson back to proposed with no stamp, so it leaves active.json
// and Pick never sends it, and approval takes it again. A proposed lesson
// and an unknown id have nothing to withdraw.
func TestLRN17w_WithdrawSendsALessonBackToProposed(t *testing.T) {
	root := t.TempDir()
	l := open(t, root, t0)
	importText(t, l, twoLessons)
	if err := l.Withdraw("npm-ci"); err == nil || !strings.Contains(err.Error(), "nothing to withdraw") {
		t.Errorf("withdrawing a proposed lesson: %v; want nothing to withdraw", err)
	}
	if err := l.Withdraw("absent"); err == nil {
		t.Error("withdrawing an unknown lesson succeeded")
	}
	if err := l.Approve("npm-ci", t0); err != nil {
		t.Fatal(err)
	}
	if err := l.Save(); err != nil {
		t.Fatal(err)
	}
	if n := len(activeOf(t, root).Lessons); n != 1 {
		t.Fatalf("premise: active.json holds %d lessons, want the approved one", n)
	}
	if err := l.Withdraw("npm-ci"); err != nil {
		t.Fatal(err)
	}
	if err := l.Save(); err != nil {
		t.Fatal(err)
	}
	l = open(t, root, t1)
	if ls := lessonOf(t, l, "npm-ci"); ls.Tier != TierProposed || ls.ApprovedUnixMS != 0 || l.Quarantined {
		t.Errorf("after a withdrawal: npm-ci %s at %d, quarantined %v; want proposed with no stamp", ls.Tier, ls.ApprovedUnixMS, l.Quarantined)
	}
	if n := len(activeOf(t, root).Lessons); n != 0 {
		t.Errorf("active.json still holds %d lessons after the withdrawal", n)
	}
	if err := l.Approve("npm-ci", t1); err != nil {
		t.Errorf("approving again after a withdrawal: %v", err)
	}
}
