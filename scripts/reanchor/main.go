// Command reanchor re-anchors rows of test/perf/budgets.json from the
// artifacts of one CI run (BENCH-PROVENANCE1, RSS-GROWTH1, PF-BENCH-ENV1).
//
// A row is anchored where it is gated: darwin/arm64 rows on the macos
// runner's own Perf gates job, linux/amd64 rows on the ubuntu runner's,
// each under the toolchain budgets.json pins (D214, D215, Q1). The reference
// machine stays an instrument for diagnosing drift, not an anchor.
//
//	go run ./scripts/reanchor --run 41166204201 --platform darwin/arm64 --report --all
//	go run ./scripts/reanchor --run 41166204201 --platform darwin/arm64 --rows RunPost/edit_314k,hook.400B
//	go run ./scripts/reanchor --run 41166204201 --platform linux/amd64 --provisional
//	go run ./scripts/reanchor --run 41166204201 --dir ./artifacts --platform linux/amd64 --all
//
// Inputs. --run names the CI run (it is what a row's basis cites). Without
// --dir the tool fetches with gh (run view, run download: read-only) the
// bench-macos or bench-linux artifact and the Perf gates job's log. --dir
// reads what is already downloaded: <dir>/bench-<macos|linux>/bench.txt (gh
// run download's layout) or <dir>/bench.txt, and optionally
// <dir>/perf-<macos|linux>.log or <dir>/perf.log. Benchmark rows come from
// bench.txt, taken by the rule benchgate gates with: per benchmark the one
// whole run with the lowest B/op (ties by allocs/op). RSS rows (the
// platform's entries and case_budgets) come from the job log, which is the
// only place CI records them (no job uploads an RSS artifact): the line
// "rss_test.go:522: hook.400B: 5.64 MiB against 5.26 (+10%)" carries the
// median of nine the gate read.
//
// Rows. --rows takes benchmark names (BenchmarkRunPost/edit_314k, or
// RunPost/edit_314k, or a prefix ending in *) and RSS ids (hook.400B,
// post.edit_314k); --provisional selects the benchmark rows whose basis says
// "provisional"; --all selects every row of the platform. --kind
// bench|rss narrows --all and --provisional.
//
// Output. budgets.json is edited in place by replacing the byte spans of the
// values that change, so every other byte stays: each row remains one line
// in json.dumps format and line 6 keeps the whole platforms object. A
// changed benchmark row gets measured and max both equal to the artifact's
// figure (TestRES36d: no hidden margin) and a basis naming the run, the
// runner and the toolchain; on darwin, the top-level entries whose benchmark
// is that row get their alloc_bytes_max set to match (TestRES36f). RSS rows
// carry no per-row basis field, so the platform's set_from gains one
// sentence naming the run, the runner, the toolchain and each old and new
// figure. Then go generate ./internal/budget. --report prints, per row, the
// old value, the artifact's and the delta, and writes nothing.
//
// Refusals (exit 2, nothing written): the toolchain the run used (from the
// job log's setup-go lines, or --toolchain) is not budgets.json's go_version,
// as benchgate refuses; the artifact's goos/goarch is not --platform; a
// selected row has no value in the artifact; a selector matches nothing.
//
// PF-BENCH-ENV1 findings (2026-10-31; bench-macos and bench-linux of four
// green runs, all go1.27.3: main 826aa3b7 (41113334565), PR #151 (41149672814),
// PR #156 (41153908829), PR #158 (41166204201); 74 rows a platform):
//
//   - The runners: macos-26-arm64, GOMAXPROCS 3, no GODEBUG; ubuntu-24.04,
//     GOMAXPROCS 4, no GODEBUG. The reference pair runs GOMAXPROCS=1 under
//     GODEBUG=gcstoptheworld=2.
//   - What the gate flagged is the anchor, not runner noise. The darwin row
//     for RunPost/edit_314k says 271 allocs/op; main's own runner reads 294
//     to 298 (+8.5% to +10%), so the row had 1.5 points of headroom left
//     before any card touched it, and #156's step (+8 allocs on this row and
//     on RecapPending/10, on both platforms alike) put it at 302 (+11.4%).
//     The linux row, 248, sat the same way (main 268 to 275, #156 280: +12.9%).
//   - Cross-run spread of a row's best-of-3 over the three runs before #156's
//     step (main, #151, #158): macos 3 rows move more than 2% on B/op
//     (OrderedAppendHold/serial_deferred 4.4%, serial_full 4.3%,
//     HookStoreSequence/full 3.1%) and RecapPending/preflight swings 11% on
//     allocs/op (1,462 to 1,467 against #158's 1,321); linux the same rows at
//     up to 4.5% and 3.6%. RunPost/edit_314k moves 0.03% on B/op and 0.3% on
//     allocs/op on macos, 0.3% and 1.5% on linux.
//   - Within one run the three -count repetitions of a row spread by up to
//     8.8% B/op (OrderedAppendHold/serial_deferred) and 4% allocs/op on
//     macos, 7.1% and 2.6% on linux (HookStoreSequence/full); best-of-3 is
//     what keeps that out of the figure, and why a row is the best run, never
//     a mean.
//   - GOMAXPROCS is a cause of that spread, measured locally (a 10-core
//     M-series Mac, go1.27.2, main's code, -benchtime 20x -count 3, twice):
//     under GOMAXPROCS=1 with gcstoptheworld=2 the rows repeat to the byte
//     (RunPost/edit_314k 473,392 B and 275 allocs, six times of six; RunHook
//     /bash_short 54,888 B and 375); under GOMAXPROCS=3 the same binary reads
//     473,900 to 476,056 B and 277 to 282 allocs on edit_314k; under 10,
//     476,451 to 479,402 B and 282 to 290. Concurrent collection adds the
//     background workers' own allocations and makes each repetition differ,
//     and the allocation count grows with the processor count. It explains
//     0.7 to 2.5 points of the 6.9 the runner reads over the reference
//     figure on edit_314k (294 against 275), not all of it: the runner's 294
//     is above this Mac's 277 to 282 at the same GOMAXPROCS, so the other
//     4 to 6 points are the machine, the macOS image and 1.27.3 against
//     1.27.2. Karthik's M3 at GOMAXPROCS=3 is still the discriminating run
//     (PF-BENCH-ENV1); this one is the direction, not the number.
//   - Headroom: a row anchored from the runner is the runner's figure with
//     the gate's ten percent above it, which clears the between-run spread
//     of every row but OrderedAppendHold/serial_* (4.3% to 4.5% on B/op
//     between runs, up to 8.8% across one run's three repetitions, which
//     best-of-3 absorbs) and, in one run, RecapPending/preflight on allocs.
//     So the macOS rows need no more than ten percent once anchored on the
//     runner; what they cannot absorb is a card's own step, which must
//     re-anchor its row from its own run.
package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/altrace-dev-role/rashomon/internal/budget"
)

