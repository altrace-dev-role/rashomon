//go:build !windows

package mutation

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"
	"unicode"
	"unicode/utf8"
)

// The CI Fuzz job (rubric fix list entry 23, test-1 and errors-7), read the
// way TestDEL17_EveryJobHasATimeout reads the workflows: by its lines.

// fuzzCommand returns the fuzz job's `go test ... -fuzz` line.
func fuzzCommand(t *testing.T, doc string) string {
	t.Helper()
	var found []string
	for _, ln := range strings.Split(doc, "\n") {
		if strings.Contains(ln, "go test ") && strings.Contains(ln, " -fuzz ") {
			found = append(found, strings.TrimSpace(ln))
		}
	}
	if len(found) != 1 {
		t.Fatalf("want one `go test ... -fuzz` line in ci.yaml, found %d: %q", len(found), found)
	}
	return found[0]
}

func ciWorkflow(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(moduleRoot(t), ".github", "workflows", "ci.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// TestRubricTest1_FuzzJobBoundsMinimization: every new interesting input is
// minimized before the fuzzer moves on, for up to a minute by default. On
// FuzzTranscriptLines, whose inputs each write a file a line, one such input
// took the whole 30-second window: 112 execs and nothing new, against 28,363
// execs and 167 new inputs with minimization bounded to ten runs. A target
// that spends its window minimizing is a target CI does not fuzz.
func TestRubricTest1_FuzzJobBoundsMinimization(t *testing.T) {
	cmd := fuzzCommand(t, ciWorkflow(t))
	fuzzTime := regexp.MustCompile(`-fuzztime (\d+)s\b`).FindStringSubmatch(cmd)
	if fuzzTime == nil {
		t.Fatalf("fuzz command has no -fuzztime in seconds: %s", cmd)
	}
	m := regexp.MustCompile(`-fuzzminimizetime (\d+)(x|s)\b`).FindStringSubmatch(cmd)
	if m == nil {
		t.Fatalf("fuzz command leaves minimization at its one-minute default, which can take a target's whole window: %s", cmd)
	}
	n, err := strconv.Atoi(m[1])
	if err != nil {
		t.Fatal(err)
	}
	window, err := strconv.Atoi(fuzzTime[1])
	if err != nil {
		t.Fatal(err)
	}
	switch {
	case m[2] == "x" && (n < 1 || n > 100):
		t.Errorf("-fuzzminimizetime %dx: want 1 to 100 runs per input", n)
	case m[2] == "s" && time.Duration(n)*time.Second*4 > time.Duration(window)*time.Second:
		t.Errorf("-fuzzminimizetime %ds is over a quarter of the %ds window", n, window)
	}
}

// TestRubricTest1_FuzzJobFloorIsTheTreeCount: the job fails if it fuzzes
// fewer targets than its floor, which only means something while the floor
// is the number the tree holds. A floor left behind when targets are added
// lets the job lose that many unnoticed. The tree's count is the targets go
// test would run on the job's runner (fuzzTargets), not lines that read
// "func Fuzz": a file a build tag leaves out of linux, or a function with the
// name and not the signature, is no target.
func TestRubricTest1_FuzzJobFloorIsTheTreeCount(t *testing.T) {
	m := regexp.MustCompile(`(?m)^\s*test "\$found" -ge (\d+)\s*$`).FindStringSubmatch(ciWorkflow(t))
	if m == nil {
		t.Fatal(`the fuzz job has no 'test "$found" -ge N' floor`)
	}
	floor, err := strconv.Atoi(m[1])
	if err != nil {
		t.Fatal(err)
	}
	if targets := fuzzTargets(t); floor != len(targets) {
		t.Errorf("the fuzz job's floor is %d, the tree holds %d fuzz targets: %v", floor, len(targets), targets)
	}
}

// fuzzTargets lists every fuzz target go test runs for linux/amd64, the Fuzz
// job's runner: go list names each package's test files for that build, and
// a target is a top-level func whose name go test takes as one (Fuzz, then
// anything but a lower-case letter) with a single *testing.F parameter.
func fuzzTargets(t *testing.T) []string {
	t.Helper()
	root := moduleRoot(t)
	cmd := exec.Command("go", "list", "-f", "{{.Dir}}{{range .TestGoFiles}} {{.}}{{end}}{{range .XTestGoFiles}} {{.}}{{end}}", "./...")
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "GOOS=linux", "GOARCH=amd64", "GOFLAGS=")
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("go list: %v", err)
	}
	var targets []string
	fset := token.NewFileSet()
	for _, ln := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		fields := strings.Fields(ln)
		if len(fields) < 2 {
			continue
		}
		dir := fields[0]
		for _, name := range fields[1:] {
			f, err := parser.ParseFile(fset, filepath.Join(dir, name), nil, parser.SkipObjectResolution)
			if err != nil {
				t.Fatal(err)
			}
			rel, err := filepath.Rel(root, dir)
			if err != nil {
				t.Fatal(err)
			}
			for _, fn := range fuzzFuncs(f) {
				targets = append(targets, rel+"."+fn)
			}
		}
	}
	return targets
}

