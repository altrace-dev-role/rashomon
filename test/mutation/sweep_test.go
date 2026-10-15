//go:build !windows

// The mutation sweep's own tests: the acceptance tests of DEL-1, DEL-2 and
// DEL-17. sweep.py rewrites tracked source files in place and restores them
// afterwards, so these tests drive its read-only modes (--list, --check)
// against the real tree and against fixtures, and read the workflow files
// directly. TestStrict_ is the one exception: it runs the full sweep, but
// only against a throwaway module of its own under a TempDir, never this tree.
package mutation

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// sweep runs sweep.py with dir as the working directory and returns its stdout
// and exit status. Thirty seconds is a generous bound for the read-only modes,
// which exec a few dozen small files and read their targets.
//
// On timeout the whole process group gets SIGTERM, never SIGKILL. The sweep
// backs tracked files up in memory and restores them from a signal handler;
// SIGKILL skips that handler and leaves a deliberately broken source file in
// the tree, the one residue the script says it cannot prevent. The group
// matters too: a `go test ./...` the sweep may have started would otherwise
// outlive it.
func sweep(t *testing.T, dir string, args ...string) (string, int) {
	t.Helper()
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("python3 is not on PATH; the sweep cannot run here")
	}
	script, err := filepath.Abs("sweep.py")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, python, append([]string{script}, args...)...)
	cmd.Dir = dir
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM) }
	cmd.WaitDelay = 5 * time.Second
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err = cmd.Run()
	if ctx.Err() != nil {
		t.Fatalf("sweep.py %s did not finish within 30s; stderr:\n%s", strings.Join(args, " "), stderr.String())
	}
	// A bad mutant file is a reported problem, never an unhandled Python
	// exception: a traceback here means a malformed input reached code that
	// assumed it was well-formed, which is the defect itself, not evidence
	// that the input was rejected.
	if strings.Contains(stderr.String(), "Traceback (most recent call last)") {
		t.Fatalf("sweep.py %s crashed with an unhandled exception instead of reporting a problem:\n%s",
			strings.Join(args, " "), stderr.String())
	}
	if err != nil {
		var exit *exec.ExitError
		if !errors.As(err, &exit) {
			t.Fatalf("sweep.py %s: %v", strings.Join(args, " "), err)
		}
		t.Logf("sweep.py %s exited %d; stderr:\n%s", strings.Join(args, " "), exit.ExitCode(), stderr.String())
		return stdout.String(), exit.ExitCode()
	}
	return stdout.String(), 0
}

func moduleRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	return root
}

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// mutant renders one m(...) call, and importEntry the IMPORTS assignment a
// break with an import edit adds beside it. Go's %q and Python's double-quoted
// string share the escapes these fixtures need.
func mutant(name, file, old, new, test string) string {
	return fmt.Sprintf("m(%q, %q, %q, %q, %q)\n", name, file, old, new, test)
}

func importEntry(name, file, old, new string) string {
	return fmt.Sprintf("IMPORTS[%q] = (%q, %q, %q)\n", name, file, old, new)
}

func nonEmptyLines(s string) []string {
	var out []string
	for _, ln := range strings.Split(s, "\n") {
		if ln != "" {
			out = append(out, ln)
		}
	}
	return out
}

// DEL-1. --list is the concatenation, in sorted file order, of what each file
// under mutants/ declares, and no name repeats. Names are counted rather than
// m( calls, deliberately: one call may declare several mutants from a loop, so
// a count of calls is wrong in both directions.
func TestDEL1_SweepLoadsEveryMutantFileOnce(t *testing.T) {
	root := moduleRoot(t)
	out, code := sweep(t, root, "--list")
	if code != 0 {
		t.Fatalf("--list exited %d", code)
	}
	full := nonEmptyLines(out)
	if len(full) == 0 {
		t.Fatal("--list printed no mutant")
	}
	seen := map[string]int{}
	for i, name := range full {
		if j, ok := seen[name]; ok {
			t.Errorf("name %q is listed twice, at %d and %d", name, j+1, i+1)
		}
		seen[name] = i
	}

	files, err := filepath.Glob("mutants/*.py")
	if err != nil {
		t.Fatal(err)
	}
	if len(files) == 0 {
		t.Fatal("no files under mutants/")
	}
	sort.Strings(files)
	var joined []string
	for _, f := range files {
		out, code := sweep(t, root, "--list", "--files", f)
		if code != 0 {
			t.Errorf("--list --files %s exited %d", f, code)
			continue
		}
		names := nonEmptyLines(out)
		if len(names) == 0 {
			t.Errorf("%s declares no mutant", f)
		}
		joined = append(joined, names...)
	}
	if strings.Join(joined, "\n") != strings.Join(full, "\n") {
		t.Errorf("--list (%d names) is not the per-file lists in sorted order (%d names from %d files)",
			len(full), len(joined), len(files))
	}
}