// benchLinePattern is scripts/benchgate's, verbatim (a test compares the
// two): one `go test -bench` line with -benchmem figures.
const benchLinePattern = `^(Benchmark\S+?)(?:-\d+)?\s+\d+\s+[\d.]+ ns/op(?:\s+[\d.]+ [A-Za-z0-9/_-]+)*?\s+(\d+) B/op\s+(\d+) allocs/op`

const defaultRepo = "altrace-dev-role/rashomon"

// flagTolerance marks a row in --report whose artifact figure is more than
// this over its old value: the gate's ten percent.
const flagTolerance = 0.10

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr, time.Now()))
}

type options struct {
	run, repo, dir, platform, budgets, root string
	rows                                    string
	provisional, all, report, noGenerate    bool
	kind, toolchain, runner                 string
}

func run(args []string, stdout, stderr io.Writer, now time.Time) int {
	fs := flag.NewFlagSet("reanchor", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var o options
	fs.StringVar(&o.run, "run", "", "CI run id the artifacts come from (cited by every changed basis)")
	fs.StringVar(&o.repo, "repo", defaultRepo, "GitHub repository, for gh")
	fs.StringVar(&o.dir, "dir", "", "read already-downloaded artifacts from this directory instead of fetching")
	fs.StringVar(&o.platform, "platform", "", "linux/amd64 or darwin/arm64")
	fs.StringVar(&o.budgets, "budgets", "", "budgets.json to edit (default test/perf/budgets.json under the module root)")
	fs.StringVar(&o.rows, "rows", "", "comma-separated benchmark names and RSS ids")
	fs.BoolVar(&o.provisional, "provisional", false, "select the benchmark rows whose basis says provisional")
	fs.BoolVar(&o.all, "all", false, "select every row of the platform")
	fs.BoolVar(&o.report, "report", false, "print old, artifact and delta per row; write nothing (defaults to --all)")
	fs.BoolVar(&o.noGenerate, "no-generate", false, "skip go generate ./internal/budget")
	fs.StringVar(&o.kind, "kind", "all", "bench, rss or all: narrows --all and --provisional")
	fs.StringVar(&o.toolchain, "toolchain", "", "the run's toolchain (go1.27.3), when its job log is not available")
	fs.StringVar(&o.runner, "runner", "", "runner label for the basis (default ubuntu-latest or macos-latest)")
	date := fs.String("date", now.UTC().Format("2006-01-02"), "date the basis states")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if err := reanchor(o, *date, stdout, stderr); err != nil {
		fmt.Fprintln(stderr, "reanchor:", err)
		return 2
	}
	return 0
}

func reanchor(o options, date string, stdout, stderr io.Writer) error {
	osName := map[string]string{"linux/amd64": "linux", "darwin/arm64": "macos"}[o.platform]
	if osName == "" {
		return fmt.Errorf("--platform must be linux/amd64 or darwin/arm64, not %q", o.platform)
	}
	if o.run == "" || strings.Trim(o.run, "0123456789") != "" {
		return fmt.Errorf("--run must be the numeric CI run id the artifacts come from, not %q", o.run)
	}
	if o.kind != "bench" && o.kind != "rss" && o.kind != "all" {
		return fmt.Errorf("--kind must be bench, rss or all")
	}
	if o.report && o.rows == "" && !o.provisional {
		o.all = true
	}
	if o.rows == "" && !o.provisional && !o.all {
		return fmt.Errorf("select rows with --rows, --provisional or --all")
	}
	if o.budgets == "" {
		root, err := moduleRoot()
		if err != nil {
			return err
		}
		o.root = root
		o.budgets = filepath.Join(root, "test", "perf", "budgets.json")
	} else if o.root == "" {
		o.root = filepath.Dir(filepath.Dir(filepath.Dir(o.budgets)))
	}
	src, err := os.ReadFile(o.budgets)
	if err != nil {
		return err
	}
	b, err := budget.Decode(src)
	if err != nil {
		return err
	}

	art, err := loadArtifact(o, osName, stderr)
	if err != nil {
		return err
	}
	if art.Toolchain == "" {
		return fmt.Errorf("the run's toolchain is unknown (no job log, no --toolchain): refusing, since a row is one toolchain's figure")
	}
	if err := b.CheckToolchain(art.Toolchain); err != nil {
		return err
	}
	if art.GOOS+"/"+art.GOARCH != o.platform {
		return fmt.Errorf("the artifact was measured on %s/%s and --platform is %s: nothing compared", art.GOOS, art.GOARCH, o.platform)
	}
	runner := o.runner
	if runner == "" {
		runner = map[string]string{"linux": "ubuntu-latest", "macos": "macos-latest"}[osName]
	}
	meta := meta{Run: o.run, OS: osName, Runner: runner, Date: date, Art: art}

	p, err := plan(src, b, o, meta)
	if err != nil {
		return err
	}
	printReport(stdout, p, meta, o.platform)
	if o.report {
		return nil
	}
	out := p.apply(src)
	nb, err := budget.Decode(out)
	if err != nil {
		return fmt.Errorf("the edited file does not decode (nothing written): %w", err)
	}
	if err := nb.CheckToolchain(art.Toolchain); err != nil {
		return err
	}
	if bytes.Equal(out, src) {
		fmt.Fprintln(stdout, "reanchor: nothing changed")
		return nil
	}
	if err := os.WriteFile(o.budgets, out, 0o644); err != nil {
		return err
	}
	fmt.Fprintf(stdout, "reanchor: wrote %s (%d rows changed)\n", o.budgets, p.changed())
	if o.noGenerate {
		return nil
	}
	cmd := exec.Command("go", "generate", "./internal/budget")
	cmd.Dir = o.root
	cmd.Stdout, cmd.Stderr = stdout, stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("go generate ./internal/budget: %w", err)
	}
	fmt.Fprintln(stdout, "reanchor: go generate ./internal/budget done; run go test ./test/perf ./internal/budget ./scripts/benchgate -count=1")
	return nil
}

func moduleRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("no go.mod above the working directory; run from the module root or pass --budgets")
		}
		dir = parent
	}
}

// ---- artifacts ----------------------------------------------------------

type result struct{ Bytes, Allocs int64 }

// artifact is one run's evidence for one platform.
type artifact struct {
	Bench        map[string]result
	GOOS, GOARCH string
	Readings     map[string]float64 // RSS id -> median MiB, from the log
	HaveLog      bool
	Toolchain    string // "go1.27.3"
	Image        string // "macos-26-arm64 20261013.0424.1"
	Env          string // the harness's "GOMAXPROCS 3 of 3 CPUs, measured children under []"
	JobID        string
	JobConc      string
}

type meta struct {
	Run, OS, Runner, Date string
	Art                   *artifact
}

func loadArtifact(o options, osName string, stderr io.Writer) (*artifact, error) {
	dir := o.dir
	var jobID, jobConc string
	if dir == "" {
		tmp, err := os.MkdirTemp("", "reanchor-")
		if err != nil {
			return nil, err
		}
		dir = tmp
		jobID, jobConc, err = fetch(o, osName, dir, stderr)
		if err != nil {
			return nil, err
		}
	}
	var benchRaw, logRaw []byte
	for _, c := range []string{filepath.Join(dir, "bench-"+osName, "bench.txt"), filepath.Join(dir, "bench.txt")} {
		if raw, err := os.ReadFile(c); err == nil {
			benchRaw = raw
			break
		}
	}
	for _, c := range []string{filepath.Join(dir, "perf-"+osName+".log"), filepath.Join(dir, "perf.log")} {
		if raw, err := os.ReadFile(c); err == nil {
			logRaw = raw
			break
		}
	}
	if benchRaw == nil {
		return nil, fmt.Errorf("no bench.txt under %s (bench-%s/bench.txt or bench.txt)", dir, osName)
	}
	res, goos, goarch, err := parseBench(bytes.NewReader(benchRaw))
	if err != nil {
		return nil, err
	}
	a := &artifact{Bench: res, GOOS: goos, GOARCH: goarch, Readings: map[string]float64{}, JobID: jobID, JobConc: jobConc}
	if logRaw != nil {
		a.HaveLog = true
		if err := parseLog(logRaw, a); err != nil {
			return nil, err
		}
	}
	if o.toolchain != "" {
		a.Toolchain = o.toolchain
	}
	return a, nil
}

