// Package budget holds the resource budgets the perf gates and `rashomon
// disclose` read: test/perf/budgets.json is the one source, Table in
// budgets_gen.go is its generated Go form, and TestRES36_GeneratedBudgetsMatch
// keeps the two equal (tech spec 8.2.6; archive 85.3.13).
package budget

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
)

//go:generate go run ../../scripts/budgetsgen

// Decode reads raw as a Budgets, refusing any field the schema does not
// name: a misspelled key (case_budget for case_budgets, say) would otherwise
// decode as a silent zero rather than the override it was meant to be. Every
// reader of test/perf/budgets.json goes through this, so a schema change
// only has to be taught here once.
func Decode(raw []byte) (Budgets, error) {
	var b Budgets
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&b); err != nil {
		return Budgets{}, err
	}
	return b, nil
}

// Budgets is the shape of test/perf/budgets.json.
type Budgets struct {
	Version           int                 `json:"budgets_version"`
	GoVersion         string              `json:"go_version"`
	BenchmarkRows     string              `json:"benchmark_rows"`
	BenchmarkPlatform string              `json:"benchmark_platform"`
	Platforms         map[string]Platform `json:"platforms"`
	Entries           []Entry             `json:"entries"`
	RSSCases          []RSSCase           `json:"rss_cases"`
	PerToolCall       PerToolCall         `json:"per_tool_call"`
	Parallel          Parallel            `json:"parallel"`
	Benchmarks        []Benchmark         `json:"benchmarks"`
	Growth            []Growth            `json:"growth"`
	Disk              Disk                `json:"disk"`
}

// Platform is one gated platform: what the figures were measured on, how a
// platform without its own measurements derives them, and the fraction over
// budget its runner is still allowed (archive 85.3.13: 10%, and 15% on the
// macOS runner until 2026-11-06 while its variance is learned).
//
// Entries and CaseBudgets hold this platform's OWN readings, taken through
// the reaper on this platform's own runner. Both are empty for the reference
// platform the rest of this file's rows were measured on, because its rows
// already are that platform's own data; for any other platform, an entry or
// case absent from these maps has no reading of its own yet, and
// TestRES28b_ and TestRES28_ gate it report-only rather than assume it reads
// no higher than the reference platform's row, an assumption Linux's first
// run (archive 85.4.3) disproved for several of them.
//
// Benchmarks is the same idea for RES-36's allocation rows: a platform other
// than BenchmarkPlatform has none of its own B/op figures (long $TMPDIR
// paths on darwin skew B/op against the same benchmark on linux, so nothing
// here is derived from another platform's row), until a measure run on that
// platform's own CI runner fills it in (RES-36p; `go run ./scripts/benchgate
// -measure`). Empty for the reference platform, whose rows are the
// top-level Benchmarks slice.
type Platform struct {
	Reference      string             `json:"reference"`
	SetFrom        string             `json:"set_from"`
	Tolerance      float64            `json:"tolerance"`
	ToleranceUntil string             `json:"tolerance_until"`
	ToleranceAfter float64            `json:"tolerance_after"`
	Entries        map[string]RSS     `json:"entries"`
	CaseBudgets    map[string]float64 `json:"case_budgets,omitempty"`
	Benchmarks     []Benchmark        `json:"benchmarks,omitempty"`
}

// RSS is a peak-RSS budget pair in MiB.
type RSS struct {
	P50 float64 `json:"p50"`
	P95 float64 `json:"p95"`
}

// Entry is one hook entry on one payload class (archive 85.4.1). Two of its
// figures are gated. MaxRSSMiBP50 is the median peak RSS of nine runs on the
// reference machine, and TestRES28b_ fails a platform whose median is more
// than the platform's tolerance over it, or over the platform's own figure
// where it has one. AllocBytesMax, the bytes one call allocates
// (runtime.MemStats.TotalAlloc), is held equal to the measured B/op of the
// benchmark row it names (TestRES36f_), and benchgate gates that row: a hook
// process runs no GC cycle at these sizes, so its peak RSS is the floor plus
// what it allocated, and the allocation figure is the same on every platform.
// MaxRSSMiBP95, ParentMaxRSSMiBP50 and the wall-time figures are information
// only; no gate compares a measurement with them.
type Entry struct {
	ID                 string   `json:"id"`
	Events             []string `json:"events"`
	Command            []string `json:"command"`
	PayloadClass       string   `json:"payload_class"`
	PayloadBytes       int      `json:"payload_bytes"`
	Benchmark          string   `json:"benchmark"`
	AllocBytesMax      int64    `json:"alloc_bytes_max"`
	MaxRSSMiBP50       float64  `json:"maxrss_mib_p50"`
	MaxRSSMiBP95       float64  `json:"maxrss_mib_p95"`
	ParentMaxRSSMiBP50 *float64 `json:"parent_maxrss_mib_p50"`
	WallMsP50          float64  `json:"wall_ms_p50"`
	WallMsP95          float64  `json:"wall_ms_p95"`
	Basis              string   `json:"basis"`
}

