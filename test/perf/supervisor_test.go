//go:build perf

package perf

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/altrace-dev-role/rashomon/test/perf/payload"
)

// Supervisor budgets: the STR-34 test ceiling on the supervisor's wall
// overhead, and RD-2's thresholds, above which the same-binary supervisor
// is to be replaced by a launcher. RD-2's are reported, not gated: the
// switch is a decision, not a test result.
const (
	supervisorOverheadCeiling = 6 * time.Millisecond
	rd2Overhead               = 2500 * time.Microsecond
	rd2ParentMiB              = 4.5
	supervisedTotalMiB        = 11.0
	supervisorRuns            = 31
)

// TestSTR34_SupervisorOverheadUnder6ms: the supervisor's wall overhead, a
// supervised hook against the same hook run directly as the child, the
// harness holding the pipe at fd 3 the supervisor would, has a p50 of at
// most 6 ms. The two are alternated, after warm-up runs of each, so drift
// on the machine lands on both. It also reports RD-2's figures: the
// overhead at p50 and p95 against 2.5 ms, and the parent's own memory
// against 4.5 MiB (its resident set while the child is mid-payload, and on
// darwin its peak footprint, which /usr/bin/time reads for the pid it ran
// alone), with the whole invocation's against 11.0 MiB.
func TestSTR34_SupervisorOverheadUnder6ms(t *testing.T) {
	r := newRig(t)
	maxRSSMiB(t, payload.SessionStart(), r.env, r.args([]string{"probe", "start"})...)
	hook := r.args([]string{"hook"})
	n := 0
	next := func() []byte { n++; return payload.BashPre("git status --short", fmt.Sprintf("toolu_sup_%04d", n)) }

	for i := 0; i < 3; i++ {
		supervisedWall(t, next(), r.env, hook)
		directWall(t, next(), r.env, hook)
	}
	var sup, dir []time.Duration
	for i := 0; i < supervisorRuns; i++ {
		sup = append(sup, supervisedWall(t, next(), r.env, hook))
		dir = append(dir, directWall(t, next(), r.env, hook))
	}
	sp50, sp95 := percentiles(sup)
	dp50, dp95 := percentiles(dir)
	over50, over95 := sp50-dp50, sp95-dp95
	t.Logf("%s, %d runs each: supervised p50 %v p95 %v; direct child p50 %v p95 %v; overhead p50 %v p95 %v",
		platform(), supervisorRuns, sp50, sp95, dp50, dp95, over50, over95)
	t.Logf("RD-2 overhead: p50 %v against %v: %s", over50, rd2Overhead, verdict(over50 > rd2Overhead))
	if over50 > supervisorOverheadCeiling {
		t.Errorf("supervisor overhead p50 %v exceeds the STR-34 ceiling %v", over50, supervisorOverheadCeiling)
	}

	rss := parentRSSMiB(t, next(), r.env, hook)
	t.Logf("RD-2 parent: resident %.2f MiB mid-payload against %.1f MiB: %s", rss, rd2ParentMiB, verdict(rss > rd2ParentMiB))
	if runtime.GOOS == "darwin" {
		parent := footprintMiB(t, next(), r.env, hook, false)
		child := footprintMiB(t, next(), r.env, hook, true)
		t.Logf("darwin peak footprint: parent %.2f MiB, child %.2f MiB, whole invocation %.2f MiB against %.1f MiB: %s",
			parent, child, parent+child, supervisedTotalMiB, verdict(parent+child > supervisedTotalMiB))
	}
}

func verdict(over bool) string {
	if over {
		return "OVER"
	}
	return "within"
}

// percentiles are the p50 and p95 of xs, nearest rank.
func percentiles(xs []time.Duration) (time.Duration, time.Duration) {
	s := append([]time.Duration(nil), xs...)
	sort.Slice(s, func(i, j int) bool { return s[i] < s[j] })
	rank := func(p int) time.Duration { return s[(len(s)*p+99)/100-1] }
	return rank(50), rank(95)
}