// fetch downloads the run's bench artifact and the Perf gates job's log with
// gh, read-only. It returns the job's id and conclusion.
func fetch(o options, osName, dir string, stderr io.Writer) (string, string, error) {
	gh := func(args ...string) ([]byte, error) {
		cmd := exec.Command("gh", args...)
		var eb bytes.Buffer
		cmd.Stderr = &eb
		out, err := cmd.Output()
		if err != nil {
			return nil, fmt.Errorf("gh %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(eb.String()))
		}
		return out, nil
	}
	raw, err := gh("run", "view", o.run, "--repo", o.repo, "--json", "jobs")
	if err != nil {
		return "", "", err
	}
	var jobs struct {
		Jobs []struct {
			ID         int64  `json:"databaseId"`
			Name       string `json:"name"`
			Conclusion string `json:"conclusion"`
		} `json:"jobs"`
	}
	if err := json.Unmarshal(raw, &jobs); err != nil {
		return "", "", err
	}
	var id, conc string
	for _, j := range jobs.Jobs {
		if j.Name == "Perf gates ("+osName+")" {
			id, conc = strconv.FormatInt(j.ID, 10), j.Conclusion
		}
	}
	if id == "" {
		return "", "", fmt.Errorf("run %s has no job named %q", o.run, "Perf gates ("+osName+")")
	}
	if _, err := gh("run", "download", o.run, "--repo", o.repo, "-n", "bench-"+osName, "-D", dir); err != nil {
		return "", "", err
	}
	logRaw, err := gh("run", "view", o.run, "--repo", o.repo, "--log", "--job", id)
	if err != nil {
		return "", "", err
	}
	if err := os.WriteFile(filepath.Join(dir, "perf-"+osName+".log"), logRaw, 0o644); err != nil {
		return "", "", err
	}
	if conc != "success" {
		fmt.Fprintf(stderr, "reanchor: warning: run %s's Perf gates (%s) job concluded %q\n", o.run, osName, conc)
	}
	return id, conc, nil
}

// parseBench keeps, per benchmark, the one whole run with the lowest B/op
// (ties by allocs/op): benchgate's ParseAll rule, which gates, so what
// anchors a row is what the gate then reads.
func parseBench(r io.Reader) (map[string]result, string, string, error) {
	re := regexp.MustCompile(benchLinePattern)
	best := map[string]result{}
	var goos, goarch string
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, bufio.MaxScanTokenSize), 1<<20)
	for sc.Scan() {
		ln := sc.Text()
		for _, h := range []struct {
			key string
			dst *string
		}{{"goos", &goos}, {"goarch", &goarch}} {
			if v, ok := strings.CutPrefix(ln, h.key+": "); ok {
				v = strings.TrimSpace(v)
				if *h.dst != "" && *h.dst != v {
					return nil, "", "", fmt.Errorf("mixed platforms in one benchmark output: %s %s then %s", h.key, *h.dst, v)
				}
				*h.dst = v
			}
		}
		if !strings.HasPrefix(ln, "Benchmark") || !strings.Contains(ln, "ns/op") {
			continue
		}
		m := re.FindStringSubmatch(ln)
		if m == nil {
			return nil, "", "", fmt.Errorf("benchmark line without allocation figures: %q", ln)
		}
		bpo, _ := strconv.ParseInt(m[2], 10, 64)
		apo, _ := strconv.ParseInt(m[3], 10, 64)
		run := result{bpo, apo}
		if prev, ok := best[m[1]]; !ok || run.Bytes < prev.Bytes || (run.Bytes == prev.Bytes && run.Allocs < prev.Allocs) {
			best[m[1]] = run
		}
	}
	if err := sc.Err(); err != nil {
		return nil, "", "", err
	}
	if len(best) == 0 {
		return nil, "", "", fmt.Errorf("the bench output holds no benchmark results")
	}
	return best, goos, goarch, nil
}