// DEL-1, the import edits. An IMPORTS entry is declared beside the mutant it
// belongs to: each file loads into a namespace of its own, and an entry whose
// mutant another file declares, or that no file declares, is refused rather
// than merged, so no file can overwrite another's import edit.
func TestDEL1b_ImportEntryDeclaredBesideItsMutant(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "target.go"), "package x\n\nimport (\n\t\"fmt\"\n)\n\nfunc a() int { return 1 }\n")
	const name = "fixture mutant with an import edit"
	declaring := mutant(name, "target.go", "return 1", "return 2", "TestFixture_")
	entry := importEntry(name, "target.go", "\t\"fmt\"\n", "\t\"fmt\"\n\t\"os\"\n")
	other := mutant("fixture other mutant", "target.go", "func a", "func z", "TestFixture_")
	cases := []struct {
		name   string
		files  map[string]string
		wantOK bool
	}{
		{"beside its mutant", map[string]string{"a.py": declaring + entry}, true},
		{"in another file", map[string]string{"a.py": declaring, "b.py": other + entry}, false},
		{"for a mutant no file declares", map[string]string{"b.py": other + entry}, false},
		{"IMPORTS rebound instead of updated in place", map[string]string{
			"a.py": declaring + fmt.Sprintf("IMPORTS = {%q: (%q, %q, %q)}\n", name, "target.go", "\t\"fmt\"\n", "\t\"fmt\"\n\t\"os\"\n"),
		}, false},
		{"IMPORTS entry is not a 3-tuple", map[string]string{
			"a.py": declaring + fmt.Sprintf("IMPORTS[%q] = (%q, %q)\n", name, "target.go", "\t\"fmt\"\n"),
		}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fixtures := t.TempDir()
			args := []string{"--check"}
			for _, f := range []string{"a.py", "b.py"} {
				if text, ok := tc.files[f]; ok {
					path := filepath.Join(fixtures, f)
					write(t, path, text)
					args = append(args, "--files", path)
				}
			}
			_, code := sweep(t, dir, args...)
			if (code == 0) != tc.wantOK {
				t.Errorf("--check exited %d with the import entry %s", code, tc.name)
			}
		})
	}
}

// --files is documented as relative to test/mutation, which is where a path
// typed by hand usually is -- but the sweep is run from the module root, and
// a path tab-completed from there carries the test/mutation/ prefix. That
// shape must resolve too.
func TestFilesPatternResolvesAgainstCurrentDirectoryFirst(t *testing.T) {
	root := moduleRoot(t)
	out, code := sweep(t, root, "--list", "--files", "test/mutation/mutants/DEL-1_per-change-files.py")
	if code != 0 {
		t.Fatalf("--files with a module-root-relative path exited %d", code)
	}
	if len(nonEmptyLines(out)) == 0 {
		t.Fatal("--files with a module-root-relative path listed no mutant")
	}
}