// supervisedWall is one supervised run's wall time, from start to exit.
func supervisedWall(t *testing.T, stdin []byte, env, args []string) time.Duration {
	t.Helper()
	cmd := exec.Command(rashomonBin, args...)
	return timed(t, cmd, stdin, env)
}

// directWall runs the hook as the supervised child with no supervisor: the
// harness writes the clock record to a pipe at fd 3 and holds its write end,
// as the supervisor does, so the child polls a live parent.
func directWall(t *testing.T, stdin []byte, env, args []string) time.Duration {
	t.Helper()
	cmd, w := directChild(t, nil, args)
	defer w.Close()
	return timed(t, cmd, stdin, env)
}

// directChild is the child's command with the clock pipe at fd 3, and the
// pipe's write end, which the caller holds until the child exits.
func directChild(t *testing.T, prefix, args []string) (*exec.Cmd, *os.File) {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	record := "rashomon-supervisor 1 " + strconv.FormatInt(time.Now().UnixNano(), 10) + " 4000000000\n"
	if _, err := w.WriteString(record); err != nil {
		t.Fatal(err)
	}
	argv := append(append(append([]string(nil), prefix...), rashomonBin, "--supervised-child"), args...)
	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.ExtraFiles = []*os.File{r}
	t.Cleanup(func() { r.Close() })
	return cmd, w
}

func timed(t *testing.T, cmd *exec.Cmd, stdin []byte, env []string) time.Duration {
	t.Helper()
	var stderr bytes.Buffer
	cmd.Stdin, cmd.Env, cmd.Stderr = bytes.NewReader(stdin), env, &stderr
	start := time.Now()
	err := cmd.Run()
	took := time.Since(start)
	if err != nil || stderr.Len() > 0 {
		t.Fatalf("%s: %v\n%s", strings.Join(cmd.Args, " "), err, stderr.String())
	}
	return took
}

// parentRSSMiB is the supervisor's resident set while its child waits on
// the rest of the payload: all of it but the last byte is written, the
// parent read with ps, and the last byte sent. Nothing the parent does after
// that point allocates more than a few KiB.
func parentRSSMiB(t *testing.T, stdin []byte, env, args []string) float64 {
	t.Helper()
	pr, pw, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(rashomonBin, args...)
	cmd.Stdin, cmd.Env = pr, env
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	pr.Close()
	if _, err := pw.Write(stdin[:len(stdin)-1]); err != nil {
		t.Fatal(err)
	}
	time.Sleep(300 * time.Millisecond)
	out, psErr := exec.Command("ps", "-o", "rss=", "-p", strconv.Itoa(cmd.Process.Pid)).Output()
	if _, err := pw.Write(stdin[len(stdin)-1:]); err != nil {
		t.Fatal(err)
	}
	pw.Close()
	if err := cmd.Wait(); err != nil {
		t.Fatal(err)
	}
	if psErr != nil {
		t.Fatalf("ps: %v", psErr)
	}
	kib, err := strconv.ParseFloat(strings.TrimSpace(string(out)), 64)
	if err != nil {
		t.Fatalf("ps printed %q", out)
	}
	return kib / 1024
}

// footprintMiB is /usr/bin/time -l's peak memory footprint for the process
// it runs: the supervisor alone when supervised, since the figure is the
// pid's own, and the child alone when the child is run directly.
func footprintMiB(t *testing.T, stdin []byte, env, args []string, child bool) float64 {
	t.Helper()
	var cmd *exec.Cmd
	if child {
		var w *os.File
		cmd, w = directChild(t, []string{"/usr/bin/time", "-l"}, args)
		defer w.Close()
	} else {
		cmd = exec.Command("/usr/bin/time", append([]string{"-l", rashomonBin}, args...)...)
	}
	var stderr bytes.Buffer
	cmd.Stdin, cmd.Env, cmd.Stderr = bytes.NewReader(stdin), env, &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("%v\n%s", err, stderr.String())
	}
	n, ok := ParseFootprint(stderr.String())
	if !ok {
		t.Fatalf("no footprint line in:\n%s", stderr.String())
	}
	return float64(n) / (1 << 20)
}
