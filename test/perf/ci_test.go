package perf

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/altrace-dev-role/rashomon/internal/budget"
)

// workflowSteps reads a workflow by its lines into job name -> step texts: a
// job is a key at two spaces under `jobs:`, a step starts at `      - `. No
// YAML parser, as with DEL-17's reader: the file is written by hand in this
// shape, and a step this reader cannot see fails the checks below.
func workflowSteps(doc string) map[string][]string {
	jobLine := regexp.MustCompile(`^  ([A-Za-z0-9_-]+):\s*$`)
	jobs := map[string][]string{}
	inJobs, job := false, ""
	for _, ln := range strings.Split(doc, "\n") {
		switch {
		case ln == "jobs:":
			inJobs = true
			continue
		case !inJobs:
			continue
		case strings.TrimSpace(ln) != "" && !strings.HasPrefix(ln, " ") && !strings.HasPrefix(ln, "#"):
			inJobs = false
			continue
		}
		if m := jobLine.FindStringSubmatch(ln); m != nil {
			job = m[1]
			continue
		}
		if job == "" {
			continue
		}
		if strings.HasPrefix(ln, "      - ") {
			jobs[job] = append(jobs[job], "")
		}
		if n := len(jobs[job]); n > 0 {
			jobs[job][n-1] += ln + "\n"
		}
	}
	return jobs
}

// piped reports whether a step's commands pipe one into another.
func piped(step string) bool {
	for _, ln := range strings.Split(step, "\n") {
		trim := strings.TrimSpace(ln)
		if strings.HasPrefix(trim, "#") || strings.HasSuffix(trim, "run: |") {
			continue
		}
		if strings.Contains(strings.ReplaceAll(trim, "||", ""), " | ") {
			return true
		}
	}
	return false
}

