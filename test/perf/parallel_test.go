//go:build perf

package perf

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/altrace-dev-role/rashomon/test/perf/payload"
	"github.com/altrace-dev-role/rashomon/test/perf/peakrss"
)

// TestRES30_ParallelHooksStayInBudget: Ways hook-and-post pairs on one
// session, all released at once, each a hook and then its post as Claude Code
// runs them, land every record in the ordered stream with no spill and no
// lock timeout, the pairs' peak footprints sum to within budget, and every
// pair's peak, the largest standing for them all, is at most
// PerProcessPeakRatio times the 1-way figure, measured first, in a store of
// its own, with the runner tolerance only on a CI runner (RatioTolerance;
// RES-30; H-16 at the same width
// covers the hook alone). What a pair costs at any instant is one process, so
// a pair's peak is the larger of its two.
//
// Steer candidates per call and lock_timeout suppressions on steer.ndjson,
// which RES-30's row also names, are STEER's and arrive with it.
func TestRES30_ParallelHooksStayInBudget(t *testing.T) {
	b := loadBudgets(t)
	tol := tolerance(t, b)
	ratioTol := RatioTolerance(tol, os.Getenv)
	ways := b.Parallel.Ways
	oneWay := oneWayPeakMiB(t)
	// The burst runs on the rig every entry row is measured on (rss_test.go):
	// a pinned store, the entries installed in a config directory of its own,
	// and each command run as an installed entry runs it.
	r := newRig(t)
	// The session starts as SessionStart starts it, so the burst measures
	// recording rather than first use creating the store over and over.
	maxRSSMiB(t, payload.SessionStart(), r.env, r.args([]string{"probe", "start"})...)

	type pair struct {
		hook, post float64
		err        error
	}
	pairs := make([]pair, ways)
	release := make(chan struct{})
	var wg sync.WaitGroup
	for i := range pairs {
		wg.Add(1)
		go func(p *pair, id string) {
			defer wg.Done()
			<-release
			if p.hook, p.err = peakMiB(payload.BashPre("git status --short", id), r.env, r.args([]string{"hook"})...); p.err != nil {
				return
			}
			p.post, p.err = peakMiB(payload.BashPost("git status --short", id, " M README.md\n"), r.env, r.args([]string{"post"})...)
		}(&pairs[i], fmt.Sprintf("toolu_par_%03d", i))
	}
	began := time.Now()
	close(release)
	wg.Wait()
	took := time.Since(began)

	burst := readBurst(t, filepath.Join(r.home, "runs", payload.Session), ways)
	burst.OneWayPeakMiB = oneWay
	all := 0.0
	for _, p := range pairs {
		if p.err != nil {
			t.Fatal(p.err)
		}
		burst.PairPeakMiB = append(burst.PairPeakMiB, max(p.hook, p.post))
		all += p.hook + p.post
	}
	for _, problem := range burst.Problems(b.Parallel.SummedPeakMiBMax, tol, ratioTol) {
		t.Error(problem)
	}
	sum := 0.0
	for _, m := range burst.PairPeakMiB {
		sum += m
	}
	median, _ := Median(burst.PairPeakMiB)
	t.Logf("%d-way on %s: summed pair peak %.1f MiB against %.0f (+%.0f%%); all %d processes %.1f MiB; "+
		"largest pair peak %.2f MiB (%.3fx) against %.2fx (+%.0f%%) the 1-way %.2f MiB, median %.2f MiB (%.3fx, reported); "+
		"%d declarations, %d executions, %d spilled, %d lock timeouts; burst %v (reported, not gated)",
		ways, platform(), sum, b.Parallel.SummedPeakMiBMax, tol*100, 2*ways, all,
		burst.LargestMiB(), burst.LargestMiB()/oneWay, PerProcessPeakRatio, ratioTol*100, oneWay, median, median/oneWay,
		burst.Declarations, burst.Executions, burst.Spilled, burst.LockTimeouts, took.Round(time.Millisecond))
}

