package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/test/tools/hygiene"
)

const budgetsRel = "test/perf/budgets.json"

// tripRunner is a runner label the entropy test reads as a token (a curl
// format string, as test/fixtures/nono/audit-events.ndjson holds), so the
// basis that names it is a finding.
const tripRunner = "ok1=%{http_code}&ok2=%{size_download}"

const reviewedHead = "# Findings a person has read and judged.\n# <path> <fingerprint> <why>\n\n"

// moduleOf lays out a module root holding the fixture budgets file (with the
// replacements applied) and a reviewed.txt, and returns the root and the
// budgets path.
func moduleOf(t *testing.T, reviewed string, replace ...string) (string, string) {
	t.Helper()
	raw, err := os.ReadFile("testdata/budgets.json")
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	for i := 0; i < len(replace); i += 2 {
		if !strings.Contains(text, replace[i]) {
			t.Fatalf("fixture lacks %q", replace[i])
		}
		text = strings.Replace(text, replace[i], replace[i+1], 1)
	}
	root := t.TempDir()
	budgets := filepath.Join(root, filepath.FromSlash(budgetsRel))
	rev := filepath.Join(root, "test", "tools", "hygiene", "reviewed.txt")
	for p, data := range map[string]string{budgets: text, rev: reviewed} {
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(data), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root, budgets
}

// withHygiene is macos() without --no-hygiene.
func withHygiene(budgets string, extra ...string) []string {
	return append([]string{"--run", "41166204201", "--dir", "testdata/macos", "--platform", "darwin/arm64", "--budgets", budgets, "--no-generate"}, extra...)
}

func reviewedText(t *testing.T, root string) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(root, "test", "tools", "hygiene", "reviewed.txt"))
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func TestReviewedEndsExactlyRightAfterARewrittenBasis(t *testing.T) {
	const stale = "0123456789abcdef"
	other := "test/fixtures/other.json fedcba9876543210 an entry for another file, which is not the tool's to touch"
	root, budgets := moduleOf(t, reviewedHead+
		other+"\n"+
		budgetsRel+" "+stale+" the old basis, which no longer matches any finding\n")
	code, out, errb := runTool(t, withHygiene(budgets, "--rows", "RunPost/edit_314k", "--runner", tripRunner)...)
	if code != 0 {
		t.Fatalf("exit %d: %s%s", code, out, errb)
	}
	after, err := os.ReadFile(budgets)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]bool{}
	for _, f := range hygiene.Scan(budgetsRel, after) {
		want[f.Fingerprint] = true
	}
	if len(want) == 0 {
		t.Fatal("the new basis trips nothing, so this test proves nothing")
	}
	got := map[string]bool{}
	kept := 0
	for _, ln := range strings.Split(strings.TrimSuffix(reviewedText(t, root), "\n"), "\n") {
		f := strings.Fields(ln)
		switch {
		case strings.HasPrefix(ln, "#") || ln == "":
		case f[0] == budgetsRel:
			if f[1] == stale {
				t.Errorf("the stale entry stayed: %s", ln)
			}
			if !strings.HasSuffix(ln, " "+hygieneReason) {
				t.Errorf("entry without the fixed reason: %s", ln)
			}
			got[f[1]] = true
		case ln == other:
			kept++
		default:
			t.Errorf("unexpected line %q", ln)
		}
	}
	if kept != 1 {
		t.Errorf("the other file's entry was kept %d times, want once", kept)
	}
	for fp := range want {
		if !got[fp] {
			t.Errorf("finding %s has no entry", fp)
		}
	}
	for fp := range got {
		if !want[fp] {
			t.Errorf("entry %s matches no finding", fp)
		}
	}
	if !strings.Contains(out, "1 stale") {
		t.Errorf("the run did not say what it dropped: %s", out)
	}
	// With the entries in place H-113's own walk of the module passes, but
	// for the one entry that excuses nothing, the other file's (line 4).
	clean := func() {
		t.Helper()
		_, found, err := hygiene.Walk(root)
		if err != nil {
			t.Fatal(err)
		}
		for _, f := range found {
			if f.Path != hygiene.Reviewed || f.Line != 4 {
				t.Errorf("H-113 would still say: %s", f)
			}
		}
	}
	clean()
	// A second run replaces the basis again (it now cites the new figures as
	// the ones it replaces), so its fingerprint changes: the first run's
	// entry goes, the new one comes, and the file is clean again.
	if code, out, errb := runTool(t, withHygiene(budgets, "--rows", "RunPost/edit_314k", "--runner", tripRunner)...); code != 0 {
		t.Fatalf("rerun exit %d: %s%s", code, out, errb)
	}
	clean()
	if n := strings.Count(reviewedText(t, root), budgetsRel+" "); n != 1 {
		t.Errorf("%d entries for the file after a rerun, want 1", n)
	}
}

func TestNoHygieneLeavesReviewedAlone(t *testing.T) {
	text := reviewedHead + budgetsRel + " 0123456789abcdef stale\n"
	root, budgets := moduleOf(t, text)
	if code, out, errb := runTool(t, withHygiene(budgets, "--rows", "RunPost/edit_314k", "--no-hygiene")...); code != 0 {
		t.Fatalf("exit %d: %s%s", code, out, errb)
	}
	if got := reviewedText(t, root); got != text {
		t.Errorf("reviewed.txt changed:\n%s", got)
	}
}

func TestFindingOutsideTheRewrittenRowsRefuses(t *testing.T) {
	// The parallel block's basis is not text this run rewrites.
	root, budgets := moduleOf(t, reviewedHead,
		`"basis": "fixture"}`, `"basis": "ask jane.doe@gmail.com"}`)
	before, _ := os.ReadFile(budgets)
	code, out, errb := runTool(t, withHygiene(budgets, "--rows", "RunPost/edit_314k", "--runner", tripRunner)...)
	if code != 2 {
		t.Fatalf("exit %d, want 2: %s%s", code, out, errb)
	}
	if !strings.Contains(errb, "outside the text this run rewrote") || !strings.Contains(errb, "an email address") {
		t.Errorf("the refusal does not print the finding: %s", errb)
	}
	if strings.Contains(errb, "jane.doe") {
		t.Errorf("the refusal printed the value: %s", errb)
	}
	if after, _ := os.ReadFile(budgets); !bytes.Equal(before, after) {
		t.Errorf("budgets.json was written despite the refusal")
	}
	if got := reviewedText(t, root); got != reviewedHead {
		t.Errorf("reviewed.txt changed: %s", got)
	}
	// Once a person has judged and recorded it, it no longer stops the tool,
	// and the tool keeps the person's entry.
	var entry string
	for _, f := range hygiene.Scan(budgetsRel, before) {
		entry = budgetsRel + " " + f.Fingerprint + " a person judged it\n"
	}
	rev := filepath.Join(root, "test", "tools", "hygiene", "reviewed.txt")
	if err := os.WriteFile(rev, []byte(reviewedHead+entry), 0o644); err != nil {
		t.Fatal(err)
	}
	if code, out, errb := runTool(t, withHygiene(budgets, "--rows", "RunPost/edit_314k", "--runner", tripRunner)...); code != 0 {
		t.Fatalf("exit %d with the finding excused by a person: %s%s", code, out, errb)
	}
	if got := reviewedText(t, root); !strings.Contains(got, entry) {
		t.Errorf("the person's entry was lost: %s", got)
	}
}