// TestRES36h_PerfJobsRunPinnedAndFailClosed: the perf jobs build with the
// toolchain budgets.json was measured under (go_version; the CI failure on
// #59 was go1.25.8 against go1.27.2 rows); every piped step runs under
// `shell: bash`, which GitHub runs with -o pipefail, so a failed `go test`
// before `| benchgate` cannot pass on benchgate's exit; and the RSS steps
// require every owed gate to have run.
func TestRES36h_PerfJobsRunPinnedAndFailClosed(t *testing.T) {
	b := loadBudgets(t)
	if !regexp.MustCompile(`^go\d+\.\d+\.\d+$`).MatchString(b.GoVersion) {
		t.Fatalf("budgets.json go_version %q is not a released toolchain", b.GoVersion)
	}
	raw, err := os.ReadFile(filepath.Join(moduleRoot(t), ".github", "workflows", "ci.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	jobs := workflowSteps(string(raw))
	pin := `go-version: "` + strings.TrimPrefix(b.GoVersion, "go") + `"`
	for _, job := range []string{"perf-linux", "perf-macos"} {
		steps := jobs[job]
		pinned, rss := false, 0
		for _, s := range steps {
			if strings.Contains(s, "actions/setup-go@") {
				pinned = strings.Contains(s, pin)
			}
			// The harness is ./test/perf. Another package built with the tag,
			// the detector replay of test/acceptance, owes it no gates.
			if strings.Contains(s, "-tags perf ./test/perf") {
				rss++
				// A red benchgate before it skipped the RSS gates on every
				// run while linux had no benchmark rows.
				if !strings.Contains(s, "        if: success() || failure()\n") {
					t.Errorf("%s: the RSS step runs only when every step before it passed:\n%s", job, s)
				}
				if !strings.Contains(s, RequireAllEnv+`: "1"`) {
					t.Errorf("%s: the RSS step does not require every gate to run:\n%s", job, s)
				}
			}
		}
		if !pinned {
			t.Errorf("%s: setup-go is not pinned to %s, the toolchain budgets.json was measured under", job, pin)
		}
		if rss == 0 {
			t.Errorf("%s: no step runs the perf-tagged harness", job)
		}
	}
	for job, steps := range jobs {
		for _, s := range steps {
			if piped(s) && !strings.Contains(s, "shell: bash\n") {
				t.Errorf("%s: a piped step runs without `shell: bash`, so without pipefail:\n%s", job, s)
			}
		}
	}
}

// TestRES29_CIChecksTheRSSBoundPastARedGate: the linux perf job is the only
// place RES-29's peak-RSS bound is checked (the test job runs under -race,
// where it skips), and its step runs whatever the steps before it did. With
// no `if:` it ran only on success, so a benchgate exit -- 2 while
// budgets.json had no linux rows -- skipped it on every run.
func TestRES29_CIChecksTheRSSBoundPastARedGate(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join(moduleRoot(t), ".github", "workflows", "ci.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, s := range workflowSteps(string(raw))["perf-linux"] {
		if !strings.Contains(s, "TestRES29_PathologicalInputsStayBounded") {
			continue
		}
		found = true
		if !regexp.MustCompile(`(?m)^        if: (success\(\) \|\| failure\(\)|always\(\)|\$\{\{ !cancelled\(\) \}\}|!cancelled\(\))\s*$`).MatchString(s) {
			t.Errorf("perf-linux: the RES-29 step runs only when every step before it passed:\n%s", s)
		}
	}
	if !found {
		t.Error("perf-linux: no step runs TestRES29_PathologicalInputsStayBounded")
	}
}

// owedGateTests is the test that marks each kind of gate Ungated owes (the
// gate's name up to its first space): the test a perf run must select, or
// RequireAllEnv fails the run with that gate unrun.
var owedGateTests = map[string]string{
	"init":       "TestRES2_InitBudget",
	"entry":      "TestRES28b_EveryEntryStaysWithinItsRow",
	"case":       "TestRES28_MaxRSSFollowsThePayloadBudget",
	"cold":       "TestColdAllocBudget",
	"background": "TestPW1b_TheBackgroundHookStaysWithinItsRow",
}

// TestRES36i_EveryPerfRunSelectsEveryOwedGatesTest: every kind of gate
// Ungated can owe (one budgets row of each kind) has its test in
// owedGateTests, that test is declared in this package, and every perf job's
// RSS step selects it with its -run pattern. The background row (PW1b-ON-230
// 1b) was owed and never selected, so CI's Perf gate failed with "1 owed
// gates did not run: background pgraph.sync_hook" while every local run
// named the test in its own -run (B329). Break: add a gate kind with no
// test here, or drop a test from a job's -run, and this fails.
func TestRES36i_EveryPerfRunSelectsEveryOwedGatesTest(t *testing.T) {
	b := budget.Budgets{
		Entries:    []budget.Entry{{ID: "e", ColdAllocBytesMax: 1}},
		RSSCases:   []budget.RSSCase{{ID: "c", GatesFrom: "P-08"}},
		Background: []budget.Background{{ID: "b"}},
	}
	tests := map[string]bool{}
	for _, g := range Ungated(b, nil) {
		kind, _, _ := strings.Cut(g, " ")
		name, ok := owedGateTests[kind]
		if !ok {
			t.Errorf("gate %q: its kind %q has no test in owedGateTests", g, kind)
			continue
		}
		tests[name] = true
	}
	src, err := filepath.Glob(filepath.Join(moduleRoot(t), "test", "perf", "*_test.go"))
	if err != nil {
		t.Fatal(err)
	}
	var all strings.Builder
	for _, f := range src {
		raw, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		all.Write(raw)
	}
	for name := range tests {
		if !strings.Contains(all.String(), "\nfunc "+name+"(t *testing.T) {") {
			t.Errorf("%s: owedGateTests names it and no file in test/perf declares it", name)
		}
	}
	raw, err := os.ReadFile(filepath.Join(moduleRoot(t), ".github", "workflows", "ci.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	runRE := regexp.MustCompile(`-tags perf \./test/perf -run '([^']+)'`)
	for job, steps := range workflowSteps(string(raw)) {
		for _, s := range steps {
			m := runRE.FindStringSubmatch(s)
			if m == nil {
				continue
			}
			re, err := regexp.Compile(m[1])
			if err != nil {
				t.Errorf("%s: -run %q: %v", job, m[1], err)
				continue
			}
			for name := range tests {
				if !re.MatchString(name) {
					t.Errorf("%s: -run %q does not select %s, so its gate never runs", job, m[1], name)
				}
			}
		}
	}
}
