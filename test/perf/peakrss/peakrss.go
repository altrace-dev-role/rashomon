// Package peakrss measures a child process's peak resident set size, the
// figure /usr/bin/time prints, for the tests that gate on it (RES-28).
//
// A Reaper runs each child: a small binary built from ./reaper that starts
// the child, waits for it, and reports the child's ru_maxrss on a pipe. It
// exists for Linux. There, wait4's ru_maxrss for a child its parent vforked is
// at least the parent's own peak, because exec records the shared address
// space's high-water mark into the child before replacing it (fs/exec.c,
// exec_mmap), and Go starts every child with CLONE_VFORK|CLONE_VM. Read that
// way, every child of the perf test binary reads as at least the test
// binary's own peak. The reaper holds nothing, so its own peak is a
// property of the runner, not a constant: about 2 MiB on a bare linux/arm64
// VM, about 6 MiB in a GitHub Actions ubuntu-latest container (job
// 117482527831). A reading that does not exceed the reaper's own peak is
// refused rather than taken, which is why an entry near that floor cannot be
// measured on every runner yet (test/perf/rss_test.go's belowFloor).
//
// Windows has no ru_maxrss, so there NewReaper returns ErrUnavailable and the
// caller skips by name rather than failing to build; that is what keeps
// `GOOS=windows go vet ./...` clean.
package peakrss

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// ErrUnavailable is NewReaper's error on a platform whose process state
// carries no ru_maxrss.
var ErrUnavailable = errors.New("peak RSS is not reported on this platform")

// Reaper is a built reaper binary; Run measures a child through it.
type Reaper struct{ path string }

// reaperPkg is the reaper's import path, built by NewReaper.
const reaperPkg = "github.com/altrace-dev-role/rashomon/test/perf/peakrss/reaper"

// fromMaxrss converts ru_maxrss to MiB: darwin and ios report bytes, every
// other Unix reports KiB (getrusage(2) on each).
func fromMaxrss(maxrss int64, goos string) float64 {
	rss := float64(maxrss)
	if goos != "darwin" && goos != "ios" {
		rss *= 1024
	}
	return rss / (1 << 20)
}

// attributable reports whether a child's reading is the child's own. On Linux
// the reading is the larger of the child's peak and the reaper's at the
// child's exec, so a reading at or under the reaper's peak may be the
// reaper's and is refused. A zero reading is no reading anywhere.
func attributable(child, reaper float64, goos string) error {
	if child <= 0 {
		return fmt.Errorf("peakrss: the child's peak read %.2f MiB", child)
	}
	if goos == "linux" && child <= reaper {
		return fmt.Errorf("peakrss: the child's peak %.2f MiB does not exceed the reaper's own %.2f MiB, so it may be the reaper's", child, reaper)
	}
	return nil
}

// parseReport reads the reaper's report: "peak <child MiB> <reaper MiB>", or
// "error: <why>" when the child never ran.
func parseReport(report []byte) (child, reaper float64, err error) {
	line := strings.TrimSuffix(string(report), "\n")
	if msg, ok := strings.CutPrefix(line, "error: "); ok {
		return 0, 0, errors.New("peakrss reaper: " + msg)
	}
	f := strings.Fields(line)
	if len(f) != 3 || f[0] != "peak" || bytes.Count(report, []byte("\n")) != 1 {
		return 0, 0, fmt.Errorf("peakrss: the reaper's report %q is not one peak line", report)
	}
	c, err1 := strconv.ParseFloat(f[1], 64)
	r, err2 := strconv.ParseFloat(f[2], 64)
	if err := errors.Join(err1, err2); err != nil {
		return 0, 0, fmt.Errorf("peakrss: the reaper's report %q: %w", report, err)
	}
	return c, r, nil
}

// vmHWM reads the "VmHWM: <n> kB" line of a /proc/<pid>/status in MiB.
func vmHWM(status io.Reader) (float64, error) {
	sc := bufio.NewScanner(status)
	for sc.Scan() {
		rest, ok := bytes.CutPrefix(sc.Bytes(), []byte("VmHWM:"))
		if !ok {
			continue
		}
		f := bytes.Fields(rest)
		if len(f) != 2 || string(f[1]) != "kB" {
			break
		}
		kib, err := strconv.ParseInt(string(f[0]), 10, 64)
		if err != nil {
			return 0, err
		}
		return float64(kib) / 1024, nil
	}
	if err := sc.Err(); err != nil {
		return 0, err
	}
	return 0, errors.New("no VmHWM line in /proc/self/status")
}