var (
	rssGatedRe  = regexp.MustCompile(`rss_test\.go:\d+: (\S+): ([0-9]+\.[0-9]+) MiB against [0-9.]+ \(`)
	rssOverRe   = regexp.MustCompile(`rss_test\.go:\d+: (\S+): median peak RSS ([0-9]+\.[0-9]+) MiB over budget`)
	goSetupRe   = regexp.MustCompile(`Successfully set up Go version (\S+)`)
	imageRe     = regexp.MustCompile(`Image: (\S+)`)
	imageVerRe  = regexp.MustCompile(`Version: (\d\S*)`)
	harnessEnvR = regexp.MustCompile(`perf: \S+, THP sysfs .*?(GOMAXPROCS \d+ of \d+ CPUs, measured children under .*)$`)
)

// parseLog reads RSS medians, the toolchain, the image and the harness's
// settings from a Perf gates job log. A row logged twice with two values is
// an error: which one anchored would be a guess.
func parseLog(raw []byte, a *artifact) error {
	sawImage := false
	sc := bufio.NewScanner(bytes.NewReader(raw))
	sc.Buffer(make([]byte, 0, bufio.MaxScanTokenSize), 1<<20)
	for sc.Scan() {
		ln := sc.Text()
		for _, re := range []*regexp.Regexp{rssGatedRe, rssOverRe} {
			if m := re.FindStringSubmatch(ln); m != nil {
				v, _ := strconv.ParseFloat(m[2], 64)
				if prev, ok := a.Readings[m[1]]; ok && prev != v {
					return fmt.Errorf("the log reads %s as both %.2f and %.2f MiB", m[1], prev, v)
				}
				a.Readings[m[1]] = v
			}
		}
		if m := goSetupRe.FindStringSubmatch(ln); m != nil && a.Toolchain == "" {
			a.Toolchain = "go" + strings.TrimPrefix(m[1], "go")
		}
		if m := imageRe.FindStringSubmatch(ln); m != nil && a.Image == "" {
			a.Image, sawImage = m[1], true
		} else if m := imageVerRe.FindStringSubmatch(ln); m != nil && sawImage && !strings.Contains(a.Image, " ") {
			a.Image += " " + m[1]
		}
		if m := harnessEnvR.FindStringSubmatch(ln); m != nil && a.Env == "" {
			a.Env = m[1]
		}
	}
	return sc.Err()
}

// ---- the edit plan -------------------------------------------------------

type edit struct {
	start, end int
	text       string
}

type reportRow struct {
	kind, name, old, art, delta string
	flag                        string
}

type planned struct {
	edits []edit
	rows  []reportRow
	notes []string
	n     map[string]bool
}

func (p *planned) changed() int { return len(p.n) }

func (p *planned) apply(src []byte) []byte {
	es := append([]edit(nil), p.edits...)
	sort.Slice(es, func(i, j int) bool { return es[i].start > es[j].start })
	out := append([]byte(nil), src...)
	for _, e := range es {
		out = append(out[:e.start], append([]byte(e.text), out[e.end:]...)...)
	}
	return out
}

type selector struct {
	exact, prefix []string
	all, prov     bool
	kind          string
	used          map[string]bool
}

func newSelector(o options) *selector {
	s := &selector{all: o.all, prov: o.provisional, kind: o.kind, used: map[string]bool{}}
	for _, r := range strings.Split(o.rows, ",") {
		if r = strings.TrimSpace(r); r == "" {
			continue
		}
		if p, ok := strings.CutSuffix(r, "*"); ok {
			s.prefix = append(s.prefix, p)
		} else {
			s.exact = append(s.exact, r)
		}
	}
	return s
}

func (s *selector) matches(kind, name, basis string) bool {
	hit := false
	for _, e := range s.exact {
		if e == name || (kind == "bench" && "Benchmark"+e == name) {
			s.used[e], hit = true, true
		}
	}
	for _, p := range s.prefix {
		if strings.HasPrefix(name, p) || (kind == "bench" && strings.HasPrefix(name, "Benchmark"+p)) {
			s.used[p+"*"], hit = true, true
		}
	}
	if s.kind == "all" || s.kind == kind {
		if s.all || (s.prov && kind == "bench" && strings.Contains(strings.ToLower(basis), "provisional")) {
			hit = true
		}
	}
	return hit
}

func (s *selector) unmatched() []string {
	var out []string
	for _, e := range s.exact {
		if !s.used[e] {
			out = append(out, e)
		}
	}
	for _, p := range s.prefix {
		if !s.used[p+"*"] {
			out = append(out, p+"*")
		}
	}
	return out
}

