package perf

import (
	"strings"
	"testing"
	"time"
)

// TestRES28i_ToleranceScheduleIsTheSpecs: the platform tolerances are
// RES-28's schedule and nothing looser: 15% on darwin/arm64 until 2026-11-06
// and 10% from that day, 10% on linux/amd64 always, and no other platform.
func TestRES28i_ToleranceScheduleIsTheSpecs(t *testing.T) {
	b := loadBudgets(t)
	if len(b.Platforms) != 2 {
		t.Errorf("platforms %v, want darwin/arm64 and linux/amd64 only", len(b.Platforms))
	}
	darwin, ok := b.Platforms["darwin/arm64"]
	if !ok || darwin.Tolerance != 0.15 || darwin.ToleranceUntil != "2026-11-06" || darwin.ToleranceAfter != 0.10 {
		t.Errorf("darwin/arm64 schedule %v %q %v, want 0.15 until 2026-11-06 then 0.10", darwin.Tolerance, darwin.ToleranceUntil, darwin.ToleranceAfter)
	}
	linux, ok := b.Platforms["linux/amd64"]
	if !ok || linux.Tolerance != 0.10 || linux.ToleranceUntil != "" || linux.ToleranceAfter != 0.10 {
		t.Errorf("linux/amd64 schedule %v %q %v, want 0.10 always", linux.Tolerance, linux.ToleranceUntil, linux.ToleranceAfter)
	}
	eve := time.Date(2026, 11, 5, 23, 59, 0, 0, time.UTC)
	day := time.Date(2026, 11, 6, 0, 0, 0, 0, time.UTC)
	for _, c := range []struct {
		platform string
		at       time.Time
		want     float64
	}{
		{"darwin/arm64", eve, 0.15}, {"darwin/arm64", day, 0.10},
		{"linux/amd64", eve, 0.10}, {"linux/amd64", day, 0.10},
	} {
		p := b.Platforms[c.platform]
		if got := Tolerance(p.Tolerance, p.ToleranceUntil, p.ToleranceAfter, c.at); got != c.want {
			t.Errorf("%s at %s: tolerance %v, want %v", c.platform, c.at.Format(time.DateTime), got, c.want)
		}
	}
}

// TestRES28j_TheCaseSetIsTheSpecs: RES-28's cases, exactly: hook at 400 B
// and post at 540 B (a PostToolUse envelope's fixed part is 467 bytes), both
// at 40 KB, 1, 4 and 8 MiB; the small and 40 KB cases gate from P-08 and the
// rest from P-10, and this tree carries both, so every case gates. Dropping a
// case or moving one to a later PR is a change to what is gated.
func TestRES28j_TheCaseSetIsTheSpecs(t *testing.T) {
	type c struct {
		entry string
		bytes int
		from  string
	}
	want := map[string]c{
		"hook.400B": {"hook", 400, "P-08"}, "hook.40KB": {"hook", 40000, "P-08"},
		"hook.1MiB": {"hook", 1 << 20, "P-10"}, "hook.4MiB": {"hook", 4 << 20, "P-10"}, "hook.8MiB": {"hook", 8 << 20, "P-10"},
		"post.540B": {"post", 540, "P-08"}, "post.40KB": {"post", 40000, "P-08"},
		"post.1MiB": {"post", 1 << 20, "P-10"}, "post.4MiB": {"post", 4 << 20, "P-10"}, "post.8MiB": {"post", 8 << 20, "P-10"},
	}
	b := loadBudgets(t)
	seen := map[string]bool{}
	for _, rc := range b.RSSCases {
		w, ok := want[rc.ID]
		if !ok || seen[rc.ID] {
			t.Errorf("case %s is not one of RES-28's, or appears twice", rc.ID)
			continue
		}
		seen[rc.ID] = true
		if got := (c{rc.Entry, rc.PayloadBytes, rc.GatesFrom}); got != w {
			t.Errorf("case %s: %+v, want %+v", rc.ID, got, w)
		}
		if !CaseGates(rc) {
			t.Errorf("case %s gating from %s does not gate on a tree that carries it", rc.ID, w.from)
		}
	}
	for id := range want {
		if !seen[id] {
			t.Errorf("RES-28 case %s is missing", id)
		}
	}
}

// TestRES28k_EntryRowsAreMeasured: every entry row's peak-RSS p50 is this
// tree's own median of nine on the reference machine (decided 2026-10-13:
// none derived; archive 85.4.1's derivations stay as the basis's second
// half, explaining the expectation), with its p95 at or above it. A recap
// row is measured on the session its benchmark reads, so it is seeded.
func TestRES28k_EntryRowsAreMeasured(t *testing.T) {
	b := loadBudgets(t)
	for _, e := range b.Entries {
		if !strings.HasPrefix(e.Basis, "measured ") {
			t.Errorf("entry %s: basis %q is not a measurement", e.ID, e.Basis)
		}
		if e.MaxRSSMiBP50 <= 0 || e.MaxRSSMiBP95 < e.MaxRSSMiBP50 {
			t.Errorf("entry %s: p50 %v, p95 %v", e.ID, e.MaxRSSMiBP50, e.MaxRSSMiBP95)
		}
		seed, err := SeedFor(e)
		if err != nil {
			t.Errorf("entry %s: %v", e.ID, err)
		}
		if e.Command[0] == "recap" && seed.Calls == 0 {
			t.Errorf("entry %s is measured on an empty store, the no-store early return", e.ID)
		}
	}
}

// TestRES36f_EntryRowsCarryTheirBenchmarksMeasurement: an entry row's
// alloc_bytes_max is the measured B/op of the benchmark row it names, which
// must exist; only the probe rows, which have no benchmark yet, name none,
// and they then carry no allocation figure at all.
func TestRES36f_EntryRowsCarryTheirBenchmarksMeasurement(t *testing.T) {
	b := loadBudgets(t)
	measured := map[string]int64{}
	for _, r := range b.Benchmarks {
		measured[r.Name] = r.MeasuredBytesPerOp
	}
	for _, e := range b.Entries {
		if e.Benchmark == "" {
			if e.Command[0] != "probe" || e.AllocBytesMax != 0 {
				t.Errorf("entry %s: no benchmark, alloc_bytes_max %d", e.ID, e.AllocBytesMax)
			}
			continue
		}
		m, ok := measured[e.Benchmark]
		if !ok {
			t.Errorf("entry %s names benchmark %s, which has no row", e.ID, e.Benchmark)
			continue
		}
		if e.AllocBytesMax != m || m <= 0 {
			t.Errorf("entry %s: alloc_bytes_max %d, its benchmark measured %d B/op", e.ID, e.AllocBytesMax, m)
		}
	}
}
