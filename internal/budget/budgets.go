// Package budget holds the resource budgets the perf gates and `rashomon
// disclose` read: test/perf/budgets.json is the one source, Table in
// budgets_gen.go is its generated Go form, and TestRES36_GeneratedBudgetsMatch
// keeps the two equal (tech spec 8.2.6; archive 85.3.13).
package budget

//go:generate go run ../../scripts/budgetsgen

// Budgets is the shape of test/perf/budgets.json.
type Budgets struct {
	Version       int                 `json:"budgets_version"`
	BenchmarkRows string              `json:"benchmark_rows"`
	Platforms     map[string]Platform `json:"platforms"`
	Entries       []Entry             `json:"entries"`
	RSSCases      []RSSCase           `json:"rss_cases"`
	PerToolCall   PerToolCall         `json:"per_tool_call"`
	Benchmarks    []Benchmark         `json:"benchmarks"`
	Growth        []Growth            `json:"growth"`
	Disk          Disk                `json:"disk"`
}

// Platform is one gated platform: what the figures were measured on, how a
// platform without its own measurements derives them, and the fraction over
// budget its runner is still allowed (archive 85.3.13: 10%, and 15% on the
// macOS runner until 2026-11-06 while its variance is learned).
type Platform struct {
	Reference      string         `json:"reference"`
	SetFrom        string         `json:"set_from"`
	Tolerance      float64        `json:"tolerance"`
	ToleranceUntil string         `json:"tolerance_until"`
	ToleranceAfter float64        `json:"tolerance_after"`
	Entries        map[string]RSS `json:"entries"`
}

// RSS is a peak-RSS budget pair in MiB.
type RSS struct {
	P50 float64 `json:"p50"`
	P95 float64 `json:"p95"`
}

// Entry is one hook entry on one payload class (archive 85.4.1). The budget
// is AllocBytesMax, the bytes one call allocates (runtime.MemStats.TotalAlloc,
// the benchmark's measured B/op, no margin): a hook process runs no GC cycle at these sizes, so
// its peak RSS is the floor plus what it allocated, and the allocation figure
// is deterministic and the same on every platform. Peak RSS is the end-to-end
// cross-check (archive 85.1.2). Wall time is quoted and never gated.
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

// RES28BudgetMiB is RES-28's line: 6.5 MiB plus 2.2 MiB per MiB of payload.
func RES28BudgetMiB(payloadBytes int) float64 {
	return 6.5 + 2.2*float64(payloadBytes)/(1<<20)
}