func plan(src []byte, b budget.Budgets, o options, m meta) (*planned, error) {
	p := &planned{n: map[string]bool{}}
	sel := newSelector(o)
	root, err := members(src, skipSpace(src, 0))
	if err != nil {
		return nil, err
	}
	rootAt := func(key string) (member, error) {
		mm, ok := root.get(key)
		if !ok {
			return member{}, fmt.Errorf("budgets.json has no %q", key)
		}
		return mm, nil
	}
	plats, err := rootAt("platforms")
	if err != nil {
		return nil, err
	}
	platMems, err := members(src, plats.vs)
	if err != nil {
		return nil, err
	}
	pm, ok := platMems.get(o.platform)
	if !ok {
		return nil, fmt.Errorf("budgets.json has no platform %s", o.platform)
	}
	pObj, err := members(src, pm.vs)
	if err != nil {
		return nil, err
	}

	// Benchmark rows: the top level's on the benchmark platform, else the
	// platform's own (RES-36p).
	var benchArr member
	if o.platform == b.BenchmarkPlatform {
		benchArr, err = rootAt("benchmarks")
		if err != nil {
			return nil, err
		}
	} else if benchArr, ok = pObj.get("benchmarks"); !ok {
		benchArr = member{}
	}
	var missing []string
	explicit := len(sel.exact)+len(sel.prefix) > 0
	if benchArr.vs != 0 && (o.kind != "rss" || explicit) {
		spans, err := elems(src, benchArr.vs)
		if err != nil {
			return nil, err
		}
		for _, sp := range spans {
			row, err := members(src, sp[0])
			if err != nil {
				return nil, err
			}
			name := row.str(src, "name")
			basis := row.str(src, "basis")
			if !sel.matches("bench", name, basis) {
				continue
			}
			r, ok := m.Art.Bench[name]
			oldB, oldA := row.int(src, "measured_bytes_per_op"), row.int(src, "measured_allocs_per_op")
			if !ok {
				missing = append(missing, name)
				continue
			}
			p.rows = append(p.rows, reportRow{"bench", name,
				fmt.Sprintf("%d B/op %d allocs/op", oldB, oldA),
				fmt.Sprintf("%d B/op %d allocs/op", r.Bytes, r.Allocs),
				fmt.Sprintf("%s B/op %s allocs/op", pct(float64(oldB), float64(r.Bytes)), pct(float64(oldA), float64(r.Allocs))),
				overFlag(float64(oldB), float64(r.Bytes), float64(oldA), float64(r.Allocs))})
			for _, k := range []struct {
				key string
				v   int64
			}{{"measured_bytes_per_op", r.Bytes}, {"measured_allocs_per_op", r.Allocs}, {"bytes_per_op_max", r.Bytes}, {"allocs_per_op_max", r.Allocs}} {
				mm, ok := row.get(k.key)
				if !ok {
					return nil, fmt.Errorf("row %s has no %s", name, k.key)
				}
				p.edits = append(p.edits, edit{mm.vs, mm.ve, strconv.FormatInt(k.v, 10)})
			}
			basisText := m.benchBasis(oldB, oldA)
			if bm, ok := row.get("basis"); ok {
				p.edits = append(p.edits, edit{bm.vs, bm.ve, pyString(basisText)})
			} else {
				// Before the object's closing brace, as json.dumps would append it.
				p.edits = append(p.edits, edit{sp[1] - 1, sp[1] - 1, `, "basis": ` + pyString(basisText)})
			}
			p.n[name] = true
			// An entry row's alloc_bytes_max is its benchmark's measurement
			// (TestRES36f); those rows are the benchmark platform's.
			if o.platform == b.BenchmarkPlatform {
				if err := p.syncEntries(src, root, name, r.Bytes); err != nil {
					return nil, err
				}
			}
		}
	}

	// RSS rows: this platform's entries (p50, and p95 with it, which equals
	// p50 on both platforms: "information only") and case_budgets.
	if o.kind != "bench" || explicit {
		var changedRSS []string
		if ents, ok := pObj.get("entries"); ok {
			em, err := members(src, ents.vs)
			if err != nil {
				return nil, err
			}
			for _, e := range em.list {
				if !sel.matches("rss", e.key, "") {
					continue
				}
				v, ok := m.Art.Readings[e.key]
				if !ok {
					missing = append(missing, e.key+" (rss: not in the job log)")
					continue
				}
				rm, err := members(src, e.vs)
				if err != nil {
					return nil, err
				}
				old := rm.float(src, "p50")
				p.addRSS("rss entry", e.key, old, v)
				if old != v {
					changedRSS = append(changedRSS, fmt.Sprintf("%s %s to %s", e.key, pyFloat(old), pyFloat(v)))
					for _, k := range []string{"p50", "p95"} {
						if mm, ok := rm.get(k); ok {
							p.edits = append(p.edits, edit{mm.vs, mm.ve, pyFloat(v)})
						}
					}
					p.n["entry:"+e.key] = true
				}
			}
		}
		if cb, ok := pObj.get("case_budgets"); ok {
			cm, err := members(src, cb.vs)
			if err != nil {
				return nil, err
			}
			for _, e := range cm.list {
				if !sel.matches("rss", e.key, "") {
					continue
				}
				v, ok := m.Art.Readings[e.key]
				if !ok {
					missing = append(missing, e.key+" (rss: not in the job log)")
					continue
				}
				old, _ := strconv.ParseFloat(string(src[e.vs:e.ve]), 64)
				p.addRSS("rss case", e.key, old, v)
				if old != v {
					changedRSS = append(changedRSS, fmt.Sprintf("%s %s to %s", e.key, pyFloat(old), pyFloat(v)))
					p.edits = append(p.edits, edit{e.vs, e.ve, pyFloat(v)})
					p.n["case:"+e.key] = true
				}
			}
		}
		if len(changedRSS) > 0 {
			sf, ok := pObj.get("set_from")
			if !ok {
				return nil, fmt.Errorf("platform %s has no set_from to carry the RSS basis", o.platform)
			}
			var old string
			if err := json.Unmarshal(src[sf.vs:sf.ve], &old); err != nil {
				return nil, err
			}
			p.edits = append(p.edits, edit{sf.vs, sf.ve, pyString(old + " " + m.rssBasis(changedRSS))})
		}
	}
	if len(missing) > 0 {
		return nil, fmt.Errorf("selected rows with no value in the artifact (nothing written): %s", strings.Join(missing, ", "))
	}
	if un := sel.unmatched(); len(un) > 0 {
		return nil, fmt.Errorf("selectors matching no row on %s: %s", o.platform, strings.Join(un, ", "))
	}
	if len(p.rows) == 0 {
		return nil, fmt.Errorf("no row selected")
	}
	return p, nil
}