// fuzzFuncs is go test's own test of a fuzz target, applied to one file.
func fuzzFuncs(f *ast.File) []string {
	testing := map[string]bool{} // the names "testing" is imported under
	for _, imp := range f.Imports {
		if imp.Path.Value != `"testing"` {
			continue
		}
		if imp.Name != nil {
			testing[imp.Name.Name] = true
		} else {
			testing["testing"] = true
		}
	}
	var out []string
	for _, d := range f.Decls {
		fn, ok := d.(*ast.FuncDecl)
		if !ok || fn.Recv != nil || !isFuzzName(fn.Name.Name) || fn.Type.Params == nil || len(fn.Type.Params.List) != 1 {
			continue
		}
		p := fn.Type.Params.List[0]
		star, ok := p.Type.(*ast.StarExpr)
		if !ok || len(p.Names) > 1 {
			continue
		}
		sel, ok := star.X.(*ast.SelectorExpr)
		if !ok || sel.Sel.Name != "F" {
			continue
		}
		if x, ok := sel.X.(*ast.Ident); ok && testing[x.Name] {
			out = append(out, fn.Name.Name)
		}
	}
	return out
}

func isFuzzName(name string) bool {
	rest, ok := strings.CutPrefix(name, "Fuzz")
	if !ok {
		return false
	}
	if rest == "" {
		return true
	}
	r, _ := utf8.DecodeRuneInString(rest)
	return !unicode.IsLower(r)
}

// TestRubricTest1_FuzzTargetsIsGoTestsRule: the count above takes what go
// test takes and nothing else.
func TestRubricTest1_FuzzTargetsIsGoTestsRule(t *testing.T) {
	src := `package p

import (
	"testing"
	tt "testing"
)

func Fuzz(f *testing.F)          {}
func FuzzA(f *testing.F)         {}
func Fuzz_b(f *tt.F)             {}
func Fuzzy(f *testing.F)         {}
func FuzzHelper(t *testing.T)    {}
func FuzzTwo(f *testing.F, n int) {}
func (x) FuzzMethod(f *testing.F) {}
`
	f, err := parser.ParseFile(token.NewFileSet(), "p_test.go", src, 0)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := strings.Join(fuzzFuncs(f), ","), "Fuzz,FuzzA,Fuzz_b"; got != want {
		t.Errorf("fuzz targets %s, want %s", got, want)
	}
}

// TestRubricTest1_FuzzJobRunsEveryTargetAndKeepsItsFailures: the job takes
// its targets from go test -list, counts a run only when it printed
// "fuzz: elapsed", runs every target before failing, prints a failing input
// only from a corpus directory that exists, and keeps the inputs as an
// artifact when it fails.
func TestRubricTest1_FuzzJobRunsEveryTargetAndKeepsItsFailures(t *testing.T) {
	doc := ciWorkflow(t)
	job := doc[strings.Index(doc, "\n  fuzz:\n"):]
	job = job[:strings.Index(job[1:], "\n  cross-build:")+1]
	for _, want := range []string{
		`go test "$pkg" -list '^Fuzz'`,
		`grep -q '^fuzz: elapsed' "$log"`,
		`failed+=("${pkg} ${name}")`,
		`if [ -d "$corpus" ]; then`,
		`if [ "${#failed[@]}" -gt 0 ]; then`,
		"if: failure()",
		"uses: actions/upload-artifact@",
		`path: "**/testdata/fuzz/**"`,
	} {
		if !strings.Contains(job, want) {
			t.Errorf("the fuzz job does not hold %s", want)
		}
	}
	if strings.Contains(job, "^func Fuzz") {
		t.Error("the fuzz job finds its targets by grep")
	}
	loop := job[strings.Index(job, "for pkg in"):strings.Index(job, `echo "fuzzed ${found} targets"`)]
	if strings.Contains(loop, "exit") {
		t.Error("the fuzz job exits inside its loop, so one failing target stops the rest")
	}
}
