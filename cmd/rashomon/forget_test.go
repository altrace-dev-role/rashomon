package main

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/altrace-dev-role/rashomon/internal/baseline"
)

// TestTRU58_ForgetClearsEveryBaselineItCanWhenOneFails: a baseline that cannot
// be read is an error, and it must not stop the forget at that file. The host
// still leaves every other project's baseline, and the error is still said.
func TestTRU58_ForgetClearsEveryBaselineItCanWhenOneFails(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("permissions do not stop root")
	}
	const host = "acme-secret.internal"
	root := t.TempDir()
	first := baseline.Update(root, "/a-project", "s1", time.UnixMilli(1_000), []string{host})
	second := baseline.Update(root, "/b-project", "s2", time.UnixMilli(2_000), []string{host})
	if first.Err != nil || second.Err != nil {
		t.Fatal(first.Err, second.Err)
	}
	if first.Path >= second.Path {
		t.Fatalf("premise: the unreadable baseline must be listed first: %s, %s", first.Path, second.Path)
	}
	if err := os.Chmod(first.Path, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chmod(first.Path, 0o600); err != nil {
			t.Error(err)
		}
	})

	cleared, had, err := clearBaselines(root, host)
	if err == nil {
		t.Error("an unreadable baseline gave no error, so the forget would read as complete")
	}
	if !had || cleared != 1 {
		t.Errorf("cleared = %d, had = %v; want the readable baseline cleared", cleared, had)
	}
	body, rerr := os.ReadFile(second.Path)
	if rerr != nil {
		t.Fatal(rerr)
	}
	if strings.Contains(string(body), host) {
		t.Error("the forget stopped at the unreadable baseline and left the host in the next one")
	}
}
