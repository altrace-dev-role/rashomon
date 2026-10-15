package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/altrace-dev-role/rashomon/internal/baseline"
	"github.com/altrace-dev-role/rashomon/internal/store"
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

// TestForgetHost_PrintsTheSummaryEvenWhenClearBaselinesFails: an unreadable
// baseline stops clearBaselines with an error, but it must not swallow what
// the forget already did. The records it removed and the gaps it wrote
// happened; a caller who sees only the error cannot tell that part succeeded.
func TestForgetHost_PrintsTheSummaryEvenWhenClearBaselinesFails(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("permissions do not stop root")
	}
	const host = "acme-secret.internal"
	root := t.TempDir()
	t.Setenv("RASHOMON_HOME", root)

	st, err := store.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.AppendDeclaration(store.Declaration{
		Type: "declaration", SchemaVersion: 2, SessionID: "s1",
		ToolUseID: "t1", ToolName: "Bash", RecordedAtMS: 1_000,
		Hosts: []string{host},
	}); err != nil {
		t.Fatal(err)
	}

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

	var buf strings.Builder
	err = forgetHost(host, &buf)
	if err == nil {
		t.Fatal("premise: an unreadable baseline should fail the forget")
	}
	if !strings.Contains(buf.String(), "forgot") || !strings.Contains(buf.String(), "project baseline") {
		t.Errorf("forgetHost returned an error without printing what it already cleared:\n%s", buf.String())
	}
}

// TestForgetHost_PrintsTheSummaryEvenWhenForgetHostFails: the other half of
// the same bug. st.ForgetHost itself can fail part-way through its own runs,
// after already removing records from an earlier one, and that partial
// result must reach the summary line too, not just the baseline step after
// it.
func TestForgetHost_PrintsTheSummaryEvenWhenForgetHostFails(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("permissions do not stop root")
	}
	const host = "acme-secret.internal"
	root := t.TempDir()
	t.Setenv("RASHOMON_HOME", root)

	st, err := store.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.AppendDeclaration(store.Declaration{
		Type: "declaration", SchemaVersion: 2, SessionID: "s1",
		ToolUseID: "t1", ToolName: "Bash", RecordedAtMS: 1_000,
		Hosts: []string{host},
	}); err != nil {
		t.Fatal(err)
	}
	if err := st.AppendDeclaration(store.Declaration{
		Type: "declaration", SchemaVersion: 2, SessionID: "s2",
		ToolUseID: "t2", ToolName: "Bash", RecordedAtMS: 2_000,
		Hosts: []string{host},
	}); err != nil {
		t.Fatal(err)
	}

	// Runs() orders by directory mtime, oldest first, so ForgetHost reaches
	// s1 before s2. Pin that order explicitly rather than trusting write
	// order to produce two distinct mtimes.
	s1Dir := filepath.Join(root, "runs", "s1")
	s2Dir := filepath.Join(root, "runs", "s2")
	early := time.Unix(1_700_000_000, 0)
	late := early.Add(time.Hour)
	if err := os.Chtimes(s1Dir, early, early); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(s2Dir, late, late); err != nil {
		t.Fatal(err)
	}
	runs, err := st.Runs()
	if err != nil || len(runs) != 2 || runs[0] != "s1" || runs[1] != "s2" {
		t.Fatalf("premise: s1 before s2, got %v (%v)", runs, err)
	}

	s2Records := filepath.Join(s2Dir, store.FileRecords)
	if err := os.Chmod(s2Records, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chmod(s2Records, 0o600); err != nil {
			t.Error(err)
		}
	})

	var buf strings.Builder
	err = forgetHost(host, &buf)
	if err == nil {
		t.Fatal("premise: a run whose records cannot be opened should fail the forget")
	}
	if !strings.Contains(buf.String(), "forgot 1 record") {
		t.Errorf("forgetHost discarded the gap st.ForgetHost already made in s1 before failing on s2:\n%s", buf.String())
	}
}