// DEL-2. --check refuses an anchor that occurs other than exactly once in its
// target, and an import edit's anchor likewise: the sweep applies each by
// replacing its first occurrence, so a repeated anchor mutates whichever site
// comes first and the verdict is about a break nobody wrote.
func TestDEL2_NewAnchorsAreUnique(t *testing.T) {
	dir := t.TempDir()
	// The duplicate anchor used for the import-anchor cases, "return 1", is
	// kept out of the function the main mutation targets: check() now counts
	// an import anchor against the text AFTER the main mutation is applied,
	// so an anchor that only duplicated inside the mutated span would read as
	// unique once that span is gone, which is not what these cases test.
	write(t, filepath.Join(dir, "target.go"), "package x\n\nimport (\n\t\"fmt\"\n)\n\nfunc a() int { return 1 }\n\nfunc b() int { return 1 }\n\nfunc c() int { return 9 }\n")
	const unique = "func c() int { return 9 }"
	cases := []struct {
		name, old, importAnchor string
		wantOK                  bool
	}{
		{"anchor occurs twice", "return 1", "", false},
		{"anchor occurs once", unique, "", true},
		{"anchor occurs never", "return 42", "", false},
		{"import anchor occurs once", unique, "\t\"fmt\"\n", true},
		{"import anchor occurs twice", unique, "return 1", false},
		{"import anchor occurs never", unique, "\t\"nope\"\n", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			name := "fixture " + tc.name
			text := mutant(name, "target.go", tc.old, "mutated", "TestFixture_")
			if tc.importAnchor != "" {
				text += importEntry(name, "target.go", tc.importAnchor, tc.importAnchor+"\t\"os\"\n")
			}
			file := filepath.Join(t.TempDir(), "fixture.py")
			write(t, file, text)
			_, code := sweep(t, dir, "--check", "--files", file)
			if (code == 0) != tc.wantOK {
				t.Errorf("--check exited %d when the %s", code, tc.name)
			}
		})
	}
}

// check() must count an import anchor in the text the sweep actually sees,
// which is AFTER the main mutation is applied to that file -- not in the
// pristine file, which is what check() reads by default. Here the anchor is
// duplicated in the pristine file, once inside the span the main mutation
// replaces, so the mutated file carries exactly one copy and the import edit
// is well-formed even though a pristine-only count would call it a duplicate.
func TestDEL2d_ImportAnchorCountedAfterMutation(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "target.go"),
		"package x\n\nimport (\n\t\"fmt\"\n)\n\n"+
			"func a() int { fmt.Println(\"x\"); return 1 }\n\n"+
			"func b() int { fmt.Println(\"x\"); return 2 }\n")
	name := "fixture import anchor duplicated only inside the mutated span"
	text := mutant(name, "target.go",
		"func a() int { fmt.Println(\"x\"); return 1 }", "func a() int { return 1 }", "TestFixture_")
	text += importEntry(name, "target.go", "fmt.Println(\"x\")", "fmt.Println(\"x again\")")
	file := filepath.Join(t.TempDir(), "fixture.py")
	write(t, file, text)
	_, code := sweep(t, dir, "--check", "--files", file)
	if code != 0 {
		t.Errorf("--check exited %d for an import anchor duplicated only inside the mutated span", code)
	}
}

// DEL-2, the legacy list. It may only shrink, and it started empty because
// the four duplicate anchors that existed were pinned instead; so it stays
// empty, and a pin is the only way past --check for a repeated anchor.
func TestDEL2c_LegacyListStaysEmpty(t *testing.T) {
	data, err := os.ReadFile("legacy-duplicate-anchors.txt")
	if err != nil {
		t.Fatal(err)
	}
	for i, ln := range strings.Split(string(data), "\n") {
		if s := strings.TrimSpace(ln); s != "" && !strings.HasPrefix(s, "#") {
			t.Errorf("legacy-duplicate-anchors.txt:%d: %q; pin the anchor instead", i+1, s)
		}
	}
}