func (p *planned) addRSS(kind, name string, old, v float64) {
	p.rows = append(p.rows, reportRow{kind, name, pyFloat(old) + " MiB", pyFloat(v) + " MiB", pct(old, v), overFlag(old, v, 0, 0)})
}

// syncEntries sets alloc_bytes_max on every top-level entry naming bench.
func (p *planned) syncEntries(src []byte, root *memberList, bench string, bytesPerOp int64) error {
	ents, ok := root.get("entries")
	if !ok {
		return nil
	}
	spans, err := elems(src, ents.vs)
	if err != nil {
		return err
	}
	for _, sp := range spans {
		e, err := members(src, sp[0])
		if err != nil {
			return err
		}
		if e.str(src, "benchmark") != bench {
			continue
		}
		am, ok := e.get("alloc_bytes_max")
		if !ok {
			continue
		}
		if string(src[am.vs:am.ve]) != strconv.FormatInt(bytesPerOp, 10) {
			p.notes = append(p.notes, fmt.Sprintf("entry %s: alloc_bytes_max %s to %d (its benchmark's B/op, TestRES36f); check cold_alloc_bytes_max still passes TestRES36l", e.str(src, "id"), src[am.vs:am.ve], bytesPerOp))
		}
		p.edits = append(p.edits, edit{am.vs, am.ve, strconv.FormatInt(bytesPerOp, 10)})
	}
	return nil
}

func (m meta) runnerDesc() string {
	s := m.Runner
	if m.Art.Image != "" {
		s += " image " + m.Art.Image
	}
	if m.Art.JobID != "" {
		s += ", job " + m.Art.JobID
	}
	return s
}

func (m meta) benchBasis(oldB, oldA int64) string {
	s := fmt.Sprintf("re-anchored %s from run %s's bench-%s artifact (%s, %s): best of three, -benchtime 20x, the lowest-B/op run of the three, the same rule benchgate gates with",
		m.Date, m.Run, m.OS, m.runnerDesc(), m.Art.Toolchain)
	if m.Art.Env != "" {
		s += "; " + m.Art.Env
	}
	return s + fmt.Sprintf("; replaces %d B/op and %d allocs/op", oldB, oldA)
}

func (m meta) rssBasis(changed []string) string {
	s := fmt.Sprintf("reanchor %s: %d RSS rows re-anchored from run %s's Perf gates (%s) job (%s, %s), each the job's median of nine through the reaper",
		m.Date, len(changed), m.Run, m.OS, m.runnerDesc(), m.Art.Toolchain)
	if m.Art.Env != "" {
		s += " (" + m.Art.Env + ")"
	}
	return s + ": " + strings.Join(changed, ", ") + "."
}

func printReport(w io.Writer, p *planned, m meta, platform string) {
	fmt.Fprintf(w, "run %s, %s, %s, %s", m.Run, platform, m.Runner, m.Art.Toolchain)
	if m.Art.Image != "" {
		fmt.Fprintf(w, ", image %s", m.Art.Image)
	}
	if m.Art.Env != "" {
		fmt.Fprintf(w, ", %s", m.Art.Env)
	}
	fmt.Fprintln(w)
	width := 0
	for _, r := range p.rows {
		if len(r.name) > width {
			width = len(r.name)
		}
	}
	for _, r := range p.rows {
		fmt.Fprintf(w, "%-9s %-*s  old %-32s artifact %-32s delta %s%s\n", r.kind, width, r.name, r.old, r.art, r.delta, r.flag)
	}
	for _, n := range p.notes {
		fmt.Fprintln(w, "note:", n)
	}
}

