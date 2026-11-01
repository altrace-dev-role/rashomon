//go:build !windows

// The mutation sweep's own tests: the acceptance tests of DEL-1, DEL-2,
// DEL-17 and DEL-21. sweep.py rewrites tracked source files in place and
// restores them afterwards, so these tests drive its read-only modes (--list, --check)
// against the real tree and against fixtures, and read the workflow files
// directly. TestStrict_ is one exception: it runs the full sweep, but only
// against a throwaway module of its own under a TempDir, never this tree.
package mutation

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
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
	return sweepRun(t, dir, 30*time.Second, nil, args...)
}

// sweepTimeout is sweep with an explicit bound. TestStrict_ is the one
// caller that needs more than 30s: it is the only test that runs the real
// sweep, which shells out to `go test ./...` twice against a throwaway
// module, and a cold build cache can make that slower than 30s has room for.
func sweepTimeout(t *testing.T, dir string, timeout time.Duration, args ...string) (string, int) {
	t.Helper()
	return sweepRun(t, dir, timeout, nil, args...)
}

// sweepEnv is sweep with extra environment entries, so a test can put a fake
// go on PATH.
func sweepEnv(t *testing.T, dir string, extraEnv []string, args ...string) (string, int) {
	t.Helper()
	return sweepRun(t, dir, 30*time.Second, extraEnv, args...)
}

// sweepRun is what sweep, sweepTimeout and sweepEnv share: an explicit bound
// and extra environment entries, either of which may be the default.
func sweepRun(t *testing.T, dir string, timeout time.Duration, extraEnv []string, args ...string) (string, int) {
	t.Helper()
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("python3 is not on PATH; the sweep cannot run here")
	}
	script, err := filepath.Abs("sweep.py")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, python, append([]string{script}, args...)...)
	cmd.Dir = dir
	if len(extraEnv) > 0 {
		cmd.Env = append(os.Environ(), extraEnv...)
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM) }
	cmd.WaitDelay = 5 * time.Second
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err = cmd.Run()
	if ctx.Err() != nil {
		t.Fatalf("sweep.py %s did not finish within %s; stderr:\n%s", strings.Join(args, " "), timeout, stderr.String())
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

// mutantOnPlatform is mutant with the platform= keyword a mutant adds when
// its judging test only means something on one GOOS, even though its target
// builds everywhere.
func mutantOnPlatform(name, file, old, new, test, platform string) string {
	return fmt.Sprintf("m(%q, %q, %q, %q, %q, platform=%q)\n", name, file, old, new, test, platform)
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

// pyStyleGlob matches filepath.Glob, except it drops any result whose base
// name starts with ".". Python's glob module does that by default and
// sweep.py relies on it; Go's does not, so a dotfile sitting in mutants/ --
// an editor lock, a stray ".DS_Store" -- is a file this test would expect
// the sweep to load and the sweep would never attempt to.
func pyStyleGlob(pattern string) ([]string, error) {
	all, err := filepath.Glob(pattern)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, f := range all {
		if !strings.HasPrefix(filepath.Base(f), ".") {
			out = append(out, f)
		}
	}
	return out, nil
}

// Python's glob skips a dotfile by default; filepath.Glob does not. A
// leftover editor lock or ".DS_Store" dropped into mutants/ would then be a
// file this suite expects the sweep to load, and one the sweep never does.
func TestPyStyleGlobDropsDotfiles(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "a.py"), "")
	write(t, filepath.Join(dir, ".#a.py"), "")
	write(t, filepath.Join(dir, ".DS_Store"), "")
	got, err := pyStyleGlob(filepath.Join(dir, "*"))
	if err != nil {
		t.Fatal(err)
	}
	want := []string{filepath.Join(dir, "a.py")}
	if !slicesEqual(got, want) {
		t.Errorf("pyStyleGlob = %v, want %v", got, want)
	}
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

	files, err := pyStyleGlob("mutants/*.py")
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

// DEL-2, second half. A mutant file named <PREFIX>-<n>_... claims that
// requirement row, and the tests it names must pin the same row -- a suffix
// letter (TestREC56b_) pins its parent -- unless shared-tests.txt declares the
// pair. Names that predate the rows (TestH15_) claim nothing, and neither does
// a file that is not named for a row.
func TestDEL2b_TestNameMatchesItsRow(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "target.go"), "package x\n\nfunc a() int { return 1 }\n")
	// A fixture, not the real shared-tests.txt: this test's own declared pair
	// (STR-19 TestSTR82_) is data for THIS test, not a project-wide claim,
	// and must not depend on what the real file happens to hold.
	sharedFixture := filepath.Join(t.TempDir(), "shared-tests.txt")
	write(t, sharedFixture, "STR-19 TestSTR82_\n")
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
		{"STR-19_fixture.py", "TestSTR82b_", true},
		{"STR-19_fixture.py", "TestSTR81_", false},
		{"H-15_fixture.py", "TestREC49b_", true},
	}
	for _, tc := range cases {
		t.Run(tc.file+" "+tc.test, func(t *testing.T) {
			file := filepath.Join(t.TempDir(), tc.file)
			write(t, file, mutant("fixture "+tc.file+" "+tc.test, "target.go", "return 1", "return 2", tc.test))
			_, code := sweep(t, dir, "--check", "--files", file, "--shared-tests", sharedFixture)
			if (code == 0) != tc.wantOK {
				t.Errorf("--check exited %d for %s naming %s", code, tc.file, tc.test)
			}
		})
	}
}