// DEL-2, second half. A mutant file named <PREFIX>-<n>_... claims that
// requirement row, and the tests it names must pin the same row -- a suffix
// letter (TestREC56b_) pins its parent -- unless shared-tests.txt declares the
// pair. Names that predate the rows (TestH15_) claim nothing, and neither does
// a file that is not named for a row.
func TestDEL2b_TestNameMatchesItsRow(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "target.go"), "package x\n\nfunc a() int { return 1 }\n")
	cases := []struct {
		file, test string
		wantOK     bool
	}{
		{"REC-56_fixture.py", "TestREC49b_", false},
		{"REC-56_fixture.py", "TestREC56_", true},
		{"REC-56_fixture.py", "TestREC56b_|TestREC56_Other", true},
		{"REC-56_fixture.py", "TestH15_", true},
		{"REC-56_fixture.py", "TestREC4[0-9]_", false},
		{"REC-56_fixture.py", "^TestREC49b$", false},
		{"REC-56_fixture.py", "TestREC49b", false},
		{"STR-19_fixture.py", "TestSTR82_", true},
		{"STR-19_fixture.py", "TestSTR81_", false},
		{"H-15_fixture.py", "TestREC49b_", true},
	}
	for _, tc := range cases {
		t.Run(tc.file+" "+tc.test, func(t *testing.T) {
			file := filepath.Join(t.TempDir(), tc.file)
			write(t, file, mutant("fixture "+tc.file+" "+tc.test, "target.go", "return 1", "return 2", tc.test))
			_, code := sweep(t, dir, "--check", "--files", file)
			if (code == 0) != tc.wantOK {
				t.Errorf("--check exited %d for %s naming %s", code, tc.file, tc.test)
			}
		})
	}
}

// check() only validates a mutant's judging test when the whole set was
// loaded (no --files), the same signal DEL-2's legacy-entry check uses,
// because a --files fixture directory carries no real *_test.go and every
// mutant's test name would otherwise fail to match anything that exists.
func TestCheckValidatesJudgingTestExists(t *testing.T) {
	cases := []struct {
		name, test string
		wantOK     bool
	}{
		{"names a real test", "TestReal_Something", true},
		{"does not compile", "Test(", false},
		{"compiles but matches nothing real", "TestNoSuchThing_", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			write(t, filepath.Join(dir, "target.go"), "package x\n\nfunc a() int { return 1 }\n")
			write(t, filepath.Join(dir, "real_test.go"),
				"package x\n\nimport \"testing\"\n\nfunc TestReal_Something(t *testing.T) {}\n")
			mdir := filepath.Join(dir, "mutants")
			if err := os.MkdirAll(mdir, 0o755); err != nil {
				t.Fatal(err)
			}
			write(t, filepath.Join(mdir, "x.py"), mutant("fixture "+tc.name, "target.go", "return 1", "return 2", tc.test))
			_, code := sweep(t, dir, "--check")
			if (code == 0) != tc.wantOK {
				t.Errorf("--check exited %d for test regex %q", code, tc.test)
			}
		})
	}
}

// DEL-17. Every job in every workflow carries timeout-minutes. One run in
// October 2026 lasted 1,212 minutes; a job without a bound holds a runner for
// as long as GitHub's six-hour default allows, and the sweep job in particular
// has nothing else to stop it.
func TestDEL17_EveryJobHasATimeout(t *testing.T) {
	root := moduleRoot(t)
	workflows, err := filepath.Glob(filepath.Join(root, ".github", "workflows", "*.y*ml"))
	if err != nil {
		t.Fatal(err)
	}
	if len(workflows) == 0 {
		t.Fatal("no workflow files under .github/workflows")
	}
	for _, wf := range workflows {
		data, err := os.ReadFile(wf)
		if err != nil {
			t.Fatal(err)
		}
		jobs, missing := jobsAndMissingTimeouts(string(data))
		if len(jobs) == 0 {
			t.Errorf("%s: no jobs found", filepath.Base(wf))
		}
		for _, j := range missing {
			t.Errorf("%s: job %q has no timeout-minutes", filepath.Base(wf), j)
		}
	}
}