// RSSCase is one RES-28 case: an entry on a payload of PayloadBytes, budgeted
// at 6.5 MiB + 2.2 MiB per MiB of payload. GatesFrom names the PR that
// switches the case on: the 1, 4 and 8 MiB cases gate from P-10, which builds
// the read bound they measure (archive 85.1.2), and the harness skips them
// until then, stating the reason.
type RSSCase struct {
	ID           string  `json:"id"`
	Entry        string  `json:"entry"`
	PayloadBytes int     `json:"payload_bytes"`
	BudgetMiB    float64 `json:"budget_mib"`
	GatesFrom    string  `json:"gates_from"`
}

// PerToolCall bounds the hooks of one tool call together (RES-46).
type PerToolCall struct {
	MemoryTimeMiBMsMax  float64        `json:"memory_time_mib_ms_max"`
	SummedPeakMiBMax    float64        `json:"summed_peak_mib_max"`
	ProcessCreationsMax map[string]int `json:"process_creations_max"`
}

// Parallel bounds a burst (RES-30): Ways hook-and-post pairs on one session
// at once, whose peak footprints sum to at most SummedPeakMiBMax, 64 x (6.5 +
// 4.5) MiB = 704 at 64-way (archive 85.4.4). Basis states the per-process
// rule test/perf's gate holds every pair to and the measurements it was
// restated from (ruling 2026-10-13), which TestRES30_BudgetsRowStatesTheGate
// keeps in step with the gate's constants.
type Parallel struct {
	Ways             int     `json:"ways"`
	SummedPeakMiBMax float64 `json:"summed_peak_mib_max"`
	Basis            string  `json:"basis"`
}

// Benchmark is one benchmark's allocation budget (RES-36): the measured best
// of three on the reference machine, carried beside the max it sets, which
// equals it; CI fails at more than 10% over the max, and that ten percent is
// the only margin anywhere (a row inflated above its measurement would hide
// a second one, which TestRES36d pins out).
type Benchmark struct {
	Name                string `json:"name"`
	MeasuredBytesPerOp  int64  `json:"measured_bytes_per_op"`
	MeasuredAllocsPerOp int64  `json:"measured_allocs_per_op"`
	BytesPerOpMax       int64  `json:"bytes_per_op_max"`
	AllocsPerOpMax      int64  `json:"allocs_per_op_max"`
}

// Growth bounds an order-of-growth test's allocation ratio.
type Growth struct {
	Test          string  `json:"test"`
	AllocRatioMax float64 `json:"alloc_ratio_max"`
}

// Disk bounds the bytes one call writes (RES-31).
type Disk struct {
	BytesPerCallMax int `json:"bytes_per_call_max"`
}

// CheckToolchain refuses to compare under any toolchain but the one the rows
// were measured under: allocation figures belong to a toolchain, not to the
// code alone (BenchmarkBuild/188 allocates 21,660 times per op under go1.25.8
// and 6,832 under go1.27.2, on the same machine), so a gate reading one
// toolchain's run against another's rows judges the toolchain. running is
// runtime.Version() of the process doing the comparison; the match is exact,
// and a file naming no toolchain fails the same way.
func (b Budgets) CheckToolchain(running string) error {
	if b.GoVersion == "" {
		return errors.New("budgets.json names no go_version, so its rows cannot be compared with any run")
	}
	if running != b.GoVersion {
		return fmt.Errorf("budgets.json was measured under %s and this run is %s: nothing compared (run under %s, or remeasure every row under %s)", b.GoVersion, running, b.GoVersion, running)
	}
	return nil
}

// CheckPlatform refuses to compare under any GOOS/GOARCH but one that has
// its own benchmark rows: B/op on darwin carries long $TMPDIR path strings
// that a differently-rooted platform would not, so a gate reading one
// platform's run against another's rows judges the platform, the same
// reason CheckToolchain judges the toolchain. running is
// runtime.GOOS+"/"+runtime.GOARCH of the process doing the comparison;
// running either is BenchmarkPlatform (the reference rows are its own) or
// names its own measured rows in Platforms[running].Benchmarks (RES-36p) --
// anything else, including a file naming no platform, fails the same way.
func (b Budgets) CheckPlatform(running string) error {
	if b.BenchmarkPlatform == "" {
		return errors.New("budgets.json names no benchmark_platform, so its benchmark rows cannot be compared with any run")
	}
	if running != b.BenchmarkPlatform && len(b.Platforms[running].Benchmarks) == 0 {
		return fmt.Errorf("budgets.json's benchmark rows were measured on %s and this run is %s: nothing compared", b.BenchmarkPlatform, running)
	}
	return nil
}

// BenchmarksFor is the rows CheckPlatform already proved running may compare
// against: its own measured rows (Platforms[running].Benchmarks) when
// running is not the reference platform, or the reference rows (Benchmarks)
// when it is -- never a blend, since a platform's own rows, once measured,
// are what its own CI job should read (RES-36p).
func (b Budgets) BenchmarksFor(running string) []Benchmark {
	if running != b.BenchmarkPlatform {
		return b.Platforms[running].Benchmarks
	}
	return b.Benchmarks
}

// RES28BudgetMiB is RES-28's line: 6.5 MiB plus 2.2 MiB per MiB of payload.
func RES28BudgetMiB(payloadBytes int) float64 {
	return 6.5 + 2.2*float64(payloadBytes)/(1<<20)
}
