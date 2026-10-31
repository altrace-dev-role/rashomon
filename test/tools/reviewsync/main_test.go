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

const reviewedHead = "# Findings a person has read and judged.\n# <path> <fingerprint> <why>\n\n"

// A runner label the entropy test reads as a token (a curl format string, as
// test/fixtures/nono/audit-events.ndjson holds), so the basis naming it is a
// finding.
const trip = "ok1=%{http_code}&ok2=%{size_download}"

// rows is a budgets file in the tool's layout, one row to a line.
func rows(first, second string) string {
	return "{\n  \"benchmarks\": [\n" +
		"    {\"name\": \"BenchmarkA\", \"measured_bytes_per_op\": 1, \"basis\": \"" + first + "\"},\n" +
		"    {\"name\": \"BenchmarkB\", \"measured_bytes_per_op\": 2, \"basis\": \"" + second + "\"}\n  ]\n}\n"
}

// layout writes a module root: the rewritten budgets file, a reviewed.txt and
// the neutral copy, and returns the root and the neutral path.
func layout(t *testing.T, budgets, neutral, reviewed string) (string, string) {
	t.Helper()
	root := t.TempDir()
	for p, data := range map[string]string{
		filepath.Join(root, filepath.FromSlash(budgetsRel)):       budgets,
		filepath.Join(root, filepath.FromSlash(hygiene.Reviewed)): reviewed,
		filepath.Join(root, "neutral.json"):                       neutral,
	} {
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(data), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root, filepath.Join(root, "neutral.json")
}

func runSync(root, neutral string) (int, string, string) {
	var out, errb bytes.Buffer
	code := run([]string{"-root", root, "-neutral", neutral}, &out, &errb)
	return code, out.String(), errb.String()
}

func reviewed(t *testing.T, root string) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(hygiene.Reviewed)))
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

// The re-anchor changed row A's basis: the old entry is stale, the new text
// is a finding, and reviewed.txt ends with the new entry only.
func TestReviewedEndsExactlyRightAfterARewrittenBasis(t *testing.T) {
	const stale = "0123456789abcdef"
	other := "test/fixtures/other.json fedcba9876543210 an entry for another file, which is not this command's to touch"
	now := rows("re-anchored from "+trip+" image", "plain prose")
	root, neutral := layout(t, now, rows("x", "plain prose"), reviewedHead+
		other+"\n"+
		budgetsRel+" "+stale+" the old basis, which no longer matches any finding\n")
	code, out, errb := runSync(root, neutral)
	if code != 0 {
		t.Fatalf("exit %d: %s%s", code, out, errb)
	}
	want := map[string]bool{}
	for _, f := range hygiene.Scan(budgetsRel, []byte(now)) {
		want[f.Fingerprint] = true
	}
	if len(want) != 1 {
		t.Fatalf("the new basis gives %d findings, the test needs exactly 1", len(want))
	}
	got := map[string]bool{}
	kept := 0
	for _, ln := range strings.Split(strings.TrimSuffix(reviewed(t, root), "\n"), "\n") {
		f := strings.Fields(ln)
		switch {
		case strings.HasPrefix(ln, "#") || ln == "":
		case f[0] == budgetsRel:
			if f[1] == stale {
				t.Errorf("the stale entry stayed: %s", ln)
			}
			if !strings.HasSuffix(ln, " "+reason) {
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
	if len(got) != len(want) {
		t.Errorf("entries %v, findings %v", got, want)
	}
	for fp := range want {
		if !got[fp] {
			t.Errorf("finding %s has no entry", fp)
		}
	}
	if !strings.Contains(out, "1 stale") || !strings.Contains(out, "1 added") {
		t.Errorf("the run did not say what it did: %s", out)
	}
	// A second run changes nothing.
	before := reviewed(t, root)
	if code, out, errb := runSync(root, neutral); code != 0 || reviewed(t, root) != before || !strings.Contains(out, "needs no change") {
		t.Errorf("rerun exit %d: %s%s", code, out, errb)
	}
}

func TestFindingOutsideTheComposedTextRefuses(t *testing.T) {
	// Row B's basis is not text the re-anchor wrote, so it is in both copies.
	both := "ask jane.doe@gmail.com"
	root, neutral := layout(t, rows("re-anchored from "+trip+" image", both), rows("x", both), reviewedHead)
	code, out, errb := runSync(root, neutral)
	if code != 2 {
		t.Fatalf("exit %d, want 2: %s%s", code, out, errb)
	}
	if !strings.Contains(errb, "outside the text the re-anchor rewrote") || !strings.Contains(errb, "an email address") {
		t.Errorf("the refusal does not print the finding: %s", errb)
	}
	if strings.Contains(errb, "jane.doe") {
		t.Errorf("the refusal printed the value: %s", errb)
	}
	if got := reviewed(t, root); got != reviewedHead {
		t.Errorf("reviewed.txt changed despite the refusal: %s", got)
	}
	// Once a person has judged and recorded it, it no longer stops the
	// command, and the person's entry stays.
	var entry string
	for _, f := range hygiene.Scan(budgetsRel, []byte(rows("x", both))) {
		entry = budgetsRel + " " + f.Fingerprint + " a person judged it\n"
	}
	rev := filepath.Join(root, filepath.FromSlash(hygiene.Reviewed))
	if err := os.WriteFile(rev, []byte(reviewedHead+entry), 0o644); err != nil {
		t.Fatal(err)
	}
	if code, out, errb := runSync(root, neutral); code != 0 {
		t.Fatalf("exit %d with the finding excused by a person: %s%s", code, out, errb)
	}
	if got := reviewed(t, root); !strings.Contains(got, entry) || strings.Count(got, budgetsRel+" ") != 2 {
		t.Errorf("the person's entry was lost, or the wrong entries added: %s", got)
	}
}