// DEL-17, second half. A job key is still a job key with a trailing comment,
// and a job whose bound is written as 0 has no bound at all. Separately, a
// `jobs:` line with its own trailing comment is not matched either, so the
// whole block is missed and the caller sees zero jobs -- which is why
// TestDEL17_ treats "no jobs found" as its own failure, not a silent pass.
func TestDEL17b_TimeoutParserFailsClosed(t *testing.T) {
	cases := []struct {
		name    string
		doc     string
		jobs    []string
		missing []string
	}{
		{
			name:    "commented job key is still a job",
			doc:     "jobs:\n  test:\n    timeout-minutes: 30\n  sweep:  # on demand\n    runs-on: x\n",
			jobs:    []string{"test", "sweep"},
			missing: []string{"sweep"},
		},
		{
			name:    "a zero bound is no bound",
			doc:     "jobs:\n  foo:\n    timeout-minutes: 0\n",
			jobs:    []string{"foo"},
			missing: []string{"foo"},
		},
		{
			name:    "a commented jobs line is not recognised at all",
			doc:     "jobs:  # c\n  foo:\n    timeout-minutes: 30\n",
			jobs:    nil,
			missing: nil,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			jobs, missing := jobsAndMissingTimeouts(tc.doc)
			if !slicesEqual(jobs, tc.jobs) {
				t.Errorf("jobs = %v, want %v", jobs, tc.jobs)
			}
			if !slicesEqual(missing, tc.missing) {
				t.Errorf("missing = %v, want %v", missing, tc.missing)
			}
		})
	}
}

func slicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// --strict. Without it, a mutant whose judging test SKIPPED here exits 0:
// correct on a laptop missing the tooling H-17's socket half needs. With it,
// the same skip fails the run, which is what the CI sweep job needs so a
// runner image that stops granting that tooling goes red instead of green.
// This is the one test that runs the real sweep, against a throwaway module
// of its own, never this tree.
func TestStrict_NotJudgedFailsOnlyWithTheFlag(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "go.mod"), "module fixture\n\ngo 1.21\n")
	write(t, filepath.Join(root, "target.go"), "package fixture\n\nfunc A() int { return 1 }\n")
	write(t, filepath.Join(root, "fixture_test.go"),
		"package fixture\n\nimport \"testing\"\n\nfunc TestFixtureA_(t *testing.T) { t.Skip(\"not available here\") }\n")
	if err := os.MkdirAll(filepath.Join(root, "mutants"), 0o755); err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(root, "mutants", "a.py"),
		mutant("mutant whose judging test always skips", "target.go", "return 1", "return 2", "TestFixtureA_"))

	if out, code := sweep(t, root); code != 0 {
		t.Errorf("without --strict, a NOT JUDGED mutant exited %d:\n%s", code, out)
	}
	out, code := sweep(t, root, "--strict")
	if code == 0 {
		t.Errorf("--strict exited 0 with a NOT JUDGED mutant:\n%s", out)
	}
	if !strings.Contains(out, "not judged here") {
		t.Errorf("--strict output does not mention the skipped mutant:\n%s", out)
	}
}

// jobsAndMissingTimeouts reads a workflow by its lines: a job is a key at two
// spaces of indent under `jobs:`, and its timeout is a `timeout-minutes:` line
// at four spaces with a positive integer. No YAML parser, deliberately: the
// module takes no dependency for a test, and the files are written by hand in
// exactly this shape.
func jobsAndMissingTimeouts(doc string) (jobs, missing []string) {
	jobLine := regexp.MustCompile(`^  ([A-Za-z0-9_-]+):\s*(#.*)?$`)
	timeoutLine := regexp.MustCompile(`^    timeout-minutes:\s*([0-9]+)\s*(#.*)?$`)
	inJobs := false
	current := ""
	timed := map[string]bool{}
	for _, ln := range strings.Split(doc, "\n") {
		if ln == "jobs:" {
			inJobs = true
			continue
		}
		if !inJobs {
			continue
		}
		if strings.TrimSpace(ln) != "" && !strings.HasPrefix(ln, " ") && !strings.HasPrefix(ln, "#") {
			inJobs = false // another top-level key ends the jobs block
			continue
		}
		if m := jobLine.FindStringSubmatch(ln); m != nil {
			current = m[1]
			jobs = append(jobs, current)
			continue
		}
		if m := timeoutLine.FindStringSubmatch(ln); m != nil && current != "" {
			if n, err := strconv.Atoi(m[1]); err == nil && n > 0 {
				timed[current] = true
			}
		}
	}
	for _, j := range jobs {
		if !timed[j] {
			missing = append(missing, j)
		}
	}
	return jobs, missing
}