// The literal scan above finds a row token by its spelling, so a regex that
// reaches the same real test through a metacharacter wedged between its
// letters -- TestRE[C]49_, Test(REC)49_ -- names no literal "TestREC" and
// passed with no problem reported, even though `go test -run` matches it
// exactly as it would match the spelled-out form. Checked by matching the
// regex against the real test corpus, which needs the complete set (no
// --files) and a real func Test to match against.
func TestDEL2f_RowCheckMatchesRealTestsNotJustLiteralSpelling(t *testing.T) {
	cases := []struct {
		name, test string
		wantOK     bool
	}{
		{"bracket class splits the prefix", "TestRE[C]49_", false},
		{"group splits the prefix", "Test(REC)49_", false},
		{"bracket class still pins its own row", "TestRE[C]56_", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			write(t, filepath.Join(dir, "target.go"), "package x\n\nfunc a() int { return 1 }\n")
			write(t, filepath.Join(dir, "real_test.go"),
				"package x\n\nimport \"testing\"\n\n"+
					"func TestREC49_(t *testing.T) {}\n\n"+
					"func TestREC56_(t *testing.T) {}\n")
			mdir := filepath.Join(dir, "mutants")
			if err := os.MkdirAll(mdir, 0o755); err != nil {
				t.Fatal(err)
			}
			write(t, filepath.Join(mdir, "REC-56_fixture.py"),
				mutant("fixture "+tc.name, "target.go", "return 1", "return 2", tc.test))
			_, code := sweep(t, dir, "--check")
			if (code == 0) != tc.wantOK {
				t.Errorf("--check exited %d for REC-56_fixture.py naming %s", code, tc.test)
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

// workflows is every workflow file, and there must be some.
func workflows(t *testing.T, root string) []string {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(root, ".github", "workflows", "*.y*ml"))
	if err != nil {
		t.Fatal(err)
	}
	if len(files) == 0 {
		t.Fatal("no workflow files under .github/workflows")
	}
	return files
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// DEL-23. The rubric fix list is a named mutant group, constrained like a
// row by its entry id: a file named RUBRIC_<area>-<n>_<slug>.py claims that
// entry, every TestRubric<AREA><n>_ test its mutants name must be that
// entry's, and each of its mutants must name at least one, so a mutant
// cannot be pinned by another entry's test or by none of its own. A suffix
// letter (TestRubricLLD3b_) pins its parent. Other test names claim nothing,
// as they do in a row's file. A RUBRIC_ file whose name carries no entry id
// is refused rather than read as claiming nothing.
func TestDEL23_RubricMutantsNameTheirOwnEntry(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "target.go"), "package x\n\nfunc a() int { return 1 }\n")
	cases := []struct {
		file, test string
		wantOK     bool
	}{
		{"RUBRIC_lld-3_fixture.py", "TestRubricLLD3_OverLongLine", true},
		{"RUBRIC_lld-3_fixture.py", "TestRubricLLD3b_|TestRubricLLD3_Other", true},
		{"RUBRIC_lld-3_fixture.py", "TestRubricLLD3_A|TestH15_B", true},
		{"RUBRIC_lld-3_fixture.py", "TestRubricLLD7_AnOverCapFile", false},
		{"RUBRIC_lld-3_fixture.py", "TestRubricLLD3_A|TestRubricLLD7_B", false},
		{"RUBRIC_lld-3_fixture.py", "TestH15_ZeroCap", false},
		{"RUBRIC_lld-3_fixture.py", "TestREC31_", false},
		{"RUBRIC_errors-11_fixture.py", "TestRubricERRORS1_", false},
		{"RUBRIC_errors-1_fixture.py", "TestRubricERRORS11_", false},
		{"RUBRIC_errors-1_fixture.py", "TestRubricERRORS1_", true},
		{"RUBRIC_lld3_fixture.py", "TestRubricLLD3_", false},
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

// DEL-18. A mutant whose target file is excluded from the build on this
// GOOS -- a //go:build tag naming a different platform -- is reported NOT
// JUDGED (platform), never as undetected: the file is not compiled here, so
// mutating its text changes nothing a test on this host could ever see, and
// that is a fact about the platform, not about the test's thoroughness.
func TestDEL18_PlatformExcludedMutantIsNotJudged(t *testing.T) {
	foreign := "linux"
	if runtime.GOOS == "linux" {
		foreign = "darwin"
	}

	dir := t.TempDir()
	write(t, filepath.Join(dir, "go.mod"), "module fixturemod\n\ngo 1.21\n")
	write(t, filepath.Join(dir, "value_host.go"),
		fmt.Sprintf("//go:build !%s\n\npackage fixture\n\nfunc Value() int { return 1 }\n", foreign))
	write(t, filepath.Join(dir, "value_foreign.go"),
		fmt.Sprintf("//go:build %s\n\npackage fixture\n\nfunc Value() int { return 1 }\n", foreign))
	write(t, filepath.Join(dir, "value_test.go"),
		"package fixture\n\nimport \"testing\"\n\nfunc TestFixtureX_Value(t *testing.T) {\n\tif Value() != 1 {\n\t\tt.Fatalf(\"Value() = %d, want 1\", Value())\n\t}\n}\n")

	const name = "fixture value on the foreign platform"
	mutantFile := filepath.Join(t.TempDir(), "fixture.py")
	write(t, mutantFile, mutant(name, "value_foreign.go", "return 1", "return 2", "TestFixtureX_"))

	out, code := sweep(t, dir, "--files", mutantFile)
	if code != 0 {
		t.Fatalf("sweep exited %d; a mutant whose target is not built for GOOS=%s must not fail the sweep on %s:\n%s",
			code, foreign, runtime.GOOS, out)
	}
	if !strings.Contains(out, "NOT JUDGED (platform)") {
		t.Errorf("sweep output does not report the foreign-platform mutant as NOT JUDGED (platform):\n%s", out)
	}
	if strings.Contains(out, "undetected: ["+name) {
		t.Errorf("the foreign-platform mutant was counted undetected, not merely unjudged:\n%s", out)
	}
	// --strict fails on a judging test that SKIPPED, because in CI that means
	// the tooling broke. A target this GOOS does not build is not that: no
	// tooling makes it build, so it must not fail the strict sweep either.
	out, code = sweep(t, dir, "--strict", "--files", mutantFile)
	if code != 0 {
		t.Errorf("--strict exited %d; a mutant whose target is not built for GOOS=%s must not fail the strict sweep on %s:\n%s",
			code, foreign, runtime.GOOS, out)
	}
}

// TestDEL18b_PlatformDeclaredMutantIsNotJudgedOffItsGOOS: a mutant whose
// TARGET builds everywhere but whose judging test only means something on
// one GOOS -- the test itself skips elsewhere by design -- is NOT JUDGED
// (platform) there too, the same verdict DEL-18 gives a build-excluded
// target, once the mutant declares platform=. Before that declaration
// existed, a test like this one put the mutant in the "skipped" bucket,
// which --strict treats as a failure (TestStrict_NotJudgedFailsOnlyWithTheFlag):
// that was the bug a real GOOS-only mutant (REC-34's "the hook never signals
// before flush") hit on Linux, where its judging test is darwin-only.
func TestDEL18b_PlatformDeclaredMutantIsNotJudgedOffItsGOOS(t *testing.T) {
	foreign := "linux"
	if runtime.GOOS == "linux" {
		foreign = "darwin"
	}

	dir := t.TempDir()
	write(t, filepath.Join(dir, "go.mod"), "module fixturemod\n\ngo 1.21\n")
	write(t, filepath.Join(dir, "value.go"), "package fixture\n\nfunc Value() int { return 1 }\n")
	write(t, filepath.Join(dir, "value_test.go"), fmt.Sprintf(
		"package fixture\n\nimport (\n\t\"runtime\"\n\t\"testing\"\n)\n\nfunc TestFixtureY_Value(t *testing.T) {\n"+
			"\tif runtime.GOOS != %q {\n\t\tt.Skip(\"only meaningful on \"+%q)\n\t}\n"+
			"\tif Value() != 1 {\n\t\tt.Fatalf(\"Value() = %%d, want 1\", Value())\n\t}\n}\n", foreign, foreign))

	const name = "fixture value, judged only on a declared platform"
	mutantFile := filepath.Join(t.TempDir(), "fixture.py")
	write(t, mutantFile, mutantOnPlatform(name, "value.go", "return 1", "return 2", "TestFixtureY_", foreign))

	out, code := sweep(t, dir, "--strict", "--files", mutantFile)
	if code != 0 {
		t.Fatalf("--strict exited %d; a mutant declaring platform=%q must not fail the strict sweep on GOOS=%s:\n%s",
			code, foreign, runtime.GOOS, out)
	}
	if !strings.Contains(out, "NOT JUDGED (platform)") {
		t.Errorf("sweep output does not report the declared-platform mutant as NOT JUDGED (platform):\n%s", out)
	}
	if strings.Contains(out, "undetected: ["+name) {
		t.Errorf("the declared-platform mutant was counted undetected, not merely unjudged:\n%s", out)
	}
}

// DEL-17. Every job in every workflow carries timeout-minutes. One run in
// October 2026 lasted 1,212 minutes; a job without a bound holds a runner for
// as long as GitHub's six-hour default allows, and the sweep job in particular
// has nothing else to stop it.
func TestDEL17_EveryJobHasATimeout(t *testing.T) {
	for _, wf := range workflows(t, moduleRoot(t)) {
		name := filepath.Base(wf)
		jobs, missing, problems := jobsAndMissingTimeouts(readFile(t, wf))
		if len(jobs) == 0 {
			t.Errorf("%s: no jobs found", name)
		}
		for _, j := range missing {
			t.Errorf("%s: job %q has no timeout-minutes", name, j)
		}
		for _, p := range problems {
			t.Errorf("%s: %s", name, p)
		}
	}
}

// DEL-17, the parser. A line parser that skips what it does not recognise
// turns "unrecognised" into "fine": a job key with a trailing comment was not
// a job, and its timeout was credited to the job above. The real-tree test
// cannot go red on a well-formed tree, so these fixtures are the only red
// the parser ever shows. A job whose bound is written as 0 has no bound at
// all. Separately, a `jobs:` line with its own trailing comment is not
// matched either, so the whole block is missed and the caller sees zero jobs
// -- which is why TestDEL17_ treats "no jobs found" as its own failure, not a
// silent pass.
func TestDEL17b_TimeoutParserFailsClosed(t *testing.T) {
	cases := []struct {
		name, doc             string
		wantJobs, wantMissing []string
		wantProblem           bool
		knownGap              string // non-empty: this case pins a known limitation, not a verified-safe result
	}{
		{"both bounded", "jobs:\n  a:\n    timeout-minutes: 30\n  b:\n    timeout-minutes: 15\n", []string{"a", "b"}, nil, false, ""},
		{"one unbounded", "jobs:\n  a:\n    runs-on: x\n  b:\n    timeout-minutes: 15\n", []string{"a", "b"}, []string{"a"}, false, ""},
		{"commented key is a job", "jobs:\n  a:\n    runs-on: x\n  b:  # slow one\n    timeout-minutes: 30\n", []string{"a", "b"}, []string{"a"}, false, ""},
		{"commented job key is still a job", "jobs:\n  test:\n    timeout-minutes: 30\n  sweep:  # on demand\n    runs-on: x\n", []string{"test", "sweep"}, []string{"sweep"}, false, ""},
		{"comment line is not a job", "jobs:\n  # the jobs\n  a:\n    timeout-minutes: 30\n", []string{"a"}, nil, false, ""},
		{"unrecognised line fails closed", "jobs:\n  a:\n    timeout-minutes: 30\n  b: {timeout-minutes: 30}\n", []string{"a"}, nil, true, ""},
		{"zero is no bound", "jobs:\n  a:\n    timeout-minutes: 0\n", []string{"a"}, []string{"a"}, false, ""},
		{"a zero bound is no bound", "jobs:\n  foo:\n    timeout-minutes: 0\n", []string{"foo"}, []string{"foo"}, false, ""},
		{"bound with a comment", "jobs:\n  a:\n    timeout-minutes: 30  # minutes\n", []string{"a"}, nil, false, ""},
		{"next top-level key ends the jobs", "jobs:\n  a:\n    timeout-minutes: 30\nenv:\n  b: 1\n", []string{"a"}, nil, false, ""},
		{
			name: "a commented jobs line is not recognised at all",
			doc:  "jobs:  # c\n  foo:\n    timeout-minutes: 30\n",
			knownGap: "a `jobs:` line with its own trailing comment is invisible to this parser: every " +
				"job under it, timed or not, goes unseen. Pinned here as the parser's actual behaviour, " +
				"not endorsed as safe -- the only reason it doesn't hide an untimed job in a real workflow " +
				"is that TestDEL17_ treats zero jobs found as a failure in its own right, not a quiet pass.",
		},
		// A job key at three spaces is not the two-space depth a job key
		// lives at. The old parser stepped over it silently: it was not a
		// job (so never reported missing), and the `current` job variable
		// stayed on the one above, so the timeout that followed at four
		// spaces -- meant for the three-space job -- was credited to it
		// instead. "a" never gets its own timeout and must still show up
		// as missing, not quietly satisfied by someone else's line.
		{"a three-space job line is a problem, not a credit to the job above",
			"jobs:\n  a:\n   b:\n    timeout-minutes: 15\n", []string{"a"}, []string{"a"}, true, ""},
		{"a one-space line is a problem too",
			"jobs:\n a:\n  b:\n    timeout-minutes: 15\n", []string{"b"}, nil, true, ""},
		// \r\n line endings. The exact-string "jobs:" check broke on the
		// trailing \r, so the whole block was never entered.
		{"CRLF line endings are tolerated",
			"jobs:\r\n  a:\r\n    timeout-minutes: 30\r\n", []string{"a"}, nil, false, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.knownGap != "" {
				t.Logf("known gap, not a verified-safe case: %s", tc.knownGap)
			}
			jobs, missing, problems := jobsAndMissingTimeouts(tc.doc)
			if !slicesEqual(jobs, tc.wantJobs) {
				t.Errorf("jobs = %v, want %v", jobs, tc.wantJobs)
			}
			if !slicesEqual(missing, tc.wantMissing) {
				t.Errorf("missing = %v, want %v", missing, tc.wantMissing)
			}
			if (len(problems) > 0) != tc.wantProblem {
				t.Errorf("problems = %v, want a problem: %v", problems, tc.wantProblem)
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

// A mutant's target is otherwise unconstrained, and _dirty_targets() runs
// `git status -- <targets>`, which exits non-zero for a path outside the
// repository and silently drops the already-modified note for every target
// in that run -- not just the offending one.
func TestCheckRejectsATargetOutsideTheCurrentDirectory(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "target.go"), "package x\n\nfunc a() int { return 1 }\n")
	outside := filepath.Join(t.TempDir(), "elsewhere.go")
	write(t, outside, "package x\n\nfunc a() int { return 1 }\n")
	// Escapes dir by one level, to a file that genuinely exists there --
	// otherwise the old code would reject this for an unrelated reason (the
	// file cannot be read) rather than the one under test (it is outside).
	escaped := filepath.Join(filepath.Dir(dir), "escaped-elsewhere.go")
	write(t, escaped, "package x\n\nfunc a() int { return 1 }\n")
	t.Cleanup(func() { os.Remove(escaped) })
	cases := []struct {
		name, target string
		wantOK       bool
	}{
		{"inside the current directory", "target.go", true},
		{"an absolute path", outside, false},
		{"a relative path that escapes the current directory", "../escaped-elsewhere.go", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			file := filepath.Join(t.TempDir(), "fixture.py")
			write(t, file, mutant("fixture "+tc.name, tc.target, "return 1", "return 2", "TestFixture_"))
			_, code := sweep(t, dir, "--check", "--files", file)
			if (code == 0) != tc.wantOK {
				t.Errorf("--check exited %d for target %s", code, tc.name)
			}
		})
	}
}

// shared-tests.txt's entries must themselves be shaped like what they
// declare: a <PREFIX>-<n> row paired with a Test<PREFIX><n>[a-z]?_ test, not
// merely two whitespace-separated fields. --shared-tests points sweep.py at
// a fixture, so this never touches the real file.
func TestSharedTestsEntryShape(t *testing.T) {
	root := moduleRoot(t)
	fixture := filepath.Join(t.TempDir(), "shared-tests.txt")
	write(t, fixture, "STR19 TestSTR82\n")
	// Scoped to one unrelated file: shared-tests.txt is validated
	// unconditionally, regardless of --files, and scoping avoids loading
	// this test's own mutant file, whose anchor is the validation line
	// itself and would otherwise report its own, unrelated problem.
	out, code := sweep(t, root, "--check", "--files", "mutants/DEL-17_job-timeouts.py", "--shared-tests", fixture)
	if code == 0 {
		t.Errorf("--check exited 0 with a malformed shared-tests.txt entry:\n%s", out)
	}
}

// A suffixed test, TestREC56b_, pins its parent row, and the shared-pair
// lookup is keyed on the parent's name, so an entry that names the suffixed
// form is shaped right and can still never match: it has to name the parent.
// Same fixture-pointed check as TestSharedTestsEntryShape.
func TestSharedTestsEntryNamesTheParent(t *testing.T) {
	root := moduleRoot(t)
	fixture := filepath.Join(t.TempDir(), "shared-tests.txt")
	write(t, fixture, "STR-19 TestSTR82b_\n")
	out, code := sweep(t, root, "--check", "--files", "mutants/DEL-17_job-timeouts.py", "--shared-tests", fixture)
	if code == 0 {
		t.Errorf("--check exited 0 with a shared-tests.txt entry naming a suffixed test:\n%s", out)
	}
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

	// 90s, not the usual 30s: this is the one test that runs the real sweep,
	// which shells out to `go test ./...` twice against this throwaway
	// module, and a cold build cache can make that run longer than the
	// read-only modes ever need.
	const buildBound = 90 * time.Second
	if out, code := sweepTimeout(t, root, buildBound); code != 0 {
		t.Errorf("without --strict, a NOT JUDGED mutant exited %d:\n%s", code, out)
	}
	out, code := sweepTimeout(t, root, buildBound, "--strict")
	if code == 0 {
		t.Errorf("--strict exited 0 with a NOT JUDGED mutant:\n%s", out)
	}
	if !strings.Contains(out, "not judged here") {
		t.Errorf("--strict output does not mention the skipped mutant:\n%s", out)
	}
}

// The sweep job runs only on a manual dispatch, so two dispatches share no
// state; cancelling an older one when a newer dispatch starts costs nothing
// and saves a runner's minutes.
func TestSweepJobCancelsAnOlderRunOnRedispatch(t *testing.T) {
	root := moduleRoot(t)
	data, err := os.ReadFile(filepath.Join(root, ".github", "workflows", "ci.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	doc := string(data)
	i := strings.Index(doc, "\n  sweep:\n")
	if i < 0 {
		t.Fatal("ci.yaml: no sweep: job found")
	}
	next := strings.Index(doc[i+1:], "\n  cross-build:\n")
	if next < 0 {
		t.Fatal("ci.yaml: no cross-build: job found after sweep:")
	}
	job := doc[i : i+1+next]
	if !strings.Contains(job, "concurrency:") {
		t.Error("the sweep job carries no concurrency group")
	}
}

// A push, a pull request or a tag is each a change to the tree; an advisory
// against an unchanged dependency is not, and without a schedule trigger
// nothing re-runs govulncheck between them. The on: block, not the trigger
// of any one job, since the jobs it runs (Test, with govulncheck) are the
// same ones push and pull_request already schedule.
func TestCIRunsAWeeklyScheduledVulnerabilityCheck(t *testing.T) {
	root := moduleRoot(t)
	doc := readFile(t, filepath.Join(root, ".github", "workflows", "ci.yaml"))
	i := strings.Index(doc, "\non:\n")
	if i < 0 {
		t.Fatal("ci.yaml: no on: block found")
	}
	next := strings.Index(doc[i+1:], "\npermissions:\n")
	if next < 0 {
		t.Fatal("ci.yaml: no permissions: key found after on:")
	}
	block := doc[i : i+1+next]
	if !strings.Contains(block, "schedule:") || !strings.Contains(block, "cron:") {
		t.Error("ci.yaml's on: block carries no schedule: trigger with a cron: line")
	}
}

// jobsAndMissingTimeouts reads a workflow by its lines: under `jobs:`, a key
// at two spaces of indent is a job, a comment is a comment, and anything else
// at that indent is a problem rather than something to step over. A job's
// bound is a `timeout-minutes:` line at four spaces with a positive integer.
// No YAML parser, deliberately: the module takes no dependency for a test,
// and the files are written by hand in exactly this shape.
//
// Indent is measured, not assumed to be exactly two or four: a job key typed
// at three spaces matched neither depth, so the old parser stepped over it
// -- it was never added to jobs (so never reported missing), and `current`
// stayed on the job above, so the four-space timeout that followed, meant
// for the mistyped job, was silently credited to it instead. Any indent that
// is not 0 (a top-level key), 2 (a job key) or a multiple of 2 at or past 4
// (content inside a job) is odd and is always a problem; it never becomes a
// job and never resets to anything but no job, so a timeout line right
// after it is orphaned rather than credited to whichever job came before.
func jobsAndMissingTimeouts(doc string) (jobs, missing, problems []string) {
	jobLine := regexp.MustCompile(`^  ([A-Za-z0-9_-]+):\s*(#.*)?$`)
	timeoutLine := regexp.MustCompile(`^    timeout-minutes:\s*([0-9]+)\s*(#.*)?$`)
	inJobs := false
	current := ""
	timed := map[string]bool{}
	for i, raw := range strings.Split(doc, "\n") {
		ln := strings.TrimRight(raw, "\r") // tolerate CRLF without treating \r as content
		if ln == "jobs:" {
			inJobs = true
			continue
		}
		if !inJobs {
			continue
		}
		trimmed := strings.TrimSpace(ln)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		if !strings.HasPrefix(ln, " ") {
			inJobs = false // another top-level key ends the jobs block
			continue
		}
		indent := len(ln) - len(strings.TrimLeft(ln, " "))
		if indent%2 != 0 {
			problems = append(problems, fmt.Sprintf("line %d: unexpected %d-space indent under jobs: %q", i+1, indent, trimmed))
			current = "" // an orphaned timeout after this must not land on the job above
			continue
		}
		if indent == 2 {
			if m := jobLine.FindStringSubmatch(ln); m != nil {
				current = m[1]
				jobs = append(jobs, current)
				continue
			}
			problems = append(problems, fmt.Sprintf("line %d: unrecognised line under jobs: %q", i+1, trimmed))
			current = ""
			continue
		}
		if indent == 4 {
			if m := timeoutLine.FindStringSubmatch(ln); m != nil && current != "" {
				if n, err := strconv.Atoi(m[1]); err == nil && n > 0 {
					timed[current] = true
				}
			}
			continue
		}
		// indent >= 6, even: step/key content inside a job, not structure
		// this parser tracks.
	}
	for _, j := range jobs {
		if !timed[j] {
			missing = append(missing, j)
		}
	}
	return jobs, missing, problems
}

// DEL-21. Every setup-go step carries a quoted go-version, and every pin across
// the workflows is the same version: the build toolchain is pinned by hand in
// each job, so a bump that misses one job leaves that job building with a
// toolchain nobody chose. Found when a job added on one branch kept the old
// pin while another branch bumped the rest.
func TestDEL21_GoVersionPinsAgree(t *testing.T) {
	var pins []string // "file: version"
	for _, wf := range workflows(t, moduleRoot(t)) {
		name := filepath.Base(wf)
		found, problems := goVersionPins(readFile(t, wf))
		for _, p := range problems {
			t.Errorf("%s: %s", name, p)
		}
		for _, v := range found {
			pins = append(pins, name+": "+v)
		}
	}
	if len(pins) < 2 {
		t.Fatalf("expected go-version pins in more than one place, found %v", pins)
	}
	version := func(pin string) string { return strings.SplitN(pin, ": ", 2)[1] }
	for _, p := range pins[1:] {
		if version(p) != version(pins[0]) {
			t.Errorf("go-version pins disagree: %s vs %s", pins[0], p)
		}
	}
}

// DEL-21, the parser. The first version opened a step only on
// `- uses: actions/setup-go@` and never saw the name-first form, so a step
// written as `- name:` then `uses:` could carry an old pin unseen. Counting
// steps and pins in any form, and refusing the forms a pin must not take,
// is what these fixtures hold.
func TestDEL21b_PinParserFailsClosed(t *testing.T) {
	const sha = "actions/setup-go@b7ad1dad31e06c5925ef5d2fc7ad053ef454303e"
	cases := []struct {
		name, doc   string
		wantPins    []string
		wantProblem bool
	}{
		{"uses first", "      - uses: " + sha + "\n        with:\n          go-version: \"1.27.2\"\n", []string{"1.27.2"}, false},
		{"name first", "      - name: Set up Go\n        uses: " + sha + "\n        with:\n          go-version: \"1.27.2\"\n", []string{"1.27.2"}, false},
		{"pin with a comment", "      - uses: " + sha + "\n        with:\n          go-version: \"1.27.2\"  # see the note\n", []string{"1.27.2"}, false},
		{"name first, no pin", "      - name: Set up Go\n        uses: " + sha + "\n        with:\n          cache: true\n", nil, true},
		{"two steps, one pin", "      - uses: " + sha + "\n        with:\n          go-version: \"1.27.2\"\n      - uses: " + sha + "\n        with:\n          cache: true\n", []string{"1.27.2"}, true},
		{"unquoted", "      - uses: " + sha + "\n        with:\n          go-version: 1.27.2\n", nil, true},
		{"from a file", "      - uses: " + sha + "\n        with:\n          go-version-file: go.mod\n", nil, true},
		{"pin with no step", "          go-version: \"1.27.2\"\n", []string{"1.27.2"}, true},
		// A quoted `uses:` value, as some actions pin it, carries the same
		// action and must be recognised as a setup-go step.
		{"quoted uses line", "      - uses: \"" + sha + "\"\n        with:\n          go-version: \"1.27.2\"\n", []string{"1.27.2"}, false},
		// Same quoted uses: line, but with no pin at all: this is the case
		// that actually requires the step to be recognised as setup-go --
		// the one above still collects the pin even if that recognition
		// silently failed, so it alone cannot catch a regression there.
		{"quoted uses line, no pin", "      - uses: \"" + sha + "\"\n        with:\n          cache: true\n", nil, true},
		// A quoted string is not by itself a version: "stable" and a
		// wildcard both resolve to whatever toolchain exists when the
		// runner fetches it, which is exactly the unchosen toolchain this
		// row exists to refuse.
		{"stable is not a version", "      - uses: " + sha + "\n        with:\n          go-version: \"stable\"\n", nil, true},
		{"a wildcard is not a version", "      - uses: " + sha + "\n        with:\n          go-version: \"1.27.x\"\n", nil, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			pins, problems := goVersionPins(tc.doc)
			if strings.Join(pins, ",") != strings.Join(tc.wantPins, ",") {
				t.Errorf("pins = %v, want %v", pins, tc.wantPins)
			}
			if (len(problems) > 0) != tc.wantProblem {
				t.Errorf("problems = %v, want a problem: %v", problems, tc.wantProblem)
			}
		})
	}
}

// goVersionPins walks a workflow by its lines and ties each go-version pin to
// the setup-go step that declares it, rather than merely counting the two: a
// step with no pin and a stray go-version line elsewhere in the file used to
// average out to a matching count. A step is the sequence item opened by a
// line that starts, at any indent, with "- ", running up to the next such
// line; a step is a setup-go step if any line inside it names
// actions/setup-go@, quoted or not, in `uses:`-first or `name:`-first form.
// Exactly one go-version pin is required inside a setup-go step, and it must
// be a quoted MAJOR.MINOR.PATCH string: unquoted, "stable", a wildcard like
// "1.27.x", and go-version-file are each a toolchain nobody chose here, and
// are refused as a problem rather than accepted as a pin.
func goVersionPins(doc string) (pins []string, problems []string) {
	bullet := regexp.MustCompile(`^\s*-\s`)
	usesLine := regexp.MustCompile(`^\s*(-\s+)?uses:\s*"?([^"\s]+)"?\s*(#.*)?$`)
	pin := regexp.MustCompile(`^\s*go-version:\s*(.*?)\s*(#.*)?$`)
	fromFile := regexp.MustCompile(`^\s*go-version-file:`)
	exactVersion := regexp.MustCompile(`^\d+\.\d+\.\d+$`)

	type step struct {
		line      int
		isSetupGo bool
		pinSeen   bool
	}
	var cur *step
	closeStep := func() {
		if cur != nil && cur.isSetupGo && !cur.pinSeen {
			problems = append(problems, fmt.Sprintf("line %d: setup-go step has no go-version pin", cur.line))
		}
		cur = nil
	}
	validate := func(i int, v string) {
		if len(v) < 2 || v[0] != '"' || v[len(v)-1] != '"' || !exactVersion.MatchString(v[1:len(v)-1]) {
			problems = append(problems, fmt.Sprintf("line %d: go-version %s is not a quoted MAJOR.MINOR.PATCH string", i+1, v))
			return
		}
		pins = append(pins, v[1:len(v)-1])
	}

	for i, raw := range strings.Split(doc, "\n") {
		ln := strings.TrimRight(raw, "\r")
		if bullet.MatchString(ln) {
			closeStep()
			cur = &step{line: i + 1}
		}
		if m := usesLine.FindStringSubmatch(ln); m != nil {
			if strings.HasPrefix(m[2], "actions/setup-go@") {
				if cur == nil {
					cur = &step{line: i + 1}
				}
				cur.isSetupGo = true
			}
			continue
		}
		if fromFile.MatchString(ln) {
			problems = append(problems, fmt.Sprintf("line %d: go-version-file reads the pin from a file; the pin is written here", i+1))
			if cur != nil {
				cur.pinSeen = true
			}
			continue
		}
		if m := pin.FindStringSubmatch(ln); m != nil {
			if cur == nil {
				problems = append(problems, fmt.Sprintf("line %d: go-version pin outside a setup-go step", i+1))
				validate(i, m[1])
				continue
			}
			cur.pinSeen = true
			validate(i, m[1])
		}
	}
	closeStep()
	return pins, problems
}

// DEL-21, the floor. A pin below go.mod's own `go` line would build a
// release with a toolchain older than the module declares it needs, which
// defeats the pin's purpose even when every workflow agrees with every
// other. go.mod's directive is unquoted and may carry two components; a
// workflow pin is quoted and always carries three, so the comparison is
// numeric, not lexical: a string compare puts "1.9.0" above "1.10.0".
func TestDEL21d_PinsAreAtLeastGoModsMinimum(t *testing.T) {
	root := moduleRoot(t)
	floor := goModLanguageMinimum(t, root)
	for _, wf := range workflows(t, root) {
		name := filepath.Base(wf)
		pins, _ := goVersionPins(readFile(t, wf))
		for _, p := range pins {
			atLeast, err := versionAtLeast(p, floor)
			if err != nil {
				t.Errorf("%s: go-version %q: %v", name, p, err)
				continue
			}
			if !atLeast {
				t.Errorf("%s: go-version %q is older than go.mod's minimum %q", name, p, floor)
			}
		}
	}
}

// goModLanguageMinimum reads go.mod's own `go` directive: the language
// minimum a consumer needs to build the module, not the build toolchain the
// workflows pin by hand (see the note beside go-version in ci.yaml).
func goModLanguageMinimum(t *testing.T, root string) string {
	t.Helper()
	re := regexp.MustCompile(`(?m)^go (\d+\.\d+(?:\.\d+)?)\s*$`)
	m := re.FindStringSubmatch(readFile(t, filepath.Join(root, "go.mod")))
	if m == nil {
		t.Fatal("go.mod: no go directive found")
	}
	return m[1]
}

// versionAtLeast compares two Go version strings component by component, not
// as strings, where "1.9.0" sorts above "1.10.0". A missing patch component
// compares as zero, so go.mod's two-component line can stand on either side
// of the comparison.
func versionAtLeast(version, floor string) (bool, error) {
	v, err := parseVersion(version)
	if err != nil {
		return false, err
	}
	f, err := parseVersion(floor)
	if err != nil {
		return false, err
	}
	for i := 0; i < 3; i++ {
		if v[i] != f[i] {
			return v[i] > f[i], nil
		}
	}
	return true, nil
}

// parseVersion reads a MAJOR.MINOR[.PATCH] string into three components,
// refusing anything else rather than defaulting a bad component to zero.
func parseVersion(s string) ([3]int, error) {
	var out [3]int
	parts := strings.Split(s, ".")
	if len(parts) < 2 || len(parts) > 3 {
		return out, fmt.Errorf("%q is not a MAJOR.MINOR[.PATCH] version", s)
	}
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil {
			return out, fmt.Errorf("%q: component %q is not a number", s, p)
		}
		out[i] = n
	}
	return out, nil
}

// DEL-21, the comparison. Done as strings, "1.9.0" sorts above "1.10.0";
// component by component gets it right, and a malformed version on either
// side is refused rather than silently treated as 0.
func TestDEL21e_VersionAtLeastFailsClosed(t *testing.T) {
	cases := []struct {
		version, floor string
		want           bool
		wantErr        bool
	}{
		{"1.27.2", "1.25.0", true, false},
		{"1.25.0", "1.27.2", false, false},
		{"1.27.0", "1.27.0", true, false},
		{"1.9.0", "1.10.0", false, false},
		{"1.27", "1.25.0", true, false},
		{"1.27.2", "garbage", false, true},
		{"", "1.25.0", false, true},
	}
	for _, tc := range cases {
		t.Run(tc.version+" vs "+tc.floor, func(t *testing.T) {
			got, err := versionAtLeast(tc.version, tc.floor)
			if (err != nil) != tc.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tc.wantErr)
			}
			if err == nil && got != tc.want {
				t.Errorf("versionAtLeast(%q, %q) = %v, want %v", tc.version, tc.floor, got, tc.want)
			}
		})
	}
}

// DEL-21, the clock. Go supports security fixes on only the two most recent
// majors; go1.27.2's window closes when a second newer major ships. Given
// Go's roughly six-month cadence (1.25 August 2025, 1.26 February 2026,
// 1.27 August 2026), that is expected around August 2027 with 1.29. Nothing
// here polls go.dev for that date -- every CI run would then depend on an
// endpoint nothing else calls, to answer a question that changes twice a
// year. A dated trip-wire costs nothing until the date and then fails
// loudly, which beats a security update landing quietly on an unsupported
// major with nothing here noticing. Pushing past the deadline below is the
// renewal: move it out another two releases once the pin itself has moved.
func TestDEL21f_PinReviewIsNotOverdue(t *testing.T) {
	reviewBy := time.Date(2027, time.July, 1, 0, 0, 0, 0, time.UTC)
	if time.Now().After(reviewBy) {
		t.Fatalf("go-version pin not reviewed since %s: go1.27 is expected to leave "+
			"Go's two-most-recent-majors support window around August 2027 (go1.29); "+
			"re-pin to a supported major and move this date out another two releases",
			reviewBy.Format("2006-01-02"))
	}
}

// DEL-21, the scanner. govulncheck is a tool built with the pinned toolchain,
// so a toolchain bump can break it: v1.1.4 panics in go/ssa under go1.27.2
// when it analyses linux/amd64, and only a tag would have run it. The test
// job runs the same line as the release job, so a pull request runs it first,
// and the two versions must agree.
func TestDEL21c_GovulncheckRunsInCIAtTheReleaseVersion(t *testing.T) {
	found := map[string][]string{}
	for _, wf := range workflows(t, moduleRoot(t)) {
		found[filepath.Base(wf)] = govulncheckVersions(readFile(t, wf))
	}
	for _, name := range []string{"ci.yaml", "release.yaml"} {
		if len(found[name]) != 1 {
			t.Errorf("%s: %d govulncheck steps, want 1: %v", name, len(found[name]), found[name])
		}
	}
	var first string
	for name, versions := range found {
		for _, v := range versions {
			if first == "" {
				first = v
			} else if v != first {
				t.Errorf("govulncheck versions disagree: %s runs %s, another workflow runs %s", name, v, first)
			}
		}
	}
}

// govulncheckVersions is the version of every govulncheck run in a workflow,
// read from `govulncheck@<version>` on lines that are not comments.
func govulncheckVersions(doc string) []string {
	re := regexp.MustCompile(`govulncheck@(\S+)`)
	var versions []string
	for _, ln := range strings.Split(doc, "\n") {
		if strings.HasPrefix(strings.TrimSpace(ln), "#") {
			continue
		}
		if m := re.FindStringSubmatch(ln); m != nil {
			versions = append(versions, m[1])
		}
	}
	return versions
}

// DEL-1. A test's output never ends a sweep: when a mutant's failing test
// prints bytes that are not UTF-8 (the output gate's test prints what it fed
// Marshal), the sweep still judges that mutant red, goes on, exits normally,
// and the tree is restored. A fake go on PATH stands in for the suite: green
// for the baseline, and for a mutant run a FAIL line followed by a raw 0xff.
func TestDEL1c_ATestPrintingInvalidUTF8NeverEndsTheSweep(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "a.txt"), "alpha\n")
	mutants := filepath.Join(dir, "mutants")
	if err := os.Mkdir(mutants, 0o755); err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(mutants, "T-1_alpha.py"), mutant("T-1 alpha goes", "a.txt", "alpha", "beta", "TestT1_"))
	bin := filepath.Join(dir, "bin")
	if err := os.Mkdir(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	fake := filepath.Join(bin, "go")
	write(t, fake, "#!/bin/sh\ncase \"$*\" in *-run*) printf 'FAIL\\n--- FAIL: TestT1_x\\n\\377\\n'; exit 1;; esac\necho ok\n")
	if err := os.Chmod(fake, 0o755); err != nil {
		t.Fatal(err)
	}
	out, code := sweepEnv(t, dir, []string{"PATH=" + bin + string(os.PathListSeparator) + os.Getenv("PATH")}, "--files", filepath.Join(mutants, "T-1_alpha.py"))
	if code != 0 {
		t.Fatalf("the sweep exited %d on a test that printed invalid UTF-8:\n%s", code, out)
	}
	if !strings.Contains(out, "went red") || !strings.Contains(out, "undetected: none") {
		t.Errorf("the mutant was not judged red:\n%s", out)
	}
	got, err := os.ReadFile(filepath.Join(dir, "a.txt"))
	if err != nil || string(got) != "alpha\n" {
		t.Errorf("the tree was not restored: %q, %v", got, err)
	}
}

// DEL-1. The sweep tells a broken build from a failing test by go test's own
// markers, never by a sentence in a test's output: a test that prints "the
// build failed" while failing is a red mutant, and a package go test could
// not build ([build failed] on its FAIL line) is still reported broken, and a
// non-zero exit with no test failure on record (a go.mod the tool could not
// parse) is NOT JUDGED, never red: nothing ran.
func TestDEL1d_AFailingTestThatSaysBuildFailedIsStillRed(t *testing.T) {
	for _, c := range []struct {
		mode, want, never string
		exit              int
	}{
		{"testfail", "went red", "BUILD BROKEN", 0},
		{"buildfail", "BUILD BROKEN", "went red", 1},
		{"nomodule", "NOT JUDGED", "went red", 1},
		// The target test failed AND an unrelated package in the same
		// `go test ./...` run failed to build. The target's own red test
		// is positive evidence the mutant was caught; a build failure
		// elsewhere must not override that into BUILD BROKEN.
		{"both", "went red", "BUILD BROKEN", 0},
		// No test failed and no FAIL line carries a build marker; the
		// English words "build failed" appear only in a log line. Nothing
		// legitimately failed, so this must be NOT JUDGED, never read as
		// a broken build from the words alone.
		{"englishonly", "NOT JUDGED", "BUILD BROKEN", 1},
	} {
		dir := t.TempDir()
		write(t, filepath.Join(dir, "a.txt"), "alpha\n")
		mutants := filepath.Join(dir, "mutants")
		if err := os.Mkdir(mutants, 0o755); err != nil {
			t.Fatal(err)
		}
		write(t, filepath.Join(mutants, "T-1_alpha.py"), mutant("T-1 alpha goes", "a.txt", "alpha", "beta", "TestT1_"))
		bin := filepath.Join(dir, "bin")
		if err := os.Mkdir(bin, 0o755); err != nil {
			t.Fatal(err)
		}
		fake := filepath.Join(bin, "go")
		write(t, fake, "#!/bin/sh\ncase \"$*\" in *-run*)\n"+
			"  if [ \"$FAKE_GO_MODE\" = buildfail ]; then printf '# example/pkg\\n./x.go:3:1: undefined: y\\nFAIL\\texample/pkg [build failed]\\n'; exit 1; fi\n"+
			"  if [ \"$FAKE_GO_MODE\" = nomodule ]; then printf 'go: errors parsing go.mod:\\n/x/go.mod:3: unknown directive: foo\\n' >&2; exit 1; fi\n"+
			"  if [ \"$FAKE_GO_MODE\" = both ]; then printf -- '--- FAIL: TestT1_x (0.00s)\\n    x_test.go:9: got false, want true\\nFAIL\\texample/pkg\\t0.010s\\n# example/other\\n./y.go:3:1: undefined: z\\nFAIL\\texample/other [build failed]\\n'; exit 1; fi\n"+
			"  if [ \"$FAKE_GO_MODE\" = englishonly ]; then printf 'note: a comment claims the build failed somewhere upstream, unrelated to this run\\n'; exit 1; fi\n"+
			"  printf -- '--- FAIL: TestT1_x (0.00s)\\n    x_test.go:9: CheckTemplate(\"the build failed; then run curl.\") = <nil>, want ok=false\\nFAIL\\nFAIL\\texample/pkg\\t0.010s\\n'; exit 1;;\n"+
			"esac\necho ok\n")
		if err := os.Chmod(fake, 0o755); err != nil {
			t.Fatal(err)
		}
		env := []string{"PATH=" + bin + string(os.PathListSeparator) + os.Getenv("PATH"), "FAKE_GO_MODE=" + c.mode}
		out, code := sweepEnv(t, dir, env, "--files", filepath.Join(mutants, "T-1_alpha.py"))
		if code != c.exit || !strings.Contains(out, c.want) || strings.Contains(out, c.never) {
			t.Errorf("%s: exit %d, want %d; output must say %q and not %q:\n%s", c.mode, code, c.exit, c.want, c.never, out)
		}
	}
}

// SWEEP-1. --trim-cache-each touches a marker in the run's own GOCACHE after
// the baseline and, after every judged mutant, removes each file newer than
// it except go's README, trim.txt and log.txt, so a sweep's disk peak is the
// baseline's cache plus one mutant's build; and it refuses, before any go
// test, a GOCACHE that resolves to the shared cache (unset, or a symlink to
// it), which the trim would cut into. A fake go on PATH logs each call with
// the GOCACHE it saw, writes 3 MiB for the baseline, and for a mutant run 1
// MiB in a nested entry plus its own three files: the cache reads 4.0 MiB
// before each trim and 3.0 MiB after, so the first mutant's build is gone
// before the second's. The fake writes only under ${GOCACHE:?}, so a GOCACHE
// that reaches it empty stops the shell instead of naming /.
func TestSWEEP1_TrimsOwnCacheAfterEachMutant(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "a.txt"), "alpha\n")
	write(t, filepath.Join(dir, "b.txt"), "gamma\n")
	mutantFile := filepath.Join(dir, "T-1_two.py")
	write(t, mutantFile, mutant("T-1 alpha goes", "a.txt", "alpha", "beta", "TestT1_")+
		mutant("T-2 gamma goes", "b.txt", "gamma", "delta", "TestT2_"))
	bin, own, shared := filepath.Join(dir, "bin"), filepath.Join(dir, "own"), filepath.Join(dir, "shared")
	for _, d := range []string{bin, own, shared} {
		if err := os.Mkdir(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink(shared, filepath.Join(dir, "link")); err != nil {
		t.Fatal(err)
	}
	fake := filepath.Join(bin, "go")
	write(t, fake, "#!/bin/sh\necho \"$GOCACHE|$*\" >> \"$FAKE_GO_LOG\"\ncase \"$*\" in\n"+
		"\"env GOCACHE\") echo \"${GOCACHE:-$FAKE_SHARED}\" ;;\n"+
		"*-run*) mkdir -p \"${GOCACHE:?}/ab/cd\" && head -c 1048576 /dev/zero > \"$GOCACHE/ab/cd/$$\"\n"+
		"  for f in README trim.txt log.txt; do echo go > \"$GOCACHE/$f\"; done\n"+
		"  printf -- '--- FAIL: TestT1_x (0.00s)\\nFAIL\\n'; exit 1 ;;\n"+
		"*) mkdir -p \"${GOCACHE:?}/00\" && head -c 3145728 /dev/zero > \"$GOCACHE/00/base\"; echo ok ;;\n"+
		"esac\n")
	if err := os.Chmod(fake, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		name, cache string
		exit        int
		calls       []string
	}{
		{"own cache", own, 0, []string{
			own + "|env GOCACHE",
			"|env GOCACHE",
			own + "|test ./... -count=1",
			own + "|test ./... -run TestT1_ -count=1 -v",
			own + "|test ./... -run TestT2_ -count=1 -v",
		}},
		{"unset", "", 2, []string{"|env GOCACHE", "|env GOCACHE"}},
		{"shared via link", filepath.Join(dir, "link"), 2, []string{filepath.Join(dir, "link") + "|env GOCACHE", "|env GOCACHE"}},
	} {
		t.Run(c.name, func(t *testing.T) {
			goLog := filepath.Join(t.TempDir(), "go.log")
			write(t, goLog, "")
			env := []string{"PATH=" + bin + string(os.PathListSeparator) + os.Getenv("PATH"),
				"FAKE_GO_LOG=" + goLog, "FAKE_SHARED=" + shared, "GOCACHE=" + c.cache}
			out, code := sweepEnv(t, dir, env, "--trim-cache-each", "--files", mutantFile)
			if code != c.exit {
				t.Fatalf("exit %d, want %d:\n%s", code, c.exit, out)
			}
			if got, want := strings.Join(nonEmptyLines(readFile(t, goLog)), "\n"), strings.Join(c.calls, "\n"); got != want {
				t.Errorf("go calls:\n%s\nwant:\n%s", got, want)
			}
			if c.exit != 0 {
				return
			}
			if n := strings.Count(out, "cache 4.0 MiB, trimmed to 3.0 MiB"); n != 2 {
				t.Errorf("want the cache at 4.0 MiB before each of the two trims and 3.0 MiB after, read %d:\n%s", n, out)
			}
			for _, f := range []string{"00/base", "README", "trim.txt", "log.txt"} {
				if _, err := os.Stat(filepath.Join(own, f)); err != nil {
					t.Errorf("the trim removed %s: %v", f, err)
				}
			}
		})
	}
}

// SWEEP-FINISH1. The shards of --shard K/N are disjoint and together are
// --list: one run of the sweep did not fit the job's bound, and a mutant
// dealt to no shard, or to two, would be judged by nobody or twice. Checked
// for N of 1, 2, 3 and 7, a count that does not divide the total.
func TestSweepShards_PartitionTheMutantsExactlyOnce(t *testing.T) {
	root := moduleRoot(t)
	out, code := sweep(t, root, "--list")
	if code != 0 {
		t.Fatalf("--list exited %d", code)
	}
	full := nonEmptyLines(out)
	for _, n := range []int{1, 2, 3, 7, 8} {
		owner := map[string]int{}
		for k := 1; k <= n; k++ {
			out, code := sweep(t, root, "--list", "--shard", fmt.Sprintf("%d/%d", k, n))
			if code != 0 {
				t.Fatalf("--list --shard %d/%d exited %d", k, n, code)
			}
			part := nonEmptyLines(out)
			if len(part) < len(full)/n || len(part) > len(full)/n+1 {
				t.Errorf("shard %d/%d holds %d of %d mutants, want an even share", k, n, len(part), len(full))
			}
			for _, name := range part {
				if j, ok := owner[name]; ok {
					t.Errorf("%q is in shard %d and shard %d of %d", name, j, k, n)
				}
				owner[name] = k
			}
		}
		for _, name := range full {
			if owner[name] == 0 {
				t.Errorf("%q is in no shard of %d", name, n)
			}
		}
		if len(owner) != len(full) {
			t.Errorf("%d shards hold %d mutants, --list has %d", n, len(owner), len(full))
		}
	}
}

// A shard that is not K/N with 1 <= K <= N is refused rather than read as an
// empty sweep, which would pass.
func TestSweepShards_ABadShardIsRefused(t *testing.T) {
	root := moduleRoot(t)
	for _, bad := range []string{"0/3", "4/3", "1/0", "x", "1/", "/3", "-1/3", "1/2/3"} {
		if out, code := sweep(t, root, "--list", "--shard", bad); code == 0 {
			t.Errorf("--shard %q was accepted:\n%s", bad, out)
		}
	}
}

// SWEEP-FINISH1. --platform-only darwin keeps the mutants macOS alone can
// judge: every one declaring platform="darwin" and every one whose target is
// built for darwin and not for linux. It is a subset of --list, it is not
// empty, and it composes with --shard without losing or repeating a row.
func TestSweepPlatformOnly_KeepsTheDarwinRows(t *testing.T) {
	root := moduleRoot(t)
	out, code := sweep(t, root, "--list")
	if code != 0 {
		t.Fatalf("--list exited %d", code)
	}
	full := map[string]bool{}
	for _, n := range nonEmptyLines(out) {
		full[n] = true
	}
	out, code = sweep(t, root, "--list", "--platform-only", "darwin")
	if code != 0 {
		t.Fatalf("--list --platform-only darwin exited %d", code)
	}
	rows := nonEmptyLines(out)
	if len(rows) == 0 || len(rows) >= len(full) {
		t.Fatalf("--platform-only darwin kept %d of %d mutants", len(rows), len(full))
	}
	kept := map[string]bool{}
	for _, n := range rows {
		if !full[n] {
			t.Errorf("%q is not in --list", n)
		}
		kept[n] = true
	}
	for _, want := range []string{
		"ST-2.3 a post whose flush and marker both failed runs the step",
		"ST-2.3 the step prints before its line is durable",
		"REC-34 the barrier issues F_FULLFSYNC",
	} {
		if !kept[want] {
			t.Errorf("--platform-only darwin dropped %q", want)
		}
	}
	for _, other := range []string{"ST-2.3 consent read from consent.json, not the marker"} {
		if kept[other] {
			t.Errorf("--platform-only darwin kept %q, which every GOOS judges", other)
		}
	}
	owner := map[string]int{}
	for k := 1; k <= 2; k++ {
		out, _ := sweep(t, root, "--list", "--platform-only", "darwin", "--shard", fmt.Sprintf("%d/2", k))
		for _, n := range nonEmptyLines(out) {
			owner[n]++
		}
	}
	if len(owner) != len(rows) {
		t.Errorf("two shards of the darwin rows hold %d, want %d", len(owner), len(rows))
	}
	for n, c := range owner {
		if c != 1 {
			t.Errorf("%q is in %d shards", n, c)
		}
	}
}

// A GOOS the sweep does not know is refused rather than read as an empty
// sweep, which would pass.
func TestSweepPlatformOnly_AnUnknownGOOSIsRefused(t *testing.T) {
	root := moduleRoot(t)
	if out, code := sweep(t, root, "--list", "--platform-only", "plan10"); code == 0 {
		t.Errorf("--platform-only plan10 was accepted:\n%s", out)
	}
}

// The workflow runs the darwin rows on a macOS runner, strictly.
func TestSweepJobRunsTheDarwinRows(t *testing.T) {
	doc := readFile(t, filepath.Join(moduleRoot(t), ".github", "workflows", "ci.yaml"))
	i := strings.Index(doc, "\n  sweep-darwin:\n")
	if i < 0 {
		t.Fatal("ci.yaml: no sweep-darwin job")
	}
	job := doc[i+1:]
	if j := strings.Index(job, "\n  measure-linux-benchmarks:\n"); j >= 0 {
		job = job[:j]
	}
	for _, want := range []string{"runs-on: macos-latest", "github.event_name == 'workflow_dispatch'", "sweep.py --strict --platform-only darwin"} {
		if !strings.Contains(job, want) {
			t.Errorf("the sweep-darwin job lacks %q", want)
		}
	}
}

// ciJob returns job key's lines in the workflow, from its key to the next
// job's.
func ciJob(t *testing.T, doc, key string) string {
	t.Helper()
	i := strings.Index(doc, "\n  "+key+":\n")
	if i < 0 {
		t.Fatalf("ci.yaml: no %s job", key)
	}
	job := doc[i+1:]
	if loc := regexp.MustCompile(`\n  [a-z0-9-]+:\n`).FindStringIndex(job); loc != nil {
		job = job[:loc[0]+1]
	}
	return job
}

// The REC-34 group commit mutants run on every push and pull request, on
// macOS, in a job of their own beside Perf gates (macos), not inside it: at
// 15 to 18 minutes they pushed that job past its 25-minute bound, which
// skipped the H-16 and TRU darwin steps after them (R26, CI-PERF-SPLIT1). No
// needs:, so a red perf gate skips none of them, and a bound with room for
// their growth.
func TestSweepJobRunsTheREC34RowsBesidePerfGates(t *testing.T) {
	doc := readFile(t, filepath.Join(moduleRoot(t), ".github", "workflows", "ci.yaml"))
	if strings.Contains(ciJob(t, doc, "perf-macos"), "sweep.py") {
		t.Error("perf-macos runs a mutation sweep: its 25 minutes are the gates'")
	}
	job := ciJob(t, doc, "rec34-darwin")
	for _, want := range []string{"runs-on: macos-latest", "sweep.py --strict --files mutants/REC-34_group-commit.py"} {
		if !strings.Contains(job, want) {
			t.Errorf("the rec34-darwin job lacks %q", want)
		}
	}
	for _, refuse := range []string{"\n    needs:", "\n    if:"} {
		if strings.Contains(job, refuse) {
			t.Errorf("the rec34-darwin job has %q: it runs on every push and pull request, on its own", strings.TrimSpace(refuse))
		}
	}
	m := regexp.MustCompile(`\n    timeout-minutes: (\d+)\n`).FindStringSubmatch(job)
	if m == nil {
		t.Fatal("the rec34-darwin job has no timeout-minutes")
	}
	if n, _ := strconv.Atoi(m[1]); n < 40 {
		t.Errorf("the rec34-darwin job's bound is %d minutes; its rows took 15 to 18 at 665 test files, want 40 or more", n)
	}
}

// The workflow's matrix and the --shard each job passes name the same count,
// or a shard is never run.
func TestSweepJobRunsEveryShard(t *testing.T) {
	root := moduleRoot(t)
	doc := readFile(t, filepath.Join(root, ".github", "workflows", "ci.yaml"))
	i := strings.Index(doc, "\n  sweep:\n")
	next := strings.Index(doc[i+1:], "\n  fuzz:\n")
	if i < 0 || next < 0 {
		t.Fatal("ci.yaml: sweep: or the job after it not found")
	}
	job := doc[i : i+1+next]
	m := regexp.MustCompile(`shard: \[([0-9, ]+)\]`).FindStringSubmatch(job)
	if m == nil {
		t.Fatal("the sweep job has no matrix of shards")
	}
	var n int
	for _, f := range strings.Split(m[1], ",") {
		n++
		if got, _ := strconv.Atoi(strings.TrimSpace(f)); got != n {
			t.Fatalf("matrix shard %d is %q, want %d", n, f, n)
		}
	}
	want := "--shard ${{ matrix.shard }}/" + strconv.Itoa(n)
	if !strings.Contains(job, "sweep.py --strict "+want) {
		t.Errorf("the sweep step does not run `sweep.py --strict %s`", want)
	}
}
