package perf

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
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
			if strings.Contains(s, "-tags perf") {
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