func pct(old, v float64) string {
	switch {
	case old == v:
		return "0.00%"
	case old == 0:
		return "n/a"
	}
	return fmt.Sprintf("%+.2f%%", (v-old)/old*100)
}

func overFlag(oldB, newB, oldA, newA float64) string {
	if (oldB != 0 && newB > oldB*(1+flagTolerance)) || (oldA != 0 && newA > oldA*(1+flagTolerance)) {
		return "  OVER10%"
	}
	return ""
}

// ---- json.dumps layout ----------------------------------------------------

// pyFloat is Python's repr of a float that json.dumps writes: shortest form,
// ".0" on an integral value.
func pyFloat(v float64) string {
	s := strconv.FormatFloat(v, 'f', -1, 64)
	if !strings.Contains(s, ".") {
		s += ".0"
	}
	return s
}

// pyString is json.dumps' string (ensure_ascii): non-ASCII as \uXXXX, no
// HTML escaping.
func pyString(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		switch {
		case r == '"':
			b.WriteString(`\"`)
		case r == '\\':
			b.WriteString(`\\`)
		case r == '\n':
			b.WriteString(`\n`)
		case r == '\r':
			b.WriteString(`\r`)
		case r == '\t':
			b.WriteString(`\t`)
		case r == '\b':
			b.WriteString(`\b`)
		case r == '\f':
			b.WriteString(`\f`)
		case r < 0x20:
			fmt.Fprintf(&b, `\u%04x`, r)
		case r < 0x7f || r == 0x7f:
			b.WriteRune(r)
		case r > 0xffff:
			r -= 0x10000
			fmt.Fprintf(&b, `\u%04x\u%04x`, 0xd800+(r>>10), 0xdc00+(r&0x3ff))
		default:
			fmt.Fprintf(&b, `\u%04x`, r)
		}
	}
	b.WriteByte('"')
	return b.String()
}

// ---- byte-span access to the JSON ------------------------------------------

type member struct {
	key    string
	vs, ve int
}

type memberList struct{ list []member }

func (l *memberList) get(key string) (member, bool) {
	for _, m := range l.list {
		if m.key == key {
			return m, true
		}
	}
	return member{}, false
}

func (l *memberList) str(src []byte, key string) string {
	m, ok := l.get(key)
	if !ok {
		return ""
	}
	var s string
	_ = json.Unmarshal(src[m.vs:m.ve], &s)
	return s
}

func (l *memberList) int(src []byte, key string) int64 {
	m, _ := l.get(key)
	v, _ := strconv.ParseInt(string(src[m.vs:m.ve]), 10, 64)
	return v
}

func (l *memberList) float(src []byte, key string) float64 {
	m, _ := l.get(key)
	v, _ := strconv.ParseFloat(string(src[m.vs:m.ve]), 64)
	return v
}

func skipSpace(src []byte, at int) int {
	for at < len(src) && (src[at] == ' ' || src[at] == '\n' || src[at] == '\t' || src[at] == '\r') {
		at++
	}
	return at
}

// members lists the members of the object starting at src[at] with the byte
// span of each value in src.
func members(src []byte, at int) (*memberList, error) {
	if at >= len(src) || src[at] != '{' {
		return nil, fmt.Errorf("offset %d is not an object", at)
	}
	dec := json.NewDecoder(bytes.NewReader(src[at:]))
	if _, err := dec.Token(); err != nil {
		return nil, err
	}
	l := &memberList{}
	for dec.More() {
		k, err := dec.Token()
		if err != nil {
			return nil, err
		}
		var raw json.RawMessage
		if err := dec.Decode(&raw); err != nil {
			return nil, err
		}
		end := at + int(dec.InputOffset())
		l.list = append(l.list, member{k.(string), end - len(raw), end})
	}
	return l, nil
}

// elems lists the spans of the elements of the array starting at src[at].
func elems(src []byte, at int) ([][2]int, error) {
	if at >= len(src) || src[at] != '[' {
		return nil, fmt.Errorf("offset %d is not an array", at)
	}
	dec := json.NewDecoder(bytes.NewReader(src[at:]))
	if _, err := dec.Token(); err != nil {
		return nil, err
	}
	var out [][2]int
	for dec.More() {
		var raw json.RawMessage
		if err := dec.Decode(&raw); err != nil {
			return nil, err
		}
		end := at + int(dec.InputOffset())
		out = append(out, [2]int{end - len(raw), end})
	}
	return out, nil
}