// oneWayPeakMiB is the 1-way figure RES-30's per-process rule compares the
// burst against: OneWaySamples hook-and-post pairs run one at a time on a
// started session on a rig of their own, each pair's peak the larger of its
// two processes, as in the burst, and the median of those peaks.
func oneWayPeakMiB(t *testing.T) float64 {
	t.Helper()
	r := newRig(t)
	maxRSSMiB(t, payload.SessionStart(), r.env, r.args([]string{"probe", "start"})...)
	peaks := make([]float64, 0, OneWaySamples)
	for i := 0; i < OneWaySamples; i++ {
		id := fmt.Sprintf("toolu_solo_%03d", i)
		hook, err := peakMiB(payload.BashPre("git status --short", id), r.env, r.args([]string{"hook"})...)
		if err != nil {
			t.Fatal(err)
		}
		post, err := peakMiB(payload.BashPost("git status --short", id, " M README.md\n"), r.env, r.args([]string{"post"})...)
		if err != nil {
			t.Fatal(err)
		}
		peaks = append(peaks, max(hook, post))
	}
	median, ok := Median(peaks)
	if !ok {
		t.Fatal("no 1-way sample")
	}
	return median
}

// peakMiB runs the release binary once on stdin and returns its peak memory
// in MiB: on darwin the peak memory footprint /usr/bin/time -l reports, which
// leaves out shared clean pages and so is the figure that sums across
// concurrent processes (archive 85.3.13); elsewhere peak RSS from rusage,
// an upper bound for that sum because it counts the binary's text in every
// process. It runs off the test goroutine, so it returns its failure.
func peakMiB(stdin []byte, env []string, args ...string) (float64, error) {
	name, argv := rashomonBin, args
	if runtime.GOOS == "darwin" {
		name, argv = "/usr/bin/time", append([]string{"-l", rashomonBin}, args...)
	}
	cmd := exec.Command(name, argv...)
	cmd.Stdin = bytes.NewReader(stdin)
	cmd.Env = env
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return 0, fmt.Errorf("rashomon %s: %v\n%s", strings.Join(args, " "), err, stderr.String())
	}
	if runtime.GOOS == "darwin" {
		n, ok := ParseFootprint(stderr.String())
		if !ok {
			return 0, fmt.Errorf("rashomon %s: no %q line in /usr/bin/time's report:\n%s", strings.Join(args, " "), footprintLine, stderr.String())
		}
		return float64(n) / (1 << 20), nil
	}
	return peakrss.MiB(cmd.ProcessState)
}

// readBurst counts what the burst left in one run directory: the ordered
// stream's records by kind with their seqs, the spill file's lines, and the
// call and post coverage records, with those that timed out on a lock.
func readBurst(t *testing.T, dir string, ways int) Burst {
	t.Helper()
	burst := Burst{Ways: ways}
	for _, line := range ndjsonLines(t, filepath.Join(dir, "records.ndjson")) {
		var r struct {
			Type string `json:"type"`
			Seq  *int64 `json:"seq"`
		}
		if err := json.Unmarshal(line, &r); err != nil {
			t.Fatalf("records.ndjson: %v: %s", err, line)
		}
		switch r.Type {
		case "declaration":
			burst.Declarations++
		case "terminal":
			burst.Terminals++
		case "execution":
			burst.Executions++
		}
		if r.Seq != nil {
			burst.Seqs = append(burst.Seqs, *r.Seq)
		}
	}
	burst.Spilled = len(ndjsonLines(t, filepath.Join(dir, "spill.ndjson")))
	for _, line := range ndjsonLines(t, filepath.Join(dir, "coverage.ndjson")) {
		var c struct {
			Phase  string  `json:"phase"`
			Reason *string `json:"reason"`
		}
		if err := json.Unmarshal(line, &c); err != nil {
			t.Fatalf("coverage.ndjson: %v: %s", err, line)
		}
		switch c.Phase {
		case "call":
			burst.CallCoverage++
		case "post":
			burst.PostCoverage++
		}
		if c.Reason != nil && *c.Reason == "lock_timeout" {
			burst.LockTimeouts++
		}
	}
	return burst
}

// ndjsonLines reads a file's non-empty lines; a missing file has none.
func ndjsonLines(t *testing.T, path string) [][]byte {
	t.Helper()
	f, err := os.Open(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	var out [][]byte
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64<<10), 8<<20)
	for sc.Scan() {
		if len(sc.Bytes()) > 0 {
			out = append(out, append([]byte(nil), sc.Bytes()...))
		}
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}
