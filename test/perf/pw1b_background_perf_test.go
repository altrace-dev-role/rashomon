//go:build perf

package perf

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/altrace-dev-role/rashomon/internal/budget"
)

// pgraphFixture is a module of two packages, one calling the other, with a
// .git directory at its root, as the hook finds a project.
var pgraphFixture = map[string]string{
	"go.mod":  "module example.com/fixture\n\ngo 1.22\n",
	"a/a.go":  "package a\n\nimport \"example.com/fixture/b\"\n\n// A calls B.\nfunc A() int { return b.B() + 1 }\n",
	"b/b.go":  "package b\n\n// B is two.\nfunc B() int { return 2 }\n",
	"b/c.go":  "package b\n\n// C is B's caller in its own package.\nfunc C() int { return B() * B() }\n",
	"a/x.txt": "not Go\n",
}

// TestPW1b_TheBackgroundHookStaysWithinItsRow: pgraph sync -hook, built with
// the release flags, on a fixture its first run already read: the median
// peak RSS of nine runs stays within the background row on the reference
// platform and reports elsewhere (backgroundBudget). The nine readings, the
// wall median and the first run's load are logged, since the row's
// information figures come from this log. Each of the nine must leave the
// graph as it was: a run that loaded again is not the run the row prices.
func TestPW1b_TheBackgroundHookStaysWithinItsRow(t *testing.T) {
	b := loadBudgets(t)
	dir := t.TempDir()
	bin := filepath.Join(dir, "pgraph")
	build := exec.Command("go", "build", "-trimpath", "-ldflags", "-s -w", "-o", bin, "./cmd/pgraph")
	build.Dir = filepath.Join(root, "pgraph")
	build.Env = append(os.Environ(), "CGO_ENABLED=0")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("building pgraph: %v\n%s", err, out)
	}
	repo := filepath.Join(dir, "repo")
	for name, body := range pgraphFixture {
		p := filepath.Join(repo, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Mkdir(filepath.Join(repo, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	stdin := []byte(`{"hook_event_name":"Stop","cwd":` + strconv.Quote(filepath.Join(repo, "a")) + `}`)
	var env []string
	for _, kv := range os.Environ() {
		if !strings.HasPrefix(kv, "CLAUDE") {
			env = append(env, kv)
		}
	}
	run := func() (rss, ms float64, err error) {
		cmd := exec.Command(bin, "sync", "-hook")
		cmd.Stdin = bytes.NewReader(stdin)
		cmd.Env = measuredEnv(env, runtime.GOOS)
		var out, errb bytes.Buffer
		cmd.Stdout, cmd.Stderr = &out, &errb
		start := time.Now()
		rss, err = reaper.Run(cmd)
		ms = float64(time.Since(start).Microseconds()) / 1000
		if err == nil && (out.Len() != 0 || errb.Len() != 0) {
			t.Fatalf("pgraph sync -hook printed: stdout %q, stderr %q", out.String(), errb.String())
		}
		return rss, ms, err
	}
	loadRSS, loadMs, err := run()
	if err != nil {
		t.Fatalf("the first run: %v", err)
	}
	walk := filepath.Join(repo, ".pgraph", "pgraph.walk")
	before, err := os.ReadFile(walk)
	if err != nil {
		t.Fatalf("the first run wrote no graph: %v", err)
	}
	logInfo, err := os.Stat(filepath.Join(repo, ".pgraph", "pgraph.log"))
	if err != nil {
		t.Fatal(err)
	}
	var xs, walls []float64
	for i := 0; i < runs; i++ {
		rss, ms, err := run()
		if notAttributable, floorWhy := floorOutcome(err); notAttributable {
			reportOnlyRSS(t, BackgroundGate("pgraph.sync_hook"), "pgraph.sync_hook", rss, floorWhy)
			return
		}
		if err != nil {
			t.Fatalf("run %d: %v", i, err)
		}
		xs, walls = append(xs, rss), append(walls, ms)
	}
	after, err := os.ReadFile(walk)
	if err != nil {
		t.Fatal(err)
	}
	li, err := os.Stat(filepath.Join(repo, ".pgraph", "pgraph.log"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) || !li.ModTime().Equal(logInfo.ModTime()) {
		t.Fatal("a run on the unchanged fixture wrote the graph again: it loaded, so it is not the run the row prices")
	}
	got, _ := Median(xs)
	wall, _ := Median(walls)
	t.Logf("pgraph sync -hook: first run (load) %.2f MiB, %.0f ms; unchanged tree, nine peak RSS readings in MiB: %.2f, median %.2f; wall median %.1f ms",
		loadRSS, loadMs, xs, got, wall)
	var row budget.Background
	for _, r := range b.Background {
		if strings.Join(r.Command, " ") == "pgraph sync -hook" {
			row = r
		}
	}
	if row.ID == "" {
		ran[BackgroundGate("pgraph.sync_hook")] = true
		t.Fatal("budgets.json has no background row for pgraph sync -hook")
	}
	budgetMiB, gated, why := backgroundBudget(platform(), row)
	if gated {
		overBudget(t, b, BackgroundGate(row.ID), row.ID, got, budgetMiB)
	} else {
		reportOnlyRSS(t, BackgroundGate(row.ID), row.ID, got, why)
	}
}
